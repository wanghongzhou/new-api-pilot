package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"
)

type upstreamEnvelope struct {
	Success *bool           `json:"success"`
	Message *string         `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeUpstreamEnvelope(payload []byte, destination any) error {
	if decoder, ok := destination.(interface{ decodeUpstreamResponse([]byte) error }); ok {
		return decoder.decodeUpstreamResponse(payload)
	}
	if err := validateStrictJSONFor(payload, upstreamEnvelope{}); err != nil {
		return newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var envelope upstreamEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return newUpstreamRequestError(UpstreamErrorEnvelopeInvalid)
	}
	if envelope.Success == nil || envelope.Message == nil || !*envelope.Success {
		return newUpstreamRequestError(UpstreamErrorEnvelopeInvalid)
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return newUpstreamRequestError(UpstreamErrorEnvelopeInvalid)
	}
	if err := validateStrictJSONFor(envelope.Data, destination); err != nil {
		return newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		return newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	return nil
}

func validateStrictJSON(payload []byte) error {
	return validateStrictJSONFor(payload, nil)
}

func validateStrictJSONFor(payload []byte, destination any) error {
	if len(payload) == 0 || !utf8.Valid(payload) {
		return io.ErrUnexpectedEOF
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := consumeStrictJSONValue(decoder, reflect.TypeOf(destination)); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func consumeStrictJSONValue(decoder *json.Decoder, expected reflect.Type) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	expected = indirectJSONType(expected)
	switch delimiter {
	case '{':
		fields := knownJSONFields(expected)
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return io.ErrUnexpectedEOF
			}
			if _, duplicate := keys[key]; duplicate {
				return errors.New("duplicate JSON object key")
			}
			keys[key] = struct{}{}
			fieldType, exact := fields.exact[key]
			if !exact && fields.matchesFold(key) {
				return errors.New("known JSON field has incorrect case")
			}
			if err := consumeStrictJSONValue(decoder, fieldType); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			if err != nil {
				return err
			}
			return io.ErrUnexpectedEOF
		}
	case '[':
		var elementType reflect.Type
		if expected != nil && (expected.Kind() == reflect.Array || expected.Kind() == reflect.Slice) {
			elementType = expected.Elem()
		}
		for decoder.More() {
			if err := consumeStrictJSONValue(decoder, elementType); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			if err != nil {
				return err
			}
			return io.ErrUnexpectedEOF
		}
	default:
		return io.ErrUnexpectedEOF
	}
	return nil
}

type knownJSONFieldSet struct {
	exact map[string]reflect.Type
	names []string
}

func (fields knownJSONFieldSet) matchesFold(candidate string) bool {
	for _, name := range fields.names {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

var knownJSONFieldsCache sync.Map

func knownJSONFields(value reflect.Type) knownJSONFieldSet {
	value = indirectJSONType(value)
	if value == nil || value.Kind() != reflect.Struct {
		return knownJSONFieldSet{}
	}
	if cached, exists := knownJSONFieldsCache.Load(value); exists {
		return cached.(knownJSONFieldSet)
	}
	fields := knownJSONFieldSet{exact: make(map[string]reflect.Type)}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			embedded := knownJSONFields(field.Type)
			for embeddedName, embeddedType := range embedded.exact {
				fields.exact[embeddedName] = embeddedType
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields.exact[name] = field.Type
	}
	fields.names = make([]string, 0, len(fields.exact))
	for name := range fields.exact {
		fields.names = append(fields.names, name)
	}
	knownJSONFieldsCache.Store(value, fields)
	return fields
}

func indirectJSONType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func validUpstreamString(value string, minimum, maximum int) bool {
	if !utf8.ValidString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	return length >= minimum && length <= maximum
}
