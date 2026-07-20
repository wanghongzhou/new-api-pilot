package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"new-api-pilot/internal/ops"
)

func TestParseReencryptCLI(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      reencryptCLIOptions
		wantError bool
	}{
		{name: "defaults", args: []string{"reencrypt"}, want: reencryptCLIOptions{BatchSize: 100}},
		{name: "all flags", args: []string{"reencrypt", "--dry-run", "--batch-size=250"}, want: reencryptCLIOptions{DryRun: true, BatchSize: 250}},
		{name: "missing subcommand", wantError: true},
		{name: "wrong subcommand", args: []string{"rotate"}, wantError: true},
		{name: "zero batch", args: []string{"reencrypt", "--batch-size=0"}, wantError: true},
		{name: "oversized batch", args: []string{"reencrypt", "--batch-size=1001"}, wantError: true},
		{name: "unknown argument", args: []string{"reencrypt", "extra"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseReencryptCLI(test.args)
			if (err != nil) != test.wantError {
				t.Fatalf("parseReencryptCLI() error = %v", err)
			}
			if !test.wantError && got != test.want {
				t.Fatalf("parseReencryptCLI() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseVerifyRestoreCLI(t *testing.T) {
	valid := []string{"--mode=full", "--manifest=/var/backups/manifest.json"}
	options, err := parseVerifyRestoreCLI(valid)
	if err != nil || options.ManifestPath != "/var/backups/manifest.json" {
		t.Fatalf("parseVerifyRestoreCLI() = %#v, %v", options, err)
	}
	for _, args := range [][]string{
		{"--manifest=/var/backups/manifest.json"},
		{"--mode=quick", "--manifest=/var/backups/manifest.json"},
		{"--mode=full"},
		{"--mode=full", "--manifest=relative/manifest.json"},
		{"--mode=full", "--manifest=/var/backups/backup.json"},
		{"--mode=full", "--manifest=/var/backups/manifest.json", "extra"},
	} {
		if _, err := parseVerifyRestoreCLI(args); err == nil {
			t.Fatalf("parseVerifyRestoreCLI(%v) unexpectedly succeeded", args)
		}
	}
}

func TestParseManifestPreflightCLI(t *testing.T) {
	valid := []string{"--mode=manifest-only", "--manifest=/var/backups/manifest.json"}
	options, err := parseManifestPreflightCLI(valid)
	if err != nil || options.ManifestPath != "/var/backups/manifest.json" {
		t.Fatalf("parseManifestPreflightCLI() = %#v, %v", options, err)
	}
	for _, args := range [][]string{
		{"--manifest=/var/backups/manifest.json"},
		{"--mode=full", "--manifest=/var/backups/manifest.json"},
		{"--mode=manifest-only"},
		{"--mode=manifest-only", "--manifest=relative/manifest.json"},
		{"--mode=manifest-only", "--manifest=/var/backups/backup.json"},
		{"--mode=manifest-only", "--manifest=/var/backups/manifest.json", "extra"},
	} {
		if _, err := parseManifestPreflightCLI(args); err == nil {
			t.Fatalf("parseManifestPreflightCLI(%v) unexpectedly succeeded", args)
		}
	}
}

func TestMaintenanceArgumentErrorsAreJSONAndExitTwo(t *testing.T) {
	tests := []struct {
		args []string
		code string
	}{
		{args: []string{"secrets", "rotate"}, code: "REENCRYPT_ARGUMENT_INVALID"},
		{args: []string{"verify-restore", "--mode=quick"}, code: "VERIFY_ARGUMENT_INVALID"},
		{args: []string{"verify-backup", "--mode=full"}, code: "MANIFEST_ARGUMENT_INVALID"},
		{args: []string{"database-identity", "extra"}, code: "DATABASE_IDENTITY_ARGUMENT_INVALID"},
	}
	for _, test := range tests {
		var output bytes.Buffer
		if exitCode := runMaintenanceCommand(test.args, &output); exitCode != exitArgumentError {
			t.Fatalf("runMaintenanceCommand(%v) exit = %d", test.args, exitCode)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatalf("decode JSON output %q: %v", output.String(), err)
		}
		errorPayload, ok := payload["error"].(map[string]any)
		if !ok || errorPayload["code"] != test.code {
			t.Fatalf("error payload = %#v, want code %s", errorPayload, test.code)
		}
	}
}

func TestDatabaseIdentityIsRecognizedAsMaintenanceCommand(t *testing.T) {
	if !isMaintenanceCommand([]string{"database-identity"}) {
		t.Fatal("database-identity was not recognized as a maintenance command")
	}
	if isMaintenanceCommand([]string{"database-identity-extra"}) {
		t.Fatal("unexpected database identity command was recognized")
	}
}

func TestMaintenanceFailureReportsTruncateFingerprints(t *testing.T) {
	var output bytes.Buffer
	writeReencryptFailure(
		&output, "REENCRYPT_DATABASE_ERROR", true,
		"0123456789abcdef", "fedcba9876543210",
	)
	var report ops.ReencryptReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.OldKeyID != "0123456789ab" || report.NewKeyID != "fedcba987654" {
		t.Fatalf("fingerprints were not truncated: %#v", report)
	}
}
