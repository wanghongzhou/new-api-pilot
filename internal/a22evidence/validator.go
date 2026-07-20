package a22evidence

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	AcceptanceID     = "A22"
	FormalClass      = "formal"
	DevelopmentClass = "development"
	maxJSONSize      = 8 * 1024 * 1024
	maxLogSize       = 16 * 1024 * 1024
)

var (
	sha256Pattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uuidPattern           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	backupPattern         = regexp.MustCompile(`^backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8,64}$`)
	dsnPattern            = regexp.MustCompile(`(?i)(?:DATABASE_DSN\s*=\s*[^\s"']+|\b[^:\s]+:[^@\s]*@tcp\([^)\s]+\)/[^\s]+)`)
	keyAssignmentPattern  = regexp.MustCompile(`(?i)\b(?:ENCRYPTION_KEY|SESSION_SECRET|A22_ADMIN_PASSWORD|A22_SITE_TOKEN|A22_SECRET_SETTING)\s*=\s*([^\s"']+)`)
	urlCredentialPattern  = regexp.MustCompile(`(?i)https?://[^\s]*(?:access_token|token|secret|key|signature)=[^&\s]+`)
	skipPattern           = regexp.MustCompile(`(?i)(?:\bskipped\b|\bnot[ -]?run\b|\bno[ -]?op\b|"skip"\s*:\s*true)`)
	canonicalCommand      = []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a22.ps1"}
	canonicalToolCommands = []string{
		"migrate", "seed", "snapshot-source", "backup", "negative-manifest", "negative-target-mismatch",
		"restore", "verify-restore", "snapshot-target", "app-smoke", "report",
	}
	canonicalVerifyChecks = []string{
		"backup_manifest", "read_only_transaction", "migrations", "schema", "seeds", "foreign_keys",
		"encrypted_rows", "critical_counts", "collection_windows", "collection_cursors", "active_keys", "aggregations",
	}
	canonicalFinalChecks = []string{
		"backup", "cleanup", "command", "exact_restore", "fixture", "full_verify", "isolated_environment",
		"negative_manifest", "negative_target_mismatch", "representative_fixture", "restore_gate",
		"restored_app_smoke", "rpo_rto", "secret_scan", "six_level_aggregates", "source_snapshot",
		"target_snapshot", "task_window_active_key_state",
	}
	requiredArtifacts = []string{
		"a22-command.json", "a22-environment.json", "a22-fixture.json", "a22-migration.log",
		"a22-seed.json", "a22-source-snapshot.json", "a22-backup.json", "a22-backup.log",
		"a22-negative-manifest.json", "a22-negative-target-mismatch.json", "a22-restore.json",
		"a22-restore.log", "a22-verify-restore.json", "a22-target-snapshot.json", "a22-app-smoke.json",
		"a22-rpo-rto.json", "a22-secret-scan.json", "a22-report.json", "a22-cleanup.json",
	}
	fixedSecrets = []string{
		"a22-site-token-never-log",
		"https://oapi.dingtalk.com/robot/send?access_token=a22-never-log",
		"a22-admin-password-never-log",
	}
)

type finalReport struct {
	SchemaVersion               int             `json:"schema_version"`
	AcceptanceID                string          `json:"acceptance_id"`
	Status                      string          `json:"status"`
	Passed                      bool            `json:"passed"`
	EvidenceClass               string          `json:"evidence_class"`
	AcceptanceEligible          bool            `json:"acceptance_eligible"`
	Scope                       string          `json:"scope"`
	ProductionReleaseAuthorized bool            `json:"production_release_authorized"`
	RPOSeconds                  int64           `json:"rpo_seconds"`
	RTOSeconds                  int64           `json:"rto_seconds"`
	Checks                      map[string]bool `json:"checks"`
	Violations                  []string        `json:"violations"`
}

type commandReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	Status        string   `json:"status"`
	EvidenceClass string   `json:"evidence_class"`
	Scope         string   `json:"scope"`
	Command       []string `json:"command"`
	ToolCommands  []string `json:"tool_commands"`
}

