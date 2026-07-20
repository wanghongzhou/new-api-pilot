package docscheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

const (
	messageCatalogPath = "testdata/design/message-ref-catalog.json"
	apiDesignPath      = "docs/多站点运营管理平台-详细设计-05C-平台API与Worker.md"
)

var stableCodePattern = regexp.MustCompile(`[A-Z][A-Z0-9_]+`)

type messageCatalog struct {
	SchemaVersion  int                   `json:"schema_version"`
	Source         string                `json:"source"`
	APIErrorCodes  []string              `json:"api_error_codes"`
	Entries        []messageCatalogEntry `json:"entries"`
}

type messageCatalogEntry struct {
	Code           string            `json:"code"`
	Category       string            `json:"category"`
	RequiredParams map[string]string `json:"required_params"`
	OptionalParams map[string]string `json:"optional_params"`
}

type documentedMessages struct {
	categories    map[string]string
	params        map[string]documentedParams
	apiErrorCodes map[string]struct{}
}

type documentedParams struct {
	required map[string]string
	optional map[string]string
}

func (current *checker) checkMessageCatalog() *messageCatalog {
	catalogPath := filepath.Join(current.root, filepath.FromSlash(messageCatalogPath))
	catalog, err := readMessageCatalog(catalogPath)
	if err != nil {
		current.add("message-ref", catalogPath, "%v", err)
		return nil
	}
	if catalog.SchemaVersion != 1 {
		current.add("message-ref", catalogPath, "schema_version = %d, want 1", catalog.SchemaVersion)
	}
	if strings.TrimSpace(catalog.Source) == "" {
		current.add("message-ref", catalogPath, "source is empty")
	}

	documented := current.readDocumentedMessages()
	allowedCategories := stringSet("http", "collection", "export", "delivery", "internal", "data", "capability", "alert", "slo")
	entries := make(map[string]messageCatalogEntry, len(catalog.Entries))
	orderedCodes := make([]string, 0, len(catalog.Entries))
	for index, entry := range catalog.Entries {
		if !stableCodePattern.MatchString(entry.Code) || stableCodePattern.FindString(entry.Code) != entry.Code {
			current.add("message-ref", catalogPath, "entries[%d] has invalid code %q", index, entry.Code)
		}
		if _, duplicate := entries[entry.Code]; duplicate {
			current.add("message-ref", catalogPath, "duplicate code %s", entry.Code)
		}
		if _, valid := allowedCategories[entry.Category]; !valid {
			current.add("message-ref", catalogPath, "%s has invalid category %q", entry.Code, entry.Category)
		}
		if entry.RequiredParams == nil || entry.OptionalParams == nil {
			current.add("message-ref", catalogPath, "%s params must be objects, not null", entry.Code)
		}
		for name := range entry.RequiredParams {
			if _, duplicate := entry.OptionalParams[name]; duplicate {
				current.add("message-ref", catalogPath, "%s param %s is both required and optional", entry.Code, name)
			}
		}
		entries[entry.Code] = entry
		orderedCodes = append(orderedCodes, entry.Code)
	}
	if !sort.StringsAreSorted(orderedCodes) {
		current.add("message-ref", catalogPath, "entries are not sorted by code")
	}
	if len(catalog.APIErrorCodes) == 0 {
		current.add("message-ref", catalogPath, "api_error_codes is empty")
	}
	if !sort.StringsAreSorted(catalog.APIErrorCodes) {
		current.add("message-ref", catalogPath, "api_error_codes are not sorted")
	}
	apiErrorCodes := make(map[string]struct{}, len(catalog.APIErrorCodes))
	for _, code := range catalog.APIErrorCodes {
		if _, duplicate := apiErrorCodes[code]; duplicate {
			current.add("message-ref", catalogPath, "api_error_codes repeats %s", code)
		}
		apiErrorCodes[code] = struct{}{}
		if _, exists := entries[code]; !exists {
			current.add("message-ref", catalogPath, "api_error_codes references unknown catalog code %s", code)
		}
	}
	for code := range documented.apiErrorCodes {
		if _, exists := apiErrorCodes[code]; !exists {
			current.add("message-ref", catalogPath, "documented HTTP code %s is missing from api_error_codes", code)
		}
	}
	for code := range apiErrorCodes {
		if _, exists := documented.apiErrorCodes[code]; !exists {
			current.add("message-ref", catalogPath, "api_error_codes code %s is not documented as an HTTP code", code)
		}
	}

	for code, category := range documented.categories {
		entry, exists := entries[code]
		if !exists {
			current.add("message-ref", catalogPath, "documented code %s is missing from catalog", code)
			continue
		}
		if entry.Category != category {
			current.add("message-ref", catalogPath, "%s category = %q, want %q", code, entry.Category, category)
		}
		documentedParams := documented.params[code]
		if documentedParams.required == nil {
			documentedParams.required = map[string]string{}
		}
		if documentedParams.optional == nil {
			documentedParams.optional = map[string]string{}
		}
		if !reflect.DeepEqual(entry.RequiredParams, documentedParams.required) {
			current.add("message-ref", catalogPath, "%s required_params = %s, want %s", code, formatParamMap(entry.RequiredParams), formatParamMap(documentedParams.required))
		}
		if !reflect.DeepEqual(entry.OptionalParams, documentedParams.optional) {
			current.add("message-ref", catalogPath, "%s optional_params = %s, want %s", code, formatParamMap(entry.OptionalParams), formatParamMap(documentedParams.optional))
		}
	}
	for code := range entries {
		if _, exists := documented.categories[code]; !exists {
			current.add("message-ref", catalogPath, "catalog code %s is not documented in section 33.12", code)
		}
	}
	return catalog
}

