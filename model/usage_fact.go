package model

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"unicode/utf8"

	"new-api-pilot/constant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CollectionWindowStatusPending     = "pending"
	CollectionWindowStatusComplete    = "complete"
	CollectionWindowStatusMissing     = "missing"
	CollectionWindowStatusUnavailable = "unavailable"
)

type UsageFactHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	RemoteUserID     int64  `gorm:"column:remote_user_id"`
	UsernameSnapshot string `gorm:"column:username_snapshot"`
	ModelName        string `gorm:"column:model_name"`
	ChannelID        int64  `gorm:"column:channel_id"`
	UseGroup         string `gorm:"column:use_group"`
	TokenID          int64  `gorm:"column:token_id"`
	TokenName        string `gorm:"column:token_name"`
	NodeName         string `gorm:"column:node_name"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	CollectedAt      int64  `gorm:"column:collected_at"`
}

func (UsageFactHourly) TableName() string { return "usage_fact_hourly" }

type CollectionWindow struct {
	ID               int64   `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64   `gorm:"column:site_id"`
	HourTS           int64   `gorm:"column:hour_ts"`
	Status           string  `gorm:"column:status"`
	FetchedRows      int64   `gorm:"column:fetched_rows"`
	SourceHash       string  `gorm:"column:source_hash"`
	LastFactRunID    *int64  `gorm:"column:last_fact_run_id"`
	VerifiedAt       *int64  `gorm:"column:verified_at"`
	LastErrorCode    string  `gorm:"column:last_error_code"`
	LastErrorParams  []byte  `gorm:"column:last_error_params;type:json"`
	LastErrorMessage *string `gorm:"column:last_error_message"`
	UpdatedAt        int64   `gorm:"column:updated_at"`
}

func (CollectionWindow) TableName() string { return "collection_window" }

type UsageFactInput struct {
	RemoteUserID     int64
	UsernameSnapshot string
	ModelName        string
	ChannelID        int64
	UseGroup         string
	TokenID          int64
	TokenName        string
	NodeName         string
	RequestCount     int64
	Quota            int64
	TokenUsed        int64
}

type UsageWindowMutationScope struct {
	Site   Site
	Run    CollectionRun
	Window CollectionRunWindow
}

type UsageWindowMutationResult struct {
	CollectionStatus string
	SourceHash       string
	SourceRequestID  string
	FetchedRows      int64
	WrittenRows      int64
	VerifiedOnly     bool
	ReasonCode       string
}

// UsageFactMutation is an intermediate fact-only plan. It can only be applied
// by the aggregation commit in this package, so collection callers cannot
// bypass or execute the summary rebuild separately.
type UsageFactMutation struct {
	applyFn func(context.Context, *gorm.DB, UsageWindowMutationScope) (UsageWindowMutationResult, error)
}

func (mutation UsageFactMutation) apply(
	ctx context.Context,
	tx *gorm.DB,
	scope UsageWindowMutationScope,
) (UsageWindowMutationResult, error) {
	if mutation.applyFn == nil {
		return UsageWindowMutationResult{}, ErrCollectionRunContract
	}
	return mutation.applyFn(ctx, tx, scope)
}

func (mutation UsageFactMutation) valid() bool { return mutation.applyFn != nil }

type CompleteUsageWindowRequest struct {
	RunID                 int64
	WindowID              int64
	SiteID                int64
	ExpectedConfigVersion int
	HourTS                int64
	AttemptCount          int
	RequestID             string
	Now                   int64
	FetchedRows           int64
	Validation            bool
	Facts                 []UsageFactInput
}

type FailedUsageWindowRequest struct {
	RunID                 int64
	WindowID              int64
	SiteID                int64
	ExpectedConfigVersion int
	HourTS                int64
	AttemptCount          int
	RequestID             string
	Now                   int64
	FetchedRows           int64
	ReasonCode            string
	ReasonParams          []byte
	DataMismatch          bool
}