type environmentSide struct {
	Database             string `json:"database"`
	UUIDFingerprint      string `json:"server_uuid_fingerprint"`
	Version              string `json:"version"`
	TransactionIsolation string `json:"transaction_isolation"`
	CharacterSetServer   string `json:"character_set_server"`
	CollationServer      string `json:"collation_server"`
	TimeZone             string `json:"time_zone"`
	BinaryLoggingEnabled bool   `json:"binary_logging_enabled"`
}

type environmentReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	EvidenceClass string `json:"evidence_class"`
	Commit        string `json:"commit"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	Images        struct {
		Go    imageIdentity `json:"go"`
		MySQL imageIdentity `json:"mysql"`
		Tools imageIdentity `json:"tools"`
	} `json:"images"`
	Network struct {
		Internal  bool     `json:"internal"`
		HostPorts []string `json:"host_ports"`
	} `json:"network"`
	Source                      environmentSide `json:"source"`
	Target                      environmentSide `json:"target"`
	ProductionReleaseAuthorized bool            `json:"production_release_authorized"`
}

type imageIdentity struct {
	Reference string `json:"reference"`
	ID        string `json:"id"`
	Digest    string `json:"digest"`
}

type fixtureReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	FixtureID     string `json:"fixture_id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	RPOSeconds    int64  `json:"rpo_seconds"`
	RTOSeconds    int64  `json:"rto_seconds"`
}

type aggregateMetric struct {
	Rows        int64  `json:"rows"`
	Requests    string `json:"request_count"`
	Quota       string `json:"quota"`
	Tokens      string `json:"token_used"`
	ActiveUsers string `json:"active_users"`
	DataStatus  string `json:"data_status"`
	IsFinal     bool   `json:"is_final"`
}

type scopeAggregates struct {
	Hourly aggregateMetric `json:"hourly"`
	Daily  aggregateMetric `json:"daily"`
}

type snapshotReport struct {
	SchemaVersion          int                        `json:"schema_version"`
	AcceptanceID           string                     `json:"acceptance_id"`
	Status                 string                     `json:"status"`
	Role                   string                     `json:"role"`
	Database               string                     `json:"database"`
	ServerUUID             string                     `json:"server_uuid"`
	ServerUUIDFingerprint  string                     `json:"server_uuid_fingerprint"`
	MySQLVersion           string                     `json:"mysql_version"`
	SnapshotSHA256         string                     `json:"snapshot_sha256"`
	TableCounts            map[string]int64           `json:"table_counts"`
	TaskStatuses           map[string]int64           `json:"task_statuses"`
	RunWindowStatuses      map[string]int64           `json:"run_window_statuses"`
	CollectionWindowStates map[string]int64           `json:"collection_window_statuses"`
	ActiveKeys             map[string]int64           `json:"active_keys"`
	Aggregates             map[string]scopeAggregates `json:"aggregates"`
	SiteTokenDecrypted     bool                       `json:"site_token_decrypted"`
	SecretSettingDecrypted bool                       `json:"secret_setting_decrypted"`
	LastBusinessTime       int64                      `json:"last_business_time_unix"`
	ProductionReleaseOK    bool                       `json:"production_release_authorized"`
}

type seedReport struct {
	SchemaVersion       int              `json:"schema_version"`
	AcceptanceID        string           `json:"acceptance_id"`
	Status              string           `json:"status"`
	FixtureID           string           `json:"fixture_id"`
	FixtureSHA256       string           `json:"fixture_sha256"`
	Database            string           `json:"database"`
	FixedHourUnix       int64            `json:"fixed_hour_unix"`
	DateKey             int              `json:"date_key"`
	LastBusinessTime    int64            `json:"last_business_time_unix"`
	SiteTokenEncrypted  bool             `json:"site_token_encrypted"`
	SecretEncrypted     bool             `json:"secret_setting_encrypted"`
	TaskStatuses        map[string]int64 `json:"task_statuses"`
	WindowStatuses      map[string]int64 `json:"run_window_statuses"`
	CollectionStatuses  map[string]int64 `json:"collection_window_statuses"`
	AggregateRows       map[string]int64 `json:"aggregate_rows"`
	ProductionReleaseOK bool             `json:"production_release_authorized"`
}

