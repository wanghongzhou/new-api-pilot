package model

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/common"
	"new-api-pilot/constant"
)

var (
	ErrSiteRunConfigChanged       = errors.New("site run configuration changed")
	ErrSiteRunManagementInactive  = errors.New("site run management is inactive")
	ErrSiteRunAuthorizationNeeded = errors.New("site run authorization is required")
	ErrSiteRunStatisticsEnded     = errors.New("site run statistics have ended")
	ErrSiteRunExportDisabled      = errors.New("site run data export is disabled")
	ErrSiteRunCapabilitiesPending = errors.New("site run capabilities are not ready")
	ErrSiteWindowRunOverlap       = errors.New("site window run overlaps an active range")
	ErrCollectionRunContract      = errors.New("collection run contract is invalid")
)

type RunnableSiteSnapshot struct {
	Site         Site
	Capabilities []SiteCapability
}

func ValidateRunnableSiteSnapshot(snapshot RunnableSiteSnapshot, expectedConfigVersion int) error {
	site := snapshot.Site
	if site.ID <= 0 || expectedConfigVersion <= 0 || site.ConfigVersion != expectedConfigVersion {
		return ErrSiteRunConfigChanged
	}
	if site.ManagementStatus != constant.SiteManagementActive {
		return ErrSiteRunManagementInactive
	}
	if site.AuthStatus != constant.SiteAuthAuthorized {
		return ErrSiteRunAuthorizationNeeded
	}
	if site.StatisticsEndAt != nil {
		return ErrSiteRunStatisticsEnded
	}
	if !site.DataExportEnabled {
		return ErrSiteRunExportDisabled
	}
	statuses := make(map[string]string, len(snapshot.Capabilities))
	for _, capability := range snapshot.Capabilities {
		if _, duplicate := statuses[capability.CapabilityKey]; duplicate {
			return ErrSiteRunCapabilitiesPending
		}
		statuses[capability.CapabilityKey] = capability.Status
	}
	keys := constant.SiteCapabilityKeys()
	if len(statuses) != len(keys) {
		return ErrSiteRunCapabilitiesPending
	}
	for _, key := range keys {
		status, exists := statuses[key]
		if !exists || status == constant.CapabilityStatusFailed ||
			(status == constant.CapabilityStatusSkipped && key != constant.CapabilityFlowDataConsistency) ||
			(status != constant.CapabilityStatusPassed && status != constant.CapabilityStatusSkipped) {
			return ErrSiteRunCapabilitiesPending
		}
	}
	return nil
}

func (repository *SiteRepository) LockRunnableSiteSnapshot(ctx context.Context, siteID int64, expectedConfigVersion int) (RunnableSiteSnapshot, error) {
	site, err := repository.FindByIDForUpdate(ctx, siteID)
	if err != nil {
		return RunnableSiteSnapshot{}, err
	}
	var capabilities []SiteCapability
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("site_id = ?", siteID).Order("capability_key ASC").Find(&capabilities).Error; err != nil {
		return RunnableSiteSnapshot{}, err
	}
	snapshot := RunnableSiteSnapshot{Site: site, Capabilities: capabilities}
	if err := ValidateRunnableSiteSnapshot(snapshot, expectedConfigVersion); err != nil {
		return RunnableSiteSnapshot{}, err
	}
	return snapshot, nil
}

type SiteRunSpec struct {
	TaskType       string
	TriggerType    string
	StartTimestamp *int64
	EndTimestamp   *int64
	Scope          []byte
	Priority       int
	RequestID      string
	Now            int64
}