func NewCompleteUsageWindowMutation(request CompleteUsageWindowRequest) (UsageFactMutation, UsageWindowMutationResult, error) {
	facts, sourceHash, err := canonicalUsageFacts(request.SiteID, request.HourTS, request.Now, request.Facts)
	if err != nil || !validUsageMutationRequest(request.RunID, request.WindowID, request.SiteID, request.ExpectedConfigVersion,
		request.HourTS, request.AttemptCount, request.RequestID, request.Now, request.FetchedRows) {
		return UsageFactMutation{}, UsageWindowMutationResult{}, ErrCollectionRunContract
	}
	planned := UsageWindowMutationResult{
		CollectionStatus: CollectionWindowStatusComplete,
		SourceHash:       sourceHash, SourceRequestID: request.RequestID,
		FetchedRows: request.FetchedRows, WrittenRows: int64(len(facts)),
	}
	mutation := UsageFactMutation{applyFn: func(ctx context.Context, tx *gorm.DB, scope UsageWindowMutationScope) (UsageWindowMutationResult, error) {
		lockedScope, err := lockUsageMutationScope(ctx, tx, scope, request.RunID, request.WindowID, request.SiteID,
			request.ExpectedConfigVersion, request.HourTS, request.AttemptCount, request.RequestID, request.Now)
		if err != nil {
			return UsageWindowMutationResult{}, err
		}
		window, exists, err := lockCollectionWindow(ctx, tx, request.SiteID, request.HourTS)
		if err != nil {
			return UsageWindowMutationResult{}, err
		}
		result := planned
		if request.Validation && exists && window.Status == CollectionWindowStatusComplete && window.SourceHash == sourceHash {
			if err := tx.WithContext(ctx).Model(&CollectionWindow{}).Where("id = ?", window.ID).
				Updates(map[string]any{"verified_at": request.Now, "last_error_code": "", "last_error_params": nil,
					"last_error_message": nil, "updated_at": request.Now}).Error; err != nil {
				return UsageWindowMutationResult{}, err
			}
			result.WrittenRows = 0
			result.VerifiedOnly = true
		} else {
			if err := tx.WithContext(ctx).Where("site_id = ? AND hour_ts = ?", request.SiteID, request.HourTS).
				Delete(&UsageFactHourly{}).Error; err != nil {
				return UsageWindowMutationResult{}, err
			}
			if len(facts) > 0 {
				if err := tx.WithContext(ctx).CreateInBatches(facts, 500).Error; err != nil {
					return UsageWindowMutationResult{}, err
				}
			}
			verifiedAt := request.Now
			lastRunID := request.RunID
			values := CollectionWindow{
				SiteID: request.SiteID, HourTS: request.HourTS, Status: CollectionWindowStatusComplete,
				FetchedRows: request.FetchedRows, SourceHash: sourceHash, LastFactRunID: &lastRunID,
				VerifiedAt: &verifiedAt, UpdatedAt: request.Now,
			}
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "site_id"}, {Name: "hour_ts"}},
				DoUpdates: clause.Assignments(map[string]any{
					"status": CollectionWindowStatusComplete, "fetched_rows": request.FetchedRows,
					"source_hash": sourceHash, "last_fact_run_id": request.RunID, "verified_at": request.Now,
					"last_error_code": "", "last_error_params": nil, "last_error_message": nil, "updated_at": request.Now,
				}),
			}).Create(&values).Error; err != nil {
				return UsageWindowMutationResult{}, err
			}
		}
		if _, err := ReconcileUsageCursor(ctx, tx, request.SiteID, *lockedScope.Site.StatisticsStartAt, request.Now); err != nil {
			return UsageWindowMutationResult{}, err
		}
		return result, nil
	}}
	return mutation, planned, nil
}