type backupReport struct {
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

type negativeReport struct {
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

type restoreReport struct {
	SchemaVersion     int    `json:"schema_version"`
	AcceptanceID      string `json:"acceptance_id"`
	Status            string `json:"status"`
	BackupID          string `json:"backup_id"`
	ReleaseGateExists bool   `json:"release_gate_exists"`
	VerifyReport      string `json:"verify_report"`
	ExitCode          int    `json:"exit_code"`
}

type verifyReport struct {
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

type appSmokeReport struct {
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

type timingReport struct {
	SchemaVersion       int    `json:"schema_version"`
	AcceptanceID        string `json:"acceptance_id"`
	Status              string `json:"status"`
	BackupCreatedAtUnix int64  `json:"backup_created_at_unix"`
	LastBusinessTime    int64  `json:"last_business_time_unix"`
	RecoverableAge      int64  `json:"recoverable_age_seconds"`
	ActualDataLoss      int64  `json:"actual_data_loss_seconds"`
	RPOSeconds          int64  `json:"rpo_seconds"`
	RTOSeconds          int64  `json:"rto_seconds"`
	RPOLimitSeconds     int64  `json:"rpo_limit_seconds"`
	RTOLimitSeconds     int64  `json:"rto_limit_seconds"`
	RPOPassed           bool   `json:"rpo_passed"`
	RTOPassed           bool   `json:"rto_passed"`
}

type scanReport struct {
	SchemaVersion      int    `json:"schema_version"`
	AcceptanceID       string `json:"acceptance_id"`
	Status             string `json:"status"`
	FilesScanned       int    `json:"files_scanned"`
	ForbiddenHits      int    `json:"forbidden_hits"`
	DSNLeaks           int    `json:"dsn_leaks"`
	KeyLeaks           int    `json:"key_leaks"`
	URLCredentialLeaks int    `json:"url_credential_leaks"`
}

type cleanupReport struct {
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

type artifactInventory struct {
	SchemaVersion int             `json:"schema_version"`
	AcceptanceID  string          `json:"acceptance_id"`
	EvidenceClass string          `json:"evidence_class"`
	Files         []artifactEntry `json:"files"`
}

type artifactEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type wrapperEvidence struct {
	SchemaVersion        int      `json:"schema_version"`
	AcceptanceID         string   `json:"acceptance_id"`
	Status               string   `json:"status"`
	EvidenceClass        string   `json:"evidence_class"`
	Command              []string `json:"command"`
	WorkingDirectory     string   `json:"working_directory"`
	StartedAt            string   `json:"started_at"`
	FinishedAt           string   `json:"finished_at"`
	DurationMilliseconds int64    `json:"duration_milliseconds"`
	ExitCode             int      `json:"exit_code"`
	Commit               string   `json:"commit"`
	WorktreeDirty        bool     `json:"worktree_dirty"`
	FixtureManifestPath  string   `json:"fixture_manifest_path"`
	FixtureManifestSHA   string   `json:"fixture_manifest_sha256"`
	StdoutLog            string   `json:"stdout_log"`
	StderrLog            string   `json:"stderr_log"`
	RequiredNoSkip       bool     `json:"required_no_skip"`
}

func Supports(acceptanceID string) bool { return acceptanceID == AcceptanceID }

func Classify(acceptanceID, evidenceRoot string, command []string) (string, error) {
	if !Supports(acceptanceID) {
		return "", nil
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	class := ""
	switch cleaned {
	case "artifacts/acceptance":
		class = FormalClass
	case "artifacts/smoke":
		class = DevelopmentClass
	default:
		return "", errors.New("A22 evidence root must be canonical artifacts/acceptance or artifacts/smoke")
	}
	if err := ValidateCanonicalCommand(command, class); err != nil {
		return "", err
	}
	return class, nil
}

func ValidateCanonicalCommand(command []string, class string) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A22 evidence class %q", class)
	}
	if !equalStrings(command, canonicalCommand) {
		return fmt.Errorf("A22 %s evidence requires canonical command %q", class, strings.Join(canonicalCommand, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, class string) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A22 evidence class %q", class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	innerFiles := append(append([]string{}, requiredArtifacts...), "a22-artifacts.json")
	preWrapperFiles := append(append([]string{}, innerFiles...), "stdout.log", "stderr.log")
	completeWrapperFiles := append(append([]string{}, preWrapperFiles...), "evidence.json")
	if err := validateDirectoryFileSet(runDirectory, innerFiles, preWrapperFiles, completeWrapperFiles); err != nil {
		return err
	}
	for _, name := range requiredArtifacts {
		if strings.HasSuffix(name, ".log") {
			if err := validateLog(filepath.Join(runDirectory, name), true); err != nil {
				return err
			}
		} else if err := requireRegularFile(filepath.Join(runDirectory, name), true, maxJSONSize); err != nil {
			return fmt.Errorf("validate A22 artifact %s: %w", name, err)
		}
	}
	var command commandReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-command.json"), &command); err != nil {
		return err
	}
	if command.SchemaVersion != 1 || command.AcceptanceID != AcceptanceID || command.Status != "passed" ||
		command.EvidenceClass != class || command.Scope != "controlled_technical_drill" ||
		ValidateCanonicalCommand(command.Command, class) != nil || !equalStrings(command.ToolCommands, canonicalToolCommands) {
		return errors.New("A22 command artifact contract is invalid")
	}
	var environment environmentReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-environment.json"), &environment); err != nil {
		return err
	}
	if !validEnvironment(environment, class) {
		return errors.New("A22 environment artifact contract is invalid")
	}
	var fixture fixtureReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-fixture.json"), &fixture); err != nil {
		return err
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != AcceptanceID || fixture.Status != "passed" ||
		fixture.FixtureID != "F05" || fixture.Path != "testdata/design/f05-ops-capacity.yaml" ||
		!validSHA(fixture.SHA256) || fixture.RPOSeconds != 3600 || fixture.RTOSeconds != 14400 {
		return errors.New("A22 fixture artifact contract is invalid")
	}
	var seed seedReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-seed.json"), &seed); err != nil {
		return err
	}
	if !validSeed(seed, fixture.SHA256) {
		return errors.New("A22 seed artifact contract is invalid")
	}
	var source, target snapshotReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-source-snapshot.json"), &source); err != nil {
		return err
	}
	if err := decodeJSON(filepath.Join(runDirectory, "a22-target-snapshot.json"), &target); err != nil {
		return err
	}
	if !validSnapshot(source, "source") || !validSnapshot(target, "target") || source.SnapshotSHA256 != target.SnapshotSHA256 ||
		source.Database != target.Database || source.ServerUUID == target.ServerUUID || source.LastBusinessTime != target.LastBusinessTime ||
		!equalIntMaps(source.TableCounts, target.TableCounts) || !equalIntMaps(source.TaskStatuses, target.TaskStatuses) ||
		!equalIntMaps(source.RunWindowStatuses, target.RunWindowStatuses) ||
		!equalIntMaps(source.CollectionWindowStates, target.CollectionWindowStates) ||
		!equalIntMaps(source.ActiveKeys, target.ActiveKeys) || !equalAggregates(source.Aggregates, target.Aggregates) {
		return errors.New("A22 source and target snapshots are not an exact isolated restore")
	}
	if environment.Source.UUIDFingerprint != source.ServerUUIDFingerprint ||
		environment.Target.UUIDFingerprint != target.ServerUUIDFingerprint ||
		environment.Source.Version != source.MySQLVersion || environment.Target.Version != target.MySQLVersion {
		return errors.New("A22 environment identity is not bound to the source and target snapshots")
	}
	var backup backupReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-backup.json"), &backup); err != nil {
		return err
	}
	if backup.SchemaVersion != 1 || backup.AcceptanceID != AcceptanceID || backup.Status != "passed" ||
		!backupPattern.MatchString(backup.BackupID) || !validSHA(backup.ManifestSHA256) || !validSHA(backup.DumpSHA256) ||
		backup.DumpSizeBytes <= 0 || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(backup.KeyFingerprint) ||
		!validImageID(backup.ImageDigest) ||
		!backup.SourceCoordinate || !backup.AtomicPublish {
		return errors.New("A22 backup artifact contract is invalid")
	}
	backupCreatedAt, createdAtErr := time.Parse(time.RFC3339, backup.CreatedAtUTC)
	if createdAtErr != nil {
		return errors.New("A22 backup creation time is invalid")
	}
	for name, stage := range map[string]string{
		"a22-negative-manifest.json":        "manifest_preflight",
		"a22-negative-target-mismatch.json": "target_identity",
	} {
		var negative negativeReport
		if err := decodeJSON(filepath.Join(runDirectory, name), &negative); err != nil {
			return err
		}
		if negative.SchemaVersion != 1 || negative.AcceptanceID != AcceptanceID || negative.Status != "passed" ||
			!negative.Passed || negative.FailureStage != stage || negative.ExitCode == 0 || negative.ImportStarted ||
			negative.TargetTableCount != 0 || negative.ReleaseGateExists || !negative.SourceUnchanged || negative.ProductionGate {
			return fmt.Errorf("A22 negative artifact %s contract is invalid", name)
		}
	}
	var restore restoreReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-restore.json"), &restore); err != nil {
		return err
	}
	if restore.SchemaVersion != 1 || restore.AcceptanceID != AcceptanceID || restore.Status != "passed" ||
		restore.BackupID != backup.BackupID || !restore.ReleaseGateExists ||
		restore.VerifyReport != "a22-verify-restore.json" || restore.ExitCode != 0 {
		return errors.New("A22 restore artifact contract is invalid")
	}
	var verify verifyReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-verify-restore.json"), &verify); err != nil {
		return err
	}
	if verify.SchemaVersion != 1 || verify.Command != "verify-restore" || verify.Mode != "full" ||
		verify.Status != "success" || verify.Error != nil || !validVerifyChecks(verify) ||
		verify.BackupID != backup.BackupID || verify.EncryptionKeyID != backup.KeyFingerprint ||
		backup.ImageDigest != environment.Images.Tools.Digest {
		return errors.New("A22 full verification artifact contract is invalid")
	}
	var smoke appSmokeReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-app-smoke.json"), &smoke); err != nil {
		return err
	}
	if !validSmoke(smoke, source, target) {
		return errors.New("A22 restored application smoke contract is invalid")
	}
	var timing timingReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-rpo-rto.json"), &timing); err != nil {
		return err
	}
	if !validTiming(timing, source, target) {
		return errors.New("A22 RPO/RTO artifact contract is invalid")
	}
	if backupCreatedAt.Unix() != timing.BackupCreatedAtUnix {
		return errors.New("A22 backup creation time is not bound to the RPO/RTO report")
	}
	var scan scanReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-secret-scan.json"), &scan); err != nil {
		return err
	}
	if scan.SchemaVersion != 1 || scan.AcceptanceID != AcceptanceID || scan.Status != "passed" || scan.FilesScanned <= 0 ||
		scan.ForbiddenHits != 0 || scan.DSNLeaks != 0 || scan.KeyLeaks != 0 || scan.URLCredentialLeaks != 0 {
		return errors.New("A22 secret scan artifact contract is invalid")
	}
	var cleanup cleanupReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-cleanup.json"), &cleanup); err != nil {
		return err
	}
	if !validCleanup(cleanup) {
		return errors.New("A22 cleanup artifact contract is invalid")
	}
	var report finalReport
	if err := decodeJSON(filepath.Join(runDirectory, "a22-report.json"), &report); err != nil {
		return err
	}
	if !validFinalReport(report, class, timing) {
		return errors.New("A22 final report contract is invalid")
	}
	if err := validateInventory(runDirectory, class); err != nil {
		return err
	}
	if err := scanEvidenceFiles(runDirectory); err != nil {
		return err
	}
	return nil
}

