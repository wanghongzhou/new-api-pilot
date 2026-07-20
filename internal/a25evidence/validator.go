package a25evidence

import (
	"bufio"
	"crypto/sha256"
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

	"new-api-pilot/migrations"
	"new-api-pilot/model"
)

const (
	AcceptanceID = "A25"
	FormalClass  = "formal"
	maxJSONSize  = 4 << 20
	maxLogSize   = 32 << 20
)

var (
	canonicalCommand = []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a25.ps1",
	}
	innerCommand = []string{
		"go", "test", "-json", "./tests/integration", "-run", "^TestA25MigrationAcceptance$", "-count=1", "-timeout=10m",
	}
	requiredArtifacts = []string{
		"a25-test.jsonl",
		"a25-test.stderr.log",
		"a25-test-summary.json",
		"a25-command.json",
		"a25-environment.json",
		"a25-fixture.json",
		"a25-report.json",
		"a25-cleanup.json",
	}
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern   = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	resourcePattern = regexp.MustCompile(`^new-api-pilot-a25-[0-9a-f]+-[0-9a-f]{12}-(?:network|gomod|gobuild)$`)
	secretPattern   = regexp.MustCompile(`(?i)(?:DATABASE_DSN|TEST_DATABASE_DSN|A25_(?:LEGACY|MARIADB)_DATABASE_DSN)\s*=|[[:alnum:]_.-]+:[^@[:space:]]+@tcp\(|(?:password|token|secret|dsn)\s*=\s*[^[:space:]]+`)
	noTestsPattern  = regexp.MustCompile(`(?i)no tests to run|\[no test files\]`)
)

type testSummary struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	TargetTest    string `json:"target_test"`
	Package       string `json:"package"`
	PassEvents    int    `json:"pass_events"`
	FailEvents    int    `json:"fail_events"`
	SkipEvents    int    `json:"skip_events"`
	NoTests       bool   `json:"no_tests"`
	JSONLines     int    `json:"json_lines"`
	JSONPath      string `json:"json_path"`
	StderrPath    string `json:"stderr_path"`
}

type commandReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	EvidenceClass string   `json:"evidence_class"`
	TargetTest    string   `json:"target_test"`
	WorkingDir    string   `json:"working_directory"`
	Command       []string `json:"command"`
	GoImage       string   `json:"go_image"`
	MySQLImage    string   `json:"mysql_image"`
	LegacyImage   string   `json:"legacy_mysql_image"`
	MariaDBImage  string   `json:"mariadb_image"`
}

type imageIdentity struct {
	Reference string `json:"reference"`
	ID        string `json:"id"`
	Digest    string `json:"digest"`
}

type currentServer struct {
	Version              string `json:"version"`
	TransactionIsolation string `json:"transaction_isolation"`
	CharacterSetServer   string `json:"character_set_server"`
	CollationServer      string `json:"collation_server"`
	TimeZone             string `json:"time_zone"`
}

type negativeServer struct {
	Version string `json:"version"`
}

type environmentReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	EvidenceClass string `json:"evidence_class"`
	Commit        string `json:"commit"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	IsolatedGuard bool   `json:"isolated_guard"`
	Images        struct {
		Go      imageIdentity `json:"go"`
		Current imageIdentity `json:"current_mysql"`
		Legacy  imageIdentity `json:"legacy_mysql"`
		MariaDB imageIdentity `json:"mariadb"`
	} `json:"images"`
	Servers struct {
		Current currentServer  `json:"current"`
		Legacy  negativeServer `json:"legacy_mysql"`
		MariaDB negativeServer `json:"mariadb"`
	} `json:"servers"`
	Network struct {
		Internal  bool     `json:"internal"`
		HostPorts []string `json:"host_ports"`
	} `json:"network"`
	Databases struct {
		Current string `json:"current"`
		Legacy  string `json:"legacy_mysql"`
		MariaDB string `json:"mariadb"`
	} `json:"databases"`
	Resources struct {
		Network     string `json:"network"`
		ModuleCache string `json:"module_cache"`
		BuildCache  string `json:"build_cache"`
	} `json:"resources"`
	RepositoryReadOnly bool `json:"repository_read_only"`
	EvidenceWritable   bool `json:"evidence_writable"`
	OfflineTestNetwork bool `json:"offline_test_network"`
}

type fixtureReport struct {
	SchemaVersion  int    `json:"schema_version"`
	AcceptanceID   string `json:"acceptance_id"`
	FixtureID      string `json:"fixture_id"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	FixedNowUnix   int64  `json:"fixed_now_unix"`
	MigrationCount int    `json:"migration_count"`
}

