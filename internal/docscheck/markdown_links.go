package docscheck

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\r\n]+)\)`)

func (current *checker) checkMarkdownLinks() {
	files := current.markdownFiles()
	headings := make(map[string]map[string]struct{}, len(files))
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			current.add("markdown", path, "read: %v", err)
			continue
		}
		headings[path] = markdownHeadings(string(content))
		inFence := false
		for lineNumber, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = !inFence
				continue
			}
			if inFence || strings.HasPrefix(trimmed, "<!--") {
				continue
			}
			for _, match := range markdownLinkPattern.FindAllStringSubmatch(line, -1) {
				destination := markdownDestination(match[1])
				if destination == "" {
					current.add("markdown", path, "line %d has an empty link target", lineNumber+1)
					continue
				}
				current.checkMarkdownDestination(path, lineNumber+1, destination, headings)
			}
		}
	}
}

func (current *checker) markdownFiles() []string {
	files := make([]string, 0)
	err := filepath.WalkDir(current.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".codegraph", "node_modules", "dist", "artifacts":
				if path != current.root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		current.add("markdown", current.root, "walk: %v", err)
	}
	return files
}

func (current *checker) checkMarkdownDestination(source string, lineNumber int, destination string, headingCache map[string]map[string]struct{}) {
	parsed, err := url.Parse(destination)
	if err != nil {
		current.add("markdown", source, "line %d has invalid target %q: %v", lineNumber, destination, err)
		return
	}
	if parsed.Scheme != "" {
		switch parsed.Scheme {
		case "http", "https":
			if parsed.Host == "" {
				current.add("markdown", source, "line %d external URL has no host: %s", lineNumber, destination)
			}
		case "mailto":
			if parsed.Opaque == "" && parsed.Path == "" {
				current.add("markdown", source, "line %d mailto target is empty", lineNumber)
			}
		default:
			current.add("markdown", source, "line %d uses unsupported URL scheme %q", lineNumber, parsed.Scheme)
		}
		return
	}

	target := source
	if parsed.Path != "" {
		decodedPath, err := url.PathUnescape(parsed.Path)
		if err != nil {
			current.add("markdown", source, "line %d cannot decode path %q: %v", lineNumber, parsed.Path, err)
			return
		}
		if filepath.IsAbs(filepath.FromSlash(decodedPath)) {
			current.add("markdown", source, "line %d uses absolute filesystem path %q", lineNumber, decodedPath)
			return
		}
		target = filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(decodedPath)))
		if !pathWithin(current.root, target) {
			current.add("markdown", source, "line %d target escapes repository: %s", lineNumber, destination)
			return
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		current.add("markdown", source, "line %d target does not exist: %s", lineNumber, destination)
		return
	}
	if parsed.Fragment == "" {
		return
	}
	if info.IsDir() {
		current.add("markdown", source, "line %d anchor targets a directory: %s", lineNumber, destination)
		return
	}
	if !strings.EqualFold(filepath.Ext(target), ".md") {
		current.add("markdown", source, "line %d anchor targets a non-Markdown file: %s", lineNumber, destination)
		return
	}
	anchors, exists := headingCache[target]
	if !exists {
		content, readErr := os.ReadFile(target)
		if readErr != nil {
			current.add("markdown", source, "line %d read anchor target: %v", lineNumber, readErr)
			return
		}
		anchors = markdownHeadings(string(content))
		headingCache[target] = anchors
	}
	fragment, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		current.add("markdown", source, "line %d cannot decode anchor %q: %v", lineNumber, parsed.Fragment, err)
		return
	}
	fragment = strings.ToLower(fragment)
	if _, exists := anchors[fragment]; !exists {
		current.add("markdown", source, "line %d anchor does not exist: %s", lineNumber, destination)
	}
}

func markdownDestination(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "<") {
		if end := strings.Index(value, ">"); end >= 0 {
			return value[1:end]
		}
	}
	if fields := strings.Fields(value); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func markdownHeadings(content string) map[string]struct{} {
	result := make(map[string]struct{})
	occurrences := make(map[string]int)
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := strings.TrimLeft(trimmed, "#")
		if heading == trimmed || heading == "" || (heading[0] != ' ' && heading[0] != '\t') {
			continue
		}
		heading = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(heading), "#"))
		slug := githubStyleSlug(heading)
		if slug == "" {
			continue
		}
		count := occurrences[slug]
		occurrences[slug]++
		if count > 0 {
			slug = fmt.Sprintf("%s-%d", slug, count)
		}
		result[slug] = struct{}{}
	}
	return result
}

func githubStyleSlug(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "`", ""))
	var slug strings.Builder
	lastHyphen := false
	for _, character := range value {
		switch {
		case unicode.IsLetter(character), unicode.IsNumber(character), character == '_':
			slug.WriteRune(character)
			lastHyphen = false
		case unicode.IsSpace(character), character == '-':
			if slug.Len() > 0 && !lastHyphen {
				slug.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(slug.String(), "-")
}

func pathWithin(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
