package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"new-api-pilot/constant"
)

type SettingSLOReasonCode string

const (
	SettingSLOReasonUsageDelayTooHigh      SettingSLOReasonCode = SettingSLOReasonCode(constant.MessageSLOUsageDelayTooHigh)
	SettingSLOReasonUsageConcurrencyTooLow SettingSLOReasonCode = SettingSLOReasonCode(constant.MessageSLOUsageConcurrencyTooLow)
)

type SettingItem struct {
	Key          string         `json:"key"`
	ValueType    string         `json:"value_type"`
	Value        any            `json:"value"`
	ReadOnly     bool           `json:"read_only"`
	Secret       bool           `json:"secret"`
	Configured   bool           `json:"configured"`
	DecryptError bool           `json:"decrypt_error"`
	MaskedValue  string         `json:"masked_value"`
	Constraints  map[string]any `json:"constraints"`
	UpdatedAt    *int64         `json:"updated_at"`
}

type SettingGroup struct {
	Key               string                 `json:"key"`
	LabelKey          string                 `json:"label_key"`
	Items             []SettingItem          `json:"items"`
	H15SLOEligible    bool                   `json:"h15_slo_eligible"`
	H15SLOReasonCodes []SettingSLOReasonCode `json:"h15_slo_reason_codes"`
}

func ValidateSettingSLOReasonCodes(codes []SettingSLOReasonCode) error {
	seen := make(map[SettingSLOReasonCode]struct{}, len(codes))
	for index, code := range codes {
		switch code {
		case SettingSLOReasonUsageDelayTooHigh, SettingSLOReasonUsageConcurrencyTooLow:
		default:
			return fmt.Errorf("h15_slo_reason_codes[%d] contains unsupported value %q", index, code)
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("h15_slo_reason_codes[%d] duplicates value %q", index, code)
		}
		seen[code] = struct{}{}
	}
	return nil
}

type SettingPatchRequest struct {
	Items []SettingPatchItem `json:"items"`
}

type SettingPatchItem struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
	Clear bool            `json:"clear"`
}

func (request *SettingPatchRequest) Normalize() {
	for index := range request.Items {
		request.Items[index].Key = strings.TrimSpace(request.Items[index].Key)
		request.Items[index].Value = bytes.TrimSpace(request.Items[index].Value)
	}
}

func (request SettingPatchRequest) Validate() map[string]string {
	fieldErrors := map[string]string{}
	if len(request.Items) < 1 || len(request.Items) > 22 {
		fieldErrors["items"] = "must contain between 1 and 22 items"
	}
	seen := make(map[string]struct{}, len(request.Items))
	for index, item := range request.Items {
		prefix := settingPatchField(index, "key")
		if item.Key == "" || !utf8.ValidString(item.Key) || utf8.RuneCountInString(item.Key) > 128 {
			fieldErrors[prefix] = "must contain 1 to 128 Unicode characters"
		} else if _, duplicate := seen[item.Key]; duplicate {
			fieldErrors[prefix] = "must not duplicate another setting key"
		} else {
			seen[item.Key] = struct{}{}
		}
		if len(item.Value) > 0 && !json.Valid(item.Value) {
			fieldErrors[settingPatchField(index, "value")] = "must be valid JSON"
		}
		if item.Clear && settingPatchHasNonEmptyValue(item.Value) {
			fieldErrors[settingPatchField(index, "clear")] = "cannot be combined with a non-empty value"
		}
	}
	if len(fieldErrors) == 0 {
		return nil
	}
	return fieldErrors
}

func settingPatchHasNonEmptyValue(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) != nil || value != ""
}

func settingPatchField(index int, field string) string {
	return "items[" + strconv.Itoa(index) + "]." + field
}
