package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ResourceMinuteTableInstance = "site_instance_status_minutely"
	ResourceMinuteTableSite     = "site_status_minutely"

	maximumResourceRetentionBatchSize = 5000
)

var ErrResourceRetentionContract = errors.New("resource retention contract is invalid")

var resourceRetentionLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type ResourceRetentionCursor struct {
	SiteID   int64
	MinuteTS int64
	ID       int64
}

type ResourceRetentionBatchRequest struct {
	Table       string
	Cutoff      int64
	After       ResourceRetentionCursor
	MaximumRows int
}

type ResourceRetentionBatchResult struct {
	Scanned                     int
	Deleted                     int64
	SkippedUnfinalized          int
	SkippedMissingHourly        int
	SkippedDailyNotFinal        int
	PendingRows                 bool
	BlockedDiagnosticsTruncated bool
	Last                        ResourceRetentionCursor
	HasMore                     bool
}

type ResourceRetentionRepository struct {
	db *gorm.DB
}

func NewResourceRetentionRepository(db *gorm.DB) *ResourceRetentionRepository {
	return &ResourceRetentionRepository{db: db}
}

type resourceRetentionTableContract struct {
	minuteTable string
	hourTable   string
	dayTable    string
	instance    bool
}

type resourceMinuteCandidate struct {
	ID       int64  `gorm:"column:id"`
	SiteID   int64  `gorm:"column:site_id"`
	NodeName string `gorm:"column:node_name"`
	MinuteTS int64  `gorm:"column:minute_ts"`
}

type resourceInstanceHourRow struct {
	SiteID   int64  `gorm:"column:site_id"`
	NodeName string `gorm:"column:node_name"`
	HourTS   int64  `gorm:"column:hour_ts"`
}

type resourceInstanceDayRow struct {
	SiteID   int64  `gorm:"column:site_id"`
	NodeName string `gorm:"column:node_name"`
	DateKey  int    `gorm:"column:date_key"`
}

type resourceSiteHourRow struct {
	SiteID int64 `gorm:"column:site_id"`
	HourTS int64 `gorm:"column:hour_ts"`
}

type resourceSiteDayRow struct {
	SiteID  int64 `gorm:"column:site_id"`
	DateKey int   `gorm:"column:date_key"`
}

type resourceInstanceHourKey struct {
	SiteID   int64
	NodeName string
	HourTS   int64
}

type resourceInstanceDayKey struct {
	SiteID   int64
	NodeName string
	DateKey  int
}

type resourceSiteHourKey struct {
	SiteID int64
	HourTS int64
}

type resourceSiteDayKey struct {
	SiteID  int64
	DateKey int
}

func (repository *ResourceRetentionRepository) CleanResourceMinuteBatch(
	ctx context.Context,
	request ResourceRetentionBatchRequest,
) (ResourceRetentionBatchResult, error) {
	contract, err := resourceRetentionContract(request.Table)
	if repository == nil || repository.db == nil || err != nil || request.Cutoff <= 0 || request.Cutoff%60 != 0 ||
		request.MaximumRows <= 0 || request.MaximumRows > maximumResourceRetentionBatchSize ||
		!validResourceRetentionCursor(request.After) {
		return ResourceRetentionBatchResult{}, ErrResourceRetentionContract
	}
	var result ResourceRetentionBatchResult
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidates, listErr := listFinalizedResourceMinuteCandidates(ctx, tx, contract, request)
		if listErr != nil {
			return listErr
		}
		result.Scanned = len(candidates)
		result.HasMore = len(candidates) == request.MaximumRows
		if len(candidates) > 0 {
			last := candidates[len(candidates)-1]
			result.Last = ResourceRetentionCursor{SiteID: last.SiteID, MinuteTS: last.MinuteTS, ID: last.ID}
			eligibleIDs := make([]int64, 0, len(candidates))
			for _, candidate := range candidates {
				eligibleIDs = append(eligibleIDs, candidate.ID)
			}
			deleted := tx.Table(contract.minuteTable).
				Where("id IN ? AND minute_ts < ?", eligibleIDs, request.Cutoff).
				Delete(&resourceMinuteCandidate{})
			if deleted.Error != nil {
				return fmt.Errorf("delete finalized %s rows: %w", contract.minuteTable, deleted.Error)
			}
			if deleted.RowsAffected != int64(len(eligibleIDs)) {
				return ErrResourceRetentionContract
			}
			result.Deleted = deleted.RowsAffected
		}
		if len(candidates) < request.MaximumRows {
			diagnosticLimit := request.MaximumRows - len(candidates)
			blocked, diagnosticErr := listResourceMinuteCandidates(ctx, tx, contract, ResourceRetentionBatchRequest{
				Table: contract.minuteTable, Cutoff: request.Cutoff, MaximumRows: diagnosticLimit + 1,
			})
			if diagnosticErr != nil {
				return diagnosticErr
			}
			result.BlockedDiagnosticsTruncated = len(blocked) > diagnosticLimit
			if result.BlockedDiagnosticsTruncated {
				blocked = blocked[:diagnosticLimit]
			}
			result.PendingRows = len(blocked) > 0
			if len(blocked) > 0 {
				finalization, filterErr := finalizedResourceMinuteIDs(ctx, tx, contract, blocked)
				if filterErr != nil {
					return filterErr
				}
				result.Scanned += len(blocked)
				result.SkippedUnfinalized = len(blocked) - len(finalization.eligibleIDs)
				result.SkippedMissingHourly = finalization.missingHourly
				result.SkippedDailyNotFinal = finalization.dailyNotFinal
			}
		}
		return nil
	})
	if err != nil {
		return ResourceRetentionBatchResult{}, err
	}
	return result, nil
}

