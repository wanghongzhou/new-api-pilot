package docscheck

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const frontendMessageCodesPath = "web/src/lib/message-codes.ts"

var typescriptCodeArrayPattern = regexp.MustCompile(`(?s)export\s+const\s+([A-Z_]+)\s*=\s*\[\s*(.*?)\s*\]\s+as\s+const`)
var typescriptCodePattern = regexp.MustCompile(`['"]([A-Z][A-Z0-9_]+)['"]`)

func (current *checker) checkFrontendMessageContract(catalog *messageCatalog) {
	if catalog == nil {
		return
	}
	path := filepath.Join(current.root, filepath.FromSlash(frontendMessageCodesPath))
	content, err := os.ReadFile(path)
	if err != nil {
		current.add("openapi", path, "read frontend MessageRef code union: %v", err)
		return
	}
	arrays := make(map[string][]string)
	for _, match := range typescriptCodeArrayPattern.FindAllStringSubmatch(string(content), -1) {
		arrays[match[1]] = extractTypeScriptCodes(match[2])
	}
	current.checkTypeScriptCodeArray(path, "API_ERROR_CODES", arrays["API_ERROR_CODES"], catalog.APIErrorCodes)
	expectedMessageCodes := make([]string, 0)
	for _, entry := range catalog.Entries {
		if entry.Category != "http" {
			expectedMessageCodes = append(expectedMessageCodes, entry.Code)
		}
	}
	current.checkTypeScriptCodeArray(path, "MESSAGE_CODES", arrays["MESSAGE_CODES"], expectedMessageCodes)
}

func (current *checker) checkTypeScriptCodeArray(path, name string, actual, expected []string) {
	if actual == nil {
		current.add("openapi", path, "%s array is missing", name)
		return
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, code := range actual {
		if _, duplicate := actualSet[code]; duplicate {
			current.add("openapi", path, "%s repeats %s", name, code)
		}
		actualSet[code] = struct{}{}
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, code := range expected {
		expectedSet[code] = struct{}{}
	}
	missing := setDifference(sortedSetKeys(expectedSet), actualSet)
	extra := setDifference(sortedSetKeys(actualSet), expectedSet)
	if len(missing) > 0 || len(extra) > 0 {
		current.add("openapi", path, "%s differs from catalog; missing=[%s] extra=[%s]", name, strings.Join(missing, ", "), strings.Join(extra, ", "))
	}
}

func extractTypeScriptCodes(value string) []string {
	matches := typescriptCodePattern.FindAllStringSubmatch(value, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, match[1])
	}
	return result
}

func sortedSetKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
