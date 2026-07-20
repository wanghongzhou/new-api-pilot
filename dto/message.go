package dto

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"new-api-pilot/constant"
)

type MessageRef struct {
	Code            constant.MessageCode `json:"code"`
	Params          map[string]any       `json:"params"`
	TechnicalDetail string               `json:"technical_detail"`
}

func NewMessageRef(code constant.MessageCode, params map[string]any, technicalDetail string) (MessageRef, error) {
	if err := ValidateMessageParams(code, params); err != nil {
		return MessageRef{}, err
	}
	return MessageRef{Code: code, Params: params, TechnicalDetail: technicalDetail}, nil
}

func MustMessageRef(code constant.MessageCode, params map[string]any, technicalDetail string) MessageRef {
	result, err := NewMessageRef(code, params, technicalDetail)
	if err != nil {
		panic(err)
	}
	return result
}

func ValidateMessageParams(code constant.MessageCode, params map[string]any) error {
	schema, exists := constant.MessageRegistry[code]
	if !exists {
		return fmt.Errorf("unknown message code %q", code)
	}
	if params == nil {
		params = map[string]any{}
	}
	allowed := make(map[string]constant.MessageParamSpec, len(schema.Required)+len(schema.Optional))
	for _, key := range sortedParamNames(schema.Required) {
		allowed[key] = schema.Required[key]
		if _, present := params[key]; !present {
			return fmt.Errorf("message code %s requires param %s", code, key)
		}
	}
	for key, spec := range schema.Optional {
		allowed[key] = spec
	}
	unknown := make([]string, 0)
	for key := range params {
		if _, present := allowed[key]; !present {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("message code %s contains unknown params %v", code, unknown)
	}
	for _, key := range sortedAnyParamNames(params) {
		if err := validateMessageParam(key, allowed[key], params[key]); err != nil {
			return fmt.Errorf("message code %s param %s: %w", code, key, err)
		}
	}
	return nil
}

func validateMessageParam(name string, spec constant.MessageParamSpec, value any) error {
	if value == nil {
		if spec.Nullable {
			return nil
		}
		return fmt.Errorf("must not be null")
	}

	switch spec.Kind {
	case constant.MessageParamString:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("must be a string, got %T", value)
		}
		switch spec.Format {
		case constant.MessageParamFormatNone:
		case constant.MessageParamFormatIDString:
			if !isPositiveDecimalInt64(text) {
				return fmt.Errorf("must be a canonical positive decimal int64 string")
			}
		case constant.MessageParamFormatNonNegativeIntegerString:
			if !isNonNegativeDecimalInt64(text) {
				return fmt.Errorf("must be a canonical non-negative decimal int64 string")
			}
		default:
			return fmt.Errorf("uses unsupported string format %q", spec.Format)
		}
		if len(spec.AllowedValues) > 0 && !containsString(spec.AllowedValues, text) {
			return fmt.Errorf("must be one of %v", spec.AllowedValues)
		}
	case constant.MessageParamInteger:
		nonNegative := spec.Format == constant.MessageParamFormatTimestamp
		if spec.Format != constant.MessageParamFormatNone && spec.Format != constant.MessageParamFormatTimestamp {
			return fmt.Errorf("uses unsupported integer format %q", spec.Format)
		}
		if !isIntegerValue(value, nonNegative) {
			if nonNegative {
				return fmt.Errorf("must be a non-negative integer, got %T", value)
			}
			return fmt.Errorf("must be an integer, got %T", value)
		}
	case constant.MessageParamNumber:
		if !isNumberValue(value) {
			return fmt.Errorf("must be a finite number, got %T", value)
		}
	case constant.MessageParamBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be a boolean, got %T", value)
		}
	default:
		return fmt.Errorf("param %s uses unsupported type %q", name, spec.Kind)
	}
	return nil
}

func isPositiveDecimalInt64(value string) bool {
	if value == "" || value[0] == '0' || !containsOnlyDecimalDigits(value) {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0
}

func isNonNegativeDecimalInt64(value string) bool {
	if value == "" || !containsOnlyDecimalDigits(value) || (len(value) > 1 && value[0] == '0') {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed >= 0
}

func containsOnlyDecimalDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isIntegerValue(value any, nonNegative bool) bool {
	switch typed := value.(type) {
	case int:
		return !nonNegative || typed >= 0
	case int8:
		return !nonNegative || typed >= 0
	case int16:
		return !nonNegative || typed >= 0
	case int32:
		return !nonNegative || typed >= 0
	case int64:
		return !nonNegative || typed >= 0
	case uint:
		return uint64(typed) <= uint64(1<<63-1)
	case uint8:
		return true
	case uint16:
		return true
	case uint32:
		return true
	case uint64:
		return typed <= uint64(1<<63-1)
	case float32:
		return validIntegralFloat(float64(typed), nonNegative)
	case float64:
		return validIntegralFloat(typed, nonNegative)
	case json.Number:
		text := typed.String()
		if strings.ContainsAny(text, ".eE") {
			return false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		return err == nil && (!nonNegative || parsed >= 0)
	default:
		return false
	}
}

func validIntegralFloat(value float64, nonNegative bool) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return false
	}
	if value < float64(-1<<63) || value >= float64(1<<63) {
		return false
	}
	return !nonNegative || value >= 0
}

func isNumberValue(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0)
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case json.Number:
		parsed, err := strconv.ParseFloat(typed.String(), 64)
		return err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedParamNames(params map[string]constant.MessageParamSpec) []string {
	result := make([]string, 0, len(params))
	for key := range params {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func sortedAnyParamNames(params map[string]any) []string {
	result := make([]string, 0, len(params))
	for key := range params {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
