package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadFromValidDevelopment(t *testing.T) {
	values := validEnvironment()
	loaded, err := LoadFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if loaded.Port != "3000" || loaded.EncryptionKeyID == "" {
		t.Fatalf("unexpected config: %#v", loaded)
	}
	if len(loaded.UpstreamAllowedCIDRs) != 1 || len(loaded.MetricsAllowedCIDRs) != 1 {
		t.Fatalf("CIDRs were not parsed: %#v", loaded)
	}
	if len(loaded.DingTalkAllowedHosts) != 1 || loaded.DingTalkAllowedHosts[0] != "oapi.dingtalk.com" {
		t.Fatalf("unexpected DingTalk allowlist: %v", loaded.DingTalkAllowedHosts)
	}
}

func TestLoadFromRejectsUnsafeProduction(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = EnvironmentProduction
	values["PUBLIC_ORIGIN"] = "http://pilot.example.com"
	values["SESSION_COOKIE_SECURE"] = "false"
	values["METRICS_ALLOWED_CIDRS"] = ""
	_, err := LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "SESSION_COOKIE_SECURE") {
		t.Fatalf("expected secure cookie error, got %v", err)
	}

	values["SESSION_COOKIE_SECURE"] = "true"
	_, err = LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected HTTPS origin error, got %v", err)
	}

	values["PUBLIC_ORIGIN"] = "https://pilot.example.com"
	_, err = LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Fatalf("expected trusted proxy error, got %v", err)
	}

	values["TRUSTED_PROXIES"] = "10.0.0.0/8"
	_, err = LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "METRICS_ALLOWED_CIDRS") {
		t.Fatalf("expected metrics allowlist error, got %v", err)
	}
}

func TestLoadFromRequiresAbsoluteProductionExportDirectory(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = EnvironmentProduction
	values["SESSION_COOKIE_SECURE"] = "true"
	values["PUBLIC_ORIGIN"] = "https://pilot.example.com"
	values["TRUSTED_PROXIES"] = "10.0.0.0/8"
	values["EXPORT_DIR"] = filepath.Join("data", "exports")
	if _, err := LoadFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative production EXPORT_DIR error = %v", err)
	}
	values["EXPORT_DIR"] = filepath.Join(t.TempDir(), "exports")
	if _, err := LoadFrom(mapLookup(values)); err != nil {
		t.Fatalf("absolute production EXPORT_DIR: %v", err)
	}
}

func TestLoadFromAllowsEmptyTrustedProxiesOutsideProduction(t *testing.T) {
	values := validEnvironment()
	delete(values, "TRUSTED_PROXIES")
	loaded, err := LoadFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if len(loaded.TrustedProxies) != 0 {
		t.Fatalf("trusted proxies = %v, want empty", loaded.TrustedProxies)
	}
}

func TestLoadFromRequiresProductionRedisDSN(t *testing.T) {
	values := validProductionEnvironment(t)
	delete(values, "REDIS_DSN")
	if _, err := LoadFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "REDIS_DSN is required") {
		t.Fatalf("missing production REDIS_DSN error = %v", err)
	}
	values["REDIS_DSN"] = "redis://"
	if _, err := LoadFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "REDIS_DSN is invalid") {
		t.Fatalf("invalid production REDIS_DSN error = %v", err)
	}
}

func TestLoadFromUsesSafeRedisDefaultOutsideProduction(t *testing.T) {
	for _, environment := range []string{EnvironmentDevelopment, EnvironmentTest} {
		t.Run(environment, func(t *testing.T) {
			values := validEnvironment()
			values["APP_ENV"] = environment
			delete(values, "REDIS_DSN")
			loaded, err := LoadFrom(mapLookup(values))
			if err != nil {
				t.Fatalf("LoadFrom() error = %v", err)
			}
			if loaded.RedisDSN != "redis://localhost:6379/0" {
				t.Fatalf("RedisDSN = %q", loaded.RedisDSN)
			}
		})
	}
}

func TestLoadFromRejectsInvalidDatabaseContract(t *testing.T) {
	values := validEnvironment()
	values["DATABASE_DSN"] = "pilot:pilot@tcp(localhost:3306)/pilot?parseTime=true&loc=Asia%2FShanghai"
	_, err := LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "charset=utf8mb4") {
		t.Fatalf("expected charset error, got %v", err)
	}
}

func TestLoadFromRejectsMissingUpstreamBoundary(t *testing.T) {
	values := validEnvironment()
	values["UPSTREAM_ALLOWED_CIDRS"] = ""
	values["UPSTREAM_ALLOWED_HOST_SUFFIXES"] = ""
	_, err := LoadFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "cannot both be empty") {
		t.Fatalf("expected upstream boundary error, got %v", err)
	}
}

