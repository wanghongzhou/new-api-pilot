package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
)

type LockedSiteRun struct {
	Site    Site
	Run     CollectionRun
	Windows []CollectionRunWindow
}

type CollectionRunFailure struct {
	Code   string
	Params []byte
}

type SiteRunWindowMaterializationExpectation struct {
	Hours   []int64
	SetHash string
}

func NewSiteRunWindowMaterializationExpectation(
	run CollectionRun,
	hours []int64,
) (SiteRunWindowMaterializationExpectation, error) {
	if err := validateSiteRunWindowHours(run, hours); err != nil {
		return SiteRunWindowMaterializationExpectation{}, err
	}
	copyHours := append([]int64(nil), hours...)
	hash, err := siteRunWindowSetHash(run, copyHours)
	if err != nil {
		return SiteRunWindowMaterializationExpectation{}, err
	}
	return SiteRunWindowMaterializationExpectation{Hours: copyHours, SetHash: hash}, nil
}

func (repository *SiteRepository) LockSiteRunForUpdate(ctx context.Context, siteID, runID int64) (LockedSiteRun, error) {
	if siteID <= 0 || runID <= 0 {
		return LockedSiteRun{}, ErrCollectionRunContract
	}
	site, err := repository.FindByIDForUpdate(ctx, siteID)
	if err != nil {
		return LockedSiteRun{}, err
	}
	var run CollectionRun
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND site_id = ?", runID, siteID).First(&run).Error; err != nil {
		return LockedSiteRun{}, err
	}
	var windows []CollectionRunWindow
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_id = ?", run.ID).Order("hour_ts ASC, id ASC").Find(&windows).Error; err != nil {
		return LockedSiteRun{}, err
	}
	return LockedSiteRun{Site: site, Run: run, Windows: windows}, nil
}

func (repository *SiteRepository) FailLockedCollectionRun(
	ctx context.Context,
	run *CollectionRun,
	windows []CollectionRunWindow,
	failure CollectionRunFailure,
	now int64,
) error {
	if run == nil || run.ID <= 0 || failure.Code == "" || now <= 0 {
		return ErrCollectionRunContract
	}
	if err := repository.db.WithContext(ctx).Model(&CollectionRunWindow{}).
		Where("run_id = ? AND status IN ('pending','running')", run.ID).
		Updates(map[string]any{
			"status":        "failed",
			"error_code":    failure.Code,
			"error_params":  nullableJSON(failure.Params),
			"error_message": nil,
			"finished_at":   now,
			"updated_at":    now,
		}).Error; err != nil {
		return err
	}
	for index := range windows {
		if windows[index].Status == "pending" || windows[index].Status == "running" {
			windows[index].Status = "failed"
			windows[index].ErrorCode = failure.Code
			windows[index].ErrorParams = append([]byte(nil), failure.Params...)
			windows[index].ErrorMessage = nil
			windows[index].FinishedAt = int64Pointer(now)
			windows[index].UpdatedAt = now
		}
	}
	return repository.RecalculateLockedCollectionRun(ctx, run, windows, now, &failure)
}

