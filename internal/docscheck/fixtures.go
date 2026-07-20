package docscheck

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	fixtureRootPath             = "testdata/design"
	fixtureChecksumManifestPath = fixtureRootPath + "/manifest.sha256"
	fixtureManifestHeader       = "# fixture_manifest_version=1"
)

var checksumLinePattern = regexp.MustCompile(`^([a-f0-9]{64})  ([^\s].*)$`)

func (current *checker) checkFixtureChecksums(manifest *acceptanceManifest) {
	if manifest == nil {
		return
	}
	manifestRelative := manifest.Baseline.FixtureChecksumManifest
	manifestPath, err := resolveRepositoryPath(current.root, manifestRelative)
	if err != nil {
		current.add("fixtures", "", "%v", err)
		return
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		current.add("fixtures", manifestPath, "open checksum manifest: %v", err)
		return
	}
	defer file.Close()

	checksums := make(map[string]string)
	orderedPaths := make([]string, 0)
	versionFound := false
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if line == fixtureManifestHeader {
				versionFound = true
			}
			continue
		}
		if rawLine != line {
			current.add("fixtures", manifestPath, "line %d has non-canonical whitespace", lineNumber)
			continue
		}
		match := checksumLinePattern.FindStringSubmatch(line)
		if match == nil {
			current.add("fixtures", manifestPath, "line %d is not '<sha256>  <path>'", lineNumber)
			continue
		}
		relative, err := canonicalFixtureChecksumPath(match[2])
		if err != nil {
			current.add("fixtures", manifestPath, "line %d: %v", lineNumber, err)
			continue
		}
		if _, duplicate := checksums[relative]; duplicate {
			current.add("fixtures", manifestPath, "line %d repeats %s", lineNumber, relative)
			continue
		}
		checksums[relative] = match[1]
		orderedPaths = append(orderedPaths, relative)
	}
	if err := scanner.Err(); err != nil {
		current.add("fixtures", manifestPath, "read checksum manifest: %v", err)
	}
	if !versionFound {
		current.add("fixtures", manifestPath, "missing %q", fixtureManifestHeader)
	}
	if !sort.StringsAreSorted(orderedPaths) {
		current.add("fixtures", manifestPath, "checksum paths are not sorted")
	}

	actualFiles := current.fixtureFiles(manifestPath)
	for _, relative := range sortedKeys(actualFiles) {
		expectedHash, exists := checksums[relative]
		if !exists {
			current.add("fixtures", manifestPath, "fixture is not checksummed: %s", relative)
			continue
		}
		actualHash, err := hashFile(actualFiles[relative])
		if err != nil {
			current.add("fixtures", actualFiles[relative], "hash: %v", err)
			continue
		}
		if actualHash != expectedHash {
			current.add("fixtures", actualFiles[relative], "checksum mismatch: got %s, manifest has %s", actualHash, expectedHash)
		}
	}
	for relative := range checksums {
		if _, exists := actualFiles[relative]; !exists {
			current.add("fixtures", manifestPath, "checksum references missing fixture: %s", relative)
		}
	}

	for fixtureID, definition := range manifest.Fixtures {
		fixturePath, err := resolveRepositoryPath(current.root, definition.Path)
		if err != nil {
			continue
		}
		info, err := os.Stat(fixturePath)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			relative, _ := filepath.Rel(current.root, fixturePath)
			if _, exists := checksums[filepath.ToSlash(relative)]; !exists {
				current.add("fixtures", manifestPath, "%s is absent from checksum manifest", fixtureID)
			}
			continue
		}
		prefix, _ := filepath.Rel(current.root, fixturePath)
		prefix = filepath.ToSlash(prefix) + "/"
		matched := false
		for relative := range checksums {
			if strings.HasPrefix(relative, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			current.add("fixtures", manifestPath, "%s directory has no checksummed files", fixtureID)
		}
	}
}

func (current *checker) fixtureFiles(checksumManifestPath string) map[string]string {
	fixtureRoot := filepath.Join(current.root, filepath.FromSlash(fixtureRootPath))
	result := make(map[string]string)
	err := filepath.WalkDir(fixtureRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if path == checksumManifestPath {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			current.add("fixtures", path, "fixture symlinks are not allowed")
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			current.add("fixtures", path, "fixture must be a regular file")
			return nil
		}
		relative, err := filepath.Rel(current.root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = path
		return nil
	})
	if err != nil {
		current.add("fixtures", fixtureRoot, "walk: %v", err)
	}
	return result
}

// WriteFixtureChecksumManifest records the complete, sorted fixture inventory.
// It deliberately excludes only the manifest itself, whose digest would be
// self-referential. contract-generate calls this after it refreshes OpenAPI.
func WriteFixtureChecksumManifest(root string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	manifestPath := filepath.Join(absoluteRoot, filepath.FromSlash(fixtureChecksumManifestPath))
	files, err := fixtureFilesForWrite(absoluteRoot, manifestPath)
	if err != nil {
		return err
	}

	lines := make([]string, 0, len(files)+1)
	lines = append(lines, fixtureManifestHeader)
	for _, relative := range sortedKeys(files) {
		digest, err := hashFile(files[relative])
		if err != nil {
			return fmt.Errorf("hash fixture %s: %w", relative, err)
		}
		lines = append(lines, checksumLine(digest, relative))
	}
	payload := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(manifestPath, payload, 0o644); err != nil {
		return fmt.Errorf("write fixture checksum manifest: %w", err)
	}
	return nil
}

func fixtureFilesForWrite(root, checksumManifestPath string) (map[string]string, error) {
	fixtureRoot := filepath.Join(root, filepath.FromSlash(fixtureRootPath))
	files := make(map[string]string)
	err := filepath.WalkDir(fixtureRoot, func(pathname string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if pathname == checksumManifestPath {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink is not allowed: %s", filepath.ToSlash(pathname))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture must be a regular file: %s", filepath.ToSlash(pathname))
		}
		relative, err := filepath.Rel(root, pathname)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, err := canonicalFixtureChecksumPath(relative); err != nil {
			return fmt.Errorf("canonicalize fixture path %s: %w", relative, err)
		}
		files[relative] = pathname
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk fixture inventory: %w", err)
	}
	return files, nil
}

func canonicalFixtureChecksumPath(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.Contains(value, `\`) {
		return "", fmt.Errorf("fixture checksum path is not canonical: %q", value)
	}
	cleaned := path.Clean(value)
	if cleaned != value || !strings.HasPrefix(cleaned, fixtureRootPath+"/") || cleaned == fixtureChecksumManifestPath {
		return "", fmt.Errorf("fixture checksum path is outside the fixture inventory: %q", value)
	}
	return cleaned, nil
}

func hashFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

func checksumLine(hash string, relative string) string {
	return fmt.Sprintf("%s  %s", hash, filepath.ToSlash(relative))
}
