package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const (
	defaultAlertResourceFreshness = 2 * time.Minute
	defaultAlertChannelFreshness  = 2 * time.Hour
)

type AlertEvaluationSnapshotReader interface {
	LoadSnapshot(context.Context) (model.AlertEvaluationSnapshot, error)
}

type AlertScanRequestIDGenerator func() (string, error)

type AlertEvaluationScannerOptions struct {
	Database           *gorm.DB
	Reader             AlertEvaluationSnapshotReader
	Evaluator          AlertEvaluator
	Clock              common.Clock
	ResourceFreshness  time.Duration
	RequestIDGenerator AlertScanRequestIDGenerator
}

type AlertEvaluationScanner struct {
	reader            AlertEvaluationSnapshotReader
	evaluator         AlertEvaluator
	clock             common.Clock
	resourceFreshness time.Duration
	requestID         AlertScanRequestIDGenerator
}

type AlertScanResult struct {
	EvaluationCount int
	FiringCount     int
	ResolvedCount   int
}

func NewAlertEvaluationScanner(options AlertEvaluationScannerOptions) (*AlertEvaluationScanner, error) {
	if options.Reader == nil && options.Database != nil {
		options.Reader = model.NewAlertEvaluationRepository(options.Database)
	}
	if options.Reader == nil || options.Evaluator == nil || options.Clock == nil {
		return nil, errors.New("alert evaluation scanner dependencies are required")
	}
	if options.ResourceFreshness <= 0 {
		options.ResourceFreshness = defaultAlertResourceFreshness
	}
	if options.RequestIDGenerator == nil {
		options.RequestIDGenerator = newAlertScanRequestID
	}
	return &AlertEvaluationScanner{
		reader: options.Reader, evaluator: options.Evaluator, clock: options.Clock,
		resourceFreshness: options.ResourceFreshness,
		requestID:         options.RequestIDGenerator,
	}, nil
}

func (scanner *AlertEvaluationScanner) RunOnce(ctx context.Context) (AlertScanResult, error) {
	snapshot, err := scanner.reader.LoadSnapshot(ctx)
	if err != nil {
		return AlertScanResult{}, err
	}
	requestID, err := scanner.requestID()
	if err != nil || !validDingTalkRequestID(requestID) {
		return AlertScanResult{}, errors.New("generate alert scan request ID")
	}
	now := scanner.clock.Now().Unix()
	if now <= 0 {
		return AlertScanResult{}, errors.New("alert scan clock is invalid")
	}
	evaluations, err := buildAlertEvaluations(
		snapshot,
		now,
		scanner.resourceFreshness,
		requestID,
	)
	if err != nil {
		return AlertScanResult{}, err
	}
	result := AlertScanResult{EvaluationCount: len(evaluations)}
	for _, evaluation := range evaluations {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		outcome, err := scanner.evaluator.Evaluate(ctx, evaluation)
		if err != nil {
			return result, err
		}
		switch outcome.Transition {
		case "firing":
			result.FiringCount++
		case "resolved":
			result.ResolvedCount++
		}
	}
	return result, nil
}

func buildAlertEvaluations(
	snapshot model.AlertEvaluationSnapshot,
	now int64,
	resourceFreshness time.Duration,
	requestID string,
) ([]AlertEvaluation, error) {
	result := make([]AlertEvaluation, 0, len(snapshot.Sites)*5+len(snapshot.Instances)*5+len(snapshot.Accounts)*4+len(snapshot.Channels)*3)
	for _, site := range snapshot.Sites {
		evaluations, err := siteAlertEvaluations(site, now, resourceFreshness, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluations...)
	}
	for _, instance := range snapshot.Instances {
		evaluations, err := instanceAlertEvaluations(instance, now, resourceFreshness, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluations...)
	}
	for _, account := range snapshot.Accounts {
		evaluations, err := accountAlertEvaluations(account, now, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluations...)
	}
	for _, channel := range snapshot.Channels {
		evaluations, err := channelAlertEvaluations(channel, now, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluations...)
	}
	validationTargets := make(map[string]*validationEvaluationTarget)
	for _, window := range snapshot.CollectionWindows {
		evaluation, err := collectionMissingEvaluation(window, now, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluation)
		key := collectionEvaluationKey(window.SiteID, window.HourTS)
		target := validationTargets[key]
		if target == nil {
			target = &validationEvaluationTarget{}
			validationTargets[key] = target
		}
		copy := window
		target.collection = &copy
	}
	for _, validation := range snapshot.Validations {
		key := collectionEvaluationKey(validation.SiteID, validation.HourTS)
		target := validationTargets[key]
		if target == nil {
			target = &validationEvaluationTarget{}
			validationTargets[key] = target
		}
		copy := validation
		target.validation = &copy
	}
	validationKeys := make([]string, 0, len(validationTargets))
	for key := range validationTargets {
		validationKeys = append(validationKeys, key)
	}
	sortAlertStrings(validationKeys)
	for _, key := range validationKeys {
		evaluation, err := validationFailedEvaluation(*validationTargets[key], now, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluation)
	}
	for _, backfill := range snapshot.Backfills {
		evaluation, err := backfillFailedEvaluation(backfill, now, requestID)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluation)
	}
	return result, nil
}

