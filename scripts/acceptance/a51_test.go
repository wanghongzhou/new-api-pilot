package main

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"new-api-pilot/config"
	"new-api-pilot/internal/ops"
)

type a51FinalInputs struct {
	seed        a51SeedReport
	dry         ops.ReencryptReport
	full        ops.ReencryptReport
	post        ops.ReencryptReport
	verify      a51VerifyReport
	integration a51IntegrationSummary
	scan        a51ScanReport
	environment a51EnvironmentReport
}

func TestValidateA51PreflightRequiresEveryAcceptanceVariable(t *testing.T) {
	values := validA51PreflightEnvironment()
	if err := validateA51Preflight(a51MapLookup(values)); err != nil {
		t.Fatalf("valid A51 preflight environment was rejected: %v", err)
	}

	for _, name := range a51RequiredPreflightVariables {
		t.Run(name, func(t *testing.T) {
			incomplete := make(map[string]string, len(values)-1)
			for key, value := range values {
				incomplete[key] = value
			}
			delete(incomplete, name)
			err := validateA51Preflight(a51MapLookup(incomplete))
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("missing %s error=%v", name, err)
			}
		})
	}

}

func TestA51RunnerPreflightsBeforeAnyDockerResource(t *testing.T) {
	payload, err := os.ReadFile("run-a51.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	tryStart := strings.Index(text, "try {")
	if tryStart < 0 {
		t.Fatal("A51 runner try block is missing")
	}
	preflight := strings.Index(text[tryStart:], "Invoke-A51ConfigurationPreflight -Environment")
	resourceRegistration := strings.Index(text[tryStart:], "$networkName = \"new-api-pilot-a51-$runToken-network\"")
	firstDocker := strings.Index(text[tryStart:], "Invoke-OpsDocker -Arguments @('version'")
	if preflight < 0 || resourceRegistration < 0 || firstDocker < 0 || preflight >= resourceRegistration ||
		resourceRegistration >= firstDocker {
		t.Fatalf("A51 preflight/resource order is invalid: preflight=%d resource=%d docker=%d", preflight, resourceRegistration, firstDocker)
	}
	for _, required := range []string{
		"a51-preflight.log",
		"'not_created'",
		"created_and_removed",
		"$networkCreated",
		"$mysqlVolumeCreated",
		"$createdContainers",
		"'stdout.log', 'stderr.log'",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A51 runner is missing preflight contract %q", required)
		}
	}
	common, err := os.ReadFile("ops-runner-common.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(common), "EnvironmentVariables") {
		t.Fatal("native process helper cannot pass an isolated preflight environment")
	}
	a51Source, err := os.ReadFile("a51.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(a51Source), `_ "time/tzdata"`) {
		t.Fatal("A51 Windows preflight binary must embed the Asia/Shanghai time-zone database")
	}
}

func TestA51WrapperClosesLogsBeforeSecretAndInnerValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join(stdoutFile.Sync(), stderrFile.Sync(), stdoutFile.Close(), stderrFile.Close())")
	scanLogs := strings.Index(text, "opsevidence.ValidateWrapperLogs(runDirectory, *acceptanceID)")
	innerValidation := strings.Index(text, "opsevidence.ValidateInnerArtifacts(runDirectory, *acceptanceID, opsEvidenceClass)")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("wrapper log lifecycle order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestBuildA51FinalReportAcceptsCompleteFormalEvidence(t *testing.T) {
	inputs := validA51FinalInputs()
	report := buildA51FinalReport(
		inputs.seed,
		inputs.dry,
		inputs.full,
		inputs.post,
		inputs.verify,
		inputs.integration,
		inputs.scan,
		inputs.environment,
	)

	if !report.Passed || report.Status != "passed" || !report.AcceptanceEligible {
		t.Fatalf("complete A51 evidence was rejected: %#v", report)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("complete A51 evidence produced violations: %v", report.Violations)
	}
}

func TestBuildA51FinalReportRejectsEachContractViolation(t *testing.T) {
	tests := []struct {
		name      string
		violation string
		mutate    func(*a51FinalInputs)
	}{
		{
			name:      "seed",
			violation: "seed_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.seed.SiteTokensOld++
			},
		},
		{
			name:      "dry run",
			violation: "dry_run_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.dry.Counts.Updated++
			},
		},
		{
			name:      "full run",
			violation: "full_run_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.full.Status = "failed"
			},
		},
		{
			name:      "post dry run",
			violation: "post_dry_run_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.post.Counts.OldKey++
			},
		},
		{
			name:      "key identity",
			violation: "key_identity_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.dry.OldKeyID = "cccccccccccc"
			},
		},
		{
			name:      "verification",
			violation: "verification_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.verify.ActiveMaintenanceJobs++
			},
		},
		{
			name:      "completed history boundary",
			violation: "verification_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.verify.CompletedHistoryJobs--
			},
		},
		{
			name:      "fault injection",
			violation: "fault_injection_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.integration.TestsSkipped++
			},
		},
		{
			name:      "secret scan",
			violation: "secret_scan_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.scan.ForbiddenHits++
			},
		},
		{
			name:      "environment",
			violation: "environment_contract",
			mutate: func(inputs *a51FinalInputs) {
				inputs.environment.Network.Internal = false
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := validA51FinalInputs()
			test.mutate(&inputs)
			report := buildA51FinalReport(
				inputs.seed,
				inputs.dry,
				inputs.full,
				inputs.post,
				inputs.verify,
				inputs.integration,
				inputs.scan,
				inputs.environment,
			)
			if report.Passed || report.Status != "failed" {
				t.Fatalf("invalid A51 evidence passed: %#v", report)
			}
			if !containsA51Violation(report.Violations, test.violation) {
				t.Fatalf("violations=%v, want %q", report.Violations, test.violation)
			}
		})
	}
}

