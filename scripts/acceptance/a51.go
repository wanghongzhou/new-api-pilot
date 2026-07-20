package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/internal/ops"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
)

const (
	a51AcceptanceID = "A51"
	a51DatabaseName = "pilot_a51"
)

var a51RequiredPreflightVariables = []string{
	"APP_ENV",
	"PORT",
	"DATABASE_DSN",
	"SQL_MAX_IDLE_CONNS",
	"SQL_MAX_OPEN_CONNS",
	"SQL_MAX_LIFETIME_SECONDS",
	"SESSION_SECRET",
	"ENCRYPTION_KEY",
	"SESSION_COOKIE_SECURE",
	"EXPORT_DIR",
	"PUBLIC_ORIGIN",
	"UPSTREAM_ALLOWED_CIDRS",
	"UPSTREAM_CONNECT_TIMEOUT_SECONDS",
	"UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS",
	"UPSTREAM_REQUEST_TIMEOUT_SECONDS",
	"UPSTREAM_EXPORT_TIMEOUT_SECONDS",
	"METRICS_ALLOWED_CIDRS",
	"TZ",
	"OLD_ENCRYPTION_KEY",
	"NEW_ENCRYPTION_KEY",
}

var a51SiteSecrets = []struct {
	Name      string
	Plaintext string
}{
	{Name: "A51 加密站点 Alpha", Plaintext: "a51-site-token-alpha-never-log"},
	{Name: "A51 加密站点 Beta", Plaintext: "a51-site-token-beta-never-log"},
}

var a51SettingSecrets = []struct {
	Key       string
	Plaintext string
	UseNewKey bool
}{
	{Key: "a51_dingtalk_webhook", Plaintext: "https://oapi.dingtalk.com/robot/send?access_token=a51-never-log"},
	{Key: "a51_dingtalk_signing_secret", Plaintext: "a51-signing-secret-never-log", UseNewKey: true},
}

type a51Fixture struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Encryption    struct {
		Algorithm              string `yaml:"algorithm"`
		KeyBytes               int    `yaml:"key_bytes"`
		NonceBytes             int    `yaml:"nonce_bytes"`
		FixtureOldKeyID        string `yaml:"fixture_old_key_id"`
		FixtureNewKeyID        string `yaml:"fixture_new_key_id"`
		ContainsSecretMaterial bool   `yaml:"contains_secret_material"`
	} `yaml:"encryption"`
}

type a51SeedReport struct {
	SchemaVersion           int             `json:"schema_version"`
	AcceptanceID            string          `json:"acceptance_id"`
	Status                  string          `json:"status"`
	EvidenceClass           string          `json:"evidence_class"`
	FixturePath             string          `json:"fixture_path"`
	FixtureSHA256           string          `json:"fixture_sha256"`
	OldKeyID                string          `json:"old_key_id"`
	NewKeyID                string          `json:"new_key_id"`
	SiteTokensOld           int             `json:"site_tokens_old"`
	SecretSettingsOld       int             `json:"secret_settings_old"`
	SecretSettingsNew       int             `json:"secret_settings_new"`
	HistoricalCompletedJobs int             `json:"historical_completed_jobs"`
	CiphertextSHA256        []string        `json:"ciphertext_sha256"`
	Checks                  map[string]bool `json:"checks"`
	GeneratedAt             string          `json:"generated_at"`
}

type a51VerifyReport struct {
	SchemaVersion         int             `json:"schema_version"`
	AcceptanceID          string          `json:"acceptance_id"`
	Status                string          `json:"status"`
	EvidenceClass         string          `json:"evidence_class"`
	OldKeyID              string          `json:"old_key_id"`
	NewKeyID              string          `json:"new_key_id"`
	VerifiedSiteTokens    int             `json:"verified_site_tokens"`
	VerifiedSettings      int             `json:"verified_secret_settings"`
	ActiveMaintenanceJobs int64           `json:"active_maintenance_jobs"`
	CurrentCompletedJobs  int64           `json:"current_completed_jobs"`
	CompletedHistoryJobs  int64           `json:"completed_history_jobs"`
	MaintenanceItems      int64           `json:"maintenance_items"`
	CiphertextSHA256      []string        `json:"ciphertext_sha256"`
	Checks                map[string]bool `json:"checks"`
	GeneratedAt           string          `json:"generated_at"`
}

