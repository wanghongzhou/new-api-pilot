package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"new-api-pilot/internal/a22evidence"
)

var backupIDPattern = regexp.MustCompile(`^backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8,64}$`)

type commandArtifact struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	Status        string   `json:"status"`
	EvidenceClass string   `json:"evidence_class"`
	Scope         string   `json:"scope"`
	Command       []string `json:"command"`
	ToolCommands  []string `json:"tool_commands"`
}

type environmentArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	EvidenceClass string `json:"evidence_class"`
	Commit        string `json:"commit"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	Images        struct {
		Go    imageArtifact `json:"go"`
		MySQL imageArtifact `json:"mysql"`
		Tools imageArtifact `json:"tools"`
	} `json:"images"`
	Network struct {
		Internal  bool     `json:"internal"`
		HostPorts []string `json:"host_ports"`
	} `json:"network"`
	Source struct {
		Database             string `json:"database"`
		UUIDFingerprint      string `json:"server_uuid_fingerprint"`
		Version              string `json:"version"`
		TransactionIsolation string `json:"transaction_isolation"`
		CharacterSetServer   string `json:"character_set_server"`
		CollationServer      string `json:"collation_server"`
		TimeZone             string `json:"time_zone"`
		BinaryLoggingEnabled bool   `json:"binary_logging_enabled"`
	} `json:"source"`
	Target struct {
		Database             string `json:"database"`
		UUIDFingerprint      string `json:"server_uuid_fingerprint"`
		Version              string `json:"version"`
		TransactionIsolation string `json:"transaction_isolation"`
		CharacterSetServer   string `json:"character_set_server"`
		CollationServer      string `json:"collation_server"`
		TimeZone             string `json:"time_zone"`
		BinaryLoggingEnabled bool   `json:"binary_logging_enabled"`
	} `json:"target"`
	ProductionReleaseAuthorized bool `json:"production_release_authorized"`
}

type imageArtifact struct {
	Reference string `json:"reference"`
	ID        string `json:"id"`
	Digest    string `json:"digest"`
}

type fixtureArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	FixtureID     string `json:"fixture_id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	RPOSeconds    int64  `json:"rpo_seconds"`
	RTOSeconds    int64  `json:"rto_seconds"`
}

type backupArtifact struct {
	SchemaVersion    int    `json:"schema_version"`
	AcceptanceID     string `json:"acceptance_id"`
	Status           string `json:"status"`
	BackupID         string `json:"backup_id"`
	CreatedAtUTC     string `json:"created_at_utc"`
	ManifestSHA256   string `json:"manifest_sha256"`
	DumpSHA256       string `json:"dump_sha256"`
	DumpSizeBytes    int64  `json:"dump_size_bytes"`
	KeyFingerprint   string `json:"encryption_key_fingerprint"`
	ImageDigest      string `json:"image_digest"`
	SourceCoordinate bool   `json:"source_coordinate_present"`
	AtomicPublish    bool   `json:"atomic_publish"`
}

type negativeArtifact struct {
	SchemaVersion     int    `json:"schema_version"`
	AcceptanceID      string `json:"acceptance_id"`
	Status            string `json:"status"`
	Passed            bool   `json:"passed"`
	FailureStage      string `json:"failure_stage"`
	ExitCode          int    `json:"exit_code"`
	ImportStarted     bool   `json:"import_started"`
	TargetTableCount  int64  `json:"target_table_count"`
	ReleaseGateExists bool   `json:"release_gate_exists"`
	SourceUnchanged   bool   `json:"source_unchanged"`
	ProductionGate    bool   `json:"production_release_authorized"`
}

type restoreArtifact struct {
	SchemaVersion     int    `json:"schema_version"`
	AcceptanceID      string `json:"acceptance_id"`
	Status            string `json:"status"`
	BackupID          string `json:"backup_id"`
	ReleaseGateExists bool   `json:"release_gate_exists"`
	VerifyReport      string `json:"verify_report"`
	ExitCode          int    `json:"exit_code"`
}