func NewFailedUsageWindowMutation(request FailedUsageWindowRequest) (UsageFactMutation, error) {
	if !validUsageMutationRequest(request.RunID, request.WindowID, request.SiteID, request.ExpectedConfigVersion,
		request.HourTS, request.AttemptCount, request.RequestID, request.Now, request.FetchedRows) || request.ReasonCode == "" {
		return UsageFactMutation{}, ErrCollectionRunContract
	}
	return UsageFactMutation{applyFn: func(ctx context.Context, tx *gorm.DB, scope UsageWindowMutationScope) (UsageWindowMutationResult, error) {
		lockedScope, err := lockUsageMutationScope(ctx, tx, scope, request.RunID, request.WindowID, request.SiteID,
			request.ExpectedConfigVersion, request.HourTS, request.AttemptCount, request.RequestID, request.Now)
		if err != nil {
			return UsageWindowMutationResult{}, err
		}
		window, exists, err := lockCollectionWindow(ctx, tx, request.SiteID, request.HourTS)
		if err != nil {
			return UsageWindowMutationResult{}, err
		}
		result := UsageWindowMutationResult{
			CollectionStatus: CollectionWindowStatusMissing,
			SourceRequestID:  request.RequestID, FetchedRows: request.FetchedRows, ReasonCode: request.ReasonCode,
		}
		if !request.DataMismatch && exists &&
			(window.Status == CollectionWindowStatusComplete || window.Status == CollectionWindowStatusUnavailable) {
			result.CollectionStatus = window.Status
			result.SourceHash = window.SourceHash
			return result, nil
		}
		lastRunID := request.RunID
		if exists {
			updates := map[string]any{
				"status": CollectionWindowStatusMissing, "fetched_rows": request.FetchedRows,
				"last_fact_run_id": request.RunID, "verified_at": nil,
				"last_error_code": request.ReasonCode, "last_error_params": nullableJSON(request.ReasonParams),
				"last_error_message": nil, "updated_at": request.Now,
			}
			if err := tx.WithContext(ctx).Model(&CollectionWindow{}).Where("id = ?", window.ID).Updates(updates).Error; err != nil {
				return UsageWindowMutationResult{}, err
			}
			result.SourceHash = window.SourceHash
		} else {
			created := CollectionWindow{
				SiteID: request.SiteID, HourTS: request.HourTS, Status: CollectionWindowStatusMissing,
				FetchedRows: request.FetchedRows, LastFactRunID: &lastRunID,
				LastErrorCode: request.ReasonCode, LastErrorParams: request.ReasonParams, UpdatedAt: request.Now,
			}
			if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
				return UsageWindowMutationResult{}, err
			}
		}
		if _, err := ReconcileUsageCursor(ctx, tx, request.SiteID, *lockedScope.Site.StatisticsStartAt, request.Now); err != nil {
			return UsageWindowMutationResult{}, err
		}
		return result, nil
	}}, nil
}

