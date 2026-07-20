package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const designAcceptancePath = "docs/多站点运营管理平台-详细设计-07-运维与验收.md"

var referencePattern = regexp.MustCompile(`([DRA])(\d{2,3})(?:\s*[～~-]\s*([DRA])(\d{2,3}))?`)

type traceability struct {
	designPath      string
	decisions       map[string]traceDecision
	requirements    map[string]map[string]struct{}
	acceptanceCases map[string]struct{}
}

type traceDecision struct {
	requirements []string
	acceptance   []string
	absenceScan  bool
}

func (current *checker) checkTraceability() traceability {
	path := filepath.Join(current.root, filepath.FromSlash(designAcceptancePath))
	result := traceability{
		designPath:      path,
		decisions:       make(map[string]traceDecision),
		requirements:    make(map[string]map[string]struct{}),
		acceptanceCases: make(map[string]struct{}),
	}
	content, err := os.ReadFile(path)
	if err != nil {
		current.add("traceability", path, "read design acceptance document: %v", err)
		return result
	}

	for lineNumber, line := range strings.Split(string(content), "\n") {
		columns := splitMarkdownRow(line)
		if len(columns) == 0 {
			continue
		}
		identifier := columns[0]
		switch {
		case matchesIdentifier(identifier, "D", 2) || matchesIdentifier(identifier, "D", 3):
			if len(columns) != 5 {
				current.add("traceability", path, "line %d: decision row has %d columns, want 5", lineNumber+1, len(columns))
				continue
			}
			if _, duplicate := result.decisions[identifier]; duplicate {
				current.add("traceability", path, "line %d: duplicate decision %s", lineNumber+1, identifier)
				continue
			}
			result.decisions[identifier] = traceDecision{
				requirements: expandReferences(columns[3], "R"),
				acceptance:   expandReferences(columns[4], "A"),
				absenceScan:  strings.TrimSpace(columns[4]) == "N/A-absence-scan",
			}
		case matchesIdentifier(identifier, "R", 2):
			if len(columns) != 5 {
				current.add("traceability", path, "line %d: requirement row has %d columns, want 5", lineNumber+1, len(columns))
				continue
			}
			if _, duplicate := result.requirements[identifier]; duplicate {
				current.add("traceability", path, "line %d: duplicate requirement %s", lineNumber+1, identifier)
				continue
			}
			linked := make(map[string]struct{})
			for _, acceptanceID := range expandReferences(columns[4], "A") {
				linked[acceptanceID] = struct{}{}
			}
			result.requirements[identifier] = linked
		case matchesIdentifier(identifier, "A", 2) || matchesIdentifier(identifier, "A", 3):
			if len(columns) != 3 {
				current.add("traceability", path, "line %d: acceptance row has %d columns, want 3", lineNumber+1, len(columns))
				continue
			}
			if _, duplicate := result.acceptanceCases[identifier]; duplicate {
				current.add("traceability", path, "line %d: duplicate acceptance case %s", lineNumber+1, identifier)
			}
			result.acceptanceCases[identifier] = struct{}{}
		}
	}

	checkIdentifierRange(current, path, "D", 1, 139, keys(result.decisions))
	checkIdentifierRange(current, path, "R", 1, 10, keys(result.requirements))
	checkIdentifierRange(current, path, "A", 1, 100, keys(result.acceptanceCases))

	for decisionID, decision := range result.decisions {
		if len(decision.requirements) == 0 {
			current.add("traceability", path, "%s has no valid R reference", decisionID)
		}
		if len(decision.acceptance) == 0 && !decision.absenceScan {
			current.add("traceability", path, "%s has neither A reference nor N/A-absence-scan", decisionID)
		}
		for _, requirementID := range decision.requirements {
			if _, exists := result.requirements[requirementID]; !exists {
				current.add("traceability", path, "%s references unknown %s", decisionID, requirementID)
			}
		}
		for _, acceptanceID := range decision.acceptance {
			if _, exists := result.acceptanceCases[acceptanceID]; !exists {
				current.add("traceability", path, "%s references unknown %s", decisionID, acceptanceID)
			}
		}
	}

	for acceptanceID := range result.acceptanceCases {
		linked := false
		for _, requirementCases := range result.requirements {
			if _, exists := requirementCases[acceptanceID]; exists {
				linked = true
				break
			}
		}
		if !linked {
			current.add("traceability", path, "%s is not linked from any R row", acceptanceID)
		}
	}
	return result
}

func matchesIdentifier(value string, prefix string, digits int) bool {
	if len(value) != len(prefix)+digits || !strings.HasPrefix(value, prefix) {
		return false
	}
	_, err := strconv.Atoi(value[len(prefix):])
	return err == nil
}

func expandReferences(value string, prefix string) []string {
	seen := make(map[string]struct{})
	references := make([]string, 0)
	for _, match := range referencePattern.FindAllStringSubmatch(value, -1) {
		if match[1] != prefix {
			continue
		}
		start, _ := strconv.Atoi(match[2])
		end := start
		if match[3] != "" && match[4] != "" {
			if match[3] != prefix {
				continue
			}
			end, _ = strconv.Atoi(match[4])
		}
		if end < start {
			start, end = end, start
		}
		for number := start; number <= end; number++ {
			width := 2
			if prefix == "D" {
				width = 2
				if number >= 100 {
					width = 3
				}
			}
			identifier := fmt.Sprintf("%s%0*d", prefix, width, number)
			if _, exists := seen[identifier]; !exists {
				seen[identifier] = struct{}{}
				references = append(references, identifier)
			}
		}
	}
	return references
}

func checkIdentifierRange(current *checker, path string, prefix string, first int, last int, actual []string) {
	present := make(map[string]struct{}, len(actual))
	for _, identifier := range actual {
		present[identifier] = struct{}{}
	}
	for number := first; number <= last; number++ {
		width := 2
		if prefix == "D" && number >= 100 {
			width = 3
		}
		identifier := fmt.Sprintf("%s%0*d", prefix, width, number)
		if _, exists := present[identifier]; !exists {
			current.add("traceability", path, "missing %s", identifier)
		}
	}
	for _, identifier := range actual {
		number, _ := strconv.Atoi(strings.TrimPrefix(identifier, prefix))
		if number < first || number > last {
			current.add("traceability", path, "out-of-range identifier %s", identifier)
		}
	}
}

func keys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