type verifyArtifact struct {
	SchemaVersion   int    `json:"schema_version"`
	Command         string `json:"command"`
	Mode            string `json:"mode"`
	Status          string `json:"status"`
	EncryptionKeyID string `json:"encryption_key_id"`
	BackupID        string `json:"backup_id,omitempty"`
	Checks          []struct {
		Name    string         `json:"name"`
		Status  string         `json:"status"`
		Code    string         `json:"code,omitempty"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"checks"`
	Summary struct {
		Passed int `json:"passed"`
		Failed int `json:"failed"`
	} `json:"summary"`
	Error *struct {
		Code    string `json:"code"`
		RowType string `json:"row_type,omitempty"`
		RowID   string `json:"row_id,omitempty"`
	} `json:"error,omitempty"`
}

type appSmokeArtifact struct {
	SchemaVersion           int    `json:"schema_version"`
	AcceptanceID            string `json:"acceptance_id"`
	Status                  string `json:"status"`
	HealthStatus            int    `json:"health_status"`
	ReadyStatus             int    `json:"ready_status"`
	LoginStatus             int    `json:"login_status"`
	SelfStatus              int    `json:"self_status"`
	SitesStatus             int    `json:"sites_status"`
	FixtureSiteFound        bool   `json:"fixture_site_found"`
	ConnectedToTarget       bool   `json:"connected_to_target"`
	Database                string `json:"database"`
	SourceUUIDFingerprint   string `json:"source_uuid_fingerprint"`
	TargetUUIDFingerprint   string `json:"target_uuid_fingerprint"`
	ObservedUUIDFingerprint string `json:"observed_uuid_fingerprint"`
	ReleaseGateRequired     bool   `json:"release_gate_required"`
	ProductionGate          bool   `json:"production_release_authorized"`
}

type secretScanArtifact struct {
	SchemaVersion      int    `json:"schema_version"`
	AcceptanceID       string `json:"acceptance_id"`
	Status             string `json:"status"`
	FilesScanned       int    `json:"files_scanned"`
	ForbiddenHits      int    `json:"forbidden_hits"`
	DSNLeaks           int    `json:"dsn_leaks"`
	KeyLeaks           int    `json:"key_leaks"`
	URLCredentialLeaks int    `json:"url_credential_leaks"`
}

type cleanupArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	Passed        bool   `json:"passed"`
	Lifecycle     struct {
		Containers string `json:"containers"`
		Networks   string `json:"networks"`
		Volumes    string `json:"volumes"`
		Images     string `json:"images"`
	} `json:"lifecycle"`
	Residuals struct {
		Containers []string `json:"containers"`
		Networks   []string `json:"networks"`
		Volumes    []string `json:"volumes"`
		Images     []string `json:"images"`
	} `json:"residuals"`
}

func runReport(arguments []string) error {
	directory := ""
	evidenceClass := "development"
	output := ""
	_, err := parseNoPositionals("report", arguments, func(flags *flag.FlagSet) {
		flags.StringVar(&directory, "evidence-dir", directory, "A22 evidence directory")
		flags.StringVar(&evidenceClass, "evidence-class", evidenceClass, "development or formal")
		flags.StringVar(&output, "output", output, "final report path")
	})
	if err != nil {
		return err
	}
	if directory == "" || (evidenceClass != "development" && evidenceClass != "formal") {
		return errors.New("report requires an evidence directory and a supported evidence class")
	}
	if output == "" {
		output = filepath.Join(directory, "a22-report.json")
	}
	report, err := buildFinalReport(directory, evidenceClass)
	if err != nil {
		return err
	}
	if err := writeJSON(output, report); err != nil {
		return err
	}
	if !report.Passed {
		return fmt.Errorf("A22 report failed: %v", report.Violations)
	}
	return nil
}

