package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const (
	settingDingTalkEnabled = "notification.dingtalk.enabled"
	settingDingTalkWebhook = "notification.dingtalk.webhook"
	settingDingTalkSecret  = "notification.dingtalk.secret"
	settingPublicOrigin    = "system.public_origin"
	settingSecretMask      = "********"
	settingMaxSafeInteger  = int64(9_007_199_254_740_991)
)

var ErrSettingContract = errors.New("platform setting contract is invalid")

type SettingValidationError struct{ Fields map[string]string }

func (err *SettingValidationError) Error() string { return "platform setting validation failed" }

type SettingServiceOptions struct {
	Repository    *model.SettingRepository
	Cipher        *common.Cipher
	Clock         common.Clock
	PublicOrigin  string
	DingTalkHosts []string
	Runtime       *RuntimeSettingsStore
}

type SettingService struct {
	settings      *model.SettingRepository
	cipher        *common.Cipher
	clock         common.Clock
	publicOrigin  string
	dingTalkHosts map[string]struct{}
	runtime       *RuntimeSettingsStore
}

type settingDefinition struct {
	Key           string
	Group         string
	ValueType     string
	ReadOnly      bool
	Secret        bool
	Minimum       int64
	Maximum       int64
	Optional      bool
	StringInteger bool
}