func channelAlertEvaluations(
	channel model.AlertChannelEvaluationSnapshot,
	now int64,
	requestID string,
) ([]AlertEvaluation, error) {
	targetKey := strconv.FormatInt(channel.SiteID, 10)
	lifecycleAt := alertScanObservedAt(channel.SiteUpdatedAt, now)
	lifecycleIdentity, err := BuildLifecycleAlertSampleIdentity("site", channel.SiteID, lifecycleAt)
	if err != nil {
		return nil, fmt.Errorf("build channel lifecycle alert identity: %w", err)
	}
	observedAt := alertScanPointerObservedAt(channel.CollectedAt, lifecycleAt)
	hourTS := int64(0)
	if channel.HourTS != nil {
		hourTS = *channel.HourTS
	}
	identity := lifecycleIdentity
	if hourTS > 0 {
		identity, err = BuildChannelAlertSampleIdentity(
			channel.SiteID,
			hourTS,
			observedAt,
			alertOptionalInt64Evidence("channel_count", channel.ChannelCount),
			alertOptionalStringEvidence("data_status", channel.DataStatus),
			alertOptionalIntEvidence("config_version", channel.ConfigVersion),
			alertOptionalStringEvidence("balance_total", channel.BalanceTotal),
			alertOptionalStringEvidence("response_time_avg_ms", channel.ResponseTimeAvgMS),
			alertOptionalStringEvidence("availability_rate", channel.AvailabilityRate),
		)
		if err != nil {
			return nil, fmt.Errorf("build channel alert identity: %w", err)
		}
	}
	base := func(ruleKey string) AlertEvaluation {
		return newAlertScanEvaluation(ruleKey, channel.SiteID, "site", targetKey, channel.SiteName, requestID, identity)
	}
	rules := []AlertEvaluation{base("channel_balance_low"), base("channel_response_time_high"), base("channel_availability_low")}
	if !siteAuthorizedMonitoringApplicable(channel.ManagementStatus, channel.AuthStatus, channel.StatisticsEndAt) {
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index] = scopeInactiveAlertEvaluation(rules[index], "site", channel.SiteID, channel.SiteName)
		}
		return rules, nil
	}
	complete := channel.HourTS != nil && channel.CollectedAt != nil && channel.ChannelCount != nil && channel.DataStatus != nil && *channel.DataStatus == "complete" &&
		channel.SiteConfigVersion > 0 && channel.ConfigVersion != nil && *channel.ConfigVersion == channel.SiteConfigVersion &&
		channel.BalanceTotal != nil && channel.ResponseTimeAvgMS != nil && channel.AvailabilityRate != nil &&
		*channel.CollectedAt > 0 && *channel.CollectedAt <= now && now-*channel.CollectedAt <= int64(defaultAlertChannelFreshness/time.Second)
	if !complete {
		for index := range rules {
			rules[index].State = AlertSampleUnknown
		}
		return rules, nil
	}
	rules[0] = knownAlertEvaluation(rules[0], *channel.BalanceTotal)
	rules[1] = knownAlertEvaluation(rules[1], *channel.ResponseTimeAvgMS)
	rules[2] = knownAlertEvaluation(rules[2], *channel.AvailabilityRate)
	return rules, nil
}