func buildFinalReport(directory, evidenceClass string) (finalReport, error) {
	checks := make(map[string]bool)
	violations := make([]string, 0)
	check := func(name string, passed bool) {
		checks[name] = passed
		if !passed {
			violations = append(violations, name)
		}
	}
	var command commandArtifact
	if err := readJSON(filepath.Join(directory, "a22-command.json"), &command); err != nil {
		return finalReport{}, err
	}
	check("command", command.SchemaVersion == 1 && command.AcceptanceID == acceptanceID && command.Status == "passed" &&
		command.EvidenceClass == evidenceClass && command.Scope == "controlled_technical_drill")
	var environment environmentArtifact
	if err := readJSON(filepath.Join(directory, "a22-environment.json"), &environment); err != nil {
		return finalReport{}, err
	}
	check("isolated_environment", validEnvironment(environment, evidenceClass))
	var fixture fixtureArtifact
	if err := readJSON(filepath.Join(directory, "a22-fixture.json"), &fixture); err != nil {
		return finalReport{}, err
	}
	check("fixture", fixture.SchemaVersion == 1 && fixture.AcceptanceID == acceptanceID && fixture.Status == "passed" &&
		fixture.FixtureID == fixtureID && fixture.Path == defaultFixturePath && len(fixture.SHA256) == 64 &&
		fixture.RPOSeconds == 3600 && fixture.RTOSeconds == 14400)
	var seed seedReport
	if err := readJSON(filepath.Join(directory, "a22-seed.json"), &seed); err != nil {
		return finalReport{}, err
	}
	check("representative_fixture", validSeed(seed, fixture.SHA256))
	var source, target snapshotReport
	if err := readJSON(filepath.Join(directory, "a22-source-snapshot.json"), &source); err != nil {
		return finalReport{}, err
	}
	if err := readJSON(filepath.Join(directory, "a22-target-snapshot.json"), &target); err != nil {
		return finalReport{}, err
	}
	check("source_snapshot", validSnapshot(source, "source"))
	check("target_snapshot", validSnapshot(target, "target"))
	check("exact_restore", source.SnapshotSHA256 != "" && source.SnapshotSHA256 == target.SnapshotSHA256 &&
		source.Database == target.Database && source.ServerUUID != target.ServerUUID && source.LastBusinessTime == target.LastBusinessTime)
	check("task_window_active_key_state", equalIntMaps(source.TaskStatuses, target.TaskStatuses) &&
		equalIntMaps(source.RunWindowStatuses, target.RunWindowStatuses) &&
		equalIntMaps(source.CollectionWindowStates, target.CollectionWindowStates) && equalIntMaps(source.ActiveKeys, target.ActiveKeys))
	check("six_level_aggregates", equalAggregates(source.Aggregates, target.Aggregates))
	var backup backupArtifact
	if err := readJSON(filepath.Join(directory, "a22-backup.json"), &backup); err != nil {
		return finalReport{}, err
	}
	check("backup", backup.SchemaVersion == 1 && backup.AcceptanceID == acceptanceID && backup.Status == "passed" &&
		backupIDPattern.MatchString(backup.BackupID) && len(backup.ManifestSHA256) == 64 && len(backup.DumpSHA256) == 64 &&
		backup.DumpSizeBytes > 0 && len(backup.KeyFingerprint) == 12 && validImageID(backup.ImageDigest) &&
		backup.SourceCoordinate && backup.AtomicPublish)
	for name, expectedStage := range map[string]string{
		"negative_manifest": "manifest_preflight", "negative_target_mismatch": "target_identity",
	} {
		fileName := "a22-negative-manifest.json"
		if name == "negative_target_mismatch" {
			fileName = "a22-negative-target-mismatch.json"
		}
		var negative negativeArtifact
		if err := readJSON(filepath.Join(directory, fileName), &negative); err != nil {
			return finalReport{}, err
		}
		check(name, negative.SchemaVersion == 1 && negative.AcceptanceID == acceptanceID && negative.Status == "passed" &&
			negative.Passed && negative.FailureStage == expectedStage && negative.ExitCode != 0 && !negative.ImportStarted &&
			negative.TargetTableCount == 0 && !negative.ReleaseGateExists && negative.SourceUnchanged && !negative.ProductionGate)
	}
	var restore restoreArtifact
	if err := readJSON(filepath.Join(directory, "a22-restore.json"), &restore); err != nil {
		return finalReport{}, err
	}
	check("restore_gate", restore.SchemaVersion == 1 && restore.AcceptanceID == acceptanceID && restore.Status == "passed" &&
		restore.BackupID == backup.BackupID && restore.ReleaseGateExists && restore.VerifyReport == "a22-verify-restore.json" && restore.ExitCode == 0)
	var verify verifyArtifact
	if err := readJSON(filepath.Join(directory, "a22-verify-restore.json"), &verify); err != nil {
		return finalReport{}, err
	}
	check("full_verify", verify.SchemaVersion == 1 && verify.Command == "verify-restore" && verify.Mode == "full" &&
		verify.Status == "success" && verify.Summary.Passed >= 10 && verify.Summary.Failed == 0)
	var smoke appSmokeArtifact
	if err := readJSON(filepath.Join(directory, "a22-app-smoke.json"), &smoke); err != nil {
		return finalReport{}, err
	}
	check("restored_app_smoke", validAppSmoke(smoke, source, target))
	var timing rpoRTOReport
	if err := readJSON(filepath.Join(directory, "a22-rpo-rto.json"), &timing); err != nil {
		return finalReport{}, err
	}
	check("rpo_rto", validTiming(timing, source, target))
	var scan secretScanArtifact
	if err := readJSON(filepath.Join(directory, "a22-secret-scan.json"), &scan); err != nil {
		return finalReport{}, err
	}
	check("secret_scan", scan.SchemaVersion == 1 && scan.AcceptanceID == acceptanceID && scan.Status == "passed" &&
		scan.FilesScanned > 0 && scan.ForbiddenHits == 0 && scan.DSNLeaks == 0 && scan.KeyLeaks == 0 && scan.URLCredentialLeaks == 0)
	var cleanup cleanupArtifact
	if err := readJSON(filepath.Join(directory, "a22-cleanup.json"), &cleanup); err != nil {
		return finalReport{}, err
	}
	check("cleanup", validCleanup(cleanup))
	sort.Strings(violations)
	passed := len(violations) == 0
	status := "failed"
	if passed {
		status = "passed"
	}
	return finalReport{
		SchemaVersion: 1, AcceptanceID: acceptanceID, Status: status, Passed: passed,
		EvidenceClass: evidenceClass, AcceptanceEligible: evidenceClass == "formal",
		Scope: "controlled_technical_drill", ProductionReleaseAuthorized: false,
		RPOSeconds: timing.RPOSeconds, RTOSeconds: timing.RTOSeconds, Checks: checks, Violations: violations,
	}, nil
}

