package docscheck

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const i18nConfigPath = "web/src/i18n/config.ts"

var (
	localeImportPattern       = regexp.MustCompile(`(?m)^import\s+([A-Za-z][A-Za-z0-9_]*)\s+from\s+'\./locales/([^']+\.json)'\s*$`)
	appLanguagePattern        = regexp.MustCompile(`(?m)(?:export\s+)?const\s+appLanguage\s*=\s*['"]([^'"]+)['"]`)
	supportedLanguagesPattern = regexp.MustCompile(`(?s)(?:export\s+)?const\s+supportedLanguages\s*=\s*\[([^\]]*)\]`)
	inlineSupportedPattern    = regexp.MustCompile(`(?s)supportedLngs:\s*\[([^\]]*)\]`)
	quotedStringPattern       = regexp.MustCompile(`['"]([^'"]+)['"]`)
)

type localeDefinition struct {
	language string
	alias    string
	path     string
	values   map[string]string
}

func (current *checker) checkLocales(catalog *messageCatalog) {
	configPath := filepath.Join(current.root, filepath.FromSlash(i18nConfigPath))
	content, err := os.ReadFile(configPath)
	if err != nil {
		current.add("locales", configPath, "read i18n config: %v", err)
		return
	}
	appLanguage := ""
	if match := appLanguagePattern.FindStringSubmatch(string(content)); match != nil {
		appLanguage = match[1]
		if appLanguage != "zh-CN" {
			current.add("locales", configPath, "appLanguage = %q, want zh-CN", appLanguage)
		}
	}
	definitions := make(map[string]*localeDefinition)
	for _, match := range localeImportPattern.FindAllStringSubmatch(string(content), -1) {
		alias := match[1]
		filename := match[2]
		language := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		if _, duplicate := definitions[language]; duplicate {
			current.add("locales", configPath, "locale %s imported more than once", language)
			continue
		}
		localePath := filepath.Join(filepath.Dir(configPath), "locales", filepath.FromSlash(filename))
		definition := &localeDefinition{language: language, alias: alias, path: localePath}
		definition.values = current.readLocale(definition)
		definitions[language] = definition

		resourcePattern := regexp.MustCompile(`(?m)^\s*(?:['"]` + regexp.QuoteMeta(language) + `['"]|\[appLanguage\]):\s*\{\s*translation:\s*` + regexp.QuoteMeta(alias) + `\s*\},?\s*$`)
		if !resourcePattern.Match(content) {
			current.add("locales", configPath, "imported locale %s is not wired into resources", language)
		}
	}
	if len(definitions) == 0 {
		current.add("locales", configPath, "no locale imports found")
		return
	}
	base, exists := definitions["zh-CN"]
	if !exists {
		current.add("locales", configPath, "the only supported locale must be zh-CN.json")
		return
	}
	if len(definitions) != 1 {
		current.add("locales", configPath, "expected only zh-CN, found: %s", strings.Join(sortedKeys(definitions), ", "))
	}
	current.checkLocaleDirectory(configPath, definitions)
	current.checkLanguageSwitchAbsence(configPath, content)

	supported := make(map[string]struct{})
	supportedSourceFound := false
	if match := supportedLanguagesPattern.FindStringSubmatch(string(content)); match != nil {
		supportedSourceFound = true
		for _, languageMatch := range quotedStringPattern.FindAllStringSubmatch(match[1], -1) {
			supported[languageMatch[1]] = struct{}{}
		}
		if !regexp.MustCompile(`supportedLngs:\s*supportedLanguages`).Match(content) {
			current.add("locales", configPath, "i18next does not use the supportedLanguages source")
		}
	} else if match := inlineSupportedPattern.FindStringSubmatch(string(content)); match != nil {
		supportedSourceFound = true
		for _, languageMatch := range quotedStringPattern.FindAllStringSubmatch(match[1], -1) {
			supported[languageMatch[1]] = struct{}{}
		}
		if regexp.MustCompile(`^\s*appLanguage\s*$`).MatchString(match[1]) && appLanguage != "" {
			supported[appLanguage] = struct{}{}
		}
	}
	if !supportedSourceFound {
		current.add("locales", configPath, "supportedLngs is missing")
	} else {
		if difference := setDifference(keys(definitions), supported); len(difference) > 0 {
			current.add("locales", configPath, "imported locales missing from supportedLngs: %s", strings.Join(difference, ", "))
		}
		if difference := setDifference(keys(supported), definitions); len(difference) > 0 {
			current.add("locales", configPath, "supportedLngs without imported locale: %s", strings.Join(difference, ", "))
		}
	}

	baseKeys := keys(base.values)
	for language, definition := range definitions {
		missing := setDifference(baseKeys, definition.values)
		extra := setDifference(keys(definition.values), base.values)
		if len(missing) > 0 || len(extra) > 0 {
			current.add("locales", definition.path, "%s key set differs from zh-CN; missing=[%s] extra=[%s]", language, strings.Join(missing, ", "), strings.Join(extra, ", "))
		}
	}
	if catalog == nil {
		return
	}
	requiredCodes := make([]string, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		requiredCodes = append(requiredCodes, entry.Code)
	}
	sort.Strings(requiredCodes)
	for language, definition := range definitions {
		missing := setDifference(requiredCodes, definition.values)
		if len(missing) > 0 {
			current.add("locales", definition.path, "%s is missing MessageRef keys: %s", language, strings.Join(missing, ", "))
		}
	}
}

