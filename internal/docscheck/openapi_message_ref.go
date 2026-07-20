package docscheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const messageRefOpenAPIPath = "testdata/design/message-ref-openapi.json"

// WriteMessageRefOpenAPI regenerates the checked-in OpenAPI contract from the
// MessageRef catalog. It is intentionally separate from docs-check: CI must
// detect drift rather than silently rewrite a release contract.
func WriteMessageRefOpenAPI(root string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	catalogPath := filepath.Join(absoluteRoot, filepath.FromSlash(messageCatalogPath))
	catalog, err := readMessageCatalog(catalogPath)
	if err != nil {
		return err
	}
	payload, err := messageRefOpenAPIPayload(catalog)
	if err != nil {
		return err
	}
	path := filepath.Join(absoluteRoot, filepath.FromSlash(messageRefOpenAPIPath))
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o644); err != nil {
		return fmt.Errorf("write temporary OpenAPI contract: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish OpenAPI contract: %w", err)
	}
	return nil
}

func (current *checker) checkMessageRefOpenAPI(catalog *messageCatalog) {
	if catalog == nil {
		return
	}
	path := filepath.Join(current.root, filepath.FromSlash(messageRefOpenAPIPath))
	actual, err := os.ReadFile(path)
	if err != nil {
		current.add("openapi", path, "read MessageRef contract: %v", err)
		return
	}
	expected, err := messageRefOpenAPIPayload(catalog)
	if err != nil {
		current.add("openapi", path, "generate expected MessageRef contract: %v", err)
		return
	}
	if !bytes.Equal(actual, expected) {
		current.add("openapi", path, "MessageRef contract differs from generated catalog contract; run make contract-generate")
	}
}

func messageRefOpenAPIPayload(catalog *messageCatalog) ([]byte, error) {
	if catalog == nil {
		return nil, fmt.Errorf("MessageRef catalog is nil")
	}
	document := buildMessageRefOpenAPI(catalog)
	payload, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI contract: %w", err)
	}
	return append(payload, '\n'), nil
}

func buildMessageRefOpenAPI(catalog *messageCatalog) map[string]any {
	apiErrorCodes := append([]string(nil), catalog.APIErrorCodes...)
	messageEntries := make([]messageCatalogEntry, 0)
	for _, entry := range catalog.Entries {
		if entry.Category != "http" {
			messageEntries = append(messageEntries, entry)
		}
	}
	sort.Strings(apiErrorCodes)
	sort.Slice(messageEntries, func(left, right int) bool {
		return messageEntries[left].Code < messageEntries[right].Code
	})

	messageCodes := make([]string, 0, len(messageEntries))
	variants := make([]any, 0, len(messageEntries))
	mapping := make(map[string]string, len(messageEntries))
	schemas := map[string]any{
		"ApiErrorCode": map[string]any{
			"type": "string",
			"enum": apiErrorCodes,
		},
		"MessageCode": map[string]any{
			"type": "string",
			"enum": messageCodes,
		},
	}
	for _, entry := range messageEntries {
		messageCodes = append(messageCodes, entry.Code)
		name := messageRefSchemaName(entry.Code)
		variants = append(variants, map[string]any{"$ref": "#/components/schemas/" + name})
		mapping[entry.Code] = "#/components/schemas/" + name
		schemas[name] = messageRefVariantSchema(entry)
	}
	// The MessageCode enum must be built after all message entries are known.
	schemas["MessageCode"] = map[string]any{"type": "string", "enum": messageCodes}
	schemas["AnyMessageRef"] = map[string]any{
		"oneOf": variants,
		"discriminator": map[string]any{
			"propertyName": "code",
			"mapping":      mapping,
		},
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "New API Pilot MessageRef Contract",
			"version": "1",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": schemas,
		},
	}
}

func messageRefSchemaName(code string) string {
	return "MessageRef_" + code
}

func messageRefVariantSchema(entry messageCatalogEntry) map[string]any {
	properties := make(map[string]any, len(entry.RequiredParams)+len(entry.OptionalParams))
	required := sortedKeys(entry.RequiredParams)
	for name, valueType := range entry.RequiredParams {
		properties[name] = messageParamSchema(valueType)
	}
	for name, valueType := range entry.OptionalParams {
		properties[name] = messageParamSchema(valueType)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"code", "params", "technical_detail"},
		"properties": map[string]any{
			"code": map[string]any{"const": entry.Code},
			"params": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             required,
				"properties":           properties,
			},
			"technical_detail": map[string]any{"type": "string"},
		},
	}
}

func messageParamSchema(valueType string) map[string]any {
	switch valueType {
	case "string":
		return map[string]any{"type": "string"}
	case "IdString":
		return map[string]any{"type": "string", "pattern": "^[1-9][0-9]*$"}
	case "IdString|null":
		return map[string]any{"type": []string{"string", "null"}, "pattern": "^[1-9][0-9]*$"}
	case "NonNegativeIntegerString":
		return map[string]any{"type": "string", "pattern": "^(?:0|[1-9][0-9]*)$"}
	case "Timestamp":
		return map[string]any{"type": "integer", "minimum": 0}
	case "number":
		return map[string]any{"type": "number"}
	case "boolean":
		return map[string]any{"type": "boolean"}
	default:
		values := strings.Split(valueType, "|")
		return map[string]any{"type": "string", "enum": values}
	}
}
