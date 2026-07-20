package constant

const (
	SiteManagementActive   = "active"
	SiteManagementDisabled = "disabled"

	SiteOnlineUnknown = "unknown"
	SiteOnlineOnline  = "online"
	SiteOnlineOffline = "offline"

	SiteAuthUnauthorized = "unauthorized"
	SiteAuthAuthorized   = "authorized"
	SiteAuthExpired      = "expired"

	SiteStatisticsPendingConfig = "pending_config"
	SiteStatisticsBackfilling   = "backfilling"
	SiteStatisticsReady         = "ready"
	SiteStatisticsPartial       = "partial"
	SiteStatisticsError         = "error"
	SiteStatisticsPaused        = "paused"

	SiteHealthOK          = "ok"
	SiteHealthWarning     = "warning"
	SiteHealthCritical    = "critical"
	SiteHealthUnavailable = "unavailable"
)

const (
	CapabilityStatusContract      = "status_contract"
	CapabilitySelfIdentity        = "self_identity"
	CapabilityRootIdentity        = "root_identity"
	CapabilityFirstUserProof      = "first_user_proof"
	CapabilityUserPagination      = "user_pagination"
	CapabilityChannelPagination   = "channel_pagination"
	CapabilityDataExportEnabled   = "data_export_enabled"
	CapabilityFlowContract        = "flow_contract"
	CapabilityDataContract        = "data_contract"
	CapabilityFlowDataConsistency = "flow_data_consistency"
	CapabilityInstanceContract    = "instance_contract"
	CapabilityRealtimeContract    = "realtime_contract"
	CapabilityStatusPassed        = "passed"
	CapabilityStatusFailed        = "failed"
	CapabilityStatusSkipped       = "skipped"
)

const (
	TaskTypeSiteProbe        = "site_probe"
	TaskTypeRealtimeStat     = "realtime_stat"
	TaskTypeResourceSnapshot = "resource_snapshot"
	TaskTypeUserSync         = "user_sync"
	TaskTypeChannelSync      = "channel_sync"
	TaskTypePerformanceSync  = "performance_sync"
	TaskTypeTopupSync        = "topup_sync"
	TaskTypeRedemptionSync   = "redemption_sync"
	TaskTypeUpstreamTaskSync = "upstream_task_sync"
	TaskTypeModelMetaSync    = "model_meta_sync"
	TaskTypePlanSync         = "plan_sync"
	TaskTypePricingSync      = "pricing_group_sync"
	TaskTypeSystemTaskSync   = "system_task_sync"
	TaskTypeLogSync          = "log_sync"
	TaskTypeUsageHour        = "usage_hour"
	TaskTypeUsageBackfill    = "usage_backfill"
	TaskTypeUsageValidation  = "usage_validation"
	TaskTypeAccountRebuild   = "account_rebuild"
	TaskTypeCustomerRebuild  = "customer_rebuild"
)

const (
	CollectionFamilyProbe        = "probe"
	CollectionFamilyRealtime     = "realtime"
	CollectionFamilyResource     = "resource"
	CollectionFamilyMetadata     = "metadata"
	CollectionFamilyUsage        = "usage"
	CollectionFamilyLocalRebuild = "local_rebuild"
)

const (
	CollectionTriggerSchedule   = "schedule"
	CollectionTriggerManual     = "manual"
	CollectionTriggerRecovery   = "recovery"
	CollectionTriggerDependency = "dependency"
)

const (
	CollectionPriorityUsageRealtime    = 100
	CollectionPrioritySiteRecovery     = 90
	CollectionPriorityManualBackfill   = 80
	CollectionPriorityDailyValidation  = 70
	CollectionPriorityWeeklyValidation = 60
	CollectionPriorityInitialBackfill  = 50
	CollectionPriorityLocalRebuild     = 40
)

const (
	CodeBaseURLPreflightRequired   = "BASE_URL_PREFLIGHT_REQUIRED"
	CodeSiteExportDisabled         = "SITE_EXPORT_DISABLED"
	CodeSiteIncompatible           = "SITE_INCOMPATIBLE"
	CodeTokenRotationResultUnknown = "TOKEN_ROTATION_RESULT_UNKNOWN"
	CodeBackfillRunning            = "BACKFILL_RUNNING"
	CodeTaskOverlap                = "TASK_OVERLAP"
)

func SiteCapabilityKeys() []string {
	return []string{
		CapabilityStatusContract, CapabilitySelfIdentity,
		CapabilityRootIdentity, CapabilityFirstUserProof, CapabilityUserPagination,
		CapabilityChannelPagination, CapabilityDataExportEnabled, CapabilityFlowContract,
		CapabilityDataContract, CapabilityFlowDataConsistency, CapabilityInstanceContract,
		CapabilityRealtimeContract,
	}
}