func validA51FinalInputs() a51FinalInputs {
	const (
		fixtureSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		oldKeyID   = "111111111111"
		newKeyID   = "222222222222"
	)
	newReencryptReport := func(dryRun bool, oldCount, newCount, updated int64) ops.ReencryptReport {
		return ops.ReencryptReport{
			SchemaVersion: ops.ReportSchemaVersion,
			Command:       "secrets reencrypt",
			Status:        "success",
			DryRun:        dryRun,
			OldKeyID:      oldKeyID,
			NewKeyID:      newKeyID,
			Counts: ops.ReencryptCounts{
				Total:      4,
				SiteTokens: 2,
				Settings:   2,
				OldKey:     oldCount,
				NewKey:     newCount,
				Updated:    updated,
			},
		}
	}
	inputs := a51FinalInputs{
		seed: a51SeedReport{
			SchemaVersion:           evidenceSchemaVersion,
			AcceptanceID:            a51AcceptanceID,
			Status:                  "passed",
			EvidenceClass:           "formal",
			FixtureSHA256:           fixtureSHA,
			OldKeyID:                oldKeyID,
			NewKeyID:                newKeyID,
			SiteTokensOld:           2,
			SecretSettingsOld:       1,
			SecretSettingsNew:       1,
			HistoricalCompletedJobs: 1,
			CiphertextSHA256:        []string{"1", "2", "3", "4"},
		},
		dry:  newReencryptReport(true, 3, 1, 0),
		full: newReencryptReport(false, 3, 1, 3),
		post: newReencryptReport(true, 0, 4, 0),
		verify: a51VerifyReport{
			SchemaVersion:        evidenceSchemaVersion,
			AcceptanceID:         a51AcceptanceID,
			Status:               "passed",
			EvidenceClass:        "formal",
			OldKeyID:             oldKeyID,
			NewKeyID:             newKeyID,
			VerifiedSiteTokens:   2,
			VerifiedSettings:     2,
			CurrentCompletedJobs: 1,
			CompletedHistoryJobs: 2,
		},
		integration: a51IntegrationSummary{
			SchemaVersion: evidenceSchemaVersion,
			AcceptanceID:  a51AcceptanceID,
			Status:        "passed",
			TestsPassed:   4,
			TestNames: []string{
				"TestMySQLReencryptCASFailureRollsBackAllUpdates",
				"TestMySQLReencryptDryRunAndSuccess",
				"TestMySQLReencryptRejectsBadCiphertextWithoutWrites",
				"TestMySQLReencryptResumesAndRejectsDifferentKeyPair",
			},
			LogPath: "a51-integration-tests.jsonl",
		},
		scan: a51ScanReport{
			SchemaVersion: evidenceSchemaVersion,
			AcceptanceID:  a51AcceptanceID,
			Status:        "passed",
			FilesScanned:  1,
		},
		environment: a51EnvironmentReport{
			SchemaVersion: evidenceSchemaVersion,
			AcceptanceID:  a51AcceptanceID,
			EvidenceClass: "formal",
			Commit:        "0123456789abcdef",
			Databases:     []string{"pilot_a51", "pilot_a51_tests"},
		},
	}
	inputs.environment.MySQL.Version = "8.4.6"
	inputs.environment.MySQL.TransactionIsolation = "READ-COMMITTED"
	inputs.environment.MySQL.CharacterSetServer = "utf8mb4"
	inputs.environment.MySQL.CollationServer = "utf8mb4_unicode_ci"
	inputs.environment.MySQL.TimeZone = "+08:00"
	inputs.environment.Network.Internal = true
	inputs.environment.Network.HostPorts = []int{}
	return inputs
}

func validA51PreflightEnvironment() map[string]string {
	encode := func(value string) string {
		return base64.StdEncoding.EncodeToString([]byte(value))
	}
	oldKey := encode(strings.Repeat("o", 32))
	return map[string]string{
		"APP_ENV":                  "test",
		"PORT":                     "3000",
		"DATABASE_DSN":             "root:@tcp(mysql-a51:3306)/pilot_a51?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai",
		"SQL_MAX_IDLE_CONNS":       "2",
		"SQL_MAX_OPEN_CONNS":       "4",
		"SQL_MAX_LIFETIME_SECONDS": "60",
		"SESSION_SECRET":           encode(strings.Repeat("s", 32)),
		"ENCRYPTION_KEY":           oldKey,
		"SESSION_COOKIE_SECURE":    "false",
		"EXPORT_DIR":               "/tmp/a51-exports",
		"PUBLIC_ORIGIN":            "http://a51.invalid",
		"TRUSTED_PROXIES":          "",
		"UPSTREAM_CA_FILE":         "",
		"METRICS_ALLOWED_CIDRS":    "127.0.0.0/8",
		"DINGTALK_ALLOWED_HOSTS":   "",
		"OLD_ENCRYPTION_KEY":       oldKey,
		"NEW_ENCRYPTION_KEY":       encode(strings.Repeat("n", 32)),
	}
}

func a51MapLookup(values map[string]string) config.LookupFunc {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}

func containsA51Violation(violations []string, wanted string) bool {
	for _, violation := range violations {
		if violation == wanted {
			return true
		}
	}
	return false
}
