package dto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"new-api-pilot/constant"
)

func TestInternalContractErrorIsRegistered(t *testing.T) {
	message, err := NewMessageRef(constant.MessageInternalContractError, map[string]any{"component": "task_type", "value": "unknown"}, "")
	if err != nil {
		t.Fatalf("NewMessageRef() error = %v", err)
	}
	if message.Code != constant.MessageInternalContractError {
		t.Fatalf("message code = %q", message.Code)
	}
	if _, err := NewMessageRef(constant.MessageInternalContractError, map[string]any{"component": "task_type"}, ""); err != nil {
		t.Fatalf("optional internal contract value was required: %v", err)
	}
}

func TestMessageRefRequiresExactParams(t *testing.T) {
	if _, err := NewMessageRef(constant.MessageExportExpired, nil, ""); err == nil {
		t.Fatal("NewMessageRef() accepted missing params")
	}
	if _, err := NewMessageRef(constant.MessageExportExpired, map[string]any{"export_id": "1", "extra": true}, ""); err == nil {
		t.Fatal("NewMessageRef() accepted an unknown param")
	}
}

func TestMessageRefStrictlyValidatesParamValues(t *testing.T) {
	tests := []struct {
		name   string
		code   constant.MessageCode
		params map[string]any
	}{
		{
			name:   "non-decimal ID",
			code:   constant.MessageExportExpired,
			params: map[string]any{"export_id": "not-an-id"},
		},
		{
			name:   "zero ID",
			code:   constant.MessageExportExpired,
			params: map[string]any{"export_id": "0"},
		},
		{
			name:   "non-canonical ID",
			code:   constant.MessageExportExpired,
			params: map[string]any{"export_id": "01"},
		},
		{
			name:   "overflowing ID",
			code:   constant.MessageExportExpired,
			params: map[string]any{"export_id": "9223372036854775808"},
		},
		{
			name:   "non-decimal bigint string",
			code:   constant.MessageUpstreamResponseTooLarge,
			params: map[string]any{"site_id": "1", "response_bytes": "64MiB", "limit_bytes": "67108864"},
		},
		{
			name:   "number encoded as string",
			code:   constant.MessageSiteConfigChanged,
			params: map[string]any{"site_id": "1", "expected_config_version": "2", "actual_config_version": 3},
		},
		{
			name:   "timestamp encoded as string",
			code:   constant.MessageDataWindowMissing,
			params: map[string]any{"site_id": "1", "start_timestamp": "1", "end_timestamp": 2},
		},
		{
			name:   "fractional timestamp",
			code:   constant.MessageDataWindowMissing,
			params: map[string]any{"site_id": "1", "start_timestamp": 1.5, "end_timestamp": 2},
		},
		{
			name:   "boolean encoded as number",
			code:   constant.MessageDataPending,
			params: map[string]any{"scope_type": "global", "scope_id": nil, "progress": true},
		},
		{
			name:   "invalid enum",
			code:   constant.MessageAlertValidationFailed,
			params: map[string]any{"site_id": "1", "start_timestamp": 1, "end_timestamp": 2, "failure_kind": "unknown"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateMessageParams(test.code, test.params); err == nil {
				t.Fatalf("ValidateMessageParams(%s, %#v) succeeded", test.code, test.params)
			}
		})
	}
}