func siteAlertEvaluations(
	site model.AlertSiteEvaluationSnapshot,
	now int64,
	resourceFreshness time.Duration,
	requestID string,
) ([]AlertEvaluation, error) {
	updatedAt := alertScanObservedAt(site.UpdatedAt, now)
	probeAt := alertScanPointerObservedAt(site.LastProbeAt, updatedAt)
	probeIdentity, err := BuildProbeAlertSampleIdentity(site.ID, probeAt)
	if err != nil {
		return nil, fmt.Errorf("build site probe alert identity: %w", err)
	}
	authIdentity, err := BuildAuthAlertSampleIdentity(site.ID, updatedAt,
		"config_version:"+strconv.Itoa(site.ConfigVersion))
	if err != nil {
		return nil, fmt.Errorf("build site auth alert identity: %w", err)
	}
	lifecycleIdentity, err := BuildLifecycleAlertSampleIdentity("site", site.ID, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("build site lifecycle alert identity: %w", err)
	}
	resourceAt := alertScanPointerObservedAt(site.ResourceSampleAt, updatedAt)
	resourceIdentity, err := BuildResourceAlertSampleIdentity(site.ID, "site", resourceAt,
		alertOptionalInt64Evidence("sample_id", site.ResourceSampleID))
	if err != nil {
		return nil, fmt.Errorf("build site resource alert identity: %w", err)
	}
	base := func(ruleKey string, identity AlertSampleIdentity) AlertEvaluation {
		return newAlertScanEvaluation(ruleKey, site.ID, "site", strconv.FormatInt(site.ID, 10), site.Name, requestID, identity)
	}
	offline := base("site_offline", probeIdentity)
	if !siteMonitoringApplicable(site.ManagementStatus, site.StatisticsEndAt) {
		offline = alertEvaluationWithIdentity(offline, lifecycleIdentity)
		offline = scopeInactiveAlertEvaluation(offline, "site", site.ID, site.Name)
	} else if site.LastProbeAt == nil || site.ProbeFailCount < 0 {
		offline.State = AlertSampleUnknown
	} else {
		offline = knownAlertEvaluation(offline, strconv.Itoa(site.ProbeFailCount))
	}
	auth := base("site_auth_expired", authIdentity)
	if !siteMonitoringApplicable(site.ManagementStatus, site.StatisticsEndAt) {
		auth = alertEvaluationWithIdentity(auth, lifecycleIdentity)
		auth = scopeInactiveAlertEvaluation(auth, "site", site.ID, site.Name)
	} else {
		switch site.AuthStatus {
		case constant.SiteAuthExpired:
			auth = knownAlertEvaluation(auth, "1")
		case constant.SiteAuthAuthorized, constant.SiteAuthUnauthorized:
			auth = knownAlertEvaluation(auth, "0")
		default:
			auth.State = AlertSampleUnknown
		}
	}
	export := base("site_export_disabled", lifecycleIdentity)
	if !siteAuthorizedMonitoringApplicable(site.ManagementStatus, site.AuthStatus, site.StatisticsEndAt) {
		export = scopeInactiveAlertEvaluation(export, "site", site.ID, site.Name)
	} else if site.DataExportEnabled {
		export = knownAlertEvaluation(export, "1")
	} else {
		export = knownAlertEvaluation(export, "0")
	}
	noInstance := base("site_no_instance", resourceIdentity)
	if !siteAuthorizedMonitoringApplicable(site.ManagementStatus, site.AuthStatus, site.StatisticsEndAt) {
		noInstance = alertEvaluationWithIdentity(noInstance, lifecycleIdentity)
		noInstance = scopeInactiveAlertEvaluation(noInstance, "site", site.ID, site.Name)
	} else if !freshAlertSample(site.ResourceSampleAt, now, resourceFreshness) || site.ResourceInstanceCount == nil || *site.ResourceInstanceCount < 0 {
		noInstance.State = AlertSampleUnknown
	} else {
		noInstance = knownAlertEvaluation(noInstance, strconv.Itoa(*site.ResourceInstanceCount))
	}
	return []AlertEvaluation{offline, auth, export, noInstance}, nil
}