func ValidateWrapperLogs(runDirectory string) error {
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := validateLog(filepath.Join(runDirectory, name), false); err != nil {
			return err
		}
	}
	return nil
}

func ValidateRunDirectory(runDirectory, class string) error {
	innerFiles := append(append([]string{}, requiredArtifacts...), "a22-artifacts.json")
	expectedFiles := append(append([]string{}, innerFiles...), "stdout.log", "stderr.log", "evidence.json")
	if err := validateDirectoryFileSet(runDirectory, expectedFiles); err != nil {
		return err
	}
	if err := ValidateWrapperLogs(runDirectory); err != nil {
		return err
	}
	if err := ValidateInnerArtifacts(runDirectory, class); err != nil {
		return err
	}
	if err := scanEvidenceFile(filepath.Join(runDirectory, "evidence.json")); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSON(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return err
	}
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != AcceptanceID || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || evidence.ExitCode != 0 || !evidence.RequiredNoSkip ||
		evidence.WorkingDirectory != "." || evidence.StdoutLog != "stdout.log" || evidence.StderrLog != "stderr.log" ||
		evidence.FixtureManifestPath != "testdata/design/manifest.sha256" || !validSHA(evidence.FixtureManifestSHA) ||
		(evidence.Commit != "unborn" && !regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(evidence.Commit)) ||
		ValidateCanonicalCommand(evidence.Command, class) != nil {
		return errors.New("A22 wrapper evidence contract is invalid")
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if startErr != nil || finishErr != nil || finished.Before(started) ||
		finished.Sub(started).Milliseconds() != evidence.DurationMilliseconds {
		return errors.New("A22 wrapper timing contract is invalid")
	}
	return nil
}

func ValidateEvidenceRoot(evidenceRoot, class string) error {
	entries, err := os.ReadDir(evidenceRoot)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	reasons := make([]string, 0, 3)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		err := ValidateRunDirectory(filepath.Join(evidenceRoot, entry.Name()), class)
		if err == nil {
			return nil
		}
		if len(reasons) < 3 {
			reasons = append(reasons, entry.Name()+": "+err.Error())
		}
	}
	return fmt.Errorf("no valid A22 %s evidence run found: %s", class, strings.Join(reasons, "; "))
}