func TestMessageRefAcceptsCatalogValueRepresentations(t *testing.T) {
	valid := []struct {
		code   constant.MessageCode
		params map[string]any
	}{
		{
			code:   constant.MessageDataValidationMismatch,
			params: map[string]any{"site_id": "9223372036854775807", "start_timestamp": json.Number("1"), "end_timestamp": float64(2)},
		},
		{
			code:   constant.MessageSiteConfigChanged,
			params: map[string]any{"site_id": "1", "expected_config_version": json.Number("2"), "actual_config_version": 3},
		},
		{
			code:   constant.MessageDataPending,
			params: map[string]any{"scope_type": "global", "scope_id": nil, "progress": 0.5},
		},
		{
			code:   constant.MessageUpstreamResponseTooLarge,
			params: map[string]any{"site_id": "1", "response_bytes": "0", "limit_bytes": "67108864"},
		},
		{
			code:   constant.MessageAlertValidationFailed,
			params: map[string]any{"site_id": "1", "start_timestamp": 1, "end_timestamp": 2, "failure_kind": "execution_failed"},
		},
	}
	for _, test := range valid {
		if err := ValidateMessageParams(test.code, test.params); err != nil {
			t.Errorf("ValidateMessageParams(%s) error = %v", test.code, err)
		}
	}
}

func TestMessageParamPrimitiveKinds(t *testing.T) {
	tests := []struct {
		name  string
		spec  constant.MessageParamSpec
		value any
		valid bool
	}{
		{name: "string", spec: constant.MessageParamSpec{Kind: constant.MessageParamString}, value: "value", valid: true},
		{name: "string mismatch", spec: constant.MessageParamSpec{Kind: constant.MessageParamString}, value: 1},
		{name: "integer", spec: constant.MessageParamSpec{Kind: constant.MessageParamInteger}, value: int64(-1), valid: true},
		{name: "integer mismatch", spec: constant.MessageParamSpec{Kind: constant.MessageParamInteger}, value: 1.25},
		{name: "number", spec: constant.MessageParamSpec{Kind: constant.MessageParamNumber}, value: 1.25, valid: true},
		{name: "number mismatch", spec: constant.MessageParamSpec{Kind: constant.MessageParamNumber}, value: "1.25"},
		{name: "boolean", spec: constant.MessageParamSpec{Kind: constant.MessageParamBoolean}, value: true, valid: true},
		{name: "boolean mismatch", spec: constant.MessageParamSpec{Kind: constant.MessageParamBoolean}, value: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMessageParam("value", test.spec, test.value)
			if (err == nil) != test.valid {
				t.Fatalf("validateMessageParam() error = %v, valid=%t", err, test.valid)
			}
		})
	}
}

