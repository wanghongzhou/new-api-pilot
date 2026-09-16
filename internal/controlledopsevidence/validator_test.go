package controlledopsevidence

import (
	"archive/zip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCanonicalCommandsAreCaseSpecificAndFailClosed(t *testing.T) {
	for _, id := range []string{"A52", "A74", "A75"} {
		command := CanonicalCommand(id)
		if _, err := Classify(id, "artifacts/acceptance", command); err != nil {
			t.Fatalf("%s canonical command: %v", id, err)
		}
		if _, err := Classify(id, "artifacts/acceptance", []string{"powershell.exe", "-Command", "exit 0"}); err == nil {
			t.Fatalf("%s accepted arbitrary success command", id)
		}
		if _, err := Classify(id, "artifacts/smoke", command); err == nil {
			t.Fatalf("%s accepted non-formal evidence root", id)
		}
	}
}

func TestClosedArtifactsDetectTamperAndBlockedEvidenceCannotPass(t *testing.T) {
	for _, id := range []string{"A52", "A74", "A75"} {
		t.Run(id, func(t *testing.T) {
			dir := buildValidRun(t, id)
			if err := ValidateInnerArtifacts(dir, id, FormalClass); err != nil {
				t.Fatalf("valid inner evidence: %v", err)
			}
			reportPath := filepath.Join(dir, stringsLower(id)+"-report.json")
			if err := os.WriteFile(reportPath, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateInnerArtifacts(dir, id, FormalClass); err == nil {
				t.Fatal("tampered report passed")
			}

			blocked := t.TempDir()
			writeJSONTest(t, filepath.Join(blocked, "blocked-report.json"), map[string]any{"schema_version": 1, "acceptance_id": id, "status": "blocked", "passed": false})
			if err := ValidateInnerArtifacts(blocked, id, FormalClass); err == nil {
				t.Fatal("blocked evidence passed")
			}
		})
	}
}

func TestPowerShellRunnersFailClosedAndProduceValidInnerArtifacts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the canonical runners require Windows PowerShell")
	}
	powerShell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell.exe is unavailable")
	}
	repositoryRoot := findRepositoryRoot(t)
	for _, id := range []string{"A52", "A74", "A75"} {
		t.Run(id+"Blocked", func(t *testing.T) {
			evidenceDir := t.TempDir()
			command := exec.Command(powerShell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(repositoryRoot, "scripts", "acceptance", "run-"+stringsLower(id)+".ps1"))
			command.Dir = repositoryRoot
			command.Env = append(os.Environ(),
				"ACCEPTANCE_ID="+id,
				"ACCEPTANCE_EVIDENCE_CLASS=formal",
				"ACCEPTANCE_EVIDENCE_DIR="+evidenceDir,
				id+"_CONTROLLED_INPUT=",
			)
			if err := command.Run(); err == nil {
				t.Fatal("runner succeeded without controlled input")
			}
			if _, err := os.Stat(filepath.Join(evidenceDir, "blocked-report.json")); err != nil {
				t.Fatalf("blocked report was not written: %v", err)
			}
			if err := ValidateInnerArtifacts(evidenceDir, id, FormalClass); err == nil {
				t.Fatal("blocked runner output passed validation")
			}
		})

		t.Run(id+"Passing", func(t *testing.T) {
			sourceDir := buildValidRun(t, id)
			materialPath := filepath.Join(sourceDir, stringsLower(id)+"-controlled-material.zip")
			contract := caseContracts[id]
			assertions := map[string]bool{}
			for _, key := range contract.assertions {
				assertions[key] = true
			}
			inputPath := filepath.Join(t.TempDir(), stringsLower(id)+"-input.json")
			writeJSONTest(t, inputPath, map[string]any{
				"schema_version": 1, "acceptance_id": id, "status": "passed", "passed": true,
				"evidence_class": FormalClass, "acceptance_eligible": true, "scope": contract.scope,
				"target_identity": "sanitized-controlled-target", "immutable_reference": stringsRepeat("a", 64),
				"started_at": "2026-01-01T00:00:00Z", "finished_at": "2026-01-01T00:01:00Z",
				"assertions": assertions, "controlled_material_path": materialPath,
			})
			evidenceDir := t.TempDir()
			command := exec.Command(powerShell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(repositoryRoot, "scripts", "acceptance", "run-"+stringsLower(id)+".ps1"))
			command.Dir = repositoryRoot
			command.Env = append(os.Environ(),
				"ACCEPTANCE_ID="+id,
				"ACCEPTANCE_EVIDENCE_CLASS=formal",
				"ACCEPTANCE_EVIDENCE_DIR="+evidenceDir,
				id+"_CONTROLLED_INPUT="+inputPath,
			)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("runner failed: %v\n%s", err, output)
			}
			if err := ValidateInnerArtifacts(evidenceDir, id, FormalClass); err != nil {
				entries, _ := os.ReadDir(evidenceDir)
				names := make([]string, 0, len(entries))
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				t.Fatalf("runner output failed validation: %v; files=%v; output=%s", err, names, output)
			}
		})
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatal("repository root containing go.mod was not found")
		}
		current = parent
	}
}