func validEnvironment(value environmentReport, class string) bool {
	validSide := func(side environmentSide) bool {
		return side.Database == "pilot_a22" && len(side.UUIDFingerprint) == 12 && side.Version != "" &&
			side.TransactionIsolation == "READ-COMMITTED" && side.CharacterSetServer == "utf8mb4" &&
			side.CollationServer == "utf8mb4_unicode_ci" && side.TimeZone == "+08:00" && side.BinaryLoggingEnabled
	}
	validImage := func(value imageIdentity, reference string, local bool) bool {
		if value.Reference != reference && (!local || value.Reference == "") {
			return false
		}
		return validImageID(value.ID) && (validImageID(value.Digest) ||
			regexp.MustCompile(`^[^@]+@sha256:[0-9a-f]{64}$`).MatchString(value.Digest))
	}
	return value.SchemaVersion == 1 && value.AcceptanceID == AcceptanceID && value.Status == "passed" &&
		value.EvidenceClass == class && value.Network.Internal && len(value.Network.HostPorts) == 0 &&
		(value.Commit == "unborn" || regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(value.Commit)) &&
		validImage(value.Images.Go, "golang:1.25.1", false) && validImage(value.Images.MySQL, "mysql:8.4", false) &&
		validImage(value.Images.Tools, value.Images.Tools.Reference, true) &&
		validSide(value.Source) && validSide(value.Target) && value.Source.UUIDFingerprint != value.Target.UUIDFingerprint &&
		!value.ProductionReleaseAuthorized
}