func TestMessageRegistryMatchesDesignCatalog(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "testdata", "design", "message-ref-catalog.json"))
	if err != nil {
		t.Fatalf("read message catalog: %v", err)
	}
	var catalog struct {
		Entries []struct {
			Code           string            `json:"code"`
			Category       string            `json:"category"`
			RequiredParams map[string]string `json:"required_params"`
			OptionalParams map[string]string `json:"optional_params"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(contents, &catalog); err != nil {
		t.Fatalf("decode message catalog: %v", err)
	}

	seen := make(map[constant.MessageCode]struct{})
	for _, entry := range catalog.Entries {
		if entry.Category == "http" {
			continue
		}
		code := constant.MessageCode(entry.Code)
		schema, exists := constant.MessageRegistry[code]
		if !exists {
			t.Errorf("catalog message %s is missing from runtime registry", entry.Code)
			continue
		}
		seen[code] = struct{}{}
		assertCatalogParamSet(t, code, "required", schema.Required, entry.RequiredParams)
		assertCatalogParamSet(t, code, "optional", schema.Optional, entry.OptionalParams)
	}
	for code := range constant.MessageRegistry {
		if _, exists := seen[code]; !exists {
			t.Errorf("runtime message %s is missing from design catalog", code)
		}
	}
}

func TestMessageRegistrySchemasAreInternallyValid(t *testing.T) {
	for code, schema := range constant.MessageRegistry {
		for name, spec := range schema.Required {
			if _, exists := schema.Optional[name]; exists {
				t.Errorf("message %s param %s is both required and optional", code, name)
			}
			assertMessageParamSpecValid(t, code, name, spec)
		}
		for name, spec := range schema.Optional {
			assertMessageParamSpecValid(t, code, name, spec)
		}
	}
}

func assertCatalogParamSet(t *testing.T, code constant.MessageCode, setName string, actual map[string]constant.MessageParamSpec, expected map[string]string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("message %s %s param count = %d, want %d", code, setName, len(actual), len(expected))
	}
	for name, catalogType := range expected {
		spec, exists := actual[name]
		if !exists {
			t.Errorf("message %s is missing %s param %s", code, setName, name)
			continue
		}
		if !messageSpecMatchesCatalogType(name, spec, catalogType) {
			t.Errorf("message %s param %s spec = %#v, want catalog type %s", code, name, spec, catalogType)
		}
	}
}

func messageSpecMatchesCatalogType(name string, spec constant.MessageParamSpec, catalogType string) bool {
	switch catalogType {
	case "string":
		expectedFormat := constant.MessageParamFormatNone
		if strings.HasSuffix(name, "_bytes") {
			expectedFormat = constant.MessageParamFormatNonNegativeIntegerString
		}
		return spec.Kind == constant.MessageParamString && spec.Format == expectedFormat && !spec.Nullable && len(spec.AllowedValues) == 0
	case "IdString":
		return spec.Kind == constant.MessageParamString && spec.Format == constant.MessageParamFormatIDString && !spec.Nullable && len(spec.AllowedValues) == 0
	case "IdString|null":
		return spec.Kind == constant.MessageParamString && spec.Format == constant.MessageParamFormatIDString && spec.Nullable && len(spec.AllowedValues) == 0
	case "NonNegativeIntegerString":
		return spec.Kind == constant.MessageParamString && spec.Format == constant.MessageParamFormatNonNegativeIntegerString && !spec.Nullable && len(spec.AllowedValues) == 0
	case "Timestamp":
		return spec.Kind == constant.MessageParamInteger && spec.Format == constant.MessageParamFormatTimestamp && !spec.Nullable && len(spec.AllowedValues) == 0
	case "number":
		return spec.Kind == constant.MessageParamNumber && spec.Format == constant.MessageParamFormatNone && !spec.Nullable && len(spec.AllowedValues) == 0
	case "boolean":
		return spec.Kind == constant.MessageParamBoolean && spec.Format == constant.MessageParamFormatNone && !spec.Nullable && len(spec.AllowedValues) == 0
	default:
		values := strings.Split(catalogType, "|")
		return spec.Kind == constant.MessageParamString && spec.Format == constant.MessageParamFormatNone && !spec.Nullable && reflect.DeepEqual(spec.AllowedValues, values)
	}
}

func assertMessageParamSpecValid(t *testing.T, code constant.MessageCode, name string, spec constant.MessageParamSpec) {
	t.Helper()
	if spec.Kind == "" {
		t.Errorf("message %s param %s has no kind", code, name)
	}
	switch spec.Format {
	case constant.MessageParamFormatNone:
	case constant.MessageParamFormatIDString, constant.MessageParamFormatNonNegativeIntegerString:
		if spec.Kind != constant.MessageParamString {
			t.Errorf("message %s param %s format %s requires string kind", code, name, spec.Format)
		}
	case constant.MessageParamFormatTimestamp:
		if spec.Kind != constant.MessageParamInteger {
			t.Errorf("message %s param %s Timestamp format requires integer kind", code, name)
		}
	default:
		t.Errorf("message %s param %s has unsupported format %q", code, name, spec.Format)
	}
	if len(spec.AllowedValues) > 0 && spec.Kind != constant.MessageParamString {
		t.Errorf("message %s param %s has enum values on non-string kind", code, name)
	}
	seenValues := make(map[string]struct{}, len(spec.AllowedValues))
	for _, value := range spec.AllowedValues {
		if value == "" {
			t.Errorf("message %s param %s has an empty enum value", code, name)
		}
		if _, duplicate := seenValues[value]; duplicate {
			t.Errorf("message %s param %s repeats enum value %q", code, name, value)
		}
		seenValues[value] = struct{}{}
	}
}
