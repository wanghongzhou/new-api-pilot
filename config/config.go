package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"golang.org/x/net/idna"

	"new-api-pilot/common"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentTest        = "test"
	EnvironmentProduction  = "production"
)

type LookupFunc func(string) (string, bool)

type Config struct {
	AppEnv string
	Port   string
	TZ     string

	DatabaseDSN          string
	SQLMaxIdleConns      int
	SQLMaxOpenConns      int
	SQLMaxLifetime       time.Duration
	SessionSecret        []byte
	EncryptionKey        []byte
	EncryptionKeyID      string
	BootstrapAdminSecret string
	SessionCookieSecure  bool
	ExportDir            string
	RedisDSN             string
	RedisDB              int
	RedisTimeout         time.Duration
	FastTaskRetention    time.Duration
	FastTaskHistoryCount int

	PublicOrigin                string
	TrustedProxies              []string
	UpstreamAllowedHostSuffixes []string
	UpstreamAllowedCIDRs        []netip.Prefix
	UpstreamCAFile              string
	UpstreamConnectTimeout      time.Duration
	UpstreamHeaderTimeout       time.Duration
	UpstreamRequestTimeout      time.Duration
	UpstreamExportTimeout       time.Duration
	DingTalkAllowedHosts        []string
	MetricsAllowedCIDRs         []netip.Prefix
}