func validImageID(value string) bool {
	return regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(value)
}

func validSeed(value seedReport, fixtureSHA string) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == AcceptanceID && value.Status == "passed" &&
		value.FixtureID == "F05" && value.FixtureSHA256 == fixtureSHA && value.Database == "pilot_a22" &&
		value.FixedHourUnix > 0 && value.FixedHourUnix%3600 == 0 && value.DateKey > 20000101 && value.LastBusinessTime > 0 &&
		value.SiteTokenEncrypted && value.SecretEncrypted && !value.ProductionReleaseOK && len(value.AggregateRows) == 12 &&
		value.TaskStatuses["pending"] == 1 && value.TaskStatuses["running"] == 1 && value.TaskStatuses["success"] == 1 &&
		value.TaskStatuses["failed"] == 1 && value.WindowStatuses["running"] == 1 &&
		value.WindowStatuses["success"] == 1 && value.WindowStatuses["failed"] == 1 &&
		value.CollectionStatuses["complete"] == 1
}

func validSnapshot(value snapshotReport, role string) bool {
	if value.SchemaVersion != 1 || value.AcceptanceID != AcceptanceID || value.Status != "passed" || value.Role != role ||
		value.Database != "pilot_a22" || !uuidPattern.MatchString(value.ServerUUID) || len(value.ServerUUIDFingerprint) != 12 ||
		value.ServerUUIDFingerprint != shortFingerprint(value.ServerUUID) ||
		!validSHA(value.SnapshotSHA256) || len(value.TableCounts) < 30 || !value.SiteTokenDecrypted ||
		!value.SecretSettingDecrypted || value.LastBusinessTime <= 0 || value.ProductionReleaseOK {
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
	return len(value.Aggregates) == 6
}

func validMetric(value aggregateMetric, daily bool) bool {
	return value.Rows == 1 && value.Requests == "3" && value.Quota == "5" && value.Tokens == "7" &&
		(value.ActiveUsers == "0" || value.ActiveUsers == "1") && value.DataStatus == "complete" && value.IsFinal == daily
}

func validVerifyChecks(value verifyReport) bool {
	if value.Summary.Passed != len(canonicalVerifyChecks) || value.Summary.Failed != 0 ||
		len(value.Checks) != len(canonicalVerifyChecks) {
		return false
	}
	seen := make(map[string]struct{}, len(value.Checks))
	for _, check := range value.Checks {
		if check.Status != "passed" {
			return false
		}
		if _, duplicate := seen[check.Name]; duplicate {
			return false
		}
		seen[check.Name] = struct{}{}
	}
	for _, name := range canonicalVerifyChecks {
		if _, exists := seen[name]; !exists {
			return false
		}
	}
	return true
}

func shortFingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:12]
}