func readMessageCatalog(path string) (*messageCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var catalog messageCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (current *checker) readDocumentedMessages() documentedMessages {
	path := filepath.Join(current.root, filepath.FromSlash(apiDesignPath))
	result := documentedMessages{
		categories:    make(map[string]string),
		params:        make(map[string]documentedParams),
		apiErrorCodes: make(map[string]struct{}),
	}
	content, err := os.ReadFile(path)
	if err != nil {
		current.add("message-ref", path, "read: %v", err)
		return result
	}
	section := between(string(content), "### 33.12 HTTP 与业务错误", "### 33.13 DTO 字段闭环")
	if section == "" {
		current.add("message-ref", path, "cannot locate section 33.12")
		return result
	}

	paramsTable := false
	for lineNumber, line := range strings.Split(section, "\n") {
		columns := splitMarkdownRow(line)
		if len(columns) == 0 {
			continue
		}
		if len(columns) == 3 && columns[0] == "code" && columns[1] == "required params" && columns[2] == "optional params" {
			paramsTable = true
			continue
		}
		if isTableSeparator(columns) {
			continue
		}
		if paramsTable {
			codes := extractStableCodes(columns[0])
			if len(columns) != 3 || len(codes) == 0 {
				continue
			}
			required, requiredErr := parseParamList(columns[1])
			optional, optionalErr := parseParamList(columns[2])
			if requiredErr != nil {
				current.add("message-ref", path, "section 33.12 params line %d: %v", lineNumber+1, requiredErr)
			}
			if optionalErr != nil {
				current.add("message-ref", path, "section 33.12 params line %d: %v", lineNumber+1, optionalErr)
			}
			for _, code := range codes {
				if _, duplicate := result.params[code]; duplicate {
					current.add("message-ref", path, "params document code %s more than once", code)
				}
				result.params[code] = documentedParams{required: required, optional: optional}
			}
			continue
		}

		switch {
		case len(columns) == 3 && isHTTPStatusCell(columns[0]):
			for _, code := range extractStableCodes(columns[1]) {
				result.apiErrorCodes[code] = struct{}{}
				registerDocumentedCategory(result.categories, code, "http")
			}
		case len(columns) == 3 && (columns[0] == "collection" || columns[0] == "export" || columns[0] == "delivery" || columns[0] == "internal"):
			for _, code := range extractStableCodes(columns[1]) {
				result.categories[code] = columns[0]
			}
		case len(columns) == 2:
			category := ""
			switch columns[0] {
			case "数据状态原因":
				category = "data"
			case "能力检查":
				category = "capability"
			case "告警主文案":
				category = "alert"
			case "SLO eligibility":
				category = "slo"
			}
			if category != "" {
				for _, code := range extractStableCodes(columns[1]) {
					result.categories[code] = category
				}
			}
		}
	}

	for code, category := range result.categories {
		if category != "http" {
			if _, exists := result.params[code]; !exists {
				current.add("message-ref", path, "registry code %s has no params row", code)
			}
		}
	}
	for code := range result.params {
		if _, exists := result.categories[code]; !exists {
			current.add("message-ref", path, "params code %s is absent from stable registry", code)
		}
	}
	return result
}

func registerDocumentedCategory(categories map[string]string, code string, category string) {
	if existing, exists := categories[code]; !exists || existing == "http" {
		categories[code] = category
	}
}

func parseParamList(value string) (map[string]string, error) {
	result := make(map[string]string)
	value = strings.TrimSpace(strings.ReplaceAll(value, "`", ""))
	if value == "" || value == "-" {
		return result, nil
	}
	for _, raw := range strings.Split(value, "、") {
		specification := strings.TrimSpace(raw)
		name, parameterType, hasType := strings.Cut(specification, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			return result, fmt.Errorf("empty parameter in %q", value)
		}
		if !hasType {
			parameterType = inferParamType(name)
			if parameterType == "" {
				return result, fmt.Errorf("parameter %s has no documented or inferred type", name)
			}
		} else {
			parameterType = strings.TrimSpace(parameterType)
		}
		if _, duplicate := result[name]; duplicate {
			return result, fmt.Errorf("duplicate parameter %s", name)
		}
		result[name] = parameterType
	}
	return result, nil
}

func inferParamType(name string) string {
	if strings.HasSuffix(name, "_id") || name == "site_id" || name == "scope_id" {
		return "IdString"
	}
	switch name {
	case "start_timestamp", "end_timestamp", "hour_ts":
		return "Timestamp"
	}
	return ""
}

func extractStableCodes(value string) []string {
	return stableCodePattern.FindAllString(value, -1)
}

func isHTTPStatusCell(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isTableSeparator(columns []string) bool {
	if len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		trimmed := strings.Trim(column, " :-")
		if trimmed != "" {
			return false
		}
	}
	return true
}

func between(value string, start string, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	startIndex += len(start)
	endIndex := strings.Index(value[startIndex:], end)
	if endIndex < 0 {
		return value[startIndex:]
	}
	return value[startIndex : startIndex+endIndex]
}

func formatParamMap(values map[string]string) string {
	keys := sortedKeys(values)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+":"+values[key])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return fmt.Errorf("multiple JSON values are not allowed")
}