func NewSiteCollectionRun(site Site, spec SiteRunSpec) (CollectionRun, error) {
	targetType, targetValid := constant.CollectionTaskTarget(spec.TaskType)
	if site.ID <= 0 || site.ConfigVersion <= 0 || !targetValid || targetType != "site" ||
		!constant.ValidCollectionTriggerType(spec.TriggerType) || !constant.ValidCollectionTaskPriority(spec.TaskType, spec.TriggerType, spec.Priority) ||
		!validCollectionRequestID(spec.RequestID) || spec.Now <= 0 {
		return CollectionRun{}, ErrCollectionRunContract
	}
	windowed := constant.CollectionTaskWindowed(spec.TaskType)
	if windowed {
		if spec.StartTimestamp == nil || spec.EndTimestamp == nil || *spec.StartTimestamp <= 0 || *spec.EndTimestamp <= 0 ||
			*spec.StartTimestamp%3600 != 0 || *spec.EndTimestamp%3600 != 0 || *spec.EndTimestamp < *spec.StartTimestamp {
			return CollectionRun{}, ErrCollectionRunContract
		}
	} else if spec.StartTimestamp != nil || spec.EndTimestamp != nil {
		return CollectionRun{}, ErrCollectionRunContract
	}
	start := copyInt64Pointer(spec.StartTimestamp)
	end := copyInt64Pointer(spec.EndTimestamp)
	scope, err := CanonicalCollectionRunScope(spec.TaskType, spec.Scope)
	if err != nil {
		return CollectionRun{}, err
	}
	activeKey, err := CollectionRunActiveKey(spec.TaskType, "site", site.ID, start, end)
	if err != nil {
		return CollectionRun{}, err
	}
	siteID := site.ID
	run := CollectionRun{
		SiteID: &siteID, SiteConfigVersion: site.ConfigVersion, TaskType: spec.TaskType,
		TargetType: "site", TargetID: site.ID, TriggerType: spec.TriggerType,
		StartTimestamp: start, EndTimestamp: end, Scope: scope, ActiveKey: &activeKey, Status: "pending",
		Priority: spec.Priority, NextAttemptAt: spec.Now, CreatedRequestID: spec.RequestID,
		LastRequestID: spec.RequestID, CreatedAt: spec.Now, UpdatedAt: spec.Now,
	}
	if !windowed {
		run.WindowsInitializedAt = copyInt64Pointer(&spec.Now)
	}
	return run, nil
}

func ValidateCollectionRunForCreate(run CollectionRun) error {
	expectedTarget, valid := constant.CollectionTaskTarget(run.TaskType)
	if !valid || expectedTarget != run.TargetType || run.TargetID <= 0 ||
		!constant.ValidCollectionTriggerType(run.TriggerType) || !constant.ValidCollectionTaskPriority(run.TaskType, run.TriggerType, run.Priority) ||
		!validCollectionRequestID(run.CreatedRequestID) || run.LastRequestID != run.CreatedRequestID ||
		run.CreatedAt <= 0 || run.UpdatedAt != run.CreatedAt || run.NextAttemptAt <= 0 ||
		run.TotalWindows != 0 || run.CompletedWindows != 0 || run.FailedWindows != 0 || run.RetryCount != 0 {
		return ErrCollectionRunContract
	}
	if run.TargetType == "site" {
		if run.SiteID == nil || *run.SiteID != run.TargetID || run.SiteConfigVersion <= 0 {
			return ErrCollectionRunContract
		}
	} else if run.SiteID != nil || run.SiteConfigVersion != 0 {
		return ErrCollectionRunContract
	}
	canonicalScope, err := CanonicalCollectionRunScope(run.TaskType, run.Scope)
	if err != nil || !bytes.Equal(canonicalScope, run.Scope) {
		return ErrCollectionRunContract
	}
	windowed := constant.CollectionTaskWindowed(run.TaskType)
	if windowed {
		if run.StartTimestamp == nil || run.EndTimestamp == nil || *run.StartTimestamp <= 0 ||
			*run.EndTimestamp < *run.StartTimestamp || *run.StartTimestamp%3600 != 0 || *run.EndTimestamp%3600 != 0 {
			return ErrCollectionRunContract
		}
	} else if run.StartTimestamp != nil || run.EndTimestamp != nil || run.WindowsInitializedAt == nil {
		return ErrCollectionRunContract
	}
	if run.Status == "pending" {
		if run.ActiveKey == nil || (windowed && run.WindowsInitializedAt != nil) || run.FinishedAt != nil {
			return ErrCollectionRunContract
		}
		expectedKey, err := CollectionRunActiveKey(run.TaskType, run.TargetType, run.TargetID, run.StartTimestamp, run.EndTimestamp)
		if err != nil || *run.ActiveKey != expectedKey {
			return ErrCollectionRunContract
		}
		return nil
	}
	return ErrCollectionRunContract
}

func CollectionRunActiveKey(taskType, targetType string, targetID int64, start, end *int64) (string, error) {
	expectedTarget, taskValid := constant.CollectionTaskTarget(taskType)
	if !taskValid || expectedTarget != targetType || targetID <= 0 ||
		(start == nil) != (end == nil) {
		return "", ErrCollectionRunContract
	}
	startValue := ""
	endValue := ""
	if start != nil {
		if *start <= 0 || *end <= 0 {
			return "", ErrCollectionRunContract
		}
		startValue = strconv.FormatInt(*start, 10)
		endValue = strconv.FormatInt(*end, 10)
	}
	return taskType + ":" + targetType + ":" + strconv.FormatInt(targetID, 10) + ":" + startValue + ":" + endValue, nil
}

