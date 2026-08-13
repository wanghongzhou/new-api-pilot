package model

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGORMLoggerNeverExpandsSensitiveParameters(t *testing.T) {
	var output bytes.Buffer
	databaseLogger := newParameterizedGORMLogger(&output, logger.Info)
	filter, ok := databaseLogger.(gorm.ParamsFilter)
	if !ok {
		t.Fatal("GORM logger does not expose a parameter filter")
	}
	const sentinel = "access-token-sentinel-never-log"
	sql, params := filter.ParamsFilter(context.Background(),
		"INSERT INTO `sensitive_log_test` (`id`,`value`) VALUES (?,?)", int64(1), sentinel)
	if len(params) != 0 {
		t.Fatalf("GORM parameter filter retained %d parameters", len(params))
	}
	databaseLogger.Trace(context.Background(), time.Now(), func() (string, int64) { return sql, 1 }, nil)
	logged := output.String()
	if strings.Contains(logged, sentinel) {
		t.Fatalf("GORM log expanded sensitive parameter: %s", logged)
	}
	if !strings.Contains(logged, "INSERT INTO `sensitive_log_test`") || !strings.Contains(logged, "VALUES (?,?)") {
		t.Fatalf("GORM log did not retain a parameterized SQL template: %s", logged)
	}
}

func TestGORMLoggerIgnoresExpectedRecordNotFoundErrors(t *testing.T) {
	var output bytes.Buffer
	databaseLogger := newParameterizedGORMLogger(&output, logger.Warn)
	databaseLogger.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM `alert_event` WHERE `active_key` = ? LIMIT 1", 0
	}, gorm.ErrRecordNotFound)
	if output.Len() != 0 {
		t.Fatalf("record-not-found lookup was logged: %s", output.String())
	}
}

func TestGORMLoggerRetainsUnexpectedDatabaseErrors(t *testing.T) {
	var output bytes.Buffer
	databaseLogger := newParameterizedGORMLogger(&output, logger.Warn)
	databaseLogger.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM `site` WHERE `id` = ?", 0
	}, context.DeadlineExceeded)
	logged := output.String()
	if !strings.Contains(logged, context.DeadlineExceeded.Error()) ||
		!strings.Contains(logged, "SELECT * FROM `site`") {
		t.Fatalf("unexpected database error was not logged: %s", logged)
	}
}

func TestValidateMySQLVersion(t *testing.T) {
	for _, version := range []string{"8.0.36", "8.4.6", "9.0.1-commercial"} {
		if err := ValidateMySQLVersion(version); err != nil {
			t.Fatalf("ValidateMySQLVersion(%q) error = %v", version, err)
		}
	}
	for _, version := range []string{"5.7.44", "10.11.4-MariaDB", "unknown"} {
		if err := ValidateMySQLVersion(version); err == nil {
			t.Fatalf("ValidateMySQLVersion(%q) succeeded", version)
		}
	}
}

func TestValidateMySQLTransactionIsolation(t *testing.T) {
	for _, isolation := range []string{"REPEATABLE-READ", "repeatable_read"} {
		if err := validateMySQLTransactionIsolation(isolation); err != nil {
			t.Fatalf("validateMySQLTransactionIsolation(%q) error = %v", isolation, err)
		}
	}
	for _, isolation := range []string{"READ-COMMITTED", "SERIALIZABLE", ""} {
		if err := validateMySQLTransactionIsolation(isolation); err == nil {
			t.Fatalf("validateMySQLTransactionIsolation(%q) succeeded", isolation)
		}
	}
}

func TestValidateMySQLCharsetAndCollation(t *testing.T) {
	if err := validateMySQLCharsetAndCollation("utf8mb4", "utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("valid charset/collation error = %v", err)
	}
	for _, test := range []struct {
		charset   string
		collation string
	}{
		{charset: "utf8mb4", collation: "utf8mb4_0900_ai_ci"},
		{charset: "utf8mb4", collation: "utf8mb4_bin"},
		{charset: "utf8", collation: "utf8mb4_unicode_ci"},
	} {
		if err := validateMySQLCharsetAndCollation(test.charset, test.collation); err == nil {
			t.Errorf("validateMySQLCharsetAndCollation(%q, %q) succeeded", test.charset, test.collation)
		}
	}
}
