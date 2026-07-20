package constant

import "sort"

type MessageCode string

type MessageParamKind string

const (
	MessageParamString  MessageParamKind = "string"
	MessageParamInteger MessageParamKind = "integer"
	MessageParamNumber  MessageParamKind = "number"
	MessageParamBoolean MessageParamKind = "boolean"
)

type MessageParamFormat string

const (
	MessageParamFormatNone                     MessageParamFormat = ""
	MessageParamFormatIDString                 MessageParamFormat = "IdString"
	MessageParamFormatNonNegativeIntegerString MessageParamFormat = "non_negative_integer_string"
	MessageParamFormatTimestamp                MessageParamFormat = "Timestamp"
)

type MessageParamSpec struct {
	Kind          MessageParamKind
	Format        MessageParamFormat
	Nullable      bool
	AllowedValues []string
}

type MessageParamSchema struct {
	Required map[string]MessageParamSpec
	Optional map[string]MessageParamSpec
}

const (
	MessageCollectionRetryExhausted       MessageCode = "COLLECTION_RETRY_EXHAUSTED"
	MessageDataValidationMismatch         MessageCode = "DATA_VALIDATION_MISMATCH"
	MessageUpstreamResponseInvalid        MessageCode = "UPSTREAM_RESPONSE_INVALID"
	MessageUpstreamResponseTooLarge       MessageCode = "UPSTREAM_RESPONSE_TOO_LARGE"
	MessageSiteConfigChanged              MessageCode = "SITE_CONFIG_CHANGED"
	MessageDependencyWindowsMissing       MessageCode = "DEPENDENCY_WINDOWS_MISSING"
	MessageWorkerLeaseLost                MessageCode = "WORKER_LEASE_LOST"
	MessageExportDiskLow                  MessageCode = "EXPORT_DISK_LOW"
	MessageExportFileTooLarge             MessageCode = "EXPORT_FILE_TOO_LARGE"
	MessageExportSnapshotFailed           MessageCode = "EXPORT_SNAPSHOT_FAILED"
	MessageExportWriteFailed              MessageCode = "EXPORT_WRITE_FAILED"
	MessageExportExpired                  MessageCode = "EXPORT_EXPIRED"
	MessageExportFileMissing              MessageCode = "EXPORT_FILE_MISSING"
	MessageNotificationDisabled           MessageCode = "NOTIFICATION_DISABLED"
	MessageNotificationNotConfigured      MessageCode = "NOTIFICATION_NOT_CONFIGURED"
	MessageNotificationTestSucceeded      MessageCode = "NOTIFICATION_TEST_SUCCEEDED"
	MessageDingTalkAddressForbidden       MessageCode = "DINGTALK_ADDRESS_FORBIDDEN"
	MessageDingTalkRejected               MessageCode = "DINGTALK_REJECTED"
	MessageDeliveryRetryExhausted         MessageCode = "DELIVERY_RETRY_EXHAUSTED"
	MessageDeliveryRetryScheduled         MessageCode = "DELIVERY_RETRY_SCHEDULED"
	MessageDataPending                    MessageCode = "DATA_PENDING"
	MessageDataBackfilling                MessageCode = "DATA_BACKFILLING"
	MessageDataWindowMissing              MessageCode = "DATA_WINDOW_MISSING"
	MessageDataUpstreamUnavailable        MessageCode = "DATA_UPSTREAM_UNAVAILABLE"
	MessageDataScopePaused                MessageCode = "DATA_SCOPE_PAUSED"
	MessageDataPartialSites               MessageCode = "DATA_PARTIAL_SITES"
	MessageDataValidationFailed           MessageCode = "DATA_VALIDATION_FAILED"
	MessageCapabilityOK                   MessageCode = "CAPABILITY_OK"
	MessageCapabilityUpstreamUnavailable  MessageCode = "CAPABILITY_UPSTREAM_UNAVAILABLE"
	MessageCapabilityResponseInvalid      MessageCode = "CAPABILITY_RESPONSE_INVALID"
	MessageCapabilityExportDisabled       MessageCode = "CAPABILITY_EXPORT_DISABLED"
	MessageCapabilityIdentityFailed       MessageCode = "CAPABILITY_IDENTITY_FAILED"
	MessageCapabilityFirstUserProofFailed MessageCode = "CAPABILITY_FIRST_USER_PROOF_FAILED"
	MessageCapabilityNoTrafficSkipped     MessageCode = "CAPABILITY_NO_TRAFFIC_SKIPPED"
	MessageAlertSiteOffline               MessageCode = "ALERT_SITE_OFFLINE"
	MessageAlertAuthExpired               MessageCode = "ALERT_AUTH_EXPIRED"
	MessageAlertExportDisabled            MessageCode = "ALERT_EXPORT_DISABLED"
	MessageAlertCollectionMissing         MessageCode = "ALERT_COLLECTION_MISSING"
	MessageAlertBackfillFailed            MessageCode = "ALERT_BACKFILL_FAILED"
	MessageAlertValidationFailed          MessageCode = "ALERT_VALIDATION_FAILED"
	MessageAlertInstanceStale             MessageCode = "ALERT_INSTANCE_STALE"
	MessageAlertInstanceOffline           MessageCode = "ALERT_INSTANCE_OFFLINE"
	MessageAlertNoInstance                MessageCode = "ALERT_NO_INSTANCE"
	MessageAlertCPUHigh                   MessageCode = "ALERT_CPU_HIGH"
	MessageAlertMemoryHigh                MessageCode = "ALERT_MEMORY_HIGH"
	MessageAlertDiskHigh                  MessageCode = "ALERT_DISK_HIGH"
	MessageAlertAccountMissing            MessageCode = "ALERT_ACCOUNT_MISSING"
	MessageAlertAccountIdentityMismatch   MessageCode = "ALERT_ACCOUNT_IDENTITY_MISMATCH"
	MessageAlertAccountDisabled           MessageCode = "ALERT_ACCOUNT_DISABLED"
	MessageAlertAccountQuotaEmpty         MessageCode = "ALERT_ACCOUNT_QUOTA_EMPTY"
	MessageAlertChannelBalanceLow         MessageCode = "ALERT_CHANNEL_BALANCE_LOW"
	MessageAlertChannelResponseTimeHigh   MessageCode = "ALERT_CHANNEL_RESPONSE_TIME_HIGH"
	MessageAlertChannelAvailabilityLow    MessageCode = "ALERT_CHANNEL_AVAILABILITY_LOW"
	MessageAlertScopeInactive             MessageCode = "ALERT_SCOPE_INACTIVE"
	MessageSLOUsageDelayTooHigh           MessageCode = "SLO_USAGE_DELAY_TOO_HIGH"
	MessageSLOUsageConcurrencyTooLow      MessageCode = "SLO_USAGE_CONCURRENCY_TOO_LOW"
	MessageInternalContractError          MessageCode = "INTERNAL_CONTRACT_ERROR"
)