func CollectionTaskFamily(taskType string) (string, bool) {
	switch taskType {
	case TaskTypeSiteProbe:
		return CollectionFamilyProbe, true
	case TaskTypeRealtimeStat:
		return CollectionFamilyRealtime, true
	case TaskTypeResourceSnapshot:
		return CollectionFamilyResource, true
	case TaskTypeUserSync, TaskTypeChannelSync, TaskTypePerformanceSync, TaskTypeTopupSync, TaskTypeRedemptionSync, TaskTypeUpstreamTaskSync, TaskTypeModelMetaSync, TaskTypePlanSync, TaskTypePricingSync, TaskTypeSystemTaskSync, TaskTypeLogSync:
		return CollectionFamilyMetadata, true
	case TaskTypeUsageHour, TaskTypeUsageBackfill, TaskTypeUsageValidation:
		return CollectionFamilyUsage, true
	case TaskTypeAccountRebuild, TaskTypeCustomerRebuild:
		return CollectionFamilyLocalRebuild, true
	default:
		return "", false
	}
}

func CollectionTaskTarget(taskType string) (string, bool) {
	switch taskType {
	case TaskTypeSiteProbe, TaskTypeRealtimeStat, TaskTypeResourceSnapshot, TaskTypeUserSync, TaskTypePerformanceSync, TaskTypeTopupSync, TaskTypeRedemptionSync, TaskTypeUpstreamTaskSync, TaskTypeModelMetaSync, TaskTypePlanSync, TaskTypePricingSync, TaskTypeSystemTaskSync,
		TaskTypeChannelSync, TaskTypeLogSync, TaskTypeUsageHour, TaskTypeUsageBackfill, TaskTypeUsageValidation:
		return "site", true
	case TaskTypeAccountRebuild:
		return "account", true
	case TaskTypeCustomerRebuild:
		return "customer", true
	default:
		return "", false
	}
}

func CollectionTaskTypesForFamily(family string) []string {
	switch family {
	case CollectionFamilyProbe:
		return []string{TaskTypeSiteProbe}
	case CollectionFamilyRealtime:
		return []string{TaskTypeRealtimeStat}
	case CollectionFamilyResource:
		return []string{TaskTypeResourceSnapshot}
	case CollectionFamilyMetadata:
		return []string{TaskTypeUserSync, TaskTypeChannelSync, TaskTypePerformanceSync, TaskTypeTopupSync, TaskTypeRedemptionSync, TaskTypeUpstreamTaskSync, TaskTypeModelMetaSync, TaskTypePlanSync, TaskTypePricingSync, TaskTypeSystemTaskSync, TaskTypeLogSync}
	case CollectionFamilyUsage:
		return []string{TaskTypeUsageHour, TaskTypeUsageBackfill, TaskTypeUsageValidation}
	case CollectionFamilyLocalRebuild:
		return []string{TaskTypeAccountRebuild, TaskTypeCustomerRebuild}
	default:
		return nil
	}
}

func CollectionTaskWindowed(taskType string) bool {
	switch taskType {
	case TaskTypeUsageHour, TaskTypeUsageBackfill, TaskTypeUsageValidation,
		TaskTypeAccountRebuild, TaskTypeCustomerRebuild:
		return true
	default:
		return false
	}
}

func ValidCollectionTaskType(taskType string) bool {
	_, valid := CollectionTaskFamily(taskType)
	return valid
}

func ValidCollectionTriggerType(triggerType string) bool {
	switch triggerType {
	case CollectionTriggerSchedule, CollectionTriggerManual, CollectionTriggerRecovery, CollectionTriggerDependency:
		return true
	default:
		return false
	}
}

func ValidCollectionPriority(taskType string, priority int) bool {
	switch taskType {
	case TaskTypeUsageHour:
		return priority == CollectionPriorityUsageRealtime
	case TaskTypeUsageBackfill:
		return priority == CollectionPrioritySiteRecovery || priority == CollectionPriorityManualBackfill ||
			priority == CollectionPriorityInitialBackfill
	case TaskTypeUsageValidation:
		return priority == CollectionPriorityDailyValidation || priority == CollectionPriorityWeeklyValidation
	case TaskTypeAccountRebuild, TaskTypeCustomerRebuild:
		return priority == CollectionPriorityLocalRebuild
	default:
		return priority == 0
	}
}

func ValidCollectionTaskPriority(taskType, triggerType string, priority int) bool {
	if !ValidCollectionPriority(taskType, priority) {
		return false
	}
	switch taskType {
	case TaskTypeUsageHour:
		return triggerType == CollectionTriggerSchedule && priority == CollectionPriorityUsageRealtime
	case TaskTypeUsageBackfill:
		switch triggerType {
		case CollectionTriggerManual:
			return priority == CollectionPriorityManualBackfill
		case CollectionTriggerRecovery:
			return priority == CollectionPrioritySiteRecovery || priority == CollectionPriorityInitialBackfill
		case CollectionTriggerDependency:
			return priority == CollectionPrioritySiteRecovery
		default:
			return false
		}
	case TaskTypeUsageValidation:
		return triggerType == CollectionTriggerSchedule &&
			(priority == CollectionPriorityDailyValidation || priority == CollectionPriorityWeeklyValidation)
	case TaskTypeAccountRebuild, TaskTypeCustomerRebuild:
		return priority == CollectionPriorityLocalRebuild
	default:
		return priority == 0
	}
}