type SiteWindowRunCreateMode string

const (
	SiteWindowRunStrict   SiteWindowRunCreateMode = "strict"
	SiteWindowRunSchedule SiteWindowRunCreateMode = "schedule"
)

type SiteWindowRunCreateRequest struct {
	SiteID                int64
	ExpectedConfigVersion int
	TaskType              string
	TriggerType           string
	StartTimestamp        int64
	EndTimestamp          int64
	Scope                 []byte
	Priority              int
	RequestID             string
	Now                   int64
	Mode                  SiteWindowRunCreateMode
}

type SiteWindowRunResult struct {
	Run          CollectionRun
	Deduplicated bool
}

type SiteWindowRunCreateResult struct {
	Runs []SiteWindowRunResult
}

func (repository *SiteRepository) CreateSiteWindowRun(ctx context.Context, request SiteWindowRunCreateRequest) (SiteWindowRunCreateResult, error) {
	targetType, targetValid := constant.CollectionTaskTarget(request.TaskType)
	if request.SiteID <= 0 || request.ExpectedConfigVersion <= 0 || !targetValid || targetType != "site" ||
		!constant.CollectionTaskWindowed(request.TaskType) ||
		request.StartTimestamp <= 0 || request.EndTimestamp < request.StartTimestamp ||
		request.StartTimestamp%3600 != 0 || request.EndTimestamp%3600 != 0 ||
		(request.Mode != SiteWindowRunStrict && request.Mode != SiteWindowRunSchedule) {
		return SiteWindowRunCreateResult{}, ErrCollectionRunContract
	}
	if request.Mode == SiteWindowRunSchedule && request.TriggerType != constant.CollectionTriggerSchedule {
		return SiteWindowRunCreateResult{}, ErrCollectionRunContract
	}
	result := SiteWindowRunCreateResult{Runs: []SiteWindowRunResult{}}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepository := &SiteRepository{db: tx}
		snapshot, err := txRepository.LockRunnableSiteSnapshot(ctx, request.SiteID, request.ExpectedConfigVersion)
		if err != nil {
			return err
		}
		family, valid := constant.CollectionTaskFamily(request.TaskType)
		if !valid {
			return ErrCollectionRunContract
		}
		familyTypes := constant.CollectionTaskTypesForFamily(family)
		var active []CollectionRun
		if request.StartTimestamp < request.EndTimestamp {
			if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("site_id = ? AND task_type IN ? AND status IN ('pending','running') AND start_timestamp < ? AND end_timestamp > ?",
					request.SiteID, familyTypes, request.EndTimestamp, request.StartTimestamp).
				Order("start_timestamp ASC, end_timestamp ASC, id ASC").Find(&active).Error; err != nil {
				return err
			}
		}
		for _, existing := range active {
			if existing.SiteConfigVersion != snapshot.Site.ConfigVersion || existing.ActiveKey == nil ||
				existing.StartTimestamp == nil || existing.EndTimestamp == nil {
				return ErrCollectionRunContract
			}
			expectedKey, err := CollectionRunActiveKey(
				existing.TaskType, existing.TargetType, existing.TargetID, existing.StartTimestamp, existing.EndTimestamp,
			)
			if err != nil || *existing.ActiveKey != expectedKey {
				return ErrCollectionRunContract
			}
			if _, err := CanonicalCollectionRunScope(existing.TaskType, existing.Scope); err != nil {
				return ErrCollectionRunContract
			}
		}
		if request.Mode == SiteWindowRunStrict {
			for _, existing := range active {
				if existing.TaskType == request.TaskType && existing.StartTimestamp != nil && existing.EndTimestamp != nil &&
					*existing.StartTimestamp == request.StartTimestamp && *existing.EndTimestamp == request.EndTimestamp {
					if existing.ActiveKey == nil {
						return ErrCollectionRunContract
					}
					result.Runs = append(result.Runs, SiteWindowRunResult{Run: existing, Deduplicated: true})
					return nil
				}
			}
			if len(active) > 0 {
				return ErrSiteWindowRunOverlap
			}
			created, deduplicated, err := txRepository.createSiteWindowGap(ctx, snapshot.Site, request, request.StartTimestamp, request.EndTimestamp)
			if err != nil {
				return err
			}
			result.Runs = append(result.Runs, SiteWindowRunResult{Run: created, Deduplicated: deduplicated})
			return nil
		}
		for _, gap := range uncoveredSiteWindowRanges(request.StartTimestamp, request.EndTimestamp, active) {
			created, deduplicated, err := txRepository.createSiteWindowGap(ctx, snapshot.Site, request, gap.start, gap.end)
			if err != nil {
				return err
			}
			result.Runs = append(result.Runs, SiteWindowRunResult{Run: created, Deduplicated: deduplicated})
		}
		return nil
	})
	if err != nil {
		return SiteWindowRunCreateResult{}, err
	}
	return result, nil
}