var MessageRegistry = map[MessageCode]MessageParamSchema{
	MessageCollectionRetryExhausted:       required(idParam("site_id"), idParam("run_id")),
	MessageDataValidationMismatch:         required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageUpstreamResponseInvalid:        optional(required(idParam("site_id")), stringParam("capability_key")),
	MessageUpstreamResponseTooLarge:       required(idParam("site_id"), decimalStringParam("response_bytes"), decimalStringParam("limit_bytes")),
	MessageSiteConfigChanged:              required(idParam("site_id"), numberParam("expected_config_version"), numberParam("actual_config_version")),
	MessageDependencyWindowsMissing:       required(idParam("site_id"), idParam("run_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageWorkerLeaseLost:                required(idParam("site_id"), idParam("run_id"), timestampParam("hour_ts")),
	MessageExportDiskLow:                  required(idParam("export_id"), decimalStringParam("free_bytes"), decimalStringParam("threshold_bytes")),
	MessageExportFileTooLarge:             required(idParam("export_id"), decimalStringParam("file_bytes"), decimalStringParam("limit_bytes")),
	MessageExportSnapshotFailed:           required(idParam("export_id")),
	MessageExportWriteFailed:              required(idParam("export_id")),
	MessageExportExpired:                  required(idParam("export_id")),
	MessageExportFileMissing:              required(idParam("export_id")),
	MessageNotificationDisabled:           required(nullableIDParam("alert_event_id"), nullableIDParam("delivery_id")),
	MessageNotificationNotConfigured:      required(nullableIDParam("alert_event_id"), nullableIDParam("delivery_id")),
	MessageNotificationTestSucceeded:      required(idParam("delivery_id")),
	MessageDingTalkAddressForbidden:       required(nullableIDParam("alert_event_id"), nullableIDParam("delivery_id")),
	MessageDingTalkRejected:               required(nullableIDParam("alert_event_id"), idParam("delivery_id"), stringParam("errcode")),
	MessageDeliveryRetryExhausted:         required(nullableIDParam("alert_event_id"), idParam("delivery_id")),
	MessageDeliveryRetryScheduled:         required(idParam("delivery_id"), decimalStringParam("next_retry_at")),
	MessageDataPending:                    required(stringParam("scope_type"), nullableIDParam("scope_id"), numberParam("progress")),
	MessageDataBackfilling:                required(stringParam("scope_type"), nullableIDParam("scope_id"), numberParam("progress")),
	MessageDataWindowMissing:              required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageDataUpstreamUnavailable:        required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageDataScopePaused:                required(stringParam("scope_type"), idParam("scope_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageDataPartialSites:               required(numberParam("complete_site_count"), numberParam("expected_site_count")),
	MessageDataValidationFailed:           required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageCapabilityOK:                   required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityUpstreamUnavailable:  required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityResponseInvalid:      required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityExportDisabled:       required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityIdentityFailed:       required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityFirstUserProofFailed: required(idParam("site_id"), stringParam("capability_key")),
	MessageCapabilityNoTrafficSkipped:     required(idParam("site_id"), stringParam("capability_key")),
	MessageAlertSiteOffline:               required(idParam("site_id"), stringParam("site_name")),
	MessageAlertAuthExpired:               required(idParam("site_id"), stringParam("site_name")),
	MessageAlertExportDisabled:            required(idParam("site_id"), stringParam("site_name")),
	MessageAlertCollectionMissing:         required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp")),
	MessageAlertBackfillFailed:            required(idParam("site_id"), idParam("run_id")),
	MessageAlertValidationFailed:          required(idParam("site_id"), timestampParam("start_timestamp"), timestampParam("end_timestamp"), enumParam("failure_kind", "data_mismatch", "execution_failed")),
	MessageAlertInstanceStale:             required(idParam("site_id"), stringParam("instance_name")),
	MessageAlertInstanceOffline:           required(idParam("site_id"), stringParam("instance_name")),
	MessageAlertNoInstance:                required(idParam("site_id"), stringParam("site_name")),
	MessageAlertCPUHigh:                   required(idParam("site_id"), stringParam("target_type"), stringParam("target_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertMemoryHigh:                required(idParam("site_id"), stringParam("target_type"), stringParam("target_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertDiskHigh:                  required(idParam("site_id"), stringParam("target_type"), stringParam("target_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertAccountMissing:            optional(required(idParam("account_id"), stringParam("account_name")), idParam("site_id")),
	MessageAlertAccountIdentityMismatch:   optional(required(idParam("account_id"), stringParam("account_name")), idParam("site_id")),
	MessageAlertAccountDisabled:           optional(required(idParam("account_id"), stringParam("account_name")), idParam("site_id")),
	MessageAlertAccountQuotaEmpty:         optional(required(idParam("account_id"), stringParam("account_name")), idParam("site_id")),
	MessageAlertChannelBalanceLow:         required(idParam("site_id"), stringParam("site_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertChannelResponseTimeHigh:   required(idParam("site_id"), stringParam("site_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertChannelAvailabilityLow:    required(idParam("site_id"), stringParam("site_name"), stringParam("value"), stringParam("threshold")),
	MessageAlertScopeInactive:             required(stringParam("scope_type"), idParam("scope_id"), stringParam("scope_name")),
	MessageSLOUsageDelayTooHigh:           required(numberParam("value"), numberParam("threshold")),
	MessageSLOUsageConcurrencyTooLow:      required(numberParam("value"), numberParam("threshold")),
	MessageInternalContractError:          optional(required(stringParam("component")), stringParam("value")),
}

func MessageCodes() []MessageCode {
	codes := make([]MessageCode, 0, len(MessageRegistry))
	for code := range MessageRegistry {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	return codes
}

type namedMessageParam struct {
	name string
	spec MessageParamSpec
}

func required(params ...namedMessageParam) MessageParamSchema {
	return MessageParamSchema{Required: paramMap(params...), Optional: map[string]MessageParamSpec{}}
}

func optional(schema MessageParamSchema, params ...namedMessageParam) MessageParamSchema {
	schema.Optional = paramMap(params...)
	return schema
}

func paramMap(params ...namedMessageParam) map[string]MessageParamSpec {
	result := make(map[string]MessageParamSpec, len(params))
	for _, param := range params {
		result[param.name] = param.spec
	}
	return result
}

func stringParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamString}}
}

func enumParam(name string, values ...string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamString, AllowedValues: values}}
}

func idParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamString, Format: MessageParamFormatIDString}}
}

func nullableIDParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamString, Format: MessageParamFormatIDString, Nullable: true}}
}

func decimalStringParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamString, Format: MessageParamFormatNonNegativeIntegerString}}
}

func timestampParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamInteger, Format: MessageParamFormatTimestamp}}
}

func numberParam(name string) namedMessageParam {
	return namedMessageParam{name: name, spec: MessageParamSpec{Kind: MessageParamNumber}}
}