func validEnvironment(value environmentArtifact, class string) bool {
	validSide := func(database, fingerprint, version, isolation, charset, collation, timezone string, binlog bool) bool {
		return database == databaseName && len(fingerprint) == 12 && version != "" && isolation == "READ-COMMITTED" &&
			charset == "utf8mb4" && collation == "utf8mb4_unicode_ci" && timezone == "+08:00" && binlog
	}
	validImage := func(value imageArtifact, reference string, local bool) bool {
		if value.Reference != reference && (!local || value.Reference == "") {
			return false
		}
		return validImageID(value.ID) && (validImageID(value.Digest) ||
			regexp.MustCompile(`^[^@]+@sha256:[0-9a-f]{64}$`).MatchString(value.Digest))
	}
	return value.SchemaVersion == 1 && value.AcceptanceID == acceptanceID && value.Status == "passed" &&
		value.EvidenceClass == class && value.Network.Internal && len(value.Network.HostPorts) == 0 &&
		(value.Commit == "unborn" || regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(value.Commit)) &&
		validImage(value.Images.Go, "golang:1.25.1", false) && validImage(value.Images.MySQL, "mysql:8.4", false) &&
		validImage(value.Images.Tools, value.Images.Tools.Reference, true) &&
		validSide(value.Source.Database, value.Source.UUIDFingerprint, value.Source.Version,
			value.Source.TransactionIsolation, value.Source.CharacterSetServer, value.Source.CollationServer,
			value.Source.TimeZone, value.Source.BinaryLoggingEnabled) &&
		validSide(value.Target.Database, value.Target.UUIDFingerprint, value.Target.Version,
			value.Target.TransactionIsolation, value.Target.CharacterSetServer, value.Target.CollationServer,
			value.Target.TimeZone, value.Target.BinaryLoggingEnabled) &&
		value.Source.UUIDFingerprint != value.Target.UUIDFingerprint && !value.ProductionReleaseAuthorized
}

func validImageID(value string) bool {
	return regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(value)
}

func validSeed(value seedReport, fixtureSHA string) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == acceptanceID && value.Status == "passed" &&
		value.FixtureID == fixtureID && value.FixtureSHA256 == fixtureSHA && value.Database == databaseName &&
		value.FixedHourUnix > 0 && value.FixedHourUnix%3600 == 0 && value.DateKey > 20000101 &&
		value.LastBusinessTime > 0 && value.SiteTokenEncrypted && value.SecretEncrypted &&
		value.TaskStatuses["pending"] == 1 && value.TaskStatuses["running"] == 1 &&
		value.TaskStatuses["success"] == 1 && value.TaskStatuses["failed"] == 1 &&
		value.WindowStatuses["running"] == 1 && value.WindowStatuses["success"] == 1 &&
		value.WindowStatuses["failed"] == 1 && value.CollectionStatuses["complete"] == 1 &&
		len(value.AggregateRows) == 12 && !value.ProductionReleaseOK
}

