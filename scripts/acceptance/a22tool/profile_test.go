package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validA22Profile = `schema_version: 2
fixture_id: F05
clock:
  timezone: Asia/Shanghai
  now: "2026-01-17T23:59:59+08:00"
  now_unix: 1768665599
mysql:
  version: "8.4"
  transaction_isolation: READ-COMMITTED
  charset: utf8mb4
  collation: utf8mb4_unicode_ci
  binlog_enabled: true
  rpo_seconds: 3600
  rto_seconds: 14400
  migration_checksum_tamper_must_fail: true
`

func TestLoadProfileEnforcesF05A22Contract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f05.yaml")
	if err := os.WriteFile(path, []byte(validA22Profile), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, digest, err := loadProfile(path)
	if err != nil {
		t.Fatalf("load valid A22 profile: %v", err)
	}
	if len(digest) != 64 || fixtureHour(profile) != 1768662000 || fixtureDateKey(profile) != 20260117 {
		t.Fatalf("profile digest/hour/date = %q/%d/%d", digest, fixtureHour(profile), fixtureDateKey(profile))
	}

	tampered := strings.Replace(validA22Profile, "rpo_seconds: 3600", "rpo_seconds: 3599", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadProfile(path); err == nil {
		t.Fatal("tampered F05 recovery objective was accepted")
	}
}

func TestSeedEnvironmentErrorDoesNotEchoSecrets(t *testing.T) {
	values := map[string]string{
		"DATABASE_DSN":       "secret-dsn-never-echo",
		"ENCRYPTION_KEY":     "secret-key-never-echo",
		"A22_ADMIN_PASSWORD": "secret-password-never-echo",
		"A22_SITE_TOKEN":     "secret-token-never-echo",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	t.Setenv("A22_SECRET_SETTING", "")
	_, _, _, _, _, err := seedEnvironment()
	if err == nil {
		t.Fatal("incomplete secret environment was accepted")
	}
	for _, secret := range values {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("secret environment value leaked in error: %v", err)
		}
	}
}
