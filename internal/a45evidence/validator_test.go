package a45evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyRequiresCanonicalA45Command(t *testing.T) {
	for _, test := range []struct {
		root  string
		class string
	}{
		{root: "artifacts/acceptance", class: FormalClass},
		{root: "artifacts/smoke", class: DevelopmentClass},
	} {
		class, err := Classify(AcceptanceID, test.root, canonicalCommand)
		if err != nil || class != test.class {
			t.Fatalf("A45 root=%q class=%q err=%v", test.root, class, err)
		}
	}
	if _, err := Classify(AcceptanceID, "artifacts/acceptance", []string{"powershell.exe", "-Command", "exit 0"}); err == nil {
		t.Fatal("arbitrary A45 exit-zero command was accepted")
	}
	if _, err := Classify(AcceptanceID, "artifacts/other", canonicalCommand); err == nil {
		t.Fatal("noncanonical A45 evidence root was accepted")
	}
	if class, err := Classify("A44", "artifacts/acceptance", []string{"anything"}); err != nil || class != "" {
		t.Fatalf("unrelated acceptance class=%q err=%v", class, err)
	}
}

func TestValidateRunDirectoryAcceptsFormalAndDevelopment(t *testing.T) {
	for _, class := range []string{FormalClass, DevelopmentClass} {
		t.Run(class, func(t *testing.T) {
			run := writeValidA45Run(t, class)
			if err := ValidateRunDirectory(run, class); err != nil {
				t.Fatalf("validate A45 %s run: %v", class, err)
			}
		})
	}
}

func TestValidateRunDirectoryRejectsFailCloseViolations(t *testing.T) {
	t.Run("artifact tamper", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		path := filepath.Join(run, "a45-report.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(run, FormalClass); err == nil ||
			(!strings.Contains(err.Error(), "size mismatch") && !strings.Contains(err.Error(), "SHA-256 mismatch")) {
			t.Fatalf("tampered A45 artifact error=%v", err)
		}
	})

	t.Run("skip event", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		path := filepath.Join(run, "a45-test.jsonl")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		event := goTestEvent{Action: "skip", Package: testPackage, Test: targetTest}
		encoded, _ := json.Marshal(event)
		payload = append(payload, encoded...)
		payload = append(payload, '\n')
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		var summary testSummary
		readA45JSON(t, filepath.Join(run, "a45-test-summary.json"), &summary)
		summary.SkipEvents = 1
		summary.JSONLines++
		writeA45JSON(t, filepath.Join(run, "a45-test-summary.json"), summary)
		writeA45Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "summary contract") {
			t.Fatalf("A45 skip event error=%v", err)
		}
	})

	t.Run("no tests", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		var summary testSummary
		readA45JSON(t, filepath.Join(run, "a45-test-summary.json"), &summary)
		summary.NoTests = true
		writeA45JSON(t, filepath.Join(run, "a45-test-summary.json"), summary)
		writeA45Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "summary contract") {
			t.Fatalf("A45 no-tests error=%v", err)
		}
	})

	t.Run("secret in raw log", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		if err := os.WriteFile(filepath.Join(run, "a45-test.stderr.log"), []byte("Authorization=Bearer-leaked-value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeA45Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "forbidden secret") {
			t.Fatalf("A45 secret error=%v", err)
		}
	})

	t.Run("extra file", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		if err := os.WriteFile(filepath.Join(run, "untracked-proof.txt"), []byte("extra"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "unexpected file") {
			t.Fatalf("A45 extra-file error=%v", err)
		}
	})

	t.Run("wrapper command", func(t *testing.T) {
		run := writeValidA45Run(t, FormalClass)
		var evidence wrapperEvidence
		readA45JSON(t, filepath.Join(run, "evidence.json"), &evidence)
		evidence.Command = append(evidence.Command, "-Unexpected")
		writeA45JSON(t, filepath.Join(run, "evidence.json"), evidence)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "wrapper evidence") {
			t.Fatalf("A45 wrapper command error=%v", err)
		}
	})
}