type migrationEvidence struct {
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

type finalReport struct {
	SchemaVersion             int                 `json:"schema_version"`
	AcceptanceID              string              `json:"acceptance_id"`
	Status                    string              `json:"status"`
	FixturePath               string              `json:"fixture_path"`
	FixtureSHA256             string              `json:"fixture_sha256"`
	FixedNowUnix              int64               `json:"fixed_now_unix"`
	RepositoryMigrations      []migrationEvidence `json:"repository_migrations"`
	AuthoritativeSchemaSHA256 string              `json:"authoritative_schema_sha256"`
	VersionGate               struct {
		CurrentVersion          string `json:"current_version"`
		LegacyMySQLVersion      string `json:"legacy_mysql_version"`
		MariaDBVersion          string `json:"mariadb_version"`
		CurrentAccepted         bool   `json:"current_accepted"`
		LegacyMySQLRejected     bool   `json:"legacy_mysql_rejected"`
		MariaDBRejected         bool   `json:"mariadb_rejected"`
		LegacyTablesBefore      int64  `json:"legacy_tables_before"`
		LegacyTablesAfter       int64  `json:"legacy_tables_after"`
		MariaDBTablesBefore     int64  `json:"mariadb_tables_before"`
		MariaDBTablesAfter      int64  `json:"mariadb_tables_after"`
		LegacyLockAbsentBefore  bool   `json:"legacy_lock_absent_before"`
		LegacyLockAbsentAfter   bool   `json:"legacy_lock_absent_after"`
		MariaDBLockAbsentBefore bool   `json:"mariadb_lock_absent_before"`
		MariaDBLockAbsentAfter  bool   `json:"mariadb_lock_absent_after"`
	} `json:"version_gate"`
	EmptyDatabase struct {
		TablesBefore           int64  `json:"tables_before"`
		MigrationCount         int    `json:"migration_count"`
		ProgressRows           int64  `json:"progress_rows"`
		SchemaSHA256           string `json:"schema_sha256"`
		AppliedAtStable        bool   `json:"applied_at_stable"`
		IdempotentSchemaStable bool   `json:"idempotent_schema_stable"`
	} `json:"empty_database"`
	Upgrade struct {
		PrefixMigrationCount  int    `json:"prefix_migration_count"`
		HistoricalRows        int64  `json:"historical_rows"`
		HistoricalSHA256      string `json:"historical_sha256"`
		HistoricalPreserved   bool   `json:"historical_preserved"`
		ForeignKeysPreserved  bool   `json:"foreign_keys_preserved"`
		BackfillScopeMigrated bool   `json:"backfill_scope_migrated"`
		SchemaSHA256          string `json:"schema_sha256"`
		MatchesAuthoritative  bool   `json:"matches_authoritative"`
	} `json:"upgrade"`
	Tamper struct {
		DatabaseChecksumRejected bool  `json:"database_checksum_rejected"`
		RepositorySourceRejected bool  `json:"repository_source_rejected"`
		UnknownVersionRejected   bool  `json:"unknown_version_rejected"`
		NoSchemaMutation         bool  `json:"no_schema_mutation"`
		ProgressRows             int64 `json:"progress_rows"`
	} `json:"tamper"`
	DMLFailure struct {
		InitialFailureObserved bool  `json:"initial_failure_observed"`
		CheckpointReady        bool  `json:"checkpoint_ready"`
		CheckpointIndex        int   `json:"checkpoint_index"`
		ResumeCompleted        bool  `json:"resume_completed"`
		IdempotentRowCount     int64 `json:"idempotent_row_count"`
		ProgressRows           int64 `json:"progress_rows"`
	} `json:"dml_failure"`
	DDLRecovery struct {
		DirtyWithoutDDLReplayed bool   `json:"dirty_without_ddl_replayed"`
		DirtyWithDDLRecognized  bool   `json:"dirty_with_ddl_recognized"`
		ReplaySchemaSHA256      string `json:"replay_schema_sha256"`
		CommittedSchemaSHA256   string `json:"committed_schema_sha256"`
		ProgressRows            int64  `json:"progress_rows"`
	} `json:"ddl_recovery"`
}

type cleanupReport struct {
	SchemaVersion   int    `json:"schema_version"`
	AcceptanceID    string `json:"acceptance_id"`
	EvidenceClass   string `json:"evidence_class"`
	Passed          bool   `json:"passed"`
	SweepsSucceeded bool   `json:"sweeps_succeeded"`
	Lifecycle       struct {
		Containers string `json:"containers"`
		Networks   string `json:"networks"`
		Volumes    string `json:"volumes"`
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

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func Supports(acceptanceID string) bool { return acceptanceID == AcceptanceID }

func Classify(acceptanceID, evidenceRoot string, command []string) (string, error) {
	if !Supports(acceptanceID) {
		return "", nil
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	if cleaned != "artifacts/acceptance" {
		return "", errors.New("A25 evidence root must be canonical artifacts/acceptance")
	}
	if err := ValidateCanonicalCommand(command, FormalClass); err != nil {
		return "", err
	}
	return FormalClass, nil
}

func ValidateCanonicalCommand(command []string, class string) error {
	if class != FormalClass || !equalStrings(command, canonicalCommand) {
		return fmt.Errorf("A25 formal evidence requires canonical command %q", strings.Join(canonicalCommand, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, class string) error {
	if class != FormalClass {
		return fmt.Errorf("unsupported A25 evidence class %q", class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-report.json"), &report); err != nil {
		return fmt.Errorf("validate A25 report: %w", err)
	}
	repository, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil {
		return fmt.Errorf("load embedded migration repository: %w", err)
	}
	if err := validateFinalReport(report, repository); err != nil {
		return err
	}
	var summary testSummary
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-test-summary.json"), &summary); err != nil {
		return fmt.Errorf("validate A25 test summary: %w", err)
	}
	if err := validateTestStream(filepath.Join(runDirectory, "a25-test.jsonl"), summary); err != nil {
		return err
	}
	if err := validateLogFile(filepath.Join(runDirectory, "a25-test.stderr.log")); err != nil {
		return err
	}
	var command commandReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-command.json"), &command); err != nil {
		return fmt.Errorf("validate A25 command report: %w", err)
	}
	if command.SchemaVersion != 1 || command.AcceptanceID != AcceptanceID || command.EvidenceClass != class ||
		command.TargetTest != "TestA25MigrationAcceptance" || command.WorkingDir != "/workspace" ||
		command.GoImage != "golang:1.25.1" || command.MySQLImage != "mysql:8.4" ||
		command.LegacyImage != "mysql:5.7" || command.MariaDBImage != "mariadb:10.11" ||
		!equalStrings(command.Command, innerCommand) {
		return errors.New("A25 command report contract is invalid")
	}
	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-environment.json"), &environment); err != nil {
		return fmt.Errorf("validate A25 environment report: %w", err)
	}
	if err := validateEnvironment(environment, class); err != nil {
		return err
	}
	if report.VersionGate.CurrentVersion != environment.Servers.Current.Version ||
		report.VersionGate.LegacyMySQLVersion != environment.Servers.Legacy.Version ||
		report.VersionGate.MariaDBVersion != environment.Servers.MariaDB.Version {
		return errors.New("A25 server versions differ between environment and test report")
	}
	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-fixture.json"), &fixture); err != nil {
		return fmt.Errorf("validate A25 fixture report: %w", err)
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != AcceptanceID || fixture.FixtureID != "F05" ||
		fixture.Path != report.FixturePath || fixture.SHA256 != report.FixtureSHA256 ||
		fixture.FixedNowUnix != report.FixedNowUnix || fixture.MigrationCount != len(repository) ||
		!sha256Pattern.MatchString(fixture.SHA256) {
		return errors.New("A25 fixture report contract is invalid")
	}
	if err := validateCleanup(runDirectory, class); err != nil {
		return err
	}
	return validateInventory(runDirectory, class)
}

func ValidateWrapperLogs(runDirectory string) error {
	for _, name := range []string{"stdout.log", "stderr.log"} {
		payload, err := readBounded(filepath.Join(runDirectory, name), maxLogSize)
		if err != nil {
			return fmt.Errorf("validate A25 wrapper log %s: %w", name, err)
		}
		if secretPattern.Match(payload) {
			return fmt.Errorf("A25 wrapper log %s contains a credential or DSN", name)
		}
	}
	return nil
}

func ValidateRunDirectory(runDirectory, class string) error {
	if err := ValidateInnerArtifacts(runDirectory, class); err != nil {
		return err
	}
	if err := ValidateWrapperLogs(runDirectory); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return fmt.Errorf("validate A25 wrapper evidence: %w", err)
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != AcceptanceID || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || ValidateCanonicalCommand(evidence.Command, class) != nil ||
		evidence.WorkingDirectory != "." || evidence.ExitCode != 0 || !evidence.RequiredNoSkip ||
		(evidence.Commit != "unborn" && !commitPattern.MatchString(evidence.Commit)) ||
		evidence.FixtureManifestPath != "testdata/design/manifest.sha256" ||
		!sha256Pattern.MatchString(evidence.FixtureManifestSHA) || evidence.StdoutLog != "stdout.log" ||
		evidence.StderrLog != "stderr.log" || startErr != nil || finishErr != nil || finished.Before(started) ||
		evidence.DurationMilliseconds < 0 {
		return errors.New("A25 wrapper evidence contract is invalid")
	}
	return nil
}

func ValidateEvidenceRoot(root, class string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	failures := make([]string, 0, len(names))
	for _, name := range names {
		if err := ValidateRunDirectory(filepath.Join(root, name), class); err == nil {
			return nil
		} else {
			failures = append(failures, name+": "+err.Error())
		}
	}
	if len(failures) == 0 {
		return errors.New("no A25 evidence run directories exist")
	}
	return fmt.Errorf("no valid A25 formal run: %s", strings.Join(failures, "; "))
}

func validateFinalReport(report finalReport, repository []model.MigrationVersion) error {
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.Status != "passed" ||
		report.FixturePath != "testdata/design/f05-ops-capacity.yaml" ||
		!sha256Pattern.MatchString(report.FixtureSHA256) || report.FixedNowUnix != 1768665599 ||
		!sha256Pattern.MatchString(report.AuthoritativeSchemaSHA256) {
		return errors.New("A25 final report header is invalid")
	}
	if len(report.RepositoryMigrations) != len(repository) {
		return fmt.Errorf("A25 migration inventory count is stale: report=%d repository=%d",
			len(report.RepositoryMigrations), len(repository))
	}
	for index, expected := range repository {
		actual := report.RepositoryMigrations[index]
		if actual.Version != expected.Version || actual.Checksum != expected.Checksum || !sha256Pattern.MatchString(actual.Checksum) {
			return fmt.Errorf("A25 migration inventory differs at index %d", index)
		}
	}
	gate := report.VersionGate
	if !strings.HasPrefix(gate.CurrentVersion, "8.4.") || !strings.HasPrefix(gate.LegacyMySQLVersion, "5.7.") ||
		!strings.Contains(strings.ToLower(gate.MariaDBVersion), "mariadb") || !gate.CurrentAccepted ||
		!gate.LegacyMySQLRejected || !gate.MariaDBRejected || gate.LegacyTablesBefore != 0 || gate.LegacyTablesAfter != 0 ||
		gate.MariaDBTablesBefore != 0 || gate.MariaDBTablesAfter != 0 || !gate.LegacyLockAbsentBefore ||
		!gate.LegacyLockAbsentAfter || !gate.MariaDBLockAbsentBefore || !gate.MariaDBLockAbsentAfter {
		return errors.New("A25 version gate proof is invalid")
	}
	empty := report.EmptyDatabase
	if empty.TablesBefore != 0 || empty.MigrationCount != len(repository) || empty.ProgressRows != 0 ||
		empty.SchemaSHA256 != report.AuthoritativeSchemaSHA256 || !empty.AppliedAtStable || !empty.IdempotentSchemaStable {
		return errors.New("A25 empty database and idempotency proof is invalid")
	}
	upgrade := report.Upgrade
	if upgrade.PrefixMigrationCount != 1 || upgrade.HistoricalRows <= 0 || !sha256Pattern.MatchString(upgrade.HistoricalSHA256) ||
		!upgrade.HistoricalPreserved || !upgrade.ForeignKeysPreserved || !upgrade.BackfillScopeMigrated ||
		upgrade.SchemaSHA256 != report.AuthoritativeSchemaSHA256 || !upgrade.MatchesAuthoritative {
		return errors.New("A25 historical upgrade proof is invalid")
	}
	tamper := report.Tamper
	if !tamper.DatabaseChecksumRejected || !tamper.RepositorySourceRejected || !tamper.UnknownVersionRejected ||
		!tamper.NoSchemaMutation || tamper.ProgressRows != 0 {
		return errors.New("A25 tamper rejection proof is invalid")
	}
	dml := report.DMLFailure
	if !dml.InitialFailureObserved || !dml.CheckpointReady || dml.CheckpointIndex != 0 || !dml.ResumeCompleted ||
		dml.IdempotentRowCount != 1 || dml.ProgressRows != 0 {
		return errors.New("A25 transactional recovery proof is invalid")
	}
	ddl := report.DDLRecovery
	if !ddl.DirtyWithoutDDLReplayed || !ddl.DirtyWithDDLRecognized ||
		ddl.ReplaySchemaSHA256 != report.AuthoritativeSchemaSHA256 ||
		ddl.CommittedSchemaSHA256 != report.AuthoritativeSchemaSHA256 || ddl.ProgressRows != 0 {
		return errors.New("A25 DDL dirty checkpoint recovery proof is invalid")
	}
	return nil
}

func validateEnvironment(environment environmentReport, class string) error {
	if environment.SchemaVersion != 1 || environment.AcceptanceID != AcceptanceID || environment.EvidenceClass != class ||
		(environment.Commit != "unborn" && !commitPattern.MatchString(environment.Commit)) || !environment.IsolatedGuard ||
		!environment.Network.Internal || environment.Network.HostPorts == nil || len(environment.Network.HostPorts) != 0 ||
		environment.Databases.Current != "pilot_a25" || environment.Databases.Legacy != "pilot_a25_legacy" ||
		environment.Databases.MariaDB != "pilot_a25_mariadb" || !environment.RepositoryReadOnly ||
		!environment.EvidenceWritable || !environment.OfflineTestNetwork {
		return errors.New("A25 environment isolation contract is invalid")
	}
	current := environment.Servers.Current
	if !strings.HasPrefix(current.Version, "8.4.") || current.TransactionIsolation != "READ-COMMITTED" ||
		current.CharacterSetServer != "utf8mb4" || current.CollationServer != "utf8mb4_unicode_ci" || current.TimeZone != "+08:00" ||
		!strings.HasPrefix(environment.Servers.Legacy.Version, "5.7.") ||
		!strings.Contains(strings.ToLower(environment.Servers.MariaDB.Version), "mariadb") {
		return errors.New("A25 server environment contract is invalid")
	}
	images := []struct {
		identity imageIdentity
		want     string
	}{
		{environment.Images.Go, "golang:1.25.1"},
		{environment.Images.Current, "mysql:8.4"},
		{environment.Images.Legacy, "mysql:5.7"},
		{environment.Images.MariaDB, "mariadb:10.11"},
	}
	for _, image := range images {
		if image.identity.Reference != image.want || !strings.HasPrefix(image.identity.ID, "sha256:") ||
			!sha256Pattern.MatchString(strings.TrimPrefix(image.identity.ID, "sha256:")) ||
			!strings.Contains(image.identity.Digest, "@sha256:") ||
			!sha256Pattern.MatchString(image.identity.Digest[strings.LastIndex(image.identity.Digest, "@sha256:")+8:]) {
			return fmt.Errorf("A25 image identity for %s is invalid", image.want)
		}
	}
	resources := []string{environment.Resources.Network, environment.Resources.ModuleCache, environment.Resources.BuildCache}
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if !resourcePattern.MatchString(resource) {
			return fmt.Errorf("A25 resource name %q is invalid", resource)
		}
		if _, duplicate := seen[resource]; duplicate {
			return errors.New("A25 resource names are not unique")
		}
		seen[resource] = struct{}{}
	}
	return nil
}

func validateTestStream(path string, summary testSummary) error {
	if summary.SchemaVersion != 1 || summary.AcceptanceID != AcceptanceID || summary.Status != "passed" ||
		summary.TargetTest != "TestA25MigrationAcceptance" || summary.Package != "new-api-pilot/tests/integration" ||
		summary.PassEvents != 1 || summary.FailEvents != 0 || summary.SkipEvents != 0 || summary.NoTests ||
		summary.JSONLines <= 0 || summary.JSONPath != "a25-test.jsonl" || summary.StderrPath != "a25-test.stderr.log" {
		return errors.New("A25 test summary contract is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maxLogSize+1))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	lines, passes, failures, skips := 0, 0, 0, 0
	noTests := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("A25 test stream line %d is invalid JSON: %w", lines, err)
		}
		if event.Action == "skip" {
			skips++
		}
		if event.Action == "fail" {
			failures++
		}
		if event.Test == summary.TargetTest && event.Action == "pass" {
			passes++
		}
		if noTestsPattern.MatchString(event.Output) {
			noTests = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if lines != summary.JSONLines || passes != summary.PassEvents || failures != summary.FailEvents ||
		skips != summary.SkipEvents || noTests != summary.NoTests {
		return errors.New("A25 raw test stream does not match its summary")
	}
	return nil
}

func validateCleanup(runDirectory, class string) error {
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate A25 cleanup report: %w", err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != AcceptanceID || cleanup.EvidenceClass != class ||
		!cleanup.Passed || !cleanup.SweepsSucceeded || cleanup.Lifecycle.Containers != "created_and_removed" ||
		cleanup.Lifecycle.Networks != "created_and_removed" || cleanup.Lifecycle.Volumes != "created_and_removed" ||
		cleanup.Residuals.Containers == nil || cleanup.Residuals.Networks == nil || cleanup.Residuals.Volumes == nil ||
		cleanup.Residuals.Images == nil || len(cleanup.Residuals.Containers) != 0 || len(cleanup.Residuals.Networks) != 0 ||
		len(cleanup.Residuals.Volumes) != 0 || len(cleanup.Residuals.Images) != 0 {
		return errors.New("A25 cleanup report contract is invalid")
	}
	return nil
}

func validateInventory(runDirectory, class string) error {
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, "a25-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate A25 artifact inventory: %w", err)
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != AcceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A25 artifact inventory contract is invalid")
	}
	wanted := make(map[string]struct{}, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for _, entry := range inventory.Files {
		if _, exists := wanted[entry.Path]; !exists || filepath.IsAbs(entry.Path) || filepath.Base(entry.Path) != entry.Path {
			return fmt.Errorf("A25 inventory contains unexpected path %q", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("A25 inventory repeats %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		path := filepath.Join(runDirectory, entry.Path)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes || entry.SizeBytes < 0 {
			return fmt.Errorf("A25 inventory size mismatch for %q", entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 || !sha256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("A25 inventory SHA-256 mismatch for %q", entry.Path)
		}
	}
	return nil
}

func validateLogFile(path string) error {
	payload, err := readBounded(path, maxLogSize)
	if err != nil {
		return err
	}
	if secretPattern.Match(payload) {
		return errors.New("A25 test stderr contains a credential or DSN")
	}
	return nil
}

func decodeJSONFile(path string, target any) error {
	payload, err := readBounded(path, maxJSONSize)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON file contains trailing data")
	}
	return nil
}

func readBounded(path string, maximum int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file size %d exceeds contract", info.Size())
	}
	return os.ReadFile(path)
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("A25 evidence path is not a directory")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