func validSmoke(value appSmokeReport, source, target snapshotReport) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == AcceptanceID && value.Status == "passed" &&
		value.HealthStatus == 200 && value.ReadyStatus == 200 && value.LoginStatus == 200 && value.SelfStatus == 200 &&
		value.SitesStatus == 200 && value.FixtureSiteFound && value.ConnectedToTarget && value.Database == target.Database &&
		value.SourceUUIDFingerprint == source.ServerUUIDFingerprint && value.TargetUUIDFingerprint == target.ServerUUIDFingerprint &&
		value.ObservedUUIDFingerprint == target.ServerUUIDFingerprint && value.SourceUUIDFingerprint != value.ObservedUUIDFingerprint &&
		value.ReleaseGateRequired && !value.ProductionGate
}

func validTiming(value timingReport, source, target snapshotReport) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == AcceptanceID && value.Status == "passed" &&
		value.BackupCreatedAtUnix >= value.LastBusinessTime && value.LastBusinessTime == source.LastBusinessTime &&
		value.LastBusinessTime == target.LastBusinessTime && value.RecoverableAge == value.BackupCreatedAtUnix-value.LastBusinessTime &&
		value.ActualDataLoss == 0 && value.RPOSeconds == value.RecoverableAge && value.RTOSeconds >= 0 &&
		value.RPOLimitSeconds == 3600 && value.RTOLimitSeconds == 14400 && value.RPOPassed && value.RTOPassed &&
		value.RPOSeconds <= value.RPOLimitSeconds && value.RTOSeconds <= value.RTOLimitSeconds
}

func validCleanup(value cleanupReport) bool {
	return value.SchemaVersion == 1 && value.AcceptanceID == AcceptanceID && value.Status == "passed" && value.Passed &&
		value.Lifecycle.Containers == "created_and_removed" && value.Lifecycle.Networks == "created_and_removed" &&
		value.Lifecycle.Volumes == "created_and_removed" && value.Lifecycle.Images == "created_and_removed" &&
		len(value.Residuals.Containers) == 0 && len(value.Residuals.Networks) == 0 &&
		len(value.Residuals.Volumes) == 0 && len(value.Residuals.Images) == 0
}

func validFinalReport(value finalReport, class string, timing timingReport) bool {
	if value.SchemaVersion != 1 || value.AcceptanceID != AcceptanceID || value.Status != "passed" || !value.Passed ||
		value.EvidenceClass != class || value.AcceptanceEligible != (class == FormalClass) ||
		value.Scope != "controlled_technical_drill" || value.ProductionReleaseAuthorized ||
		value.RPOSeconds != timing.RPOSeconds || value.RTOSeconds != timing.RTOSeconds || len(value.Violations) != 0 ||
		len(value.Checks) != len(canonicalFinalChecks) {
		return false
	}
	for _, name := range canonicalFinalChecks {
		if passed, exists := value.Checks[name]; !exists || !passed {
			return false
		}
	}
	return true
}