func writeValidA45Run(t *testing.T, class string) string {
	t.Helper()
	run := t.TempDir()
	events := []goTestEvent{
		{Action: "start", Package: testPackage},
		{Action: "run", Package: testPackage, Test: targetTest},
	}
	for _, subtest := range requiredSubtests {
		events = append(events,
			goTestEvent{Action: "run", Package: testPackage, Test: subtest},
			goTestEvent{Action: "pass", Package: testPackage, Test: subtest},
		)
	}
	events = append(events,
		goTestEvent{Action: "pass", Package: testPackage, Test: targetTest},
		goTestEvent{Action: "pass", Package: testPackage},
	)
	var stream strings.Builder
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		stream.Write(payload)
		stream.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(run, "a45-test.jsonl"), []byte(stream.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "a45-test.stderr.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeA45JSON(t, filepath.Join(run, "a45-test-summary.json"), testSummary{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", TargetTest: targetTest,
		Package: testPackage, PassEvents: 1, SubtestPassEvents: len(requiredSubtests), PackagePassEvents: 1,
		JSONLines: len(events), RequiredSubtests: requiredSubtests,
		JSONPath: "a45-test.jsonl", StderrPath: "a45-test.stderr.log",
	})
	writeA45JSON(t, filepath.Join(run, "a45-command.json"), commandReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class,
		TargetTest: targetTest, WorkingDir: "/workspace", Command: innerCommand, GoImage: "golang:1.25.1",
	})
	environment := environmentReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class,
		Commit: "unborn", WorktreeDirty: true,
		Image: imageIdentity{
			Reference: "golang:1.25.1", ID: "sha256:" + strings.Repeat("a", 64),
			Digest: "golang@sha256:" + strings.Repeat("b", 64),
		},
		RepositoryReadOnly: true, ContainerRootFSReadOnly: true, NoNewPrivileges: true,
		AllCapabilitiesDropped: true, OfflineTestNetwork: true, GoProxyOff: true, GoSumDBOff: true,
		EnvironmentProxiesCleared: true,
	}
	environment.Resources.Network = "new-api-pilot-a45-1-aaaaaaaaaaaa-network"
	environment.Resources.ModuleCache = "new-api-pilot-a45-1-aaaaaaaaaaaa-gomod"
	environment.Resources.BuildCache = "new-api-pilot-a45-1-aaaaaaaaaaaa-gobuild"
	environment.Network.Internal = true
	environment.Network.HostPorts = []string{}
	environment.Network.AttachedNetworks = []string{environment.Resources.Network}
	writeA45JSON(t, filepath.Join(run, "a45-environment.json"), environment)
	writeA45JSON(t, filepath.Join(run, "a45-fixture.json"), fixtureReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID,
		Fixtures: []fixtureEntry{
			{FixtureID: "F01", Path: fixtureF01Path, SHA256: fixtureF01SHA256},
			{FixtureID: "F02", Path: fixtureF02Path, SHA256: fixtureF02SHA256},
		},
		ManifestPath: fixtureManifest, ManifestSHA: strings.Repeat("c", 64),
	})
	report := finalReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", TargetTest: targetTest,
		RequiredSubtests: requiredSubtests,
	}
	report.Scenarios.OriginFailClosed = true
	report.Scenarios.TrustedProxyBoundary = true
	report.Scenarios.UpstreamDNSTLSCredentialBoundary = true
	report.Scenarios.SensitiveResponseAndLogs = true
	report.SecurityChecks.ForgedForwardedForRejected = true
	report.SecurityChecks.InvalidOriginRejected = true
	report.SecurityChecks.DNSRebindingRejected = true
	report.SecurityChecks.NonAllowlistedAddressRejected = true
	report.SecurityChecks.TLSDowngradeRedirectRejected = true
	report.SecurityChecks.OldTokenOriginGuarded = true
	report.SecurityChecks.EnvironmentProxyUnused = true
	report.SecurityChecks.SensitiveResponseAndLogsRedacted = true
	report.IsolationChecks.UniqueInternalNetwork = true
	report.IsolationChecks.NoHostPorts = true
	report.IsolationChecks.RepositoryReadOnly = true
	report.IsolationChecks.ContainerRootFSReadOnly = true
	report.IsolationChecks.EnvironmentProxiesCleared = true
	report.IsolationChecks.GoProxyOff = true
	writeA45JSON(t, filepath.Join(run, "a45-report.json"), report)
	cleanup := cleanupReport{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, Passed: true, SweepsSucceeded: true}
	cleanup.Lifecycle.Containers = "created_and_removed"
	cleanup.Lifecycle.Networks = "created_and_removed"
	cleanup.Lifecycle.Volumes = "created_and_removed"
	cleanup.Residuals.Containers = []string{}
	cleanup.Residuals.Networks = []string{}
	cleanup.Residuals.Volumes = []string{}
	cleanup.Residuals.Images = []string{}
	writeA45JSON(t, filepath.Join(run, "a45-cleanup.json"), cleanup)
	writeA45JSON(t, filepath.Join(run, "a45-secret-scan.json"), secretScanReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class,
		Status: "passed", Files: secretScanTargets,
	})
	writeA45Inventory(t, run, class)
	if err := os.WriteFile(filepath.Join(run, "stdout.log"), []byte("A45 test passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "stderr.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	writeA45JSON(t, filepath.Join(run, "evidence.json"), wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: class,
		Command: canonicalCommand, WorkingDirectory: ".", StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: started.Add(time.Second).Format(time.RFC3339Nano), DurationMilliseconds: 1000,
		ExitCode: 0, Commit: "unborn", WorktreeDirty: true,
		FixtureManifestPath: fixtureManifest, FixtureManifestSHA: strings.Repeat("c", 64),
		StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	return run
}

func writeA45Inventory(t *testing.T, run, class string) {
	t.Helper()
	inventory := artifactInventory{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class}
	for _, name := range requiredArtifacts {
		path := filepath.Join(run, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		inventory.Files = append(inventory.Files, artifactEntry{Path: name, SizeBytes: info.Size(), SHA256: digest})
	}
	writeA45JSON(t, filepath.Join(run, "a45-artifacts.json"), inventory)
}

func writeA45JSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readA45JSON(t *testing.T, path string, target any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatal(err)
	}
}