var settingDefinitions = []settingDefinition{
	{Key: "collector.probe_interval_seconds", Group: "collector", ValueType: "int", Minimum: 60, Maximum: 3600},
	{Key: "collector.realtime_interval_seconds", Group: "collector", ValueType: "int", Minimum: 60, Maximum: 3600},
	{Key: "collector.resource_interval_seconds", Group: "collector", ValueType: "int", Minimum: 60, Maximum: 3600},
	{Key: "collector.usage_delay_minutes", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 59},
	{Key: "collector.minute_retention_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3650},
	{Key: "logs.retention_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3650},
	{Key: "performance.retention_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3650},
	{Key: "task.retention_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3650},
	{Key: "system_task_terminal_retention_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3650},
	{Key: "collector.probe_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.realtime_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.resource_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.metadata_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.usage_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.backfill_concurrency", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "collector.manual_backfill_max_days", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 3660},
	{Key: "fast_task.history_retention_seconds", Group: "collector", ValueType: "int", Minimum: 60, Maximum: 31536000},
	{Key: "fast_task.history_count", Group: "collector", ValueType: "int", Minimum: 1, Maximum: 1000},
	{Key: "upstream.allowed_host_suffixes", Group: "upstream", ValueType: "string", Optional: true, Maximum: 8192},
	{Key: "upstream.allowed_cidrs", Group: "upstream", ValueType: "string", Optional: true, Maximum: 8192},
	{Key: "upstream.connect_timeout_seconds", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 60},
	{Key: "upstream.response_header_timeout_seconds", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 300},
	{Key: "upstream.request_timeout_seconds", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 600},
	{Key: "upstream.export_timeout_seconds", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 3600},
	{Key: "upstream.rate_limit_requests", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 10000},
	{Key: "upstream.rate_limit_window_seconds", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 3600},
	{Key: "upstream.max_inflight_per_origin", Group: "upstream", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "export.file_ttl_hours", Group: "export", ValueType: "int", Minimum: 1, Maximum: 168},
	{Key: "export.max_active_per_user", Group: "export", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "export.max_active_global", Group: "export", ValueType: "int", Minimum: 1, Maximum: 100},
	{Key: "export.max_file_bytes", Group: "export", ValueType: "int", Minimum: 1, Maximum: int64(^uint64(0) >> 1), StringInteger: true},
	{Key: "export.min_free_disk_bytes", Group: "export", ValueType: "int", Minimum: 1, Maximum: int64(^uint64(0) >> 1), StringInteger: true},
	{Key: "rate.fallback_quota_per_unit", Group: "rate", ValueType: "decimal", Optional: true},
	{Key: "rate.fallback_usd_exchange_rate", Group: "rate", ValueType: "decimal", Optional: true},
	{Key: settingDingTalkEnabled, Group: "notification", ValueType: "bool"},
	{Key: settingDingTalkWebhook, Group: "notification", ValueType: "string", Secret: true, Optional: true, Maximum: 4096},
	{Key: settingDingTalkSecret, Group: "notification", ValueType: "string", Secret: true, Optional: true, Maximum: 1024},
}

var settingGroupOrder = []string{"collector", "export", "rate", "notification", "upstream", "system"}

func NewSettingService(options SettingServiceOptions) (*SettingService, error) {
	if options.Repository == nil || options.Cipher == nil || options.Clock == nil {
		return nil, errors.New("setting service dependencies are required")
	}
	hosts, err := normalizeDingTalkAllowedHosts(options.DingTalkHosts)
	if err != nil {
		return nil, err
	}
	return &SettingService{
		settings: options.Repository, cipher: options.Cipher, clock: options.Clock,
		publicOrigin: options.PublicOrigin, dingTalkHosts: hosts, runtime: options.Runtime,
	}, nil
}

func (service *SettingService) Get(ctx context.Context) ([]dto.SettingGroup, error) {
	rows, err := service.settings.List(ctx, settingKeys())
	if err != nil {
		return nil, fmt.Errorf("list platform settings: %w", err)
	}
	indexed, err := validateSettingRows(rows)
	if err != nil {
		return nil, err
	}
	return service.settingGroups(indexed), nil
}

func (service *SettingService) Update(
	ctx context.Context,
	request dto.SettingPatchRequest,
) ([]dto.SettingGroup, error) {
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		return nil, &SettingValidationError{Fields: fieldErrors}
	}
	patches, fieldErrors := service.normalizePatches(request)
	if fieldErrors != nil {
		return nil, &SettingValidationError{Fields: fieldErrors}
	}
	var groups []dto.SettingGroup
	var nextRuntime RuntimeSettingsSnapshot
	var updateRuntime bool
	err := service.settings.Transaction(ctx, func(repository *model.SettingRepository) error {
		rows, err := repository.ListForUpdate(ctx, settingKeys())
		if err != nil {
			return fmt.Errorf("lock platform settings: %w", err)
		}
		indexed, err := validateSettingRows(rows)
		if err != nil {
			return err
		}
		finalValues := make(map[string]string, len(indexed))
		for key, row := range indexed {
			finalValues[key] = row.Value
		}
		for _, patch := range patches {
			if patch.Action == settingPatchKeep {
				continue
			}
			value := patch.Value
			if patch.Definition.Secret && patch.Action == settingPatchSet {
				value, err = service.cipher.Encrypt([]byte(patch.Value), "setting:"+patch.Definition.Key)
				if err != nil {
					return fmt.Errorf("encrypt platform setting %s: %w", patch.Definition.Key, err)
				}
			}
			finalValues[patch.Definition.Key] = value
		}
		if fields := service.validateFinalValues(finalValues); fields != nil {
			return &SettingValidationError{Fields: fields}
		}
		if service.runtime != nil {
			nextRuntime, err = service.runtime.Build(finalValues)
			if err != nil {
				return fmt.Errorf("build runtime settings: %w", err)
			}
			updateRuntime = true
		}
		now := service.clock.Now().Unix()
		for _, definition := range settingDefinitions {
			row := indexed[definition.Key]
			value := finalValues[definition.Key]
			if value == row.Value {
				continue
			}
			updatedAt := now
			if updatedAt <= row.UpdatedAt {
				updatedAt = row.UpdatedAt + 1
			}
			if err := repository.UpdateValue(ctx, row.ID, value, updatedAt); err != nil {
				return err
			}
			row.Value = value
			row.UpdatedAt = updatedAt
			indexed[definition.Key] = row
		}
		groups = service.settingGroups(indexed)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if updateRuntime {
		service.runtime.Store(nextRuntime)
	}
	return groups, nil
}

type settingPatchAction int

const (
	settingPatchKeep settingPatchAction = iota
	settingPatchSet
	settingPatchClear
)

type normalizedSettingPatch struct {
	Definition settingDefinition
	Action     settingPatchAction
	Value      string
}

func (service *SettingService) normalizePatches(
	request dto.SettingPatchRequest,
) ([]normalizedSettingPatch, map[string]string) {
	definitions := settingDefinitionsByKey()
	patches := make([]normalizedSettingPatch, 0, len(request.Items))
	fieldErrors := map[string]string{}
	for index, item := range request.Items {
		if item.Key == settingPublicOrigin {
			fieldErrors[settingItemField(index, "key")] = "is read-only"
			continue
		}
		definition, exists := definitions[item.Key]
		valueField := settingItemField(index, "value")
		if !exists {
			fieldErrors[settingItemField(index, "key")] = "is not an editable platform setting"
			continue
		}
		if definition.ReadOnly {
			fieldErrors[settingItemField(index, "key")] = "is read-only"
			continue
		}
		patch := normalizedSettingPatch{Definition: definition}
		if item.Clear {
			if !definition.Secret && !(definition.ValueType == "decimal" && definition.Optional) {
				fieldErrors[settingItemField(index, "clear")] = "is not supported for this setting"
				continue
			}
			patch.Action = settingPatchClear
			patch.Value = ""
			patches = append(patches, patch)
			continue
		}
		if definition.Secret {
			if len(item.Value) == 0 {
				patch.Action = settingPatchKeep
				patches = append(patches, patch)
				continue
			}
			value, ok := settingJSONString(item.Value)
			if !ok {
				fieldErrors[valueField] = "must be a string"
				continue
			}
			if value == "" {
				patch.Action = settingPatchKeep
				patches = append(patches, patch)
				continue
			}
			if definition.Key == settingDingTalkWebhook {
				if _, err := validateDingTalkWebhook(value, service.dingTalkHosts); err != nil {
					fieldErrors[valueField] = "must be an allowed HTTPS DingTalk webhook"
					continue
				}
			} else if !validDingTalkSecret(value) {
				fieldErrors[valueField] = "must contain 1 to 1024 safe characters"
				continue
			}
			patch.Action = settingPatchSet
			patch.Value = value
			patches = append(patches, patch)
			continue
		}
		if len(item.Value) == 0 || bytes.Equal(item.Value, []byte("null")) {
			fieldErrors[valueField] = "is required"
			continue
		}
		switch definition.ValueType {
		case "int":
			value, ok := normalizeSettingInteger(item.Value, definition.StringInteger)
			if !ok {
				fieldErrors[valueField] = "must be an integer in the documented JSON representation"
				continue
			}
			parsed, _ := strconv.ParseInt(value, 10, 64)
			if parsed < definition.Minimum || parsed > definition.Maximum {
				fieldErrors[valueField] = fmt.Sprintf("must be between %d and %d", definition.Minimum, definition.Maximum)
				continue
			}
			if isCollectorIntervalSetting(definition.Key) && parsed%60 != 0 {
				fieldErrors[valueField] = "must be a whole number of minutes"
				continue
			}
			patch.Action, patch.Value = settingPatchSet, value
		case "bool":
			if string(item.Value) != "true" && string(item.Value) != "false" {
				fieldErrors[valueField] = "must be a boolean"
				continue
			}
			patch.Action, patch.Value = settingPatchSet, string(item.Value)
		case "decimal":
			value, ok := settingJSONString(item.Value)
			if !ok {
				fieldErrors[valueField] = "must be a decimal string"
				continue
			}
			if value != "" {
				var valid bool
				value, valid = canonicalPositiveSettingDecimal(value)
				if !valid {
					fieldErrors[valueField] = "must be empty or an ASCII positive fixed-point decimal with precision 30 and scale 10"
					continue
				}
			}
			patch.Action, patch.Value = settingPatchSet, value
		case "string":
			value, ok := settingJSONString(item.Value)
			if !ok {
				fieldErrors[valueField] = "must be a string"
				continue
			}
			if int64(len(value)) > definition.Maximum {
				fieldErrors[valueField] = fmt.Sprintf("must contain at most %d bytes", definition.Maximum)
				continue
			}
			var canonical string
			var canonicalErr error
			switch definition.Key {
			case "upstream.allowed_host_suffixes":
				canonical, canonicalErr = canonicalUpstreamHostSuffixes(value)
			case "upstream.allowed_cidrs":
				canonical, canonicalErr = canonicalUpstreamCIDRs(value)
			default:
				canonicalErr = errors.New("unsupported string setting")
			}
			if canonicalErr != nil {
				fieldErrors[valueField] = canonicalErr.Error()
				continue
			}
			patch.Action, patch.Value = settingPatchSet, canonical
		default:
			fieldErrors[valueField] = "has an unsupported setting type"
			continue
		}
		patches = append(patches, patch)
	}
	if len(fieldErrors) > 0 {
		return nil, fieldErrors
	}
	return patches, nil
}

func isCollectorIntervalSetting(key string) bool {
	switch key {
	case "collector.probe_interval_seconds", "collector.realtime_interval_seconds", "collector.resource_interval_seconds":
		return true
	default:
		return false
	}
}

func (service *SettingService) validateFinalValues(values map[string]string) map[string]string {
	fieldErrors := map[string]string{}
	perUser, perUserOK := settingIntegerValue(values["export.max_active_per_user"])
	global, globalOK := settingIntegerValue(values["export.max_active_global"])
	if perUserOK && globalOK && perUser > global {
		fieldErrors["export.max_active_per_user"] = "must not exceed export.max_active_global"
	}
	connect, connectOK := settingIntegerValue(values["upstream.connect_timeout_seconds"])
	header, headerOK := settingIntegerValue(values["upstream.response_header_timeout_seconds"])
	requestTimeout, requestOK := settingIntegerValue(values["upstream.request_timeout_seconds"])
	exportTimeout, exportOK := settingIntegerValue(values["upstream.export_timeout_seconds"])
	if connectOK && requestOK && connect > requestTimeout {
		fieldErrors["upstream.connect_timeout_seconds"] = "must not exceed upstream.request_timeout_seconds"
	}
	if headerOK && requestOK && header > requestTimeout {
		fieldErrors["upstream.response_header_timeout_seconds"] = "must not exceed upstream.request_timeout_seconds"
	}
	if requestOK && exportOK && requestTimeout > exportTimeout {
		fieldErrors["upstream.request_timeout_seconds"] = "must not exceed upstream.export_timeout_seconds"
	}
	requests, requestsOK := settingIntegerValue(values["upstream.rate_limit_requests"])
	window, windowOK := settingIntegerValue(values["upstream.rate_limit_window_seconds"])
	if requestsOK && windowOK && time.Duration(window)*time.Second/time.Duration(requests) < 10*time.Millisecond {
		fieldErrors["upstream.rate_limit_requests"] = "requires an average interval of at least 10ms"
	}
	enabled := values[settingDingTalkEnabled] == "true"
	if enabled {
		webhook, webhookOK := service.decryptFinalSetting(values, settingDingTalkWebhook)
		secret, secretOK := service.decryptFinalSetting(values, settingDingTalkSecret)
		if !webhookOK || webhook == "" {
			fieldErrors[settingDingTalkWebhook] = "must be configured and decryptable while DingTalk is enabled"
		} else if _, err := validateDingTalkWebhook(webhook, service.dingTalkHosts); err != nil {
			fieldErrors[settingDingTalkWebhook] = "must be an allowed HTTPS DingTalk webhook"
		}
		if !secretOK || !validDingTalkSecret(secret) {
			fieldErrors[settingDingTalkSecret] = "must be configured and decryptable while DingTalk is enabled"
		}
	}
	if len(fieldErrors) == 0 {
		return nil
	}
	return fieldErrors
}

func (service *SettingService) decryptFinalSetting(values map[string]string, key string) (string, bool) {
	ciphertext := values[key]
	if ciphertext == "" {
		return "", true
	}
	plaintext, err := service.cipher.Decrypt(ciphertext, "setting:"+key)
	if err != nil {
		return "", false
	}
	return string(plaintext), true
}

func (service *SettingService) settingGroups(rows map[string]model.PlatformSetting) []dto.SettingGroup {
	items := make(map[string][]dto.SettingItem, len(settingGroupOrder))
	for _, definition := range settingDefinitions {
		row := rows[definition.Key]
		items[definition.Group] = append(items[definition.Group], service.settingItem(definition, row))
	}
	items["system"] = []dto.SettingItem{{
		Key: settingPublicOrigin, ValueType: "string", Value: service.publicOrigin,
		ReadOnly: true, Configured: service.publicOrigin != "", Constraints: map[string]any{"source": "environment"},
	}}
	groups := make([]dto.SettingGroup, 0, len(settingGroupOrder))
	for _, key := range settingGroupOrder {
		groups = append(groups, dto.SettingGroup{
			Key: key, LabelKey: "settings.groups." + key, Items: items[key],
		})
	}
	return groups
}

func (service *SettingService) settingItem(definition settingDefinition, row model.PlatformSetting) dto.SettingItem {
	updatedAt := row.UpdatedAt
	item := dto.SettingItem{
		Key: definition.Key, ValueType: definition.ValueType, ReadOnly: definition.ReadOnly,
		Secret: definition.Secret, Configured: row.Value != "", Constraints: settingConstraints(definition),
		UpdatedAt: &updatedAt,
	}
	if definition.Secret {
		if item.Configured {
			item.MaskedValue = settingSecretMask
			_, err := service.cipher.Decrypt(row.Value, "setting:"+definition.Key)
			item.DecryptError = err != nil
		}
		return item
	}
	switch definition.ValueType {
	case "int":
		if definition.StringInteger {
			item.Value = row.Value
		} else {
			value, _ := strconv.ParseInt(row.Value, 10, 64)
			item.Value = value
		}
	case "bool":
		item.Value = row.Value == "true"
	case "decimal":
		if row.Value != "" {
			item.Value = row.Value
		}
	case "string":
		item.Value = row.Value
	}
	return item
}

func validateSettingRows(rows []model.PlatformSetting) (map[string]model.PlatformSetting, error) {
	definitions := settingDefinitionsByKey()
	if len(rows) != len(definitions) {
		return nil, ErrSettingContract
	}
	result := make(map[string]model.PlatformSetting, len(rows))
	for _, row := range rows {
		definition, exists := definitions[row.Key]
		if !exists || row.ID <= 0 || row.UpdatedAt <= 0 || row.ValueType != definition.ValueType || row.Secret != definition.Secret {
			return nil, ErrSettingContract
		}
		if _, duplicate := result[row.Key]; duplicate {
			return nil, ErrSettingContract
		}
		if !definition.Secret && !validStoredSettingValue(definition, row.Value) {
			return nil, ErrSettingContract
		}
		result[row.Key] = row
	}
	perUser, perUserOK := settingIntegerValue(result["export.max_active_per_user"].Value)
	global, globalOK := settingIntegerValue(result["export.max_active_global"].Value)
	if !perUserOK || !globalOK || perUser > global {
		return nil, ErrSettingContract
	}
	return result, nil
}

func validStoredSettingValue(definition settingDefinition, value string) bool {
	switch definition.ValueType {
	case "int":
		parsed, err := strconv.ParseInt(value, 10, 64)
		return err == nil && strconv.FormatInt(parsed, 10) == value && parsed >= definition.Minimum && parsed <= definition.Maximum
	case "bool":
		return value == "true" || value == "false"
	case "decimal":
		if value == "" {
			return definition.Optional
		}
		canonical, valid := canonicalPositiveSettingDecimal(value)
		return valid && canonical == value
	case "string":
		if int64(len(value)) > definition.Maximum {
			return false
		}
		var canonical string
		var err error
		switch definition.Key {
		case "upstream.allowed_host_suffixes":
			canonical, err = canonicalUpstreamHostSuffixes(value)
		case "upstream.allowed_cidrs":
			canonical, err = canonicalUpstreamCIDRs(value)
		default:
			return false
		}
		return err == nil && canonical == value
	default:
		return false
	}
}

func settingDefinitionsByKey() map[string]settingDefinition {
	result := make(map[string]settingDefinition, len(settingDefinitions))
	for _, definition := range settingDefinitions {
		result[definition.Key] = definition
	}
	return result
}

func settingKeys() []string {
	result := make([]string, len(settingDefinitions))
	for index, definition := range settingDefinitions {
		result[index] = definition.Key
	}
	return result
}

func settingConstraints(definition settingDefinition) map[string]any {
	result := map[string]any{}
	if definition.ValueType == "int" {
		if definition.StringInteger {
			result["minimum"] = strconv.FormatInt(definition.Minimum, 10)
			result["maximum"] = strconv.FormatInt(definition.Maximum, 10)
			result["json_representation"] = "decimal_string"
		} else {
			result["minimum"] = definition.Minimum
			result["maximum"] = definition.Maximum
		}
	}
	if definition.ValueType == "decimal" {
		result["positive"] = true
		result["optional"] = definition.Optional
		result["maximum_digits"] = 30
		result["maximum_integer_digits"] = 20
		result["maximum_scale"] = 10
		result["json_representation"] = "decimal_string"
	}
	if definition.ValueType == "string" && !definition.Secret {
		result["maximum_length"] = definition.Maximum
		result["optional"] = definition.Optional
	}
	if definition.Secret {
		result["maximum_length"] = definition.Maximum
		result["empty_input"] = "keep"
		result["clear_requires_flag"] = true
	}
	if definition.Key == settingDingTalkWebhook {
		result["scheme"] = "https"
	}
	return result
}

func normalizeSettingInteger(raw json.RawMessage, requireString bool) (string, bool) {
	value := string(raw)
	if requireString {
		var ok bool
		value, ok = settingJSONString(raw)
		if !ok {
			return "", false
		}
	}
	if !canonicalPositiveIntegerText(value) {
		return "", false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || (!requireString && parsed > settingMaxSafeInteger) {
		return "", false
	}
	return value, true
}

func canonicalPositiveIntegerText(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func canonicalPositiveSettingDecimal(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	dot := -1
	for index := range len(value) {
		character := value[index]
		if character == '.' {
			if dot >= 0 {
				return "", false
			}
			dot = index
			continue
		}
		if character < '0' || character > '9' {
			return "", false
		}
	}
	if dot == 0 || dot == len(value)-1 {
		return "", false
	}

	integerPart := value
	fractionPart := ""
	if dot >= 0 {
		integerPart = value[:dot]
		fractionPart = value[dot+1:]
	}
	if len(fractionPart) > 10 {
		return "", false
	}
	canonicalInteger := strings.TrimLeft(integerPart, "0")
	if canonicalInteger == "" {
		canonicalInteger = "0"
	}
	canonicalFraction := strings.TrimRight(fractionPart, "0")
	integerDigits := len(canonicalInteger)
	if canonicalInteger == "0" {
		integerDigits = 0
	}
	if integerDigits > 20 || integerDigits+len(fractionPart) > 30 ||
		(canonicalInteger == "0" && canonicalFraction == "") {
		return "", false
	}
	if canonicalFraction == "" {
		return canonicalInteger, true
	}
	return canonicalInteger + "." + canonicalFraction, true
}

func settingJSONString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func settingIntegerValue(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func validDingTalkSecret(value string) bool {
	return value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\x00\r\n")
}

func settingItemField(index int, field string) string {
	return "items[" + strconv.Itoa(index) + "]." + field
}