func (repository *SiteRepository) RecalculateLockedCollectionRun(
	ctx context.Context,
	run *CollectionRun,
	windows []CollectionRunWindow,
	now int64,
	forceFailure *CollectionRunFailure,
) error {
	if run == nil || run.ID <= 0 || now <= 0 {
		return ErrCollectionRunContract
	}
	if forceFailure == nil && (run.Status == "failed" || run.Status == "success") {
		return nil
	}
	completed, failed, unavailable, pending, running := 0, 0, 0, 0, 0
	var nextAttempt *int64
	var childFailure *CollectionRunFailure
	for _, window := range windows {
		switch window.Status {
		case "success":
			completed++
		case "failed":
			failed++
			if window.ErrorCode != "" {
				childFailure = &CollectionRunFailure{Code: window.ErrorCode, Params: append([]byte(nil), window.ErrorParams...)}
			}
		case "unavailable":
			unavailable++
		case "pending":
			pending++
			candidate := now
			if window.NextRetryAt != nil && *window.NextRetryAt > candidate {
				candidate = *window.NextRetryAt
			}
			if nextAttempt == nil || candidate < *nextAttempt {
				nextAttempt = int64Pointer(candidate)
			}
		case "running":
			running++
		default:
			return fmt.Errorf("collection run window %d: %w", window.ID, ErrCollectionRunContract)
		}
	}
	if run.WindowsInitializedAt != nil && run.TotalWindows != len(windows) {
		return ErrCollectionRunContract
	}
	run.CompletedWindows = completed
	run.FailedWindows = failed
	run.UnavailableWindows = unavailable
	status := run.Status
	terminal := false
	var terminalFailure *CollectionRunFailure
	if forceFailure != nil {
		if forceFailure.Code == "" {
			return ErrCollectionRunContract
		}
		status = "failed"
		terminal = true
		terminalFailure = forceFailure
	} else {
		if run.WindowsInitializedAt == nil {
			return ErrCollectionRunContract
		}
		terminalCount := completed + failed + unavailable
		switch {
		case terminalCount == run.TotalWindows:
			terminal = true
			if failed > 0 {
				status = "failed"
				terminalFailure = childFailure
			} else {
				status = "success"
			}
		case running > 0:
			status = "running"
		case pending > 0:
			status = "pending"
		default:
			return ErrCollectionRunContract
		}
	}
	updates := map[string]any{
		"status":            status,
		"completed_windows": completed,
		"failed_windows":    failed,
		"updated_at":        now,
	}
	if terminal {
		updates["active_key"] = nil
		updates["finished_at"] = now
		updates["heartbeat_at"] = nil
		run.ActiveKey = nil
		run.FinishedAt = int64Pointer(now)
		run.HeartbeatAt = nil
		if status == "success" {
			updates["error_code"] = ""
			updates["error_params"] = nil
			updates["error_message"] = nil
			run.ErrorCode = ""
			run.ErrorParams = nil
			run.ErrorMessage = sql.NullString{}
		} else if terminalFailure != nil {
			updates["error_code"] = terminalFailure.Code
			updates["error_params"] = nullableJSON(terminalFailure.Params)
			updates["error_message"] = nil
			run.ErrorCode = terminalFailure.Code
			run.ErrorParams = append([]byte(nil), terminalFailure.Params...)
			run.ErrorMessage = sql.NullString{}
		}
	} else if nextAttempt != nil {
		updates["next_attempt_at"] = *nextAttempt
		run.NextAttemptAt = *nextAttempt
	}
	run.Status = status
	run.UpdatedAt = now
	return repository.db.WithContext(ctx).Model(&CollectionRun{}).Where("id = ?", run.ID).Updates(updates).Error
}

func (repository *SiteRepository) RecalculateSiteCollectionRun(ctx context.Context, siteID, runID, now int64) (CollectionRun, error) {
	var result CollectionRun
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepository := &SiteRepository{db: tx}
		locked, err := txRepository.LockSiteRunForUpdate(ctx, siteID, runID)
		if err != nil {
			return err
		}
		if err := txRepository.RecalculateLockedCollectionRun(ctx, &locked.Run, locked.Windows, now, nil); err != nil {
			return err
		}
		result = locked.Run
		return nil
	})
	return result, err
}

func (repository *SiteRepository) CompleteSiteRunWindowMaterialization(
	ctx context.Context,
	siteID int64,
	runID int64,
	expectedConfigVersion int,
	expectation SiteRunWindowMaterializationExpectation,
	now int64,
) (CollectionRun, error) {
	if siteID <= 0 || runID <= 0 || expectedConfigVersion <= 0 || now <= 0 || len(expectation.SetHash) != sha256.Size*2 {
		return CollectionRun{}, ErrCollectionRunContract
	}
	var result CollectionRun
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepository := &SiteRepository{db: tx}
		if _, err := txRepository.LockRunnableSiteSnapshot(ctx, siteID, expectedConfigVersion); err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND site_id = ?", runID, siteID).First(&run).Error; err != nil {
			return err
		}
		if run.SiteConfigVersion != expectedConfigVersion || !constant.CollectionTaskWindowed(run.TaskType) ||
			run.Status != "pending" || run.WindowsInitializedAt != nil {
			return ErrCollectionRunContract
		}
		if err := validateSiteRunWindowHours(run, expectation.Hours); err != nil {
			return err
		}
		expectedHash, err := siteRunWindowSetHash(run, expectation.Hours)
		if err != nil || expectedHash != expectation.SetHash {
			return ErrCollectionRunContract
		}
		var windows []CollectionRunWindow
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("run_id = ?", run.ID).Order("hour_ts ASC, id ASC").Find(&windows).Error; err != nil {
			return err
		}
		if len(windows) != len(expectation.Hours) {
			return ErrCollectionRunContract
		}
		for index, window := range windows {
			if window.SiteID != siteID || window.Status != "pending" || window.HourTS != expectation.Hours[index] {
				return ErrCollectionRunContract
			}
		}
		authoritativeHours, err := txRepository.expectedSiteRunWindowHours(ctx, run)
		if err != nil {
			return err
		}
		if !equalInt64Slices(authoritativeHours, expectation.Hours) {
			return ErrCollectionRunContract
		}
		authoritativeHash, err := siteRunWindowSetHash(run, authoritativeHours)
		if err != nil || authoritativeHash != expectation.SetHash {
			return ErrCollectionRunContract
		}
		expectedWindowCount := len(expectation.Hours)
		initializedAt := now
		update := tx.WithContext(ctx).Model(&CollectionRun{}).
			Where("id = ? AND site_id = ? AND status = 'pending' AND windows_initialized_at IS NULL", run.ID, siteID).
			Updates(map[string]any{
				"total_windows":          expectedWindowCount,
				"windows_initialized_at": initializedAt,
				"updated_at":             now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCollectionRunContract
		}
		run.TotalWindows = expectedWindowCount
		run.WindowsInitializedAt = &initializedAt
		run.UpdatedAt = now
		if err := txRepository.RecalculateLockedCollectionRun(ctx, &run, windows, now, nil); err != nil {
			return err
		}
		result = run
		return nil
	})
	return result, err
}