func TestLoadFromRequiresFixedUpstreamTimeouts(t *testing.T) {
	for _, name := range []string{
		"UPSTREAM_CONNECT_TIMEOUT_SECONDS",
		"UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS",
		"UPSTREAM_REQUEST_TIMEOUT_SECONDS",
		"UPSTREAM_EXPORT_TIMEOUT_SECONDS",
	} {
		t.Run(name, func(t *testing.T) {
			values := validEnvironment()
			delete(values, name)
			if _, err := LoadFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), name+" is required") {
				t.Fatalf("missing timeout error = %v", err)
			}
			values = validEnvironment()
			values[name] = "999"
			if _, err := LoadFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "must be exactly") {
				t.Fatalf("unsafe timeout error = %v", err)
			}
		})
	}
}

func TestValidateRuntimeFilesCreatesPrivateWritableExportDirectory(t *testing.T) {
	exportDir := filepath.Join(t.TempDir(), "exports")
	configuration := Config{AppEnv: EnvironmentProduction, ExportDir: exportDir}
	if err := configuration.ValidateRuntimeFiles(); err != nil {
		t.Fatalf("ValidateRuntimeFiles() error = %v", err)
	}
	info, err := os.Stat(exportDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("EXPORT_DIR info = %#v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("created EXPORT_DIR mode = %#o, want private", info.Mode().Perm())
	}
	entries, err := os.ReadDir(exportDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("EXPORT_DIR write check left entries = %v, %v", entries, err)
	}
}

func TestValidateRuntimeFilesRejectsExportDirectorySymlinkAncestor(t *testing.T) {
	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatalf("create real parent: %v", err)
	}
	linkedParent := filepath.Join(root, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Skipf("symlinks are unavailable on this host: %v", err)
	}
	configuration := Config{AppEnv: EnvironmentProduction, ExportDir: filepath.Join(linkedParent, "exports")}
	if err := configuration.ValidateRuntimeFiles(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("ValidateRuntimeFiles() symlink error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(realParent, "exports")); !os.IsNotExist(err) {
		t.Fatalf("validation followed the symlink and created the target directory: %v", err)
	}
}

func TestValidateRuntimeFilesRejectsNonPrivateExportDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX group/other mode bits")
	}
	for _, mode := range []os.FileMode{0o755, 0o750, 0o770} {
		t.Run(mode.String(), func(t *testing.T) {
			exportDir := filepath.Join(t.TempDir(), "exports")
			if err := os.Mkdir(exportDir, 0o700); err != nil {
				t.Fatalf("create EXPORT_DIR: %v", err)
			}
			if err := os.Chmod(exportDir, mode); err != nil {
				t.Fatalf("chmod EXPORT_DIR: %v", err)
			}
			configuration := Config{AppEnv: EnvironmentProduction, ExportDir: exportDir}
			if err := configuration.ValidateRuntimeFiles(); err == nil || !strings.Contains(err.Error(), "private mode 0700") {
				t.Fatalf("ValidateRuntimeFiles() permission error = %v", err)
			}
		})
	}
}

func TestValidateRuntimeFilesAllowsDevelopmentSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatalf("create real directory: %v", err)
	}
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Skipf("symlinks are unavailable on this host: %v", err)
	}
	configuration := Config{AppEnv: EnvironmentDevelopment, ExportDir: linkedDirectory}
	if err := configuration.ValidateRuntimeFiles(); err != nil {
		t.Fatalf("development ValidateRuntimeFiles() error = %v", err)
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		"APP_ENV":                           EnvironmentDevelopment,
		"PORT":                              "3000",
		"TZ":                                "Asia/Shanghai",
		"DATABASE_DSN":                      "pilot:pilot@tcp(localhost:3306)/pilot?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai",
		"SESSION_SECRET":                    base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")),
		"ENCRYPTION_KEY":                    base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyz123456")),
		"PLATFORM_BOOTSTRAP_ADMIN_PASSWORD": "change-me",
		"SESSION_COOKIE_SECURE":             "false",
		"EXPORT_DIR":                        "./data/test-exports",
		"REDIS_DSN":                         "redis://localhost:6379/0",
		"PUBLIC_ORIGIN":                     "http://localhost:3000",
		"UPSTREAM_ALLOWED_CIDRS":            "10.0.0.0/8",
		"UPSTREAM_CONNECT_TIMEOUT_SECONDS":  "5",
		"UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS": "15",
		"UPSTREAM_REQUEST_TIMEOUT_SECONDS":         "30",
		"UPSTREAM_EXPORT_TIMEOUT_SECONDS":          "120",
		"METRICS_ALLOWED_CIDRS":                    "127.0.0.0/8",
	}
}

func validProductionEnvironment(t *testing.T) map[string]string {
	t.Helper()
	values := validEnvironment()
	values["APP_ENV"] = EnvironmentProduction
	values["SESSION_COOKIE_SECURE"] = "true"
	values["PUBLIC_ORIGIN"] = "https://pilot.example.com"
	values["TRUSTED_PROXIES"] = "10.0.0.0/8"
	values["EXPORT_DIR"] = filepath.Join(t.TempDir(), "exports")
	values["REDIS_DSN"] = "rediss://redis.example.com:6380/0"
	return values
}

func mapLookup(values map[string]string) LookupFunc {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}
