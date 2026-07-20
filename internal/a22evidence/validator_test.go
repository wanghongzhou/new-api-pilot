package a22evidence

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyRequiresCanonicalA22CommandAndRoot(t *testing.T) {
	for root, want := range map[string]string{
		"artifacts/acceptance": FormalClass,
		"artifacts/smoke":      DevelopmentClass,
	} {
		got, err := Classify(AcceptanceID, root, append([]string(nil), canonicalCommand...))
		if err != nil || got != want {
			t.Fatalf("Classify(%q) = %q, %v; want %q", root, got, err, want)
		}
	}
	if _, err := Classify(AcceptanceID, "artifacts/other", canonicalCommand); err == nil {
		t.Fatal("noncanonical A22 evidence root was accepted")
	}
	if _, err := Classify(AcceptanceID, "artifacts/smoke", []string{"cmd.exe", "/c", "exit", "0"}); err == nil {
		t.Fatal("arbitrary A22 command was accepted")
	}
	if got, err := Classify("A21", "artifacts/other", nil); err != nil || got != "" {
		t.Fatalf("unrelated case classification = %q, %v", got, err)
	}
}

func TestSensitiveEvidenceClassification(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytesOf(32, 0x5a))
	for name, payload := range map[string]string{
		"fixed secret":   fixedSecrets[0],
		"database dsn":   "DATABASE_DSN=admin:password@tcp(mysql:3306)/pilot_a22",
		"secret env":     "A22_SITE_TOKEN=plaintext",
		"url credential": "https://example.invalid/hook?access_token=plaintext",
		"base64 key":     "key " + key + " end",
	} {
		if got := classifySensitive(payload); got == "" {
			t.Fatalf("%s was not classified", name)
		}
	}
	if got := classifySensitive("ENCRYPTION_KEY=[redacted] production_release_authorized=false"); got != "" {
		t.Fatalf("redacted safe payload classified as %q", got)
	}
}