type a51IntegrationSummary struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	Status        string   `json:"status"`
	TestsPassed   int      `json:"tests_passed"`
	TestsFailed   int      `json:"tests_failed"`
	TestsSkipped  int      `json:"tests_skipped"`
	TestNames     []string `json:"test_names"`
	LogPath       string   `json:"log_path"`
}

type a51ScanReport struct {
	SchemaVersion  int    `json:"schema_version"`
	AcceptanceID   string `json:"acceptance_id"`
	Status         string `json:"status"`
	FilesScanned   int    `json:"files_scanned"`
	ForbiddenHits  int    `json:"forbidden_hits"`
	FullKeyIDLeaks int    `json:"full_key_id_leaks"`
}

type a51EnvironmentReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	EvidenceClass string `json:"evidence_class"`
	Commit        string `json:"commit"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	MySQL         struct {
		Version              string `json:"version"`
		TransactionIsolation string `json:"transaction_isolation"`
		CharacterSetServer   string `json:"character_set_server"`
		CollationServer      string `json:"collation_server"`
		TimeZone             string `json:"time_zone"`
	} `json:"mysql"`
	Network struct {
		Internal  bool  `json:"internal"`
		HostPorts []int `json:"host_ports"`
	} `json:"network"`
	Databases []string `json:"databases"`
}

type a51FinalReport struct {
	SchemaVersion      int                   `json:"schema_version"`
	AcceptanceID       string                `json:"acceptance_id"`
	Status             string                `json:"status"`
	Passed             bool                  `json:"passed"`
	EvidenceClass      string                `json:"evidence_class"`
	AcceptanceEligible bool                  `json:"acceptance_eligible"`
	FixtureSHA256      string                `json:"fixture_sha256"`
	OldKeyID           string                `json:"old_key_id"`
	NewKeyID           string                `json:"new_key_id"`
	Violations         []string              `json:"violations"`
	Seed               a51SeedReport         `json:"seed"`
	DryRun             ops.ReencryptReport   `json:"dry_run"`
	FullRun            ops.ReencryptReport   `json:"full_run"`
	PostDryRun         ops.ReencryptReport   `json:"post_dry_run"`
	Verify             a51VerifyReport       `json:"verify"`
	Integration        a51IntegrationSummary `json:"integration"`
	Scan               a51ScanReport         `json:"scan"`
	Environment        a51EnvironmentReport  `json:"environment"`
	GeneratedAt        string                `json:"generated_at"`
}

func runA51Preflight(arguments []string, stdout io.Writer, stderr io.Writer) int {
	if len(arguments) != 0 {
		fmt.Fprintln(stderr, "a51-preflight does not accept arguments")
		return 2
	}
	if os.Getenv("ACCEPTANCE_ID") != a51AcceptanceID || os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != "formal" ||
		os.Getenv("A51_CONFIG_PREFLIGHT") != "true" {
		fmt.Fprintln(stderr, "A51 configuration preflight requires the formal acceptance guard")
		return 2
	}
	if err := validateA51Preflight(os.LookupEnv); err != nil {
		fmt.Fprintf(stderr, "A51 configuration preflight: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "A51 configuration preflight passed")
	return 0
}

func validateA51Preflight(lookup config.LookupFunc) error {
	for _, name := range a51RequiredPreflightVariables {
		raw, exists := lookup(name)
		if !exists || strings.TrimSpace(raw) == "" {
			return fmt.Errorf("%s is required by the A51 acceptance environment", name)
		}
	}

	application, err := config.LoadFrom(lookup)
	if err != nil {
		return fmt.Errorf("application configuration: %w", err)
	}
	maintenance, err := config.LoadReencryptFrom(lookup)
	if err != nil {
		return fmt.Errorf("maintenance configuration: %w", err)
	}
	parsedDSN, err := mysqldriver.ParseDSN(application.DatabaseDSN)
	if err != nil {
		return errors.New("DATABASE_DSN could not be parsed after validation")
	}
	if application.AppEnv != config.EnvironmentTest || application.Port != "3000" || application.TZ != "Asia/Shanghai" ||
		application.SessionCookieSecure || application.PublicOrigin != "http://a51.invalid" {
		return errors.New("application runtime identity does not match the A51 isolated contract")
	}
	if parsedDSN.Net != "tcp" || parsedDSN.Addr != "mysql-a51:3306" || parsedDSN.DBName != a51DatabaseName ||
		parsedDSN.User != "root" || parsedDSN.Passwd != "" {
		return errors.New("DATABASE_DSN does not target the isolated A51 MySQL schema")
	}
	if application.SQLMaxIdleConns != 2 || application.SQLMaxOpenConns != 4 || application.SQLMaxLifetime != time.Minute ||
		maintenance.Database.SQLMaxIdleConns != 2 || maintenance.Database.SQLMaxOpenConns != 4 ||
		maintenance.Database.SQLMaxLifetime != time.Minute || maintenance.Database.DatabaseDSN != application.DatabaseDSN {
		return errors.New("database pool configuration does not match the A51 bounded contract")
	}
	if !bytes.Equal(application.EncryptionKey, maintenance.OldKey) {
		return errors.New("ENCRYPTION_KEY must match OLD_ENCRYPTION_KEY for A51 migration")
	}
	if len(application.UpstreamAllowedHostSuffixes) != 0 || len(application.UpstreamAllowedCIDRs) != 1 ||
		application.UpstreamAllowedCIDRs[0].String() != "172.16.0.0/12" ||
		application.UpstreamConnectTimeout != 5*time.Second || application.UpstreamHeaderTimeout != 15*time.Second ||
		application.UpstreamRequestTimeout != 30*time.Second || application.UpstreamExportTimeout != 120*time.Second {
		return errors.New("upstream boundary does not match the A51 bounded contract")
	}
	if len(application.MetricsAllowedCIDRs) != 1 || application.MetricsAllowedCIDRs[0].String() != "127.0.0.0/8" ||
		len(application.DingTalkAllowedHosts) != 1 || application.DingTalkAllowedHosts[0] != "oapi.dingtalk.com" {
		return errors.New("monitoring boundary does not match the A51 isolated contract")
	}
	return nil
}

func runA51Seed(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a51-seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fixturePath := flags.String("fixture", "testdata/design/f05-ops-capacity.yaml", "F05 fixture path")
	reportPath := flags.String("report", "a51-seed-report.json", "seed report")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	fixtureSHA, err := validateA51FixtureFile(*fixturePath)
	if err != nil {
		fmt.Fprintf(stderr, "validate A51 fixture: %v\n", err)
		return 2
	}
	maintenance, database, ctx, cancel, err := openA51Database()
	if err != nil {
		fmt.Fprintf(stderr, "open isolated A51 database: %v\n", err)
		return 2
	}
	defer cancel()
	defer database.Close()
	oldCipher, _ := common.NewCipher(maintenance.OldKey)
	newCipher, _ := common.NewCipher(maintenance.NewKey)
	report, err := seedA51(ctx, database, oldCipher, newCipher, *fixturePath, fixtureSHA)
	if err != nil {
		fmt.Fprintf(stderr, "seed A51 encrypted fixture: %v\n", err)
		return 1
	}
	if err := writeJSONAtomic(*reportPath, report); err != nil {
		fmt.Fprintf(stderr, "write A51 seed report: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "A51 encrypted fixture seeded: rows=%d report=%s\n", len(report.CiphertextSHA256), *reportPath)
	return 0
}

func runA51Verify(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a51-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reportPath := flags.String("report", "a51-verify.json", "verification report")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	maintenance, database, ctx, cancel, err := openA51Database()
	if err != nil {
		fmt.Fprintf(stderr, "open isolated A51 database: %v\n", err)
		return 2
	}
	defer cancel()
	defer database.Close()
	oldCipher, _ := common.NewCipher(maintenance.OldKey)
	newCipher, _ := common.NewCipher(maintenance.NewKey)
	report, err := verifyA51(ctx, database, oldCipher, newCipher)
	if err != nil {
		fmt.Fprintf(stderr, "verify A51 re-encryption: %v\n", err)
		return 1
	}
	if err := writeJSONAtomic(*reportPath, report); err != nil {
		fmt.Fprintf(stderr, "write A51 verification report: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "A51 re-encryption verified: rows=%d report=%s\n", len(report.CiphertextSHA256), *reportPath)
	return 0
}

func runA51Report(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a51-report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	seedPath := flags.String("seed", "a51-seed-report.json", "seed report")
	dryPath := flags.String("dry-run", "a51-dry-run.json", "dry-run report")
	fullPath := flags.String("full-run", "a51-full.json", "full re-encryption report")
	postPath := flags.String("post-dry-run", "a51-post-dry-run.json", "post dry-run report")
	verifyPath := flags.String("verify", "a51-verify.json", "verification report")
	integrationPath := flags.String("integration", "a51-integration-tests.json", "integration summary")
	scanPath := flags.String("scan", "a51-secret-scan.json", "secret scan report")
	environmentPath := flags.String("environment", "a51-environment.json", "environment report")
	outputPath := flags.String("output", "a51-report.json", "final report")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	if os.Getenv("ACCEPTANCE_ID") != a51AcceptanceID || os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != "formal" ||
		os.Getenv("A51_ISOLATED_REPORT") != "true" {
		fmt.Fprintln(stderr, "A51 report requires the formal isolated acceptance environment")
		return 2
	}
	seed, err := readA51JSON[a51SeedReport](*seedPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 seed report: %v\n", err)
		return 1
	}
	dry, err := readA51JSON[ops.ReencryptReport](*dryPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 dry-run report: %v\n", err)
		return 1
	}
	full, err := readA51JSON[ops.ReencryptReport](*fullPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 full report: %v\n", err)
		return 1
	}
	post, err := readA51JSON[ops.ReencryptReport](*postPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 post report: %v\n", err)
		return 1
	}
	verify, err := readA51JSON[a51VerifyReport](*verifyPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 verify report: %v\n", err)
		return 1
	}
	integration, err := readA51JSON[a51IntegrationSummary](*integrationPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 integration summary: %v\n", err)
		return 1
	}
	scan, err := readA51JSON[a51ScanReport](*scanPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 scan report: %v\n", err)
		return 1
	}
	environment, err := readA51JSON[a51EnvironmentReport](*environmentPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A51 environment report: %v\n", err)
		return 1
	}
	report := buildA51FinalReport(seed, dry, full, post, verify, integration, scan, environment)
	if err := writeJSONAtomic(*outputPath, report); err != nil {
		fmt.Fprintf(stderr, "write A51 report: %v\n", err)
		return 1
	}
	if !report.Passed {
		fmt.Fprintf(stderr, "A51 report failed with %d violation(s)\n", len(report.Violations))
		return 1
	}
	fmt.Fprintf(stdout, "A51 formal report passed: %s\n", *outputPath)
	return 0
}

func validateA51FixtureFile(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var fixture a51Fixture
	if err := yaml.Unmarshal(payload, &fixture); err != nil {
		return "", err
	}
	if fixture.SchemaVersion != 2 || fixture.FixtureID != "F05" || fixture.Encryption.Algorithm != "AES-256-GCM" ||
		fixture.Encryption.KeyBytes != 32 || fixture.Encryption.NonceBytes != 12 || fixture.Encryption.ContainsSecretMaterial ||
		!validA51SHA(fixture.Encryption.FixtureOldKeyID) || !validA51SHA(fixture.Encryption.FixtureNewKeyID) ||
		fixture.Encryption.FixtureOldKeyID == fixture.Encryption.FixtureNewKeyID {
		return "", errors.New("F05 encryption contract is invalid")
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func openA51Database() (config.ReencryptConfig, *sql.DB, context.Context, context.CancelFunc, error) {
	if os.Getenv("ACCEPTANCE_ID") != a51AcceptanceID || os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != "formal" ||
		os.Getenv("A51_ISOLATED_MYSQL") != "true" {
		return config.ReencryptConfig{}, nil, nil, nil, errors.New("A51 formal isolated acceptance guards are required")
	}
	maintenance, err := config.LoadReencrypt()
	if err != nil {
		return config.ReencryptConfig{}, nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	database, err := sql.Open("mysql", maintenance.Database.DatabaseDSN)
	if err != nil {
		cancel()
		return config.ReencryptConfig{}, nil, nil, nil, err
	}
	var name string
	if err := database.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil || name != a51DatabaseName {
		_ = database.Close()
		cancel()
		return config.ReencryptConfig{}, nil, nil, nil, errors.New("A51 database is not the isolated acceptance schema")
	}
	return maintenance, database, ctx, cancel, nil
}

func seedA51(
	ctx context.Context,
	database *sql.DB,
	oldCipher *common.Cipher,
	newCipher *common.Cipher,
	fixturePath string,
	fixtureSHA string,
) (a51SeedReport, error) {
	report := a51SeedReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a51AcceptanceID, Status: "failed", EvidenceClass: "formal",
		FixturePath: fixturePath, FixtureSHA256: fixtureSHA, OldKeyID: ops.ShortFingerprint(oldCipher.KeyID()),
		NewKeyID: ops.ShortFingerprint(newCipher.KeyID()), CiphertextSHA256: []string{}, Checks: map[string]bool{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer transaction.Rollback()
	for _, table := range []string{"site", "platform_setting", "encryption_reencrypt_job", "encryption_reencrypt_item"} {
		var count int64
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			return report, fmt.Errorf("A51 requires empty table %s", table)
		}
	}
	now := time.Now().Unix()
	for _, secret := range a51SiteSecrets {
		result, err := transaction.ExecContext(ctx,
			"INSERT INTO site (name, base_url, created_at, updated_at) VALUES (?, ?, ?, ?)",
			secret.Name, fmt.Sprintf("https://a51-%d.invalid", len(report.CiphertextSHA256)+1), now, now,
		)
		if err != nil {
			return report, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return report, err
		}
		ciphertext, err := oldCipher.Encrypt([]byte(secret.Plaintext), fmt.Sprintf("site:%d:access_token", id))
		if err != nil {
			return report, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE site SET access_token_encrypted = ? WHERE id = ?", ciphertext, id); err != nil {
			return report, err
		}
		report.CiphertextSHA256 = append(report.CiphertextSHA256, a51TextSHA(ciphertext))
		report.SiteTokensOld++
	}
	for _, secret := range a51SettingSecrets {
		cipher := oldCipher
		if secret.UseNewKey {
			cipher = newCipher
		}
		ciphertext, err := cipher.Encrypt([]byte(secret.Plaintext), "setting:"+secret.Key)
		if err != nil {
			return report, err
		}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO platform_setting
  (setting_key, setting_value, value_type, is_secret, updated_at) VALUES (?, ?, 'string', 1, ?)`, secret.Key, ciphertext, now); err != nil {
			return report, err
		}
		report.CiphertextSHA256 = append(report.CiphertextSHA256, a51TextSHA(ciphertext))
		if secret.UseNewKey {
			report.SecretSettingsNew++
		} else {
			report.SecretSettingsOld++
		}
	}
	historicalOldKeyID := strings.Repeat("a", sha256.Size*2)
	historicalNewKeyID := strings.Repeat("b", sha256.Size*2)
	historicalInventory := strings.Repeat("c", sha256.Size*2)
	if _, err := transaction.ExecContext(ctx, `INSERT INTO encryption_reencrypt_job
  (old_key_id, new_key_id, active_key, state, inventory_hash, total_items, staged_items, created_at, updated_at)
  VALUES (?, ?, NULL, 'complete', ?, 1, 1, ?, ?)`,
		historicalOldKeyID, historicalNewKeyID, historicalInventory, now-60, now-60); err != nil {
		return report, err
	}
	report.HistoricalCompletedJobs = 1
	if err := transaction.Commit(); err != nil {
		return report, err
	}
	sort.Strings(report.CiphertextSHA256)
	report.Checks["fixture_contract"] = validA51SHA(fixtureSHA)
	report.Checks["old_and_new_keys_distinct"] = oldCipher.KeyID() != newCipher.KeyID()
	report.Checks["expected_secret_inventory"] = report.SiteTokensOld == 2 && report.SecretSettingsOld == 1 && report.SecretSettingsNew == 1
	report.Checks["ciphertexts_not_plaintext"] = len(report.CiphertextSHA256) == 4
	report.Checks["completed_history_boundary"] = report.HistoricalCompletedJobs == 1
	for _, passed := range report.Checks {
		if !passed {
			return report, errors.New("A51 seed invariant failed")
		}
	}
	report.Status = "passed"
	return report, nil
}