func Load() (Config, error) {
	_ = godotenv.Load()
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup LookupFunc) (Config, error) {
	appEnv := valueOrDefault(lookup, "APP_ENV", EnvironmentDevelopment)
	if appEnv != EnvironmentDevelopment && appEnv != EnvironmentTest && appEnv != EnvironmentProduction {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, or production")
	}

	port, err := parsePort(valueOrDefault(lookup, "PORT", "3000"))
	if err != nil {
		return Config{}, err
	}
	dsn := value(lookup, "DATABASE_DSN")
	if err := validateDatabaseDSN(dsn); err != nil {
		return Config{}, err
	}

	maxIdle, err := positiveInt(lookup, "SQL_MAX_IDLE_CONNS", 20)
	if err != nil {
		return Config{}, err
	}
	maxOpen, err := positiveInt(lookup, "SQL_MAX_OPEN_CONNS", 100)
	if err != nil {
		return Config{}, err
	}
	if maxOpen < maxIdle {
		return Config{}, fmt.Errorf("SQL_MAX_OPEN_CONNS must be greater than or equal to SQL_MAX_IDLE_CONNS")
	}
	lifetimeSeconds, err := positiveInt(lookup, "SQL_MAX_LIFETIME_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}

	sessionSecret, err := decodeBase64Secret(value(lookup, "SESSION_SECRET"), "SESSION_SECRET", 32, false)
	if err != nil {
		return Config{}, err
	}
	encryptionKey, err := decodeBase64Secret(value(lookup, "ENCRYPTION_KEY"), "ENCRYPTION_KEY", 32, true)
	if err != nil {
		return Config{}, err
	}

	bootstrapPassword := rawValue(lookup, "PLATFORM_BOOTSTRAP_ADMIN_PASSWORD")
	if bootstrapPassword != "" {
		if err := common.ValidatePassword(bootstrapPassword); err != nil {
			return Config{}, fmt.Errorf("PLATFORM_BOOTSTRAP_ADMIN_PASSWORD: %w", err)
		}
	}

	cookieSecure, err := boolValue(lookup, "SESSION_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	if appEnv == EnvironmentProduction && !cookieSecure {
		return Config{}, fmt.Errorf("SESSION_COOKIE_SECURE must be true in production")
	}

	tz := valueOrDefault(lookup, "TZ", "Asia/Shanghai")
	if tz != "Asia/Shanghai" {
		return Config{}, fmt.Errorf("TZ must be Asia/Shanghai")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return Config{}, fmt.Errorf("load TZ: %w", err)
	}

	publicOrigin, err := validateOrigin(value(lookup, "PUBLIC_ORIGIN"), appEnv == EnvironmentProduction)
	if err != nil {
		return Config{}, err
	}
	trustedProxies, err := parseNetworks(value(lookup, "TRUSTED_PROXIES"), "TRUSTED_PROXIES", false)
	if err != nil {
		return Config{}, err
	}
	if appEnv == EnvironmentProduction && len(trustedProxies) == 0 {
		return Config{}, fmt.Errorf("TRUSTED_PROXIES is required in production")
	}
	hostSuffixes, err := parseHostSuffixes(value(lookup, "UPSTREAM_ALLOWED_HOST_SUFFIXES"))
	if err != nil {
		return Config{}, err
	}
	upstreamCIDRs, err := parsePrefixes(value(lookup, "UPSTREAM_ALLOWED_CIDRS"), "UPSTREAM_ALLOWED_CIDRS")
	if err != nil {
		return Config{}, err
	}
	if len(hostSuffixes) == 0 && len(upstreamCIDRs) == 0 {
		return Config{}, fmt.Errorf("UPSTREAM_ALLOWED_HOST_SUFFIXES and UPSTREAM_ALLOWED_CIDRS cannot both be empty")
	}
	upstreamConnectTimeout, err := fixedDurationSeconds(lookup, "UPSTREAM_CONNECT_TIMEOUT_SECONDS", 5)
	if err != nil {
		return Config{}, err
	}
	upstreamHeaderTimeout, err := fixedDurationSeconds(lookup, "UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}
	upstreamRequestTimeout, err := fixedDurationSeconds(lookup, "UPSTREAM_REQUEST_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	upstreamExportTimeout, err := fixedDurationSeconds(lookup, "UPSTREAM_EXPORT_TIMEOUT_SECONDS", 120)
	if err != nil {
		return Config{}, err
	}

	metricsCIDRs, err := parsePrefixes(value(lookup, "METRICS_ALLOWED_CIDRS"), "METRICS_ALLOWED_CIDRS")
	if err != nil {
		return Config{}, err
	}
	if appEnv == EnvironmentProduction && len(metricsCIDRs) == 0 {
		return Config{}, fmt.Errorf("METRICS_ALLOWED_CIDRS is required in production")
	}
	dingTalkHosts, err := parseDingTalkHosts(value(lookup, "DINGTALK_ALLOWED_HOSTS"))
	if err != nil {
		return Config{}, err
	}

	exportDir := value(lookup, "EXPORT_DIR")
	if exportDir == "" {
		return Config{}, fmt.Errorf("EXPORT_DIR is required")
	}
	if appEnv == EnvironmentProduction && !filepath.IsAbs(exportDir) {
		return Config{}, fmt.Errorf("EXPORT_DIR must be an absolute path in production")
	}
	redisDSN := value(lookup, "REDIS_DSN")
	if redisDSN == "" {
		if appEnv == EnvironmentProduction {
			return Config{}, fmt.Errorf("REDIS_DSN is required in production")
		}
		redisDSN = "redis://localhost:6379/0"
	}
	if parsed, parseErr := url.Parse(redisDSN); parseErr != nil ||
		(parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" {
		return Config{}, fmt.Errorf("REDIS_DSN is invalid")
	}
	redisDB, err := nonNegativeInt(lookup, "REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	redisTimeoutSeconds, err := positiveInt(lookup, "REDIS_TIMEOUT_SECONDS", 2)
	if err != nil {
		return Config{}, err
	}
	retentionSeconds, err := positiveInt(lookup, "FAST_TASK_HISTORY_RETENTION_SECONDS", 86400)
	if err != nil {
		return Config{}, err
	}
	historyCount, err := positiveInt(lookup, "FAST_TASK_HISTORY_COUNT", 100)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:               appEnv,
		Port:                 port,
		TZ:                   tz,
		DatabaseDSN:          dsn,
		SQLMaxIdleConns:      maxIdle,
		SQLMaxOpenConns:      maxOpen,
		SQLMaxLifetime:       time.Duration(lifetimeSeconds) * time.Second,
		SessionSecret:        sessionSecret,
		EncryptionKey:        encryptionKey,
		EncryptionKeyID:      common.KeyFingerprint(encryptionKey),
		BootstrapAdminSecret: bootstrapPassword,
		SessionCookieSecure:  cookieSecure,
		ExportDir:            filepath.Clean(exportDir),
		RedisDSN:             redisDSN, RedisDB: redisDB, RedisTimeout: time.Duration(redisTimeoutSeconds) * time.Second,
		FastTaskRetention: time.Duration(retentionSeconds) * time.Second, FastTaskHistoryCount: historyCount,
		PublicOrigin:                publicOrigin,
		TrustedProxies:              trustedProxies,
		UpstreamAllowedHostSuffixes: hostSuffixes,
		UpstreamAllowedCIDRs:        upstreamCIDRs,
		UpstreamCAFile:              value(lookup, "UPSTREAM_CA_FILE"),
		UpstreamConnectTimeout:      upstreamConnectTimeout,
		UpstreamHeaderTimeout:       upstreamHeaderTimeout,
		UpstreamRequestTimeout:      upstreamRequestTimeout,
		UpstreamExportTimeout:       upstreamExportTimeout,
		DingTalkAllowedHosts:        dingTalkHosts,
		MetricsAllowedCIDRs:         metricsCIDRs,
	}, nil
}

func (c Config) ValidateRuntimeFiles() error {
	if err := validateExportDirectory(c.ExportDir, c.AppEnv == EnvironmentProduction); err != nil {
		return err
	}
	if c.UpstreamCAFile != "" {
		info, err := os.Stat(c.UpstreamCAFile)
		if err != nil {
			return fmt.Errorf("read UPSTREAM_CA_FILE: %w", err)
		}
		if info.IsDir() {
			return errors.New("UPSTREAM_CA_FILE must be a file")
		}
	}
	return nil
}

func validateExportDirectory(path string, production bool) error {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve EXPORT_DIR: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)
	_, initialStatError := os.Lstat(absolutePath)
	directoryExisted := initialStatError == nil
	if initialStatError != nil && !os.IsNotExist(initialStatError) {
		return fmt.Errorf("inspect EXPORT_DIR before creation: %w", initialStatError)
	}
	if production {
		if err := rejectSymlinkPath(absolutePath); err != nil {
			return fmt.Errorf("validate EXPORT_DIR path: %w", err)
		}
	}
	if err := os.MkdirAll(absolutePath, 0o700); err != nil {
		return fmt.Errorf("create EXPORT_DIR: %w", err)
	}
	if production && !directoryExisted && runtime.GOOS != "windows" {
		if err := os.Chmod(absolutePath, 0o700); err != nil {
			return fmt.Errorf("set private EXPORT_DIR permissions: %w", err)
		}
	}
	if production {
		if err := rejectSymlinkPath(absolutePath); err != nil {
			return fmt.Errorf("validate EXPORT_DIR path after creation: %w", err)
		}
		resolvedPath, err := filepath.EvalSymlinks(absolutePath)
		if err != nil {
			return fmt.Errorf("resolve EXPORT_DIR symlinks: %w", err)
		}
		if !sameFilesystemPath(absolutePath, resolvedPath) {
			return fmt.Errorf("EXPORT_DIR must not contain symlink components: configured=%q resolved=%q", absolutePath, resolvedPath)
		}
		info, err := os.Lstat(absolutePath)
		if err != nil {
			return fmt.Errorf("inspect EXPORT_DIR: %w", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("EXPORT_DIR must have private mode 0700, mode=%#o", info.Mode().Perm())
		}
	}
	file, err := os.CreateTemp(absolutePath, ".write-check-*")
	if err != nil {
		return fmt.Errorf("EXPORT_DIR is not writable: %w", err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return fmt.Errorf("close EXPORT_DIR write check: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove EXPORT_DIR write check: %w", err)
	}
	return nil
}

func rejectSymlinkPath(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("EXPORT_DIR path component %q is a symlink", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("EXPORT_DIR path component %q is not a directory", current)
			}
		case os.IsNotExist(err):
		case err != nil:
			return fmt.Errorf("inspect EXPORT_DIR path component %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func sameFilesystemPath(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func value(lookup LookupFunc, name string) string {
	return strings.TrimSpace(rawValue(lookup, name))
}

func rawValue(lookup LookupFunc, name string) string {
	raw, _ := lookup(name)
	return raw
}

func valueOrDefault(lookup LookupFunc, name, fallback string) string {
	if result := value(lookup, name); result != "" {
		return result
	}
	return fallback
}

func parsePort(raw string) (string, error) {
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	return strconv.Itoa(port), nil
}

func positiveInt(lookup LookupFunc, name string, fallback int) (int, error) {
	raw := valueOrDefault(lookup, name, strconv.Itoa(fallback))
	result, err := strconv.Atoi(raw)
	if err != nil || result <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return result, nil
}

func nonNegativeInt(lookup LookupFunc, name string, fallback int) (int, error) {
	raw := valueOrDefault(lookup, name, strconv.Itoa(fallback))
	result, err := strconv.Atoi(raw)
	if err != nil || result < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return result, nil
}

func fixedDurationSeconds(lookup LookupFunc, name string, expected int) (time.Duration, error) {
	raw := value(lookup, name)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds != expected {
		return 0, fmt.Errorf("%s must be exactly %d seconds", name, expected)
	}
	return time.Duration(seconds) * time.Second, nil
}

func boolValue(lookup LookupFunc, name string, fallback bool) (bool, error) {
	raw := valueOrDefault(lookup, name, strconv.FormatBool(fallback))
	result, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return result, nil
}

func decodeBase64Secret(raw, name string, minimum int, exact bool) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be standard Base64: %w", name, err)
	}
	if exact && len(decoded) != minimum {
		return nil, fmt.Errorf("%s must decode to exactly %d bytes", name, minimum)
	}
	if !exact && len(decoded) < minimum {
		return nil, fmt.Errorf("%s must decode to at least %d bytes", name, minimum)
	}
	return decoded, nil
}

func validateDatabaseDSN(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("DATABASE_DSN is required")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("DATABASE_DSN is invalid: %w", err)
	}
	if parsed.DBName == "" {
		return fmt.Errorf("DATABASE_DSN must select a database")
	}
	if !parsed.ParseTime {
		return fmt.Errorf("DATABASE_DSN must set parseTime=true")
	}
	if parsed.Loc == nil || parsed.Loc.String() != "Asia/Shanghai" {
		return fmt.Errorf("DATABASE_DSN must set loc=Asia%%2FShanghai")
	}
	if !strings.EqualFold(parsed.Params["charset"], "utf8mb4") {
		return fmt.Errorf("DATABASE_DSN must set charset=utf8mb4")
	}
	if parsed.MultiStatements {
		return fmt.Errorf("DATABASE_DSN must not enable multiStatements")
	}
	return nil
}

func validateOrigin(raw string, required bool) (string, error) {
	if raw == "" {
		if required {
			return "", fmt.Errorf("PUBLIC_ORIGIN is required in production")
		}
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("PUBLIC_ORIGIN must be an absolute HTTP(S) origin")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("PUBLIC_ORIGIN must not contain userinfo, path, query, or fragment")
	}
	if required && parsed.Scheme != "https" {
		return "", fmt.Errorf("PUBLIC_ORIGIN must use HTTPS in production")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), nil
}

func parseNetworks(raw, name string, requireCIDR bool) ([]string, error) {
	parts := splitList(raw)
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		if strings.Contains(item, "/") {
			if _, err := netip.ParsePrefix(item); err != nil {
				return nil, fmt.Errorf("%s contains invalid CIDR %q", name, item)
			}
		} else {
			if requireCIDR || net.ParseIP(item) == nil {
				return nil, fmt.Errorf("%s contains invalid IP or CIDR %q", name, item)
			}
		}
		result = append(result, item)
	}
	return uniqueStrings(result), nil
}

func parsePrefixes(raw, name string) ([]netip.Prefix, error) {
	parts := splitList(raw)
	result := make([]netip.Prefix, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, item := range parts {
		var prefix netip.Prefix
		if strings.Contains(item, "/") {
			parsed, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("%s contains invalid CIDR %q", name, item)
			}
			prefix = parsed.Masked()
		} else {
			address, err := netip.ParseAddr(item)
			if err != nil {
				return nil, fmt.Errorf("%s contains invalid IP or CIDR %q", name, item)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		if _, exists := seen[prefix]; !exists {
			seen[prefix] = struct{}{}
			result = append(result, prefix)
		}
	}
	return result, nil
}

func parseHostSuffixes(raw string) ([]string, error) {
	parts := splitList(raw)
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSuffix(strings.ToLower(item), ".")
		if strings.HasPrefix(item, "*.") {
			item = strings.TrimPrefix(item, "*.")
		}
		ascii, err := idna.Lookup.ToASCII(item)
		if err != nil || ascii == "" || net.ParseIP(ascii) != nil || strings.ContainsAny(ascii, "/:@") {
			return nil, fmt.Errorf("UPSTREAM_ALLOWED_HOST_SUFFIXES contains invalid suffix %q", item)
		}
		result = append(result, ascii)
	}
	return uniqueStrings(result), nil
}

func parseDingTalkHosts(raw string) ([]string, error) {
	items := append([]string{"oapi.dingtalk.com"}, splitList(raw)...)
	result := make([]string, 0, len(items))
	for _, item := range items {
		ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(item), "."))
		if err != nil || ascii == "" || net.ParseIP(ascii) != nil || strings.ContainsAny(ascii, "/:@") {
			return nil, fmt.Errorf("DINGTALK_ALLOWED_HOSTS contains invalid host %q", item)
		}
		result = append(result, ascii)
	}
	return uniqueStrings(result), nil
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