func TestValidateInnerArtifactsCanonicalAndFailClosedMutations(t *testing.T) {
	t.Run("canonical", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
		if err := ValidateInnerArtifacts(directory, DevelopmentClass); err != nil {
			t.Fatalf("canonical A22 inner evidence failed: %v", err)
		}
	})

	t.Run("snapshot mismatch", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
		var target snapshotReport
		readTestJSON(t, filepath.Join(directory, "a22-target-snapshot.json"), &target)
		target.SnapshotSHA256 = strings.Repeat("b", 64)
		writeTestJSON(t, filepath.Join(directory, "a22-target-snapshot.json"), target)
		refreshInventory(t, directory, DevelopmentClass)
		if err := ValidateInnerArtifacts(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "exact isolated restore") {
			t.Fatalf("snapshot mismatch was not rejected at exact-restore gate: %v", err)
		}
	})

	t.Run("negative import started", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
		var negative negativeReport
		path := filepath.Join(directory, "a22-negative-manifest.json")
		readTestJSON(t, path, &negative)
		negative.ImportStarted = true
		writeTestJSON(t, path, negative)
		refreshInventory(t, directory, DevelopmentClass)
		if err := ValidateInnerArtifacts(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "negative artifact") {
			t.Fatalf("negative branch that began import was accepted: %v", err)
		}
	})

	t.Run("negative release gate", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
		var negative negativeReport
		path := filepath.Join(directory, "a22-negative-target-mismatch.json")
		readTestJSON(t, path, &negative)
		negative.ReleaseGateExists = true
		writeTestJSON(t, path, negative)
		refreshInventory(t, directory, DevelopmentClass)
		if err := ValidateInnerArtifacts(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "negative artifact") {
			t.Fatalf("negative branch that published a release gate was accepted: %v", err)
		}
	})

	t.Run("cleanup residual", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
		var cleanup cleanupReport
		path := filepath.Join(directory, "a22-cleanup.json")
		readTestJSON(t, path, &cleanup)
		cleanup.Residuals.Networks = []string{"leftover-network"}
		writeTestJSON(t, path, cleanup)
		refreshInventory(t, directory, DevelopmentClass)
		if err := ValidateInnerArtifacts(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "cleanup artifact") {
			t.Fatalf("cleanup residual was accepted: %v", err)
		}
	})

	t.Run("production release authorization", func(t *testing.T) {
		directory := writeCanonicalInnerEvidence(t, FormalClass)
		var report finalReport
		path := filepath.Join(directory, "a22-report.json")
		readTestJSON(t, path, &report)
		report.ProductionReleaseAuthorized = true
		writeTestJSON(t, path, report)
		refreshInventory(t, directory, FormalClass)
		if err := ValidateInnerArtifacts(directory, FormalClass); err == nil || !strings.Contains(err.Error(), "final report") {
			t.Fatalf("production release authorization was accepted: %v", err)
		}
	})

	mutations := []struct {
		name   string
		want   string
		mutate func(*testing.T, string)
	}{
		{
			name: "tool command sequence", want: "command artifact",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[commandReport](t, directory, "a22-command.json", func(value *commandReport) {
					value.ToolCommands[0] = "placeholder-command"
				})
			},
		},
		{
			name: "snapshot UUID fingerprint", want: "exact isolated restore",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[snapshotReport](t, directory, "a22-source-snapshot.json", func(value *snapshotReport) {
					value.ServerUUIDFingerprint = "000000000000"
				})
			},
		},
		{
			name: "environment snapshot fingerprint", want: "environment identity",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[environmentReport](t, directory, "a22-environment.json", func(value *environmentReport) {
					value.Source.UUIDFingerprint = "000000000000"
				})
			},
		},
		{
			name: "environment snapshot version", want: "environment identity",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[environmentReport](t, directory, "a22-environment.json", func(value *environmentReport) {
					value.Target.Version = "8.4.other"
				})
			},
		},
		{
			name: "verify unknown check", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.Checks[0].Name = "unknown_check"
				})
			},
		},
		{
			name: "verify duplicate check", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.Checks[0].Name = value.Checks[1].Name
				})
			},
		},
		{
			name: "verify failed check", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.Checks[0].Status = "failed"
				})
			},
		},
		{
			name: "verify summary", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.Summary.Passed--
				})
			},
		},
		{
			name: "verify backup binding", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.BackupID = "backup-20260715T010203Z-feedface"
				})
			},
		},
		{
			name: "verify key binding", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[verifyReport](t, directory, "a22-verify-restore.json", func(value *verifyReport) {
					value.EncryptionKeyID = "dddddddddddd"
				})
			},
		},
		{
			name: "backup tools image binding", want: "full verification",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[backupReport](t, directory, "a22-backup.json", func(value *backupReport) {
					value.ImageDigest = "sha256:" + strings.Repeat("b", 64)
				})
			},
		},
		{
			name: "backup timing binding", want: "backup creation time",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[backupReport](t, directory, "a22-backup.json", func(value *backupReport) {
					parsed, err := time.Parse(time.RFC3339, value.CreatedAtUTC)
					if err != nil {
						t.Fatal(err)
					}
					value.CreatedAtUTC = parsed.Add(time.Second).Format(time.RFC3339)
				})
			},
		},
		{
			name: "final report extra check", want: "final report",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[finalReport](t, directory, "a22-report.json", func(value *finalReport) {
					value.Checks["check_00"] = true
				})
			},
		},
		{
			name: "final report missing check", want: "final report",
			mutate: func(t *testing.T, directory string) {
				mutateTestJSON[finalReport](t, directory, "a22-report.json", func(value *finalReport) {
					delete(value.Checks, canonicalFinalChecks[0])
					value.Checks["check_00"] = true
				})
			},
		},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			directory := writeCanonicalInnerEvidence(t, DevelopmentClass)
			test.mutate(t, directory)
			refreshInventory(t, directory, DevelopmentClass)
			if err := ValidateInnerArtifacts(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("mutation %q was not rejected by %q gate: %v", test.name, test.want, err)
			}
		})
	}
}

func TestArtifactInventoryDetectsTamper(t *testing.T) {
	directory := t.TempDir()
	entries := make([]artifactEntry, 0, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		payload := []byte("safe artifact " + name + "\n")
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		entries = append(entries, artifactEntry{
			Path: name, SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:]),
		})
	}
	writeTestJSON(t, filepath.Join(directory, "a22-artifacts.json"), artifactInventory{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: DevelopmentClass, Files: entries,
	})
	if err := validateInventory(directory, DevelopmentClass); err != nil {
		t.Fatalf("canonical inventory failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, requiredArtifacts[0]), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateInventory(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tampered artifact was not rejected: %v", err)
	}
}

