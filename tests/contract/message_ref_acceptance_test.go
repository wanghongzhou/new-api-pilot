package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

func TestA70MessageRefAndZhCNContractAcceptance(t *testing.T) {
	translations := loadA70Translations(t)
	for _, code := range constant.MessageCodes() {
		schema := constant.MessageRegistry[code]
		params := a70ValidParams(schema)
		if err := dto.ValidateMessageParams(code, params); err != nil {
			t.Fatalf("A70 valid params for %s rejected: %v", code, err)
		}
		for key := range schema.Required {
			missing := cloneA70Params(params)
			delete(missing, key)
			if err := dto.ValidateMessageParams(code, missing); err == nil {
				t.Fatalf("A70 missing required %s parameter %s was accepted", code, key)
			}
		}
		withUnknown := cloneA70Params(params)
		withUnknown["unexpected"] = "value"
		if err := dto.ValidateMessageParams(code, withUnknown); err == nil {
			t.Fatalf("A70 unknown parameter for %s was accepted", code)
		}
		translation, exists := translations[string(code)]
		if !exists || translation == "" {
			t.Fatalf("A70 missing or blank zh-CN translation for %s", code)
		}
	}

	for _, test := range []struct {
		name   string
		code   constant.MessageCode
		params map[string]any
	}{
		{
			name: "nullable notification configuration failure", code: constant.MessageNotificationNotConfigured,
			params: map[string]any{"alert_event_id": nil, "delivery_id": nil},
		},
		{
			name: "successful notification test has delivery", code: constant.MessageNotificationTestSucceeded,
			params: map[string]any{"delivery_id": "9007199254740993"},
		},
		{
			name: "scheduled delivery retry has canonical timestamp", code: constant.MessageDeliveryRetryScheduled,
			params: map[string]any{"delivery_id": "9007199254740993", "next_retry_at": "1768622400"},
		},
		{
			name: "internal contract error with optional safe value", code: constant.MessageInternalContractError,
			params: map[string]any{"component": "site_capability", "value": "unsupported_state"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, err := dto.NewMessageRef(test.code, test.params, "safe technical detail")
			if err != nil || message.Code != test.code {
				t.Fatalf("A70 message %s = %#v, %v", test.name, message, err)
			}
		})
	}
	if _, err := dto.NewMessageRef(constant.MessageCode("UNKNOWN_MESSAGE_CODE"), map[string]any{}, ""); err == nil {
		t.Fatal("A70 unknown MessageRef code was accepted")
	}
}

func a70ValidParams(schema constant.MessageParamSchema) map[string]any {
	params := make(map[string]any, len(schema.Required))
	for name, spec := range schema.Required {
		params[name] = a70ValueForSpec(spec)
	}
	return params
}

func a70ValueForSpec(spec constant.MessageParamSpec) any {
	switch spec.Kind {
	case constant.MessageParamString:
		switch spec.Format {
		case constant.MessageParamFormatIDString:
			return "9007199254740993"
		case constant.MessageParamFormatNonNegativeIntegerString:
			return "1768622400"
		default:
			if len(spec.AllowedValues) > 0 {
				return spec.AllowedValues[0]
			}
			return "contract-value"
		}
	case constant.MessageParamInteger:
		return int64(1_768_622_400)
	case constant.MessageParamNumber:
		return 1
	case constant.MessageParamBoolean:
		return true
	default:
		return nil
	}
}

func cloneA70Params(params map[string]any) map[string]any {
	clone := make(map[string]any, len(params))
	for key, value := range params {
		clone[key] = value
	}
	return clone
}

func loadA70Translations(t *testing.T) map[string]string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve contract test working directory: %v", err)
	}
	path := filepath.Join(workingDirectory, "..", "..", "web", "src", "i18n", "locales", "zh-CN.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read zh-CN MessageRef translations: %v", err)
	}
	translations := map[string]string{}
	if err := json.Unmarshal(contents, &translations); err != nil {
		t.Fatalf("decode zh-CN MessageRef translations: %v", err)
	}
	return translations
}