func buildValidRun(t *testing.T, id string) string {
	t.Helper()
	contract := caseContracts[id]
	dir := t.TempDir()
	prefix := stringsLower(id)
	material := filepath.Join(dir, prefix+"-controlled-material.zip")
	file, err := os.Create(material)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, name := range contract.zipFiles {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		doc := materialDocument{SchemaVersion: 1, AcceptanceID: id, ArtifactType: name[:len(name)-5], Passed: true, Sanitized: true}
		if name == "approvals.json" {
			doc.Operator = "operator"
			doc.Reviewer = "reviewer"
			doc.Approver = "approver"
			doc.Approved = true
		}
		payload, _ := json.Marshal(doc)
		if _, err := writer.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(material)
	digest, _ := hashFile(material)
	assertions := map[string]bool{}
	for _, key := range contract.assertions {
		assertions[key] = true
	}
	writeJSONTest(t, filepath.Join(dir, prefix+"-report.json"), finalReport{SchemaVersion: 1, AcceptanceID: id, Status: "passed", Passed: true, EvidenceClass: FormalClass, AcceptanceEligible: true, Scope: contract.scope, TargetIdentity: "controlled-target", ImmutableReference: stringsRepeat("a", 64), StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:01:00Z", MaterialSHA256: digest, MaterialSizeBytes: info.Size(), Assertions: assertions})
	writeJSONTest(t, filepath.Join(dir, prefix+"-command.json"), commandReport{SchemaVersion: 1, AcceptanceID: id, EvidenceClass: FormalClass, Command: CanonicalCommand(id)})
	writeJSONTest(t, filepath.Join(dir, prefix+"-fixture.json"), fixtureReport{SchemaVersion: 1, AcceptanceID: id, ManifestPath: "testdata/design/manifest.sha256", ManifestSHA256: stringsRepeat("b", 64), FixtureIDs: contract.fixtures})
	names := []string{prefix + "-command.json", prefix + "-controlled-material.zip", prefix + "-fixture.json", prefix + "-report.json"}
	entries := make([]artifactEntry, 0, len(names))
	for _, name := range names {
		info, _ := os.Stat(filepath.Join(dir, name))
		digest, _ := hashFile(filepath.Join(dir, name))
		entries = append(entries, artifactEntry{Path: name, SizeBytes: info.Size(), SHA256: digest})
	}
	writeJSONTest(t, filepath.Join(dir, prefix+"-artifacts.json"), artifactInventory{SchemaVersion: 1, AcceptanceID: id, EvidenceClass: FormalClass, Files: entries})
	return dir
}

func writeJSONTest(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}
func stringsLower(value string) string {
	if value == "A52" {
		return "a52"
	}
	if value == "A74" {
		return "a74"
	}
	return "a75"
}
func stringsRepeat(value string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += value
	}
	return result
}