func TestWrapperLogsRejectSecretsAndSkipMarkers(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateWrapperLogs(directory); err != nil {
		t.Fatalf("empty safe wrapper logs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "stderr.log"), []byte("A22_ADMIN_PASSWORD=plaintext\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrapperLogs(directory); err == nil {
		t.Fatal("wrapper log containing a secret was accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "stderr.log"), []byte("step skipped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrapperLogs(directory); err == nil {
		t.Fatal("wrapper log containing a skip marker was accepted")
	}
}

func TestValidateRunDirectoryUsesFixedSecretFreeFileSet(t *testing.T) {
	t.Run("canonical", func(t *testing.T) {
		directory := writeCanonicalRunDirectory(t, DevelopmentClass)
		if err := ValidateRunDirectory(directory, DevelopmentClass); err != nil {
			t.Fatalf("canonical A22 wrapper run failed: %v", err)
		}
	})

	t.Run("extra file", func(t *testing.T) {
		directory := writeCanonicalRunDirectory(t, DevelopmentClass)
		if err := os.WriteFile(filepath.Join(directory, "unexpected.txt"), []byte("extra\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "fixed file set") {
			t.Fatalf("extra wrapper file was accepted: %v", err)
		}
	})

	t.Run("extra directory", func(t *testing.T) {
		directory := writeCanonicalRunDirectory(t, DevelopmentClass)
		if err := os.Mkdir(filepath.Join(directory, "unexpected-directory"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "regular non-link") {
			t.Fatalf("extra wrapper directory was accepted: %v", err)
		}
	})

	t.Run("required symlink", func(t *testing.T) {
		directory := writeCanonicalRunDirectory(t, DevelopmentClass)
		path := filepath.Join(directory, "stderr.log")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("stdout.log", path); err != nil {
			t.Skipf("symlinks unavailable on this platform: %v", err)
		}
		if err := ValidateRunDirectory(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "regular non-link") {
			t.Fatalf("wrapper symlink was accepted: %v", err)
		}
	})

	t.Run("secret in evidence metadata", func(t *testing.T) {
		directory := writeCanonicalRunDirectory(t, DevelopmentClass)
		mutateTestJSON[wrapperEvidence](t, directory, "evidence.json", func(value *wrapperEvidence) {
			value.WorkingDirectory = "DATABASE_DSN=admin:password@tcp(mysql:3306)/pilot_a22"
		})
		if err := ValidateRunDirectory(directory, DevelopmentClass); err == nil || !strings.Contains(err.Error(), "database DSN") {
			t.Fatalf("secret wrapper evidence was not rejected by the scanner: %v", err)
		}
	})
}

func writeCanonicalInnerEvidence(t *testing.T, class string) string {
	t.Helper()
	directory := t.TempDir()
	hash := strings.Repeat("a", 64)
	imageID := "sha256:" + hash
	writeTestJSON(t, filepath.Join(directory, "a22-command.json"), commandReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: class,
		Scope: "controlled_technical_drill", Command: append([]string(nil), canonicalCommand...),
		ToolCommands: append([]string(nil), canonicalToolCommands...),
	})

	sourceUUID := "11111111-1111-1111-1111-111111111111"
	targetUUID := "22222222-2222-2222-2222-222222222222"
	source := exactSnapshot("source", sourceUUID, shortFingerprint(sourceUUID))
	target := exactSnapshot("target", targetUUID, shortFingerprint(targetUUID))
	var environment environmentReport
	environment.SchemaVersion = 1
	environment.AcceptanceID = AcceptanceID
	environment.Status = "passed"
	environment.EvidenceClass = class
	environment.Commit = "unborn"
	environment.Images.Go = imageIdentity{Reference: "golang:1.25.1", ID: imageID, Digest: imageID}
	environment.Images.MySQL = imageIdentity{Reference: "mysql:8.4", ID: imageID, Digest: imageID}
	environment.Images.Tools = imageIdentity{Reference: "new-api-pilot-a22-tools:test", ID: imageID, Digest: imageID}
	environment.Network.Internal = true
	environment.Source = canonicalEnvironmentSide(source.ServerUUIDFingerprint, source.MySQLVersion)
	environment.Target = canonicalEnvironmentSide(target.ServerUUIDFingerprint, target.MySQLVersion)
	writeTestJSON(t, filepath.Join(directory, "a22-environment.json"), environment)

	writeTestJSON(t, filepath.Join(directory, "a22-fixture.json"), fixtureReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", FixtureID: "F05",
		Path: "testdata/design/f05-ops-capacity.yaml", SHA256: hash, RPOSeconds: 3600, RTOSeconds: 14400,
	})
	aggregateRows := make(map[string]int64, 12)
	for index := 0; index < 12; index++ {
		aggregateRows["aggregate_"+twoDigits(index)] = 1
	}
	writeTestJSON(t, filepath.Join(directory, "a22-seed.json"), seedReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", FixtureID: "F05", FixtureSHA256: hash,
		Database: "pilot_a22", FixedHourUnix: 1768662000, DateKey: 20260117, LastBusinessTime: 1768665599,
		SiteTokenEncrypted: true, SecretEncrypted: true,
		TaskStatuses:       map[string]int64{"pending": 1, "running": 1, "success": 1, "failed": 1},
		WindowStatuses:     map[string]int64{"running": 1, "success": 1, "failed": 1},
		CollectionStatuses: map[string]int64{"complete": 1}, AggregateRows: aggregateRows,
	})

	writeTestJSON(t, filepath.Join(directory, "a22-source-snapshot.json"), source)
	writeTestJSON(t, filepath.Join(directory, "a22-target-snapshot.json"), target)

	backupID := "backup-20260715T010203Z-deadbeef"
	backupCreatedUnix := source.LastBusinessTime + 65
	keyFingerprint := "cccccccccccc"
	writeTestJSON(t, filepath.Join(directory, "a22-backup.json"), backupReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", BackupID: backupID,
		CreatedAtUTC:   time.Unix(backupCreatedUnix, 0).UTC().Format(time.RFC3339),
		ManifestSHA256: hash, DumpSHA256: hash, DumpSizeBytes: 1024,
		KeyFingerprint: keyFingerprint, ImageDigest: environment.Images.Tools.Digest,
		SourceCoordinate: true, AtomicPublish: true,
	})
	writeTestJSON(t, filepath.Join(directory, "a22-negative-manifest.json"), negativeReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", Passed: true,
		FailureStage: "manifest_preflight", ExitCode: 1, SourceUnchanged: true,
	})
	writeTestJSON(t, filepath.Join(directory, "a22-negative-target-mismatch.json"), negativeReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", Passed: true,
		FailureStage: "target_identity", ExitCode: 1, SourceUnchanged: true,
	})
	writeTestJSON(t, filepath.Join(directory, "a22-restore.json"), restoreReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", BackupID: backupID,
		ReleaseGateExists: true, VerifyReport: "a22-verify-restore.json", ExitCode: 0,
	})
	verify := verifyReport{
		SchemaVersion: 1, Command: "verify-restore", Mode: "full", Status: "success",
		EncryptionKeyID: keyFingerprint, BackupID: backupID,
	}
	for _, name := range canonicalVerifyChecks {
		verify.Checks = append(verify.Checks, struct {
			Name    string         `json:"name"`
			Status  string         `json:"status"`
			Code    string         `json:"code,omitempty"`
			Details map[string]any `json:"details,omitempty"`
		}{Name: name, Status: "passed"})
	}
	verify.Summary.Passed = len(canonicalVerifyChecks)
	writeTestJSON(t, filepath.Join(directory, "a22-verify-restore.json"), verify)
	writeTestJSON(t, filepath.Join(directory, "a22-app-smoke.json"), appSmokeReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", HealthStatus: 200, ReadyStatus: 200,
		LoginStatus: 200, SelfStatus: 200, SitesStatus: 200, FixtureSiteFound: true, ConnectedToTarget: true,
		Database: "pilot_a22", SourceUUIDFingerprint: source.ServerUUIDFingerprint,
		TargetUUIDFingerprint: target.ServerUUIDFingerprint, ObservedUUIDFingerprint: target.ServerUUIDFingerprint,
		ReleaseGateRequired: true,
	})
	timing := timingReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", BackupCreatedAtUnix: backupCreatedUnix,
		LastBusinessTime: source.LastBusinessTime, RecoverableAge: 65, ActualDataLoss: 0, RPOSeconds: 65, RTOSeconds: 15,
		RPOLimitSeconds: 3600, RTOLimitSeconds: 14400, RPOPassed: true, RTOPassed: true,
	}
	writeTestJSON(t, filepath.Join(directory, "a22-rpo-rto.json"), timing)
	writeTestJSON(t, filepath.Join(directory, "a22-secret-scan.json"), scanReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", FilesScanned: len(requiredArtifacts),
	})
	var cleanup cleanupReport
	cleanup.SchemaVersion = 1
	cleanup.AcceptanceID = AcceptanceID
	cleanup.Status = "passed"
	cleanup.Passed = true
	cleanup.Lifecycle.Containers = "created_and_removed"
	cleanup.Lifecycle.Networks = "created_and_removed"
	cleanup.Lifecycle.Volumes = "created_and_removed"
	cleanup.Lifecycle.Images = "created_and_removed"
	writeTestJSON(t, filepath.Join(directory, "a22-cleanup.json"), cleanup)
	checks := make(map[string]bool, len(canonicalFinalChecks))
	for _, name := range canonicalFinalChecks {
		checks[name] = true
	}
	writeTestJSON(t, filepath.Join(directory, "a22-report.json"), finalReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", Passed: true, EvidenceClass: class,
		AcceptanceEligible: class == FormalClass, Scope: "controlled_technical_drill", RPOSeconds: 65, RTOSeconds: 15,
		Checks: checks, Violations: []string{},
	})
	for _, name := range []string{"a22-migration.log", "a22-backup.log", "a22-restore.log"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("A22 operation completed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	refreshInventory(t, directory, class)
	return directory
}

func writeCanonicalRunDirectory(t *testing.T, class string) string {
	t.Helper()
	directory := writeCanonicalInnerEvidence(t, class)
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("A22 wrapper completed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC)
	finished := started.Add(time.Second)
	writeTestJSON(t, filepath.Join(directory, "evidence.json"), wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: class,
		Command: append([]string(nil), canonicalCommand...), WorkingDirectory: ".",
		StartedAt: started.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano),
		DurationMilliseconds: 1000, ExitCode: 0, Commit: "unborn", WorktreeDirty: true,
		FixtureManifestPath: "testdata/design/manifest.sha256", FixtureManifestSHA: strings.Repeat("d", 64),
		StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	return directory
}

func canonicalEnvironmentSide(fingerprint, version string) environmentSide {
	return environmentSide{
		Database: "pilot_a22", UUIDFingerprint: fingerprint, Version: version,
		TransactionIsolation: "READ-COMMITTED", CharacterSetServer: "utf8mb4",
		CollationServer: "utf8mb4_unicode_ci", TimeZone: "+08:00", BinaryLoggingEnabled: true,
	}
}

func refreshInventory(t *testing.T, directory, class string) {
	t.Helper()
	entries := make([]artifactEntry, 0, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		payload, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		entries = append(entries, artifactEntry{Path: name, SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])})
	}
	writeTestJSON(t, filepath.Join(directory, "a22-artifacts.json"), artifactInventory{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, Files: entries,
	})
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, value); err != nil {
		t.Fatal(err)
	}
}