func validateInventory(runDirectory, class string) error {
	var inventory artifactInventory
	if err := decodeJSON(filepath.Join(runDirectory, "a22-artifacts.json"), &inventory); err != nil {
		return err
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != AcceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A22 artifact inventory contract is invalid")
	}
	for index, expected := range requiredArtifacts {
		entry := inventory.Files[index]
		if entry.Path != expected || entry.SizeBytes <= 0 || !validSHA(entry.SHA256) {
			return fmt.Errorf("A22 artifact inventory entry %d is invalid", index)
		}
		path := filepath.Join(runDirectory, expected)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != entry.SizeBytes {
			return fmt.Errorf("A22 artifact inventory metadata mismatch for %s", expected)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			return fmt.Errorf("A22 artifact inventory checksum mismatch for %s", expected)
		}
	}
	return nil
}

func scanEvidenceFiles(runDirectory string) error {
	for _, name := range append(append([]string{}, requiredArtifacts...), "a22-artifacts.json") {
		if err := scanEvidenceFile(filepath.Join(runDirectory, name)); err != nil {
			return err
		}
	}
	return nil
}

func scanEvidenceFile(path string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if violation := classifySensitive(string(payload)); violation != "" {
		return fmt.Errorf("A22 artifact %s contains forbidden %s", filepath.Base(path), violation)
	}
	if skipPattern.Match(payload) {
		return fmt.Errorf("A22 artifact %s contains forbidden skip/no-op evidence", filepath.Base(path))
	}
	return nil
}

func validateDirectoryFileSet(path string, allowedSets ...[]string) error {
	if err := requireDirectory(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	actual := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("A22 evidence directory entry %s must be a regular non-link file", entry.Name())
		}
		actual[entry.Name()] = struct{}{}
	}
	for _, allowed := range allowedSets {
		if len(actual) != len(allowed) {
			continue
		}
		matched := true
		for _, name := range allowed {
			if _, exists := actual[name]; !exists {
				matched = false
				break
			}
		}
		if matched {
			return nil
		}
	}
	return fmt.Errorf("A22 evidence directory has an unexpected fixed file set (%d files)", len(actual))
}

func validateLog(path string, requireNonEmpty bool) error {
	if err := requireRegularFile(path, requireNonEmpty, maxLogSize); err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if violation := classifySensitive(string(payload)); violation != "" {
		return fmt.Errorf("A22 log %s contains forbidden %s", filepath.Base(path), violation)
	}
	if skipPattern.Match(payload) {
		return fmt.Errorf("A22 log %s contains skip/no-op output", filepath.Base(path))
	}
	return nil
}

func classifySensitive(payload string) string {
	for _, secret := range fixedSecrets {
		if strings.Contains(payload, secret) {
			return "plaintext secret"
		}
	}
	if dsnPattern.MatchString(payload) {
		return "database DSN"
	}
	for _, match := range keyAssignmentPattern.FindAllStringSubmatch(payload, -1) {
		if len(match) == 2 && !strings.EqualFold(match[1], "[redacted]") {
			return "secret environment value"
		}
	}
	if urlCredentialPattern.MatchString(payload) {
		return "URL credential"
	}
	for _, token := range regexp.MustCompile(`(?:^|[^A-Za-z0-9+/])([A-Za-z0-9+/]{43}=)(?:$|[^A-Za-z0-9+/=])`).FindAllStringSubmatch(payload, -1) {
		if len(token) != 2 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(token[1])
		if err == nil && len(decoded) == 32 {
			return "32-byte Base64 key"
		}
	}
	return ""
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("A22 evidence path must be a real directory")
	}
	return nil
}

func requireRegularFile(path string, nonEmpty bool, maximum int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maximum ||
		(nonEmpty && info.Size() == 0) {
		return errors.New("file must be a bounded regular file")
	}
	return nil
}

func decodeJSON(path string, value any) error {
	if err := requireRegularFile(path, true, maxJSONSize); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, maxJSONSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("decode %s: expected exactly one JSON object", filepath.Base(path))
	}
	return nil
}

func validSHA(value string) bool { return sha256Pattern.MatchString(value) }

func equalStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
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
