package service

import (
	"context"
	"errors"
	"log"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

const defaultAlertPostCommitTimeout = 3 * time.Second

type AlertWindowSource string

const (
	AlertWindowSourceCollection AlertWindowSource = "collection_window"
	AlertWindowSourceValidation AlertWindowSource = "validation_run_window"
	AlertWindowSourceBackfill   AlertWindowSource = "backfill_run"
)

// AlertPostCommitTrigger identifies the committed source row. Business
// services provide database facts only; canonical sample identities remain
// owned by the alert builders.
type AlertPostCommitTrigger struct {
	Source     AlertSampleSource
	SiteID     int64
	AccountID  int64
	RowID      int64
	HourTS     int64
	ObservedAt int64
	Window     AlertWindowSource
	ScopeType  string
	ScopeID    int64
}

type PostCommitNotifier interface {
	NotifyAfterCommit(context.Context, AlertPostCommitTrigger)
}

type AlertPostCommitSnapshotReader interface {
	LoadProbeAlertSnapshot(context.Context, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadResourceAlertSnapshot(context.Context, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadUserAlertSnapshot(context.Context, int64, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadChannelAlertSnapshot(context.Context, int64, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadWindowAlertSnapshot(context.Context, string, int64, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadAuthAlertSnapshot(context.Context, int64, int64) (model.AlertEvaluationSnapshot, error)
	LoadLifecycleAlertSnapshot(context.Context, string, int64, int64) (model.AlertEvaluationSnapshot, error)
}

type alertPostCommitBatchEvaluator interface {
	EvaluateBatchAfterCommit(context.Context, []AlertEvaluation) ([]AlertEvaluationResult, error)
}

type AlertPostCommitCoordinatorOptions struct {
	Database           *gorm.DB
	Reader             AlertPostCommitSnapshotReader
	Hook               alertPostCommitBatchEvaluator
	Clock              common.Clock
	ResourceFreshness  time.Duration
	Timeout            time.Duration
	RequestIDGenerator AlertScanRequestIDGenerator
	Logf               func(string, ...any)
}

type AlertPostCommitCoordinator struct {
	reader            AlertPostCommitSnapshotReader
	hook              alertPostCommitBatchEvaluator
	clock             common.Clock
	resourceFreshness time.Duration
	timeout           time.Duration
	requestID         AlertScanRequestIDGenerator
	logf              func(string, ...any)
}

func NewAlertPostCommitCoordinator(options AlertPostCommitCoordinatorOptions) (*AlertPostCommitCoordinator, error) {
	if options.Reader == nil && options.Database != nil {
		options.Reader = model.NewAlertEvaluationRepository(options.Database)
	}
	if options.Reader == nil || options.Hook == nil || options.Clock == nil {
		return nil, errors.New("alert post-commit coordinator dependencies are required")
	}
	if options.ResourceFreshness <= 0 {
		options.ResourceFreshness = defaultAlertResourceFreshness
	}
	if options.Timeout <= 0 {
		options.Timeout = defaultAlertPostCommitTimeout
	}
	if options.RequestIDGenerator == nil {
		options.RequestIDGenerator = newAlertScanRequestID
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	return &AlertPostCommitCoordinator{
		reader: options.Reader, hook: options.Hook, clock: options.Clock,
		resourceFreshness: options.ResourceFreshness,
		timeout:           options.Timeout, requestID: options.RequestIDGenerator, logf: options.Logf,
	}, nil
}

func (coordinator *AlertPostCommitCoordinator) NotifyAfterCommit(ctx context.Context, trigger AlertPostCommitTrigger) {
	if coordinator == nil {
		return
	}
	if err := validateAlertPostCommitTrigger(trigger); err != nil {
		coordinator.logFailure(trigger, err)
		return
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	bounded, cancel := context.WithTimeout(base, coordinator.timeout)
	defer cancel()
	if err := coordinator.evaluate(bounded, trigger); err != nil {
		coordinator.logFailure(trigger, err)
	}
}

func (coordinator *AlertPostCommitCoordinator) evaluate(ctx context.Context, trigger AlertPostCommitTrigger) error {
	requestID, err := coordinator.requestID()
	if err != nil || !validDingTalkRequestID(requestID) {
		return errors.New("generate alert post-commit request ID")
	}
	var snapshot model.AlertEvaluationSnapshot
	switch trigger.Source {
	case AlertSampleSourceProbe:
		snapshot, err = coordinator.reader.LoadProbeAlertSnapshot(ctx, trigger.SiteID, trigger.ObservedAt)
	case AlertSampleSourceResource:
		snapshot, err = coordinator.reader.LoadResourceAlertSnapshot(ctx, trigger.SiteID, trigger.ObservedAt)
	case AlertSampleSourceUser:
		snapshot, err = coordinator.reader.LoadUserAlertSnapshot(ctx, trigger.SiteID, trigger.AccountID, trigger.ObservedAt)
	case AlertSampleSourceChannel:
		snapshot, err = coordinator.reader.LoadChannelAlertSnapshot(ctx, trigger.SiteID, trigger.HourTS, trigger.ObservedAt)
	case AlertSampleSourceWindow:
		snapshot, err = coordinator.reader.LoadWindowAlertSnapshot(
			ctx, string(trigger.Window), trigger.RowID, trigger.HourTS, trigger.ObservedAt,
		)
	case AlertSampleSourceAuth:
		snapshot, err = coordinator.reader.LoadAuthAlertSnapshot(ctx, trigger.SiteID, trigger.ObservedAt)
	case AlertSampleSourceLifecycle:
		snapshot, err = coordinator.reader.LoadLifecycleAlertSnapshot(ctx, trigger.ScopeType, trigger.ScopeID, trigger.ObservedAt)
	default:
		return errors.New("unsupported alert post-commit source")
	}
	if err != nil {
		return err
	}
	evaluations, err := buildAlertEvaluations(
		snapshot,
		coordinator.clock.Now().Unix(),
		coordinator.resourceFreshness,
		requestID,
	)
	if err != nil {
		return err
	}
	evaluations = postCommitEvaluationsForSource(trigger.Source, evaluations)
	if len(evaluations) == 0 {
		return nil
	}
	_, err = coordinator.hook.EvaluateBatchAfterCommit(ctx, evaluations)
	return err
}

func validateAlertPostCommitTrigger(trigger AlertPostCommitTrigger) error {
	if trigger.ObservedAt <= 0 {
		return errors.New("alert post-commit observed_at is required")
	}
	switch trigger.Source {
	case AlertSampleSourceProbe, AlertSampleSourceResource, AlertSampleSourceAuth:
		if trigger.SiteID <= 0 {
			return errors.New("alert post-commit site is required")
		}
	case AlertSampleSourceChannel:
		if trigger.SiteID <= 0 || trigger.HourTS <= 0 {
			return errors.New("alert post-commit channel scope is required")
		}
	case AlertSampleSourceUser:
		if trigger.SiteID <= 0 && trigger.AccountID <= 0 {
			return errors.New("alert post-commit user scope is required")
		}
	case AlertSampleSourceWindow:
		if trigger.RowID <= 0 ||
			(trigger.Window != AlertWindowSourceCollection && trigger.Window != AlertWindowSourceValidation &&
				trigger.Window != AlertWindowSourceBackfill) ||
			(trigger.Window != AlertWindowSourceBackfill && trigger.HourTS <= 0) {
			return errors.New("alert post-commit window scope is required")
		}
	case AlertSampleSourceLifecycle:
		if trigger.ScopeID <= 0 ||
			(trigger.ScopeType != "site" && trigger.ScopeType != "customer" && trigger.ScopeType != "account") {
			return errors.New("alert post-commit lifecycle scope is required")
		}
	default:
		return errors.New("alert post-commit source is invalid")
	}
	return nil
}

func postCommitEvaluationsForSource(source AlertSampleSource, evaluations []AlertEvaluation) []AlertEvaluation {
	allowed := map[string]struct{}{}
	switch source {
	case AlertSampleSourceProbe:
		allowed["site_offline"] = struct{}{}
		allowed["site_export_disabled"] = struct{}{}
	case AlertSampleSourceResource:
		for _, key := range []string{"site_no_instance", "instance_stale", "instance_offline", "cpu_high", "memory_high", "disk_high"} {
			allowed[key] = struct{}{}
		}
	case AlertSampleSourceUser:
		for _, key := range []string{"account_missing", "account_identity_mismatch", "account_disabled", "account_quota_empty"} {
			allowed[key] = struct{}{}
		}
	case AlertSampleSourceChannel:
		for _, key := range []string{"channel_balance_low", "channel_response_time_high", "channel_availability_low"} {
			allowed[key] = struct{}{}
		}
	case AlertSampleSourceWindow:
		for _, key := range []string{"collection_missing", "validation_failed", "backfill_failed"} {
			allowed[key] = struct{}{}
		}
	case AlertSampleSourceAuth, AlertSampleSourceLifecycle:
		return evaluations
	}
	result := make([]AlertEvaluation, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if _, exists := allowed[evaluation.RuleKey]; exists {
			result = append(result, evaluation)
		}
	}
	return result
}

func (coordinator *AlertPostCommitCoordinator) logFailure(trigger AlertPostCommitTrigger, err error) {
	coordinator.logf(
		"alert post-commit evaluation failed source=%s site_id=%d account_id=%d row_id=%d hour_ts=%d observed_at=%d scope_type=%s scope_id=%d error_type=%T",
		trigger.Source, trigger.SiteID, trigger.AccountID, trigger.RowID, trigger.HourTS,
		trigger.ObservedAt, trigger.ScopeType, trigger.ScopeID, err,
	)
}

var _ PostCommitNotifier = (*AlertPostCommitCoordinator)(nil)