func (repository *SiteRepository) createSiteWindowGap(ctx context.Context, site Site, request SiteWindowRunCreateRequest, start, end int64) (CollectionRun, bool, error) {
	run, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: request.TaskType, TriggerType: request.TriggerType, StartTimestamp: &start, EndTimestamp: &end,
		Scope: request.Scope, Priority: request.Priority, RequestID: request.RequestID, Now: request.Now,
	})
	if err != nil {
		return CollectionRun{}, false, err
	}
	return repository.CreateOrGetRun(ctx, &run)
}

type siteWindowRange struct {
	start int64
	end   int64
}

func uncoveredSiteWindowRanges(start, end int64, active []CollectionRun) []siteWindowRange {
	if start >= end {
		return []siteWindowRange{{start: start, end: end}}
	}
	ranges := make([]siteWindowRange, 0, len(active)+1)
	cursor := start
	for _, run := range active {
		if run.StartTimestamp == nil || run.EndTimestamp == nil || *run.EndTimestamp <= cursor {
			continue
		}
		if *run.StartTimestamp > cursor {
			gapEnd := *run.StartTimestamp
			if gapEnd > end {
				gapEnd = end
			}
			if cursor < gapEnd {
				ranges = append(ranges, siteWindowRange{start: cursor, end: gapEnd})
			}
		}
		if *run.EndTimestamp > cursor {
			cursor = *run.EndTimestamp
			if cursor >= end {
				return ranges
			}
		}
	}
	if cursor < end {
		ranges = append(ranges, siteWindowRange{start: cursor, end: end})
	}
	return ranges
}

func copyInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validCollectionRequestID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

type usageBackfillScope struct {
	OnlyMissing *bool `json:"only_missing"`
}

type canonicalUsageBackfillScope struct {
	OnlyMissing bool `json:"only_missing"`
}

func NewUsageBackfillRunScope(onlyMissing bool) ([]byte, error) {
	return common.Marshal(canonicalUsageBackfillScope{OnlyMissing: onlyMissing})
}

func UsageBackfillOnlyMissing(scope []byte) (bool, error) {
	canonical, err := CanonicalCollectionRunScope(constant.TaskTypeUsageBackfill, scope)
	if err != nil {
		return false, err
	}
	var decoded canonicalUsageBackfillScope
	if err := common.DecodeJSON(bytes.NewReader(canonical), &decoded, 4096); err != nil {
		return false, ErrCollectionRunContract
	}
	return decoded.OnlyMissing, nil
}

func CanonicalCollectionRunScope(taskType string, scope []byte) ([]byte, error) {
	if !constant.ValidCollectionTaskType(taskType) || len(scope) > 4096 {
		return nil, ErrCollectionRunContract
	}
	trimmed := bytes.TrimSpace(scope)
	if len(trimmed) > 0 && (len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}') {
		return nil, ErrCollectionRunContract
	}
	if taskType == constant.TaskTypeUsageBackfill {
		if len(trimmed) == 0 {
			return NewUsageBackfillRunScope(true)
		}
		var decoded usageBackfillScope
		if err := common.DecodeJSON(bytes.NewReader(trimmed), &decoded, 4096); err != nil || decoded.OnlyMissing == nil {
			return nil, ErrCollectionRunContract
		}
		return NewUsageBackfillRunScope(*decoded.OnlyMissing)
	}
	if len(trimmed) == 0 {
		return []byte("{}"), nil
	}
	var empty struct{}
	if err := common.DecodeJSON(bytes.NewReader(trimmed), &empty, 4096); err != nil {
		return nil, ErrCollectionRunContract
	}
	return []byte("{}"), nil
}
