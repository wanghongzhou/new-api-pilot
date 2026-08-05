package dto

import (
	"encoding/json"
	"strconv"
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

func TestSettingPatchRequestAllowsCurrentFullWhitelistAndKeepsIndependentHardCap(t *testing.T) {
	request := SettingPatchRequest{Items: make([]SettingPatchItem, 39)}
	for index := range request.Items {
		request.Items[index] = SettingPatchItem{
			Key:   "setting." + strconv.Itoa(index),
			Value: json.RawMessage(`1`),
		}
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		t.Fatalf("39-item SettingPatchRequest.Validate() errors = %#v", fieldErrors)
	}

	request.Items = append(request.Items, make([]SettingPatchItem, SettingPatchMaximumItems-len(request.Items)+1)...)
	if fieldErrors := request.Validate(); fieldErrors["items"] == "" {
		t.Fatalf("oversized SettingPatchRequest.Validate() errors = %#v", fieldErrors)
	}
}
