package dto

import (
	"encoding/json"
	"testing"
)

func TestSettingPatchRequestValidate(t *testing.T) {
	request := SettingPatchRequest{Items: []SettingPatchItem{
		{Key: " collector.usage_delay_minutes ", Value: json.RawMessage(`5`)},
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`6`)},
		{Key: "notification.dingtalk.secret", Value: json.RawMessage(`"replacement"`), Clear: true},
	}}
	request.Normalize()
	fieldErrors := request.Validate()
	if fieldErrors["items[1].key"] == "" || fieldErrors["items[2].clear"] == "" {
		t.Fatalf("SettingPatchRequest.Validate() errors = %#v", fieldErrors)
	}
}

func TestSettingPatchRequestAllowsExplicitClearAndEmptySecretKeep(t *testing.T) {
	request := SettingPatchRequest{Items: []SettingPatchItem{
		{Key: "notification.dingtalk.webhook", Clear: true},
		{Key: "notification.dingtalk.secret", Value: json.RawMessage(`""`)},
	}}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		t.Fatalf("SettingPatchRequest.Validate() errors = %#v", fieldErrors)
	}
}

func TestSettingSLOReasonCodesJSONAndValidation(t *testing.T) {
	codes := []SettingSLOReasonCode{
		SettingSLOReasonUsageDelayTooHigh,
		SettingSLOReasonUsageConcurrencyTooLow,
	}
	if err := ValidateSettingSLOReasonCodes(codes); err != nil {
		t.Fatalf("valid reason codes: %v", err)
	}
	encoded, err := json.Marshal(SettingGroup{H15SLOReasonCodes: codes})
	if err != nil {
		t.Fatalf("marshal setting group: %v", err)
	}
	var decoded struct {
		Codes []SettingSLOReasonCode `json:"h15_slo_reason_codes"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal setting group: %v", err)
	}
	if len(decoded.Codes) != 2 || decoded.Codes[0] != SettingSLOReasonUsageDelayTooHigh ||
		decoded.Codes[1] != SettingSLOReasonUsageConcurrencyTooLow {
		t.Fatalf("reason code JSON = %s, decoded %#v", encoded, decoded.Codes)
	}

	for name, invalid := range map[string][]SettingSLOReasonCode{
		"unsupported": {"SLO_UNKNOWN"},
		"duplicate":   {SettingSLOReasonUsageDelayTooHigh, SettingSLOReasonUsageDelayTooHigh},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateSettingSLOReasonCodes(invalid); err == nil {
				t.Fatalf("ValidateSettingSLOReasonCodes(%#v) succeeded", invalid)
			}
		})
	}
}