func instanceAlertEvaluations(
	instance model.AlertInstanceEvaluationSnapshot,
	now int64,
	resourceFreshness time.Duration,
	requestID string,
) ([]AlertEvaluation, error) {
	updatedAt := alertScanObservedAt(instance.UpdatedAt, instance.LastSyncedAt, now)
	sampledAt := alertScanPointerObservedAt(instance.SampledAt, updatedAt)
	identity, err := BuildResourceAlertSampleIdentity(instance.SiteID, instance.NodeName, sampledAt,
		"instance_id:"+strconv.FormatInt(instance.ID, 10), alertOptionalInt64Evidence("sample_id", instance.SampleID))
	if err != nil {
		return nil, fmt.Errorf("build instance resource alert identity: %w", err)
	}
	base := func(ruleKey string) AlertEvaluation {
		return newAlertScanEvaluation(
			ruleKey, instance.SiteID, "instance",
			strconv.FormatInt(instance.SiteID, 10)+"/"+instance.NodeName,
			instance.NodeName, requestID, identity,
		)
	}
	rules := []AlertEvaluation{base("instance_stale"), base("instance_offline"), base("cpu_high"), base("memory_high"), base("disk_high")}
	if instance.RetiredAt != nil {
		lifecycleIdentity, identityErr := BuildLifecycleAlertSampleIdentity("instance", instance.ID, *instance.RetiredAt)
		if identityErr != nil {
			return nil, fmt.Errorf("build instance retirement alert identity: %w", identityErr)
		}
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index] = scopeInactiveAlertEvaluation(rules[index], "instance", instance.ID, instance.NodeName)
		}
		return rules, nil
	}
	if !siteAuthorizedMonitoringApplicable(instance.ManagementStatus, instance.AuthStatus, instance.StatisticsEndAt) {
		lifecycleAt := alertScanLatestObservedAt(instance.SiteUpdatedAt, instance.UpdatedAt, now)
		lifecycleIdentity, identityErr := BuildLifecycleAlertSampleIdentity("site", instance.SiteID, lifecycleAt)
		if identityErr != nil {
			return nil, fmt.Errorf("build instance lifecycle alert identity: %w", identityErr)
		}
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index] = scopeInactiveAlertEvaluation(rules[index], "site", instance.SiteID, instance.SiteName)
		}
		return rules, nil
	}
	fresh := freshAlertSample(instance.SampledAt, now, resourceFreshness)
	if fresh && instance.LastSeenAt != nil && *instance.LastSeenAt <= now {
		rules[0] = knownAlertEvaluation(rules[0], strconv.FormatInt(now-*instance.LastSeenAt, 10))
	} else {
		rules[0].State = AlertSampleUnknown
	}
	if fresh && instance.SampleStatus != nil && *instance.SampleStatus != "" {
		switch *instance.SampleStatus {
		case "online":
			rules[1] = knownAlertEvaluation(rules[1], "1")
		default:
			rules[1] = knownAlertEvaluation(rules[1], "0")
		}
	} else {
		rules[1].State = AlertSampleUnknown
	}
	metrics := []*string{instance.CPUPercent, instance.MemoryPercent, instance.DiskUsedPercent}
	for index, metric := range metrics {
		if fresh && metric != nil {
			rules[index+2] = knownAlertEvaluation(rules[index+2], *metric)
		} else {
			rules[index+2].State = AlertSampleUnknown
		}
	}
	return rules, nil
}