func validSnapshot(value snapshotReport, role string) bool {
	if value.SchemaVersion != 1 || value.AcceptanceID != acceptanceID || value.Status != "passed" || value.Role != role ||
		value.Database != databaseName || !serverUUIDPattern.MatchString(value.ServerUUID) ||
		len(value.ServerUUIDFingerprint) != 12 || len(value.SnapshotSHA256) != 64 || len(value.TableCounts) < 30 ||
		!value.SiteTokenDecrypted || !value.SecretSettingDecrypted || value.LastBusinessTime <= 0 || value.ProductionReleaseOK {
		return false
	}
	if value.TaskStatuses["pending"] != 1 || value.TaskStatuses["running"] != 1 ||
		value.TaskStatuses["success"] != 1 || value.TaskStatuses["failed"] != 1 ||
		value.RunWindowStatuses["running"] != 1 || value.RunWindowStatuses["success"] != 1 ||
		value.RunWindowStatuses["failed"] != 1 || value.CollectionWindowStates["complete"] != 1 ||
		value.ActiveKeys["collection_pending"] != 1 || value.ActiveKeys["collection_running"] != 1 ||
		value.ActiveKeys["collection_terminal_with_key"] != 0 || value.ActiveKeys["export_active"] != 0 ||
		value.ActiveKeys["alert_active"] != 0 || value.ActiveKeys["maintenance_active"] != 0 {
		return false
	}
	for _, scope := range []string{"account", "customer", "site", "global", "model", "channel"} {
		metrics, exists := value.Aggregates[scope]
		if !exists || !validMetric(metrics.Hourly, false) || !validMetric(metrics.Daily, true) {
			return false
		}
	}
	return true
}

func validMetric(value aggregateMetric, daily bool) bool {
	return value.Rows == 1 && value.Requests == "3" && value.Quota == "5" && value.Tokens == "7" &&
		(value.ActiveUsers == "0" || value.ActiveUsers == "1") && value.DataStatus == "complete" && value.IsFinal == daily
}

func equalIntMaps(first, second map[string]int64) bool {
	if len(first) != len(second) {
		return false
	}
	for key, value := range first {
		if second[key] != value {
			return false
		}
	}
	return true
}

func equalAggregates(first, second map[string]scopeAggregates) bool {
	if len(first) != len(second) {
		return false
	}
	for key, value := range first {
		if second[key] != value {
			return false
		}
	}
	return true
}

func validAppSmoke(value appSmokeArtifact, source, target snapshotReport) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == acceptanceID && value.Status == "passed" &&
		value.HealthStatus == 200 && value.ReadyStatus == 200 && value.LoginStatus == 200 &&
		value.SelfStatus == 200 && value.SitesStatus == 200 && value.FixtureSiteFound && value.ConnectedToTarget &&
		value.Database == target.Database && value.SourceUUIDFingerprint == source.ServerUUIDFingerprint &&
		value.TargetUUIDFingerprint == target.ServerUUIDFingerprint &&
		value.ObservedUUIDFingerprint == target.ServerUUIDFingerprint &&
		value.SourceUUIDFingerprint != value.ObservedUUIDFingerprint && value.ReleaseGateRequired && !value.ProductionGate
}

func validTiming(value rpoRTOReport, source, target snapshotReport) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == acceptanceID && value.Status == "passed" &&
		value.BackupCreatedAtUnix >= value.LastBusinessTime && value.LastBusinessTime == source.LastBusinessTime &&
		value.LastBusinessTime == target.LastBusinessTime && value.RecoverableAge == value.BackupCreatedAtUnix-value.LastBusinessTime &&
		value.ActualDataLoss == 0 && value.RPOSeconds == value.RecoverableAge && value.RTOSeconds >= 0 &&
		value.RPOLimitSeconds == 3600 && value.RTOLimitSeconds == 14400 && value.RPOPassed && value.RTOPassed &&
		value.RPOSeconds <= value.RPOLimitSeconds && value.RTOSeconds <= value.RTOLimitSeconds
}

func validCleanup(value cleanupArtifact) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == acceptanceID && value.Status == "passed" && value.Passed &&
		value.Lifecycle.Containers == "created_and_removed" && value.Lifecycle.Networks == "created_and_removed" &&
		value.Lifecycle.Volumes == "created_and_removed" && value.Lifecycle.Images == "created_and_removed" &&
		len(value.Residuals.Containers) == 0 && len(value.Residuals.Networks) == 0 &&
		len(value.Residuals.Volumes) == 0 && len(value.Residuals.Images) == 0
}

func runValidate(arguments []string) error {
	directory := ""
	evidenceClass := "development"
	_, err := parseNoPositionals("validate", arguments, func(flags *flag.FlagSet) {
		flags.StringVar(&directory, "evidence-dir", directory, "A22 evidence directory")
		flags.StringVar(&evidenceClass, "evidence-class", evidenceClass, "development or formal")
	})
	if err != nil {
		return err
	}
	if directory == "" {
		return errors.New("validate requires an evidence directory")
	}
	return a22evidence.ValidateInnerArtifacts(directory, evidenceClass)
}