func canonicalUsageFacts(siteID, hourTS, collectedAt int64, input []UsageFactInput) ([]UsageFactHourly, string, error) {
	if siteID <= 0 || hourTS <= 0 || hourTS%3600 != 0 || hourTS > math.MaxInt64-3600 || collectedAt <= 0 {
		return nil, "", ErrCollectionRunContract
	}
	type key struct {
		remoteUserID int64
		modelName    string
		channelID    int64
		useGroup     string
		tokenID      int64
		nodeName     string
	}
	byKey := make(map[key]UsageFactHourly, len(input))
	for _, value := range input {
		if value.RemoteUserID <= 0 || value.ChannelID < 0 || value.TokenID < 0 || value.RequestCount < 0 || value.Quota < 0 || value.TokenUsed < 0 ||
			!validUsageString(value.UsernameSnapshot, 255) || !validUsageString(value.ModelName, 255) ||
			!validUsageString(value.UseGroup, 128) || !validUsageString(value.TokenName, 255) || !validUsageString(value.NodeName, 128) {
			return nil, "", ErrCollectionRunContract
		}
		factKey := key{remoteUserID: value.RemoteUserID, modelName: value.ModelName, channelID: value.ChannelID,
			useGroup: value.UseGroup, tokenID: value.TokenID, nodeName: value.NodeName}
		fact, found := byKey[factKey]
		if !found {
			fact = UsageFactHourly{
				SiteID: siteID, RemoteUserID: value.RemoteUserID, UsernameSnapshot: value.UsernameSnapshot,
				ModelName: value.ModelName, ChannelID: value.ChannelID, UseGroup: value.UseGroup,
				TokenID: value.TokenID, TokenName: value.TokenName, NodeName: value.NodeName,
				HourTS: hourTS, CollectedAt: collectedAt,
			}
		} else {
			fact.UsernameSnapshot = canonicalUsageUsername(fact.UsernameSnapshot, value.UsernameSnapshot)
			fact.TokenName = canonicalUsageUsername(fact.TokenName, value.TokenName)
		}
		var ok bool
		if fact.RequestCount, ok = addUsageInt64(fact.RequestCount, value.RequestCount); !ok {
			return nil, "", ErrCollectionRunContract
		}
		if fact.Quota, ok = addUsageInt64(fact.Quota, value.Quota); !ok {
			return nil, "", ErrCollectionRunContract
		}
		if fact.TokenUsed, ok = addUsageInt64(fact.TokenUsed, value.TokenUsed); !ok {
			return nil, "", ErrCollectionRunContract
		}
		byKey[factKey] = fact
	}
	facts := make([]UsageFactHourly, 0, len(byKey))
	for _, fact := range byKey {
		facts = append(facts, fact)
	}
	sort.Slice(facts, func(left, right int) bool {
		if facts[left].RemoteUserID != facts[right].RemoteUserID {
			return facts[left].RemoteUserID < facts[right].RemoteUserID
		}
		if facts[left].ModelName != facts[right].ModelName {
			return facts[left].ModelName < facts[right].ModelName
		}
		if facts[left].ChannelID != facts[right].ChannelID {
			return facts[left].ChannelID < facts[right].ChannelID
		}
		if facts[left].UseGroup != facts[right].UseGroup {
			return facts[left].UseGroup < facts[right].UseGroup
		}
		if facts[left].TokenID != facts[right].TokenID {
			return facts[left].TokenID < facts[right].TokenID
		}
		return facts[left].NodeName < facts[right].NodeName
	})
	hash := sha256.New()
	hash.Write([]byte("usage-fact-hourly-v2\x00"))
	buffer := make([]byte, 8)
	for _, fact := range facts {
		for _, number := range []int64{fact.RemoteUserID, fact.ChannelID, fact.TokenID, fact.RequestCount, fact.Quota, fact.TokenUsed} {
			binary.BigEndian.PutUint64(buffer, uint64(number))
			hash.Write(buffer)
		}
		writeUsageHashString(hash.Write, buffer, fact.UsernameSnapshot)
		writeUsageHashString(hash.Write, buffer, fact.ModelName)
		writeUsageHashString(hash.Write, buffer, fact.UseGroup)
		writeUsageHashString(hash.Write, buffer, fact.TokenName)
		writeUsageHashString(hash.Write, buffer, fact.NodeName)
	}
	return facts, hex.EncodeToString(hash.Sum(nil)), nil
}

func writeUsageHashString(write func([]byte) (int, error), buffer []byte, value string) {
	binary.BigEndian.PutUint64(buffer, uint64(len(value)))
	_, _ = write(buffer)
	_, _ = write([]byte(value))
}