func accountAlertEvaluations(account model.AlertAccountEvaluationSnapshot, now int64, requestID string) ([]AlertEvaluation, error) {
	name := account.DisplayName
	if strings.TrimSpace(name) == "" {
		name = account.Username
	}
	observedAt := alertScanPointerObservedAt(account.LastSyncedAt, alertScanObservedAt(account.UpdatedAt, now))
	identity, err := BuildUserAlertSampleIdentity(account.SiteID, account.ID, observedAt)
	if err != nil {
		return nil, fmt.Errorf("build account alert identity: %w", err)
	}
	lifecycleAt := alertScanLatestObservedAt(account.UpdatedAt, account.CustomerUpdatedAt, account.SiteUpdatedAt, now)
	lifecycleIdentity, err := BuildLifecycleAlertSampleIdentity("account", account.ID, lifecycleAt)
	if err != nil {
		return nil, fmt.Errorf("build account lifecycle alert identity: %w", err)
	}
	base := func(ruleKey string) AlertEvaluation {
		return newAlertScanEvaluation(ruleKey, account.SiteID, "account", strconv.FormatInt(account.ID, 10), name, requestID, identity)
	}
	rules := []AlertEvaluation{base("account_missing"), base("account_identity_mismatch"), base("account_disabled"), base("account_quota_empty")}
	if account.ManagedStatus == model.AccountManagedStatusArchived || account.CustomerStatus == dto.CustomerStatusDisabled {
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index] = scopeInactiveAlertEvaluation(rules[index], "account", account.ID, name)
		}
		return rules, nil
	}
	if account.SiteManagement != constant.SiteManagementActive || account.StatisticsEndAt != nil {
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index] = scopeInactiveAlertEvaluation(rules[index], "site", account.SiteID, "")
		}
		return rules, nil
	}
	if account.SiteAuthStatus != constant.SiteAuthAuthorized || account.LastSyncedAt == nil {
		for index := range rules {
			rules[index] = alertEvaluationWithIdentity(rules[index], lifecycleIdentity)
			rules[index].State = AlertSampleUnknown
		}
		return rules, nil
	}
	switch account.RemoteState {
	case model.AccountRemoteStateMissing:
		rules[0] = knownAlertEvaluation(rules[0], "0")
		rules[1] = knownAlertEvaluation(rules[1], "1")
		rules[2] = scopeInactiveAlertEvaluation(rules[2], "account", account.ID, name)
		rules[3] = scopeInactiveAlertEvaluation(rules[3], "account", account.ID, name)
	case model.AccountRemoteStateIdentityMismatch:
		rules[0] = knownAlertEvaluation(rules[0], "1")
		rules[1] = knownAlertEvaluation(rules[1], "0")
		rules[2] = scopeInactiveAlertEvaluation(rules[2], "account", account.ID, name)
		rules[3] = scopeInactiveAlertEvaluation(rules[3], "account", account.ID, name)
	case model.AccountRemoteStateNormal:
		rules[0] = knownAlertEvaluation(rules[0], "1")
		rules[1] = knownAlertEvaluation(rules[1], "1")
		if account.RemoteStatus == 1 {
			rules[2] = knownAlertEvaluation(rules[2], "1")
		} else {
			rules[2] = knownAlertEvaluation(rules[2], "0")
		}
		rules[3] = knownAlertEvaluation(rules[3], strconv.FormatInt(account.Quota, 10))
	default:
		for index := range rules {
			rules[index].State = AlertSampleUnknown
		}
	}
	return rules, nil
}

func collectionMissingEvaluation(window model.AlertCollectionEvaluationSnapshot, now int64, requestID string) (AlertEvaluation, error) {
	observedAt := alertScanObservedAt(window.UpdatedAt, window.HourTS, now)
	identity, err := BuildWindowAlertSampleIdentity(window.SiteID, strconv.FormatInt(window.HourTS, 10), observedAt,
		"window_id:"+strconv.FormatInt(window.ID, 10))
	if err != nil {
		return AlertEvaluation{}, fmt.Errorf("build collection window alert identity: %w", err)
	}
	evaluation := newAlertScanEvaluation(
		"collection_missing", window.SiteID, "collection",
		collectionEvaluationKey(window.SiteID, window.HourTS), strconv.FormatInt(window.HourTS, 10), requestID, identity,
	)
	if !collectionAlertApplicable(
		window.ManagementStatus, window.AuthStatus, window.DataExportEnabled,
		window.StatisticsStartAt, window.StatisticsEndAt, window.HourTS,
	) {
		lifecycleAt := alertScanLatestObservedAt(window.SiteUpdatedAt, window.UpdatedAt, now)
		lifecycleIdentity, identityErr := BuildLifecycleAlertSampleIdentity("site", window.SiteID, lifecycleAt)
		if identityErr != nil {
			return AlertEvaluation{}, fmt.Errorf("build collection lifecycle alert identity: %w", identityErr)
		}
		evaluation = alertEvaluationWithIdentity(evaluation, lifecycleIdentity)
		return scopeInactiveAlertEvaluation(evaluation, "site", window.SiteID, window.SiteName), nil
	}
	switch window.Status {
	case model.CollectionWindowStatusMissing:
		// A validation mismatch is a more specific diagnosis for the same hour.
		// Close any generic gap event and let validation_failed own the incident.
		if window.LastErrorCode == string(constant.MessageDataValidationMismatch) {
			return knownAlertEvaluation(evaluation, "0"), nil
		}
		return knownAlertEvaluation(evaluation, "1"), nil
	case model.CollectionWindowStatusComplete:
		return knownAlertEvaluation(evaluation, "0"), nil
	default:
		evaluation.State = AlertSampleUnknown
		return evaluation, nil
	}
}

