package model

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var mysqlVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)`)

type Options struct {
	DSN         string
	MaxIdle     int
	MaxOpen     int
	MaxLifetime time.Duration
}

type Database struct {
	GORM *gorm.DB
	SQL  *sql.DB
}

func Open(ctx context.Context, options Options) (*Database, error) {
	orm, err := gorm.Open(mysql.Open(options.DSN), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		return nil, fmt.Errorf("open MySQL: %w", err)
	}
	sqlDB, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("get MySQL connection pool: %w", err)
	}
	sqlDB.SetMaxIdleConns(options.MaxIdle)
	sqlDB.SetMaxOpenConns(options.MaxOpen)
	sqlDB.SetConnMaxLifetime(options.MaxLifetime)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	if err := verifyMySQLRuntime(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Database{GORM: orm, SQL: sqlDB}, nil
}

func (database *Database) Close() error {
	return database.SQL.Close()
}

func verifyMySQLRuntime(ctx context.Context, db *sql.DB) error {
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return fmt.Errorf("read MySQL version: %w", err)
	}
	if err := ValidateMySQLVersion(version); err != nil {
		return err
	}

	var isolation string
	if err := db.QueryRowContext(ctx, "SELECT @@transaction_isolation").Scan(&isolation); err != nil {
		return fmt.Errorf("read MySQL transaction isolation: %w", err)
	}
	if normalized := strings.ToUpper(strings.ReplaceAll(isolation, "_", "-")); normalized != "READ-COMMITTED" {
		return fmt.Errorf("MySQL transaction isolation must be READ-COMMITTED, got %q", isolation)
	}

	var charset, collation string
	if err := db.QueryRowContext(ctx, "SELECT @@character_set_database, @@collation_database").Scan(&charset, &collation); err != nil {
		return fmt.Errorf("read MySQL database charset: %w", err)
	}
	return validateMySQLCharsetAndCollation(charset, collation)
}

func validateMySQLCharsetAndCollation(charset, collation string) error {
	if charset != "utf8mb4" || collation != "utf8mb4_unicode_ci" {
		return fmt.Errorf("MySQL database must use charset=utf8mb4 and collation=utf8mb4_unicode_ci, got charset=%q collation=%q", charset, collation)
	}
	return nil
}

func ValidateMySQLVersion(version string) error {
	if strings.Contains(strings.ToLower(version), "mariadb") {
		return fmt.Errorf("MariaDB is not supported; MySQL 8.0 or newer is required")
	}
	matches := mysqlVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if len(matches) != 3 {
		return fmt.Errorf("unrecognized MySQL version %q", version)
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("parse MySQL major version: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf("parse MySQL minor version: %w", err)
	}
	if major < 8 {
		return fmt.Errorf("MySQL 8.0 or newer is required, got %q", version)
	}
	_ = minor
	return nil
}