func validateUsageMutationScope(
	scope UsageWindowMutationScope,
	runID, windowID, siteID int64,
	expectedConfigVersion int,
	hourTS int64,
	attemptCount int,
	requestID string,
	now int64,
) error {
	if now <= 0 || hourTS > math.MaxInt64-3600 || hourTS+3600 > now {
		return ErrCollectionRunContract
	}
	if scope.Site.ID != siteID || scope.Site.ConfigVersion != expectedConfigVersion || scope.Site.StatisticsStartAt == nil ||
		*scope.Site.StatisticsStartAt <= 0 || *scope.Site.StatisticsStartAt%3600 != 0 ||
		*scope.Site.StatisticsStartAt >= hourTS+3600 ||
		scope.Site.ManagementStatus != constant.SiteManagementActive || scope.Site.AuthStatus != constant.SiteAuthAuthorized ||
		scope.Site.StatisticsEndAt != nil || !scope.Site.DataExportEnabled {
		return ErrSiteRunConfigChanged
	}
	if scope.Run.ID != runID || scope.Run.SiteID == nil || *scope.Run.SiteID != siteID ||
		scope.Run.SiteConfigVersion != expectedConfigVersion || scope.Run.Status != CollectionTaskStatusRunning ||
		scope.Run.LastRequestID != requestID || !isUsageTaskType(scope.Run.TaskType) ||
		scope.Run.StartTimestamp == nil || scope.Run.EndTimestamp == nil ||
		hourTS < *scope.Run.StartTimestamp || hourTS >= *scope.Run.EndTimestamp {
		return ErrCollectionTaskClaimLost
	}
	if scope.Window.ID != windowID || scope.Window.RunID != runID || scope.Window.SiteID != siteID ||
		scope.Window.HourTS != hourTS || scope.Window.Status != CollectionTaskStatusRunning ||
		scope.Window.AttemptCount != attemptCount {
		return ErrCollectionTaskClaimLost
	}
	return nil
}

func lockUsageMutationScope(
	ctx context.Context,
	tx *gorm.DB,
	declared UsageWindowMutationScope,
	runID, windowID, siteID int64,
	expectedConfigVersion int,
	hourTS int64,
	attemptCount int,
	requestID string,
	now int64,
) (UsageWindowMutationScope, error) {
	if tx == nil {
		return UsageWindowMutationScope{}, ErrCollectionRunContract
	}
	if declared.Site.ID != siteID {
		return UsageWindowMutationScope{}, ErrSiteRunConfigChanged
	}
	if declared.Run.ID != runID || declared.Window.ID != windowID {
		return UsageWindowMutationScope{}, ErrCollectionTaskClaimLost
	}
	var locked UsageWindowMutationScope
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked.Site, siteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UsageWindowMutationScope{}, ErrSiteRunConfigChanged
		}
		return UsageWindowMutationScope{}, err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked.Run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UsageWindowMutationScope{}, ErrCollectionTaskClaimLost
		}
		return UsageWindowMutationScope{}, err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked.Window, windowID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UsageWindowMutationScope{}, ErrCollectionTaskClaimLost
		}
		return UsageWindowMutationScope{}, err
	}
	if err := validateUsageMutationScope(locked, runID, windowID, siteID, expectedConfigVersion,
		hourTS, attemptCount, requestID, now); err != nil {
		return UsageWindowMutationScope{}, err
	}
	return locked, nil
}

func lockCollectionWindow(ctx context.Context, tx *gorm.DB, siteID, hourTS int64) (CollectionWindow, bool, error) {
	if tx == nil {
		return CollectionWindow{}, false, ErrCollectionRunContract
	}
	var window CollectionWindow
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("site_id = ? AND hour_ts = ?", siteID, hourTS).Take(&window).Error
	if err == nil {
		return window, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CollectionWindow{}, false, nil
	}
	return CollectionWindow{}, false, err
}

func validUsageMutationRequest(runID, windowID, siteID int64, expectedConfigVersion int, hourTS int64,
	attemptCount int, requestID string, now, fetchedRows int64) bool {
	return runID > 0 && windowID > 0 && siteID > 0 && expectedConfigVersion > 0 && hourTS > 0 && hourTS%3600 == 0 &&
		hourTS <= math.MaxInt64-3600 && attemptCount > 0 && validCollectionRequestID(requestID) && now > 0 && fetchedRows >= 0
}

func isUsageTaskType(taskType string) bool {
	return taskType == constant.TaskTypeUsageHour || taskType == constant.TaskTypeUsageBackfill ||
		taskType == constant.TaskTypeUsageValidation
}

func validUsageString(value string, maximum int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum
}

func canonicalUsageUsername(current, candidate string) string {
	if current == "" {
		return candidate
	}
	if candidate == "" || current <= candidate {
		return current
	}
	return candidate
}

func addUsageInt64(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, false
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}
