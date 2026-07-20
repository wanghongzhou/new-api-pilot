package docscheck

import "strings"

func splitMarkdownRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 2 || trimmed[0] != '|' || trimmed[len(trimmed)-1] != '|' {
		return nil
	}

	columns := make([]string, 0)
	var current strings.Builder
	escaped := false
	for _, character := range trimmed[1 : len(trimmed)-1] {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '|' {
			columns = append(columns, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(character)
	}
	if escaped {
		current.WriteRune('\\')
	}
	columns = append(columns, strings.TrimSpace(current.String()))
	return columns
}