func verifyA51(ctx context.Context, database *sql.DB, oldCipher, newCipher *common.Cipher) (a51VerifyReport, error) {
	report := a51VerifyReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a51AcceptanceID, Status: "failed", EvidenceClass: "formal",
		OldKeyID: ops.ShortFingerprint(oldCipher.KeyID()), NewKeyID: ops.ShortFingerprint(newCipher.KeyID()),
		CiphertextSHA256: []string{}, Checks: map[string]bool{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	expectedSites := make(map[string]string, len(a51SiteSecrets))
	for _, secret := range a51SiteSecrets {
		expectedSites[secret.Name] = secret.Plaintext
	}
	rows, err := database.QueryContext(ctx, `SELECT id, name, access_token_encrypted
FROM site WHERE name LIKE 'A51 加密站点 %' ORDER BY id ASC`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var id int64
		var name, ciphertext string
		if err := rows.Scan(&id, &name, &ciphertext); err != nil {
			_ = rows.Close()
			return report, err
		}
		expected, exists := expectedSites[name]
		plaintext, decryptErr := newCipher.Decrypt(ciphertext, fmt.Sprintf("site:%d:access_token", id))
		_, oldErr := oldCipher.Decrypt(ciphertext, fmt.Sprintf("site:%d:access_token", id))
		if !exists || decryptErr != nil || string(plaintext) != expected || oldErr == nil || strings.Contains(ciphertext, expected) {
			_ = rows.Close()
			return report, errors.New("A51 site token verification failed")
		}
		report.VerifiedSiteTokens++
		report.CiphertextSHA256 = append(report.CiphertextSHA256, a51TextSHA(ciphertext))
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	expectedSettings := make(map[string]string, len(a51SettingSecrets))
	for _, secret := range a51SettingSecrets {
		expectedSettings[secret.Key] = secret.Plaintext
	}
	rows, err = database.QueryContext(ctx, `SELECT setting_key, setting_value
FROM platform_setting WHERE setting_key LIKE 'a51_%' ORDER BY setting_key ASC`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var key, ciphertext string
		if err := rows.Scan(&key, &ciphertext); err != nil {
			_ = rows.Close()
			return report, err
		}
		expected, exists := expectedSettings[key]
		plaintext, decryptErr := newCipher.Decrypt(ciphertext, "setting:"+key)
		_, oldErr := oldCipher.Decrypt(ciphertext, "setting:"+key)
		if !exists || decryptErr != nil || string(plaintext) != expected || oldErr == nil || strings.Contains(ciphertext, expected) {
			_ = rows.Close()
			return report, errors.New("A51 setting secret verification failed")
		}
		report.VerifiedSettings++
		report.CiphertextSHA256 = append(report.CiphertextSHA256, a51TextSHA(ciphertext))
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM encryption_reencrypt_job
WHERE active_key IS NOT NULL OR state <> 'complete'`).Scan(&report.ActiveMaintenanceJobs); err != nil {
		return report, err
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM encryption_reencrypt_job
WHERE active_key IS NULL AND state = 'complete' AND old_key_id = ? AND new_key_id = ?
  AND total_items = ? AND staged_items = ?`, oldCipher.KeyID(), newCipher.KeyID(), 4, 4).Scan(&report.CurrentCompletedJobs); err != nil {
		return report, err
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM encryption_reencrypt_job
WHERE active_key IS NULL AND state = 'complete'`).Scan(&report.CompletedHistoryJobs); err != nil {
		return report, err
	}
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM encryption_reencrypt_item").Scan(&report.MaintenanceItems); err != nil {
		return report, err
	}
	sort.Strings(report.CiphertextSHA256)
	report.Checks["all_site_tokens_new_key_only"] = report.VerifiedSiteTokens == len(a51SiteSecrets)
	report.Checks["all_settings_new_key_only"] = report.VerifiedSettings == len(a51SettingSecrets)
	report.Checks["no_active_or_nonterminal_jobs"] = report.ActiveMaintenanceJobs == 0
	report.Checks["maintenance_staging_empty"] = report.MaintenanceItems == 0
	report.Checks["current_completed_job_unique"] = report.CurrentCompletedJobs == 1
	report.Checks["completed_history_preserved"] = report.CompletedHistoryJobs == 2
	report.Checks["ciphertext_inventory_complete"] = len(report.CiphertextSHA256) == len(a51SiteSecrets)+len(a51SettingSecrets)
	failedChecks := make([]string, 0, len(report.Checks))
	for name, passed := range report.Checks {
		if !passed {
			failedChecks = append(failedChecks, name)
		}
	}
	if len(failedChecks) != 0 {
		sort.Strings(failedChecks)
		return report, fmt.Errorf("A51 verification invariant failed: %s", strings.Join(failedChecks, ","))
	}
	report.Status = "passed"
	return report, nil
}

func buildA51FinalReport(
	seed a51SeedReport,
	dry ops.ReencryptReport,
	full ops.ReencryptReport,
	post ops.ReencryptReport,
	verify a51VerifyReport,
	integration a51IntegrationSummary,
	scan a51ScanReport,
	environment a51EnvironmentReport,
) a51FinalReport {
	report := a51FinalReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a51AcceptanceID, Status: "failed", EvidenceClass: "formal",
		AcceptanceEligible: true, FixtureSHA256: seed.FixtureSHA256, OldKeyID: seed.OldKeyID, NewKeyID: seed.NewKeyID,
		Violations: []string{}, Seed: seed, DryRun: dry, FullRun: full, PostDryRun: post, Verify: verify,
		Integration: integration, Scan: scan, Environment: environment, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if seed.SchemaVersion != evidenceSchemaVersion || seed.AcceptanceID != a51AcceptanceID || seed.Status != "passed" ||
		seed.EvidenceClass != "formal" || !validA51SHA(seed.FixtureSHA256) || seed.OldKeyID == seed.NewKeyID ||
		seed.SiteTokensOld != 2 || seed.SecretSettingsOld != 1 || seed.SecretSettingsNew != 1 ||
		seed.HistoricalCompletedJobs != 1 || len(seed.CiphertextSHA256) != 4 {
		report.Violations = append(report.Violations, "seed_contract")
	}
	if !validA51ReencryptReport(dry, true, false, 3, 1, 0) {
		report.Violations = append(report.Violations, "dry_run_contract")
	}
	if !validA51ReencryptReport(full, false, false, 3, 1, 3) {
		report.Violations = append(report.Violations, "full_run_contract")
	}
	if !validA51ReencryptReport(post, true, false, 0, 4, 0) {
		report.Violations = append(report.Violations, "post_dry_run_contract")
	}
	if dry.OldKeyID != seed.OldKeyID || dry.NewKeyID != seed.NewKeyID || full.OldKeyID != seed.OldKeyID ||
		full.NewKeyID != seed.NewKeyID || post.OldKeyID != seed.OldKeyID || post.NewKeyID != seed.NewKeyID {
		report.Violations = append(report.Violations, "key_identity_contract")
	}
	if verify.SchemaVersion != evidenceSchemaVersion || verify.AcceptanceID != a51AcceptanceID || verify.Status != "passed" ||
		verify.EvidenceClass != "formal" || verify.OldKeyID != seed.OldKeyID || verify.NewKeyID != seed.NewKeyID ||
		verify.VerifiedSiteTokens != 2 || verify.VerifiedSettings != 2 || verify.ActiveMaintenanceJobs != 0 ||
		verify.CurrentCompletedJobs != 1 || verify.CompletedHistoryJobs != 2 || verify.MaintenanceItems != 0 {
		report.Violations = append(report.Violations, "verification_contract")
	}
	if integration.SchemaVersion != evidenceSchemaVersion || integration.AcceptanceID != a51AcceptanceID ||
		integration.Status != "passed" || integration.TestsPassed != 4 || integration.TestsFailed != 0 ||
		integration.TestsSkipped != 0 || !stringSlicesEqualA51(integration.TestNames, []string{
		"TestMySQLReencryptCASFailureRollsBackAllUpdates",
		"TestMySQLReencryptDryRunAndSuccess",
		"TestMySQLReencryptRejectsBadCiphertextWithoutWrites",
		"TestMySQLReencryptResumesAndRejectsDifferentKeyPair",
	}) || integration.LogPath != "a51-integration-tests.jsonl" {
		report.Violations = append(report.Violations, "fault_injection_contract")
	}
	if scan.SchemaVersion != evidenceSchemaVersion || scan.AcceptanceID != a51AcceptanceID || scan.Status != "passed" ||
		scan.FilesScanned <= 0 || scan.ForbiddenHits != 0 || scan.FullKeyIDLeaks != 0 {
		report.Violations = append(report.Violations, "secret_scan_contract")
	}
	if environment.SchemaVersion != evidenceSchemaVersion || environment.AcceptanceID != a51AcceptanceID ||
		environment.EvidenceClass != "formal" || environment.Commit == "" || !strings.HasPrefix(environment.MySQL.Version, "8.4.") ||
		environment.MySQL.TransactionIsolation != "READ-COMMITTED" || environment.MySQL.CharacterSetServer != "utf8mb4" ||
		environment.MySQL.CollationServer != "utf8mb4_unicode_ci" || environment.MySQL.TimeZone != "+08:00" ||
		!environment.Network.Internal || environment.Network.HostPorts == nil || len(environment.Network.HostPorts) != 0 ||
		!stringSlicesEqualA51(environment.Databases, []string{"pilot_a51", "pilot_a51_tests"}) {
		report.Violations = append(report.Violations, "environment_contract")
	}
	sort.Strings(report.Violations)
	report.Passed = len(report.Violations) == 0
	if report.Passed {
		report.Status = "passed"
	}
	return report
}

func validA51ReencryptReport(report ops.ReencryptReport, dryRun, resumed bool, oldCount, newCount, updated int64) bool {
	return report.SchemaVersion == ops.ReportSchemaVersion && report.Command == "secrets reencrypt" && report.Status == "success" &&
		report.DryRun == dryRun && report.Resumed == resumed && report.Error == nil && len(report.OldKeyID) == 12 &&
		len(report.NewKeyID) == 12 && report.Counts.Total == 4 && report.Counts.SiteTokens == 2 && report.Counts.Settings == 2 &&
		report.Counts.OldKey == oldCount && report.Counts.NewKey == newCount && report.Counts.Updated == updated
}

func readA51JSON[T any](path string) (T, error) {
	var result T
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return result, errors.New("JSON contains multiple values")
		}
		return result, err
	}
	return result, nil
}

func validA51SHA(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func a51TextSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func stringSlicesEqualA51(left, right []string) bool {
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