type validationEvaluationTarget struct {
	collection *model.AlertCollectionEvaluationSnapshot
	validation *model.AlertValidationEvaluationSnapshot
}

func validationFailedEvaluation(target validationEvaluationTarget, now int64, requestID string) (AlertEvaluation, error) {
	var siteID, hourTS int64
	var siteName, managementStatus, authStatus string
	var dataExportEnabled bool
	var statisticsEndAt *int64
	observedAt := int64(0)
	evidence := []string{}
	if target.collection != nil {
		siteID, hourTS, siteName = target.collection.SiteID, target.collection.HourTS, target.collection.SiteName
		managementStatus, authStatus, dataExportEnabled = target.collection.ManagementStatus, target.collection.AuthStatus, target.collection.DataExportEnabled
		statisticsEndAt = target.collection.StatisticsEndAt
		observedAt = target.collection.UpdatedAt
		evidence = append(evidence, "collection_id:"+strconv.FormatInt(target.collection.ID, 10))
	} else if target.validation != nil {
		siteID, hourTS, siteName = target.validation.SiteID, target.validation.HourTS, target.validation.SiteName
		managementStatus, authStatus, dataExportEnabled = target.validation.ManagementStatus, target.validation.AuthStatus, target.validation.DataExportEnabled
		statisticsEndAt = target.validation.StatisticsEndAt
	}
	if target.validation != nil {
		if target.validation.UpdatedAt > observedAt {
			observedAt = target.validation.UpdatedAt
		}
		evidence = append(evidence, "run_window_id:"+strconv.FormatInt(target.validation.RunWindowID, 10))
	}
	observedAt = alertScanObservedAt(observedAt, hourTS, now)
	identity, err := BuildWindowAlertSampleIdentity(siteID, strconv.FormatInt(hourTS, 10), observedAt, evidence...)
	if err != nil {
		return AlertEvaluation{}, fmt.Errorf("build validation window alert identity: %w", err)
	}
	evaluation := newAlertScanEvaluation(
		"validation_failed", siteID, "collection", collectionEvaluationKey(siteID, hourTS),
		strconv.FormatInt(hourTS, 10), requestID, identity,
	)
	// Older releases selected either the validation run window or the collection
	// window as the identity evidence. When both rows are now available, retain
	// only the identity of the row that supplied the watermark as an explicit
	// upgrade alias. This lets Evaluate reconcile that exact persisted sample
	// without weakening same-timestamp conflict detection for unrelated samples.
	if target.collection != nil && target.validation != nil {
		windowKey := strconv.FormatInt(hourTS, 10)
		if target.collection.UpdatedAt > 0 && target.collection.UpdatedAt == observedAt {
			prior, priorErr := BuildWindowAlertSampleIdentity(siteID, windowKey, observedAt,
				"collection_id:"+strconv.FormatInt(target.collection.ID, 10))
			if priorErr != nil {
				return AlertEvaluation{}, fmt.Errorf("build prior collection validation identity: %w", priorErr)
			}
			evaluation.PriorSampleKeys = append(evaluation.PriorSampleKeys, prior.SampleKey)
		}
		if target.validation.UpdatedAt > 0 && target.validation.UpdatedAt == observedAt {
			prior, priorErr := BuildWindowAlertSampleIdentity(siteID, windowKey, observedAt,
				"run_window_id:"+strconv.FormatInt(target.validation.RunWindowID, 10))
			if priorErr != nil {
				return AlertEvaluation{}, fmt.Errorf("build prior run validation identity: %w", priorErr)
			}
			evaluation.PriorSampleKeys = append(evaluation.PriorSampleKeys, prior.SampleKey)
		}
	}
	if managementStatus != constant.SiteManagementActive || authStatus != constant.SiteAuthAuthorized || !dataExportEnabled || statisticsEndAt != nil {
		siteUpdatedAt := int64(0)
		if target.collection != nil {
			siteUpdatedAt = target.collection.SiteUpdatedAt
		}
		if target.validation != nil && target.validation.SiteUpdatedAt > siteUpdatedAt {
			siteUpdatedAt = target.validation.SiteUpdatedAt
		}
		lifecycleAt := alertScanLatestObservedAt(siteUpdatedAt, observedAt, now)
		lifecycleIdentity, identityErr := BuildLifecycleAlertSampleIdentity("site", siteID, lifecycleAt)
		if identityErr != nil {
			return AlertEvaluation{}, fmt.Errorf("build validation lifecycle alert identity: %w", identityErr)
		}
		evaluation = alertEvaluationWithIdentity(evaluation, lifecycleIdentity)
		return scopeInactiveAlertEvaluation(evaluation, "site", siteID, siteName), nil
	}
	if target.collection != nil && target.collection.Status == model.CollectionWindowStatusMissing &&
		target.collection.LastErrorCode == string(constant.MessageDataValidationMismatch) {
		evaluation.Source = "data_mismatch"
		return knownAlertEvaluation(evaluation, "1"), nil
	}
	if target.validation != nil {
		if target.validation.Status == model.CollectionTaskStatusFailed {
			if target.validation.ErrorCode == constant.CodeSiteConfigChanged {
				evaluation.ResolutionReason = alertResolutionSuperseded
				return knownAlertEvaluation(evaluation, "0"), nil
			}
			evaluation.Source = "execution_failed"
			return knownAlertEvaluation(evaluation, "1"), nil
		}
		if target.validation.Status == model.CollectionTaskStatusSuccess && target.validation.FactStatus != nil &&
			*target.validation.FactStatus == model.CollectionWindowStatusComplete {
			return knownAlertEvaluation(evaluation, "0"), nil
		}
	}
	if target.collection != nil && target.collection.Status == model.CollectionWindowStatusComplete {
		return knownAlertEvaluation(evaluation, "0"), nil
	}
	evaluation.State = AlertSampleUnknown
	return evaluation, nil
}