func mutateTestJSON[T any](t *testing.T, directory, name string, mutate func(*T)) {
	t.Helper()
	path := filepath.Join(directory, name)
	var value T
	readTestJSON(t, path, &value)
	mutate(&value)
	writeTestJSON(t, path, value)
}

func exactSnapshot(role, uuid, fingerprint string) snapshotReport {
	tables := make(map[string]int64, 30)
	for index := 0; index < 30; index++ {
		tables["table_"+twoDigits(index)] = 1
	}
	metric := func(daily bool) aggregateMetric {
		return aggregateMetric{Rows: 1, Requests: "3", Quota: "5", Tokens: "7", ActiveUsers: "1", DataStatus: "complete", IsFinal: daily}
	}
	aggregates := make(map[string]scopeAggregates, 6)
	for _, scope := range []string{"account", "customer", "site", "global", "model", "channel"} {
		aggregates[scope] = scopeAggregates{Hourly: metric(false), Daily: metric(true)}
	}
	return snapshotReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", Role: role, Database: "pilot_a22",
		ServerUUID: uuid, ServerUUIDFingerprint: fingerprint, MySQLVersion: "8.4.6",
		SnapshotSHA256: strings.Repeat("a", 64), TableCounts: tables,
		TaskStatuses:           map[string]int64{"pending": 1, "running": 1, "success": 1, "failed": 1},
		RunWindowStatuses:      map[string]int64{"running": 1, "success": 1, "failed": 1},
		CollectionWindowStates: map[string]int64{"complete": 1},
		ActiveKeys: map[string]int64{
			"collection_pending": 1, "collection_running": 1, "collection_terminal_with_key": 0,
			"export_active": 0, "alert_active": 0, "maintenance_active": 0,
		},
		Aggregates: aggregates, SiteTokenDecrypted: true, SecretSettingDecrypted: true, LastBusinessTime: 1_768_665_599,
	}
}

func twoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func bytesOf(length int, value byte) []byte {
	result := make([]byte, length)
	for index := range result {
		result[index] = value
	}
	return result
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
