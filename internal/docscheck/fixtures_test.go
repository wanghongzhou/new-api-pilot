package docscheck

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFixtureChecksumManifestCoversEveryFixtureFile(t *testing.T) {
	root := t.TempDir()
	fixtureRoot := filepath.Join(root, filepath.FromSlash(fixtureRootPath))
	if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureRoot, "f01-auth.json")
	writeTestFile(t, fixturePath, "first fixture\n")

	if err := WriteFixtureChecksumManifest(root); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(fixtureChecksumManifestPath))
	first, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFixtureChecksumManifest(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("fixture checksum generation is not deterministic")
	}

	manifest := &acceptanceManifest{
		Baseline: acceptanceBaseline{FixtureChecksumManifest: fixtureChecksumManifestPath},
		Fixtures: map[string]fixtureDefinition{
			"F01": {Path: fixtureRootPath + "/f01-auth.json"},
		},
	}
	current := &checker{root: root}
	current.checkFixtureChecksums(manifest)
	if len(current.issues) != 0 {
		t.Fatalf("generated checksum manifest produced issues: %#v", current.issues)
	}

	writeTestFile(t, filepath.Join(fixtureRoot, "untracked.json"), "new fixture\n")
	current = &checker{root: root}
	current.checkFixtureChecksums(manifest)
	if !containsIssue(current.issues, "fixture is not checksummed: testdata/design/untracked.json") {
		t.Fatalf("untracked fixture was accepted: %#v", current.issues)
	}
}

func TestFixtureChecksumManifestRejectsNonCanonicalInventoryPaths(t *testing.T) {
	root := t.TempDir()
	fixtureRoot := filepath.Join(root, filepath.FromSlash(fixtureRootPath))
	if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureRoot, "f01-auth.json")
	writeTestFile(t, fixturePath, "fixture\n")
	digest, err := hashFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(fixtureChecksumManifestPath))
	writeTestFile(t, manifestPath, strings.Join([]string{
		fixtureManifestHeader,
		checksumLine(digest, fixtureRootPath+"/f01-auth.json"),
		strings.Repeat("0", 64) + "  testdata/design/../outside.json",
		"",
	}, "\n"))

	current := &checker{root: root}
	current.checkFixtureChecksums(&acceptanceManifest{
		Baseline: acceptanceBaseline{FixtureChecksumManifest: fixtureChecksumManifestPath},
	})
	if !containsIssue(current.issues, "fixture checksum path is outside the fixture inventory") {
		t.Fatalf("non-canonical checksum path was accepted: %#v", current.issues)
	}
}

func containsIssue(issues []Issue, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	return false
}