func backfillFailedEvaluation(backfill model.AlertBackfillEvaluationSnapshot, now int64, requestID string) (AlertEvaluation, error) {
	observedAt := alertScanObservedAt(backfill.UpdatedAt, now)
	identity, err := BuildWindowAlertSampleIdentity(backfill.SiteID, "run/"+strconv.FormatInt(backfill.RunID, 10), observedAt)
	if err != nil {
		return AlertEvaluation{}, fmt.Errorf("build backfill alert identity: %w", err)
	}
	evaluation := newAlertScanEvaluation(
		"backfill_failed", backfill.SiteID, "collection",
		collectionEvaluationKey(backfill.SiteID, backfill.RunID), strconv.FormatInt(backfill.RunID, 10), requestID, identity,
	)
	if backfill.ManagementStatus != constant.SiteManagementActive || backfill.AuthStatus != constant.SiteAuthAuthorized ||
		!backfill.DataExportEnabled || backfill.StatisticsEndAt != nil {
		lifecycleAt := alertScanLatestObservedAt(backfill.SiteUpdatedAt, backfill.UpdatedAt, now)
		lifecycleIdentity, identityErr := BuildLifecycleAlertSampleIdentity("site", backfill.SiteID, lifecycleAt)
		if identityErr != nil {
			return AlertEvaluation{}, fmt.Errorf("build backfill lifecycle alert identity: %w", identityErr)
		}
		evaluation = alertEvaluationWithIdentity(evaluation, lifecycleIdentity)
		return scopeInactiveAlertEvaluation(evaluation, "site", backfill.SiteID, backfill.SiteName), nil
	}
	if backfill.Status == model.CollectionTaskStatusFailed && backfill.ErrorCode != constant.CodeSiteConfigChanged &&
		!(backfill.HasWindows && backfill.FactsRepaired) {
		return knownAlertEvaluation(evaluation, "1"), nil
	}
	return knownAlertEvaluation(evaluation, "0"), nil
}