func (current *checker) checkLocaleDirectory(configPath string, definitions map[string]*localeDefinition) {
	localeDir := filepath.Join(filepath.Dir(configPath), "locales")
	entries, err := os.ReadDir(localeDir)
	if err != nil {
		current.add("locales", localeDir, "read locale directory: %v", err)
		return
	}
	configuredPaths := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		configuredPaths[filepath.Clean(definition.path)] = struct{}{}
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(localeDir, entry.Name())
		if _, configured := configuredPaths[path]; !configured {
			current.add("locales", path, "extra locale resource is not allowed; product language is zh-CN only")
		}
	}
}

func (current *checker) checkLanguageSwitchAbsence(configPath string, config []byte) {
	configText := string(config)
	if !regexp.MustCompile(`lng:\s*(?:['"]zh-CN['"]|appLanguage)`).Match(config) {
		current.add("locales", configPath, "i18next must set lng to zh-CN")
	}
	if !regexp.MustCompile(`fallbackLng:\s*(?:['"]zh-CN['"]|appLanguage)`).Match(config) {
		current.add("locales", configPath, "i18next must set fallbackLng to zh-CN")
	}
	for _, forbidden := range []string{"LanguageDetector", "i18next-browser-languagedetector", "changeLanguage", "languageChanged"} {
		if strings.Contains(configText, forbidden) {
			current.add("locales", configPath, "language detection/switch token %q is not allowed", forbidden)
		}
	}

	sourceRoot := filepath.Join(current.root, "web", "src")
	err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path == configPath {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension != ".ts" && extension != ".tsx" && extension != ".js" && extension != ".jsx" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), "changeLanguage") {
			current.add("locales", path, "language switching is not allowed")
		}
		return nil
	})
	if err != nil {
		current.add("locales", sourceRoot, "scan language switching: %v", err)
	}

	packagePath := filepath.Join(current.root, "web", "package.json")
	packageContent, err := os.ReadFile(packagePath)
	if err != nil {
		current.add("locales", packagePath, "read: %v", err)
		return
	}
	var packageManifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(packageContent, &packageManifest); err != nil {
		current.add("locales", packagePath, "decode: %v", err)
		return
	}
	if _, exists := packageManifest.Dependencies["i18next-browser-languagedetector"]; exists {
		current.add("locales", packagePath, "i18next-browser-languagedetector is not allowed for a zh-CN-only product")
	}
	if _, exists := packageManifest.DevDependencies["i18next-browser-languagedetector"]; exists {
		current.add("locales", packagePath, "i18next-browser-languagedetector is not allowed for a zh-CN-only product")
	}
}

func (current *checker) readLocale(definition *localeDefinition) map[string]string {
	file, err := os.Open(definition.path)
	if err != nil {
		current.add("locales", definition.path, "open: %v", err)
		return map[string]string{}
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	values := make(map[string]string)
	if err := decodeLocaleObject(decoder, "", values); err != nil {
		current.add("locales", definition.path, "decode: %v", err)
		return values
	}
	if err := ensureJSONEOF(decoder); err != nil {
		current.add("locales", definition.path, "%v", err)
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			current.add("locales", definition.path, "key %q has an empty value", key)
		}
	}
	return values
}

func decodeLocaleObject(decoder *json.Decoder, prefix string, values map[string]string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return fmt.Errorf("locale root and nested values must be objects")
	}
	return decodeLocaleObjectAfterStart(decoder, prefix, values)
}

func decodeLocaleObjectAfterStart(decoder *json.Decoder, prefix string, values map[string]string) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("locale object key is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate locale key %q", joinLocaleKey(prefix, key))
		}
		seen[key] = struct{}{}

		valueToken, err := decoder.Token()
		if err != nil {
			return err
		}
		fullKey := joinLocaleKey(prefix, key)
		switch value := valueToken.(type) {
		case string:
			values[fullKey] = value
		case json.Delim:
			if value != '{' {
				return fmt.Errorf("locale key %q must be a string or object", fullKey)
			}
			if err := decodeLocaleObjectAfterStart(decoder, fullKey, values); err != nil {
				return err
			}
		default:
			return fmt.Errorf("locale key %q must be a string or object", fullKey)
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return fmt.Errorf("locale object is not closed")
	}
	return nil
}

func joinLocaleKey(prefix string, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func setDifference[T any](left []string, right map[string]T) []string {
	difference := make([]string, 0)
	for _, value := range left {
		if _, exists := right[value]; !exists {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}