func (repository *SiteRepository) expectedSiteRunWindowHours(ctx context.Context, run CollectionRun) ([]int64, error) {
	if run.SiteID == nil || run.StartTimestamp == nil || run.EndTimestamp == nil {
		return nil, ErrCollectionRunContract
	}
	onlyMissing := false
	if run.TaskType == constant.TaskTypeUsageBackfill {
		var err error
		onlyMissing, err = UsageBackfillOnlyMissing(run.Scope)
		if err != nil {
			return nil, err
		}
	}
	excluded := map[int64]struct{}{}
	if onlyMissing {
		var facts []struct {
			HourTS int64  `gorm:"column:hour_ts"`
			Status string `gorm:"column:status"`
		}
		if err := repository.db.WithContext(ctx).Table("collection_window").
			Select("hour_ts, status").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("site_id = ? AND hour_ts >= ? AND hour_ts < ?", *run.SiteID, *run.StartTimestamp, *run.EndTimestamp).
			Order("hour_ts ASC").Scan(&facts).Error; err != nil {
			return nil, err
		}
		previous := int64(-1)
		for _, fact := range facts {
			if fact.HourTS%3600 != 0 || fact.HourTS < *run.StartTimestamp || fact.HourTS >= *run.EndTimestamp || fact.HourTS == previous {
				return nil, ErrCollectionRunContract
			}
			previous = fact.HourTS
			switch fact.Status {
			case "complete", "unavailable":
				excluded[fact.HourTS] = struct{}{}
			case "pending", "missing":
			default:
				return nil, ErrCollectionRunContract
			}
		}
	}
	count := int((*run.EndTimestamp - *run.StartTimestamp) / 3600)
	hours := make([]int64, 0, count)
	for hour := *run.StartTimestamp; hour < *run.EndTimestamp; hour += 3600 {
		if _, skip := excluded[hour]; !skip {
			hours = append(hours, hour)
		}
	}
	return hours, nil
}

func validateSiteRunWindowHours(run CollectionRun, hours []int64) error {
	if run.ID <= 0 || run.SiteID == nil || *run.SiteID <= 0 || run.SiteConfigVersion <= 0 ||
		run.StartTimestamp == nil || run.EndTimestamp == nil || *run.StartTimestamp <= 0 ||
		*run.EndTimestamp < *run.StartTimestamp || *run.StartTimestamp%3600 != 0 || *run.EndTimestamp%3600 != 0 {
		return ErrCollectionRunContract
	}
	previous := int64(-1)
	for _, hour := range hours {
		if hour%3600 != 0 || hour < *run.StartTimestamp || hour >= *run.EndTimestamp || hour <= previous {
			return ErrCollectionRunContract
		}
		previous = hour
	}
	return nil
}

func siteRunWindowSetHash(run CollectionRun, hours []int64) (string, error) {
	if err := validateSiteRunWindowHours(run, hours); err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("site-run-window-set:v1\x00"))
	writeHashInt64(digest, *run.SiteID)
	writeHashInt64(digest, run.ID)
	writeHashInt64(digest, int64(run.SiteConfigVersion))
	writeHashInt64(digest, *run.StartTimestamp)
	writeHashInt64(digest, *run.EndTimestamp)
	writeHashInt64(digest, int64(len(run.TaskType)))
	_, _ = digest.Write([]byte(run.TaskType))
	writeHashInt64(digest, int64(len(hours)))
	for _, hour := range hours {
		writeHashInt64(digest, hour)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeHashInt64(digest interface{ Write([]byte) (int, error) }, value int64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(value))
	_, _ = digest.Write(buffer[:])
}

func equalInt64Slices(first, second []int64) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func int64Pointer(value int64) *int64 {
	result := value
	return &result
}