func newAlertScanEvaluation(
	ruleKey string,
	siteID int64,
	targetType string,
	targetKey string,
	targetName string,
	requestID string,
	identity AlertSampleIdentity,
) AlertEvaluation {
	return AlertEvaluation{
		RuleKey: ruleKey, SiteID: &siteID, TargetType: targetType, TargetKey: targetKey,
		TargetName: targetName, Source: "alert_evaluation_scan", RequestID: requestID,
		ObservedAt: identity.ObservedAt, SampleKey: identity.SampleKey,
	}
}

func alertScanObservedAt(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func alertScanLatestObservedAt(values ...int64) int64 {
	latest := int64(0)
	for _, value := range values {
		if value > latest {
			latest = value
		}
	}
	return latest
}

func alertScanPointerObservedAt(value *int64, fallback int64) int64 {
	if value != nil && *value > 0 {
		return *value
	}
	return fallback
}

func alertOptionalInt64Evidence(name string, value *int64) string {
	if value == nil {
		return name + ":null"
	}
	return name + ":" + strconv.FormatInt(*value, 10)
}

func alertOptionalIntEvidence(name string, value *int) string {
	if value == nil {
		return name + ":null"
	}
	return name + ":" + strconv.Itoa(*value)
}

func alertOptionalStringEvidence(name string, value *string) string {
	if value == nil {
		return name + ":null"
	}
	return name + ":" + *value
}

func alertEvaluationWithIdentity(evaluation AlertEvaluation, identity AlertSampleIdentity) AlertEvaluation {
	evaluation.ObservedAt, evaluation.SampleKey = identity.ObservedAt, identity.SampleKey
	return evaluation
}

func knownAlertEvaluation(evaluation AlertEvaluation, value string) AlertEvaluation {
	evaluation.State, evaluation.CurrentValue = AlertSampleKnown, &value
	return evaluation
}

func scopeInactiveAlertEvaluation(
	evaluation AlertEvaluation,
	scopeType string,
	scopeID int64,
	scopeName string,
) AlertEvaluation {
	evaluation.State, evaluation.CurrentValue = AlertSampleScopeInactive, nil
	evaluation.ScopeType, evaluation.ScopeID, evaluation.ScopeName = scopeType, scopeID, scopeName
	return evaluation
}

func siteMonitoringApplicable(managementStatus string, statisticsEndAt *int64) bool {
	return managementStatus == constant.SiteManagementActive && statisticsEndAt == nil
}

func siteAuthorizedMonitoringApplicable(managementStatus, authStatus string, statisticsEndAt *int64) bool {
	return siteMonitoringApplicable(managementStatus, statisticsEndAt) && authStatus == constant.SiteAuthAuthorized
}

func collectionAlertApplicable(
	managementStatus string,
	authStatus string,
	dataExportEnabled bool,
	statisticsStartAt *int64,
	statisticsEndAt *int64,
	hourTS int64,
) bool {
	if managementStatus != constant.SiteManagementActive || authStatus != constant.SiteAuthAuthorized || !dataExportEnabled {
		return false
	}
	if statisticsStartAt != nil && hourTS < *statisticsStartAt {
		return false
	}
	return statisticsEndAt == nil || hourTS < *statisticsEndAt
}

func freshAlertSample(timestamp *int64, now int64, freshness time.Duration) bool {
	if timestamp == nil || *timestamp > now || freshness <= 0 {
		return false
	}
	return now-*timestamp <= int64(freshness/time.Second)
}

func collectionEvaluationKey(siteID, entityID int64) string {
	return strconv.FormatInt(siteID, 10) + "/" + strconv.FormatInt(entityID, 10)
}

func sortAlertStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for current := index; current > 0 && values[current] < values[current-1]; current-- {
			values[current], values[current-1] = values[current-1], values[current]
		}
	}
}

func newAlertScanRequestID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "als_" + hex.EncodeToString(random[:]), nil
}