func listFinalizedResourceMinuteCandidates(
	ctx context.Context,
	tx *gorm.DB,
	contract resourceRetentionTableContract,
	request ResourceRetentionBatchRequest,
) ([]resourceMinuteCandidate, error) {
	const minuteAlias = "retention_minute"
	selectColumns := minuteAlias + ".id, " + minuteAlias + ".site_id, " + minuteAlias + ".minute_ts"
	hourIdentity := "retention_hour.site_id = retention_minute.site_id"
	dayIdentity := "retention_day.site_id = retention_minute.site_id"
	if contract.instance {
		selectColumns += ", " + minuteAlias + ".node_name"
		hourIdentity += " AND retention_hour.node_name = retention_minute.node_name"
		dayIdentity += " AND retention_day.node_name = retention_minute.node_name"
	}
	hourPredicate := fmt.Sprintf(`EXISTS (
SELECT 1 FROM %s AS retention_hour
WHERE %s
  AND retention_hour.hour_ts = retention_minute.minute_ts - MOD(retention_minute.minute_ts, 3600)
  AND retention_hour.last_calculated_at > 0
  AND retention_hour.data_status IN ?
)`, contract.hourTable, hourIdentity)
	dayPredicate := fmt.Sprintf(`EXISTS (
SELECT 1 FROM %s AS retention_day
WHERE %s
  AND retention_day.date_key = CAST(DATE_FORMAT(
      FROM_DAYS(FLOOR((retention_minute.minute_ts + 28800) / 86400) + 719528), '%%Y%%m%%d'
  ) AS UNSIGNED)
  AND retention_day.is_final = 1
  AND retention_day.last_calculated_at > 0
  AND retention_day.data_status IN ?
)`, contract.dayTable, dayIdentity)
	var candidates []resourceMinuteCandidate
	err := tx.WithContext(ctx).Table(contract.minuteTable+" AS "+minuteAlias).Select(selectColumns).
		Where(minuteAlias+".minute_ts < ? AND ("+minuteAlias+".site_id, "+minuteAlias+".minute_ts, "+minuteAlias+".id) > (?, ?, ?)",
			request.Cutoff, request.After.SiteID, request.After.MinuteTS, request.After.ID).
		Where(hourPredicate, finalizedResourceStatuses()).
		Where(dayPredicate, finalizedResourceStatuses()).
		Order(minuteAlias + ".site_id ASC, " + minuteAlias + ".minute_ts ASC, " + minuteAlias + ".id ASC").
		Limit(request.MaximumRows).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("list finalized %s retention candidates: %w", contract.minuteTable, err)
	}
	if err := validateResourceMinuteCandidates(contract, candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func resourceRetentionContract(table string) (resourceRetentionTableContract, error) {
	switch table {
	case ResourceMinuteTableInstance:
		return resourceRetentionTableContract{
			minuteTable: ResourceMinuteTableInstance,
			hourTable:   "site_instance_status_hourly",
			dayTable:    "site_instance_status_daily",
			instance:    true,
		}, nil
	case ResourceMinuteTableSite:
		return resourceRetentionTableContract{
			minuteTable: ResourceMinuteTableSite,
			hourTable:   "site_status_hourly",
			dayTable:    "site_status_daily",
		}, nil
	default:
		return resourceRetentionTableContract{}, ErrResourceRetentionContract
	}
}

func validResourceRetentionCursor(cursor ResourceRetentionCursor) bool {
	if cursor.SiteID < 0 || cursor.MinuteTS < 0 || cursor.ID < 0 {
		return false
	}
	if cursor.SiteID == 0 {
		return cursor.MinuteTS == 0 && cursor.ID == 0
	}
	return cursor.MinuteTS > 0 && cursor.MinuteTS%60 == 0 && cursor.ID > 0
}

func listResourceMinuteCandidates(
	ctx context.Context,
	tx *gorm.DB,
	contract resourceRetentionTableContract,
	request ResourceRetentionBatchRequest,
) ([]resourceMinuteCandidate, error) {
	selectColumns := "id, site_id, minute_ts"
	if contract.instance {
		selectColumns = "id, site_id, node_name, minute_ts"
	}
	var candidates []resourceMinuteCandidate
	err := tx.WithContext(ctx).Table(contract.minuteTable).Select(selectColumns).
		Where("minute_ts < ? AND (site_id, minute_ts, id) > (?, ?, ?)",
			request.Cutoff, request.After.SiteID, request.After.MinuteTS, request.After.ID).
		Order("site_id ASC, minute_ts ASC, id ASC").Limit(request.MaximumRows).
		Find(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("list %s retention candidates: %w", contract.minuteTable, err)
	}
	if err := validateResourceMinuteCandidates(contract, candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func validateResourceMinuteCandidates(
	contract resourceRetentionTableContract,
	candidates []resourceMinuteCandidate,
) error {
	for _, candidate := range candidates {
		if candidate.ID <= 0 || candidate.SiteID <= 0 || candidate.MinuteTS <= 0 || candidate.MinuteTS%60 != 0 ||
			(contract.instance && candidate.NodeName == "") {
			return ErrResourceRetentionContract
		}
	}
	return nil
}

type resourceMinuteFinalization struct {
	eligibleIDs   []int64
	missingHourly int
	dailyNotFinal int
}

func finalizedResourceMinuteIDs(
	ctx context.Context,
	tx *gorm.DB,
	contract resourceRetentionTableContract,
	candidates []resourceMinuteCandidate,
) (resourceMinuteFinalization, error) {
	if contract.instance {
		return finalizedInstanceResourceMinuteIDs(ctx, tx, contract, candidates)
	}
	return finalizedSiteResourceMinuteIDs(ctx, tx, contract, candidates)
}

func finalizedInstanceResourceMinuteIDs(
	ctx context.Context,
	tx *gorm.DB,
	contract resourceRetentionTableContract,
	candidates []resourceMinuteCandidate,
) (resourceMinuteFinalization, error) {
	hourTuples := make([][]any, 0, len(candidates))
	dayTuples := make([][]any, 0, len(candidates))
	seenHours := make(map[resourceInstanceHourKey]struct{}, len(candidates))
	seenDays := make(map[resourceInstanceDayKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		hour := resourceInstanceHourKey{SiteID: candidate.SiteID, NodeName: candidate.NodeName, HourTS: floorResourceHour(candidate.MinuteTS)}
		day := resourceInstanceDayKey{SiteID: candidate.SiteID, NodeName: candidate.NodeName, DateKey: resourceDateKey(candidate.MinuteTS)}
		if _, exists := seenHours[hour]; !exists {
			seenHours[hour] = struct{}{}
			hourTuples = append(hourTuples, []any{hour.SiteID, hour.NodeName, hour.HourTS})
		}
		if _, exists := seenDays[day]; !exists {
			seenDays[day] = struct{}{}
			dayTuples = append(dayTuples, []any{day.SiteID, day.NodeName, day.DateKey})
		}
	}
	var hourRows []resourceInstanceHourRow
	if err := tx.WithContext(ctx).Table(contract.hourTable).
		Select("site_id, node_name, hour_ts").
		Where("(site_id, node_name, hour_ts) IN ?", hourTuples).
		Where("last_calculated_at > 0 AND data_status IN ?", finalizedResourceStatuses()).
		Find(&hourRows).Error; err != nil {
		return resourceMinuteFinalization{}, fmt.Errorf("load finalized instance resource hours: %w", err)
	}
	var dayRows []resourceInstanceDayRow
	if err := tx.WithContext(ctx).Table(contract.dayTable).
		Select("site_id, node_name, date_key").
		Where("(site_id, node_name, date_key) IN ?", dayTuples).
		Where("is_final = 1 AND last_calculated_at > 0 AND data_status IN ?", finalizedResourceStatuses()).
		Find(&dayRows).Error; err != nil {
		return resourceMinuteFinalization{}, fmt.Errorf("load finalized instance resource days: %w", err)
	}
	finalHours := make(map[resourceInstanceHourKey]struct{}, len(hourRows))
	for _, row := range hourRows {
		finalHours[resourceInstanceHourKey{SiteID: row.SiteID, NodeName: row.NodeName, HourTS: row.HourTS}] = struct{}{}
	}
	finalDays := make(map[resourceInstanceDayKey]struct{}, len(dayRows))
	for _, row := range dayRows {
		finalDays[resourceInstanceDayKey{SiteID: row.SiteID, NodeName: row.NodeName, DateKey: row.DateKey}] = struct{}{}
	}
	result := resourceMinuteFinalization{eligibleIDs: make([]int64, 0, len(candidates))}
	for _, candidate := range candidates {
		hour := resourceInstanceHourKey{SiteID: candidate.SiteID, NodeName: candidate.NodeName, HourTS: floorResourceHour(candidate.MinuteTS)}
		day := resourceInstanceDayKey{SiteID: candidate.SiteID, NodeName: candidate.NodeName, DateKey: resourceDateKey(candidate.MinuteTS)}
		_, hourReady := finalHours[hour]
		_, dayReady := finalDays[day]
		if !hourReady {
			result.missingHourly++
		}
		if !dayReady {
			result.dailyNotFinal++
		}
		if hourReady && dayReady {
			result.eligibleIDs = append(result.eligibleIDs, candidate.ID)
		}
	}
	return result, nil
}

func finalizedSiteResourceMinuteIDs(
	ctx context.Context,
	tx *gorm.DB,
	contract resourceRetentionTableContract,
	candidates []resourceMinuteCandidate,
) (resourceMinuteFinalization, error) {
	hourTuples := make([][]any, 0, len(candidates))
	dayTuples := make([][]any, 0, len(candidates))
	seenHours := make(map[resourceSiteHourKey]struct{}, len(candidates))
	seenDays := make(map[resourceSiteDayKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		hour := resourceSiteHourKey{SiteID: candidate.SiteID, HourTS: floorResourceHour(candidate.MinuteTS)}
		day := resourceSiteDayKey{SiteID: candidate.SiteID, DateKey: resourceDateKey(candidate.MinuteTS)}
		if _, exists := seenHours[hour]; !exists {
			seenHours[hour] = struct{}{}
			hourTuples = append(hourTuples, []any{hour.SiteID, hour.HourTS})
		}
		if _, exists := seenDays[day]; !exists {
			seenDays[day] = struct{}{}
			dayTuples = append(dayTuples, []any{day.SiteID, day.DateKey})
		}
	}
	var hourRows []resourceSiteHourRow
	if err := tx.WithContext(ctx).Table(contract.hourTable).
		Select("site_id, hour_ts").Where("(site_id, hour_ts) IN ?", hourTuples).
		Where("last_calculated_at > 0 AND data_status IN ?", finalizedResourceStatuses()).
		Find(&hourRows).Error; err != nil {
		return resourceMinuteFinalization{}, fmt.Errorf("load finalized site resource hours: %w", err)
	}
	var dayRows []resourceSiteDayRow
	if err := tx.WithContext(ctx).Table(contract.dayTable).
		Select("site_id, date_key").Where("(site_id, date_key) IN ?", dayTuples).
		Where("is_final = 1 AND last_calculated_at > 0 AND data_status IN ?", finalizedResourceStatuses()).
		Find(&dayRows).Error; err != nil {
		return resourceMinuteFinalization{}, fmt.Errorf("load finalized site resource days: %w", err)
	}
	finalHours := make(map[resourceSiteHourKey]struct{}, len(hourRows))
	for _, row := range hourRows {
		finalHours[resourceSiteHourKey{SiteID: row.SiteID, HourTS: row.HourTS}] = struct{}{}
	}
	finalDays := make(map[resourceSiteDayKey]struct{}, len(dayRows))
	for _, row := range dayRows {
		finalDays[resourceSiteDayKey{SiteID: row.SiteID, DateKey: row.DateKey}] = struct{}{}
	}
	result := resourceMinuteFinalization{eligibleIDs: make([]int64, 0, len(candidates))}
	for _, candidate := range candidates {
		hour := resourceSiteHourKey{SiteID: candidate.SiteID, HourTS: floorResourceHour(candidate.MinuteTS)}
		day := resourceSiteDayKey{SiteID: candidate.SiteID, DateKey: resourceDateKey(candidate.MinuteTS)}
		_, hourReady := finalHours[hour]
		_, dayReady := finalDays[day]
		if !hourReady {
			result.missingHourly++
		}
		if !dayReady {
			result.dailyNotFinal++
		}
		if hourReady && dayReady {
			result.eligibleIDs = append(result.eligibleIDs, candidate.ID)
		}
	}
	return result, nil
}

func finalizedResourceStatuses() []string {
	return []string{"complete", "partial", "missing", "paused"}
}

func floorResourceHour(value int64) int64 {
	return value - value%3600
}

func resourceDateKey(value int64) int {
	local := time.Unix(value, 0).In(resourceRetentionLocation)
	return local.Year()*10000 + int(local.Month())*100 + local.Day()
}
