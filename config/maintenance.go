package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"

	"new-api-pilot/common"
)

type MaintenanceDatabaseConfig struct {
	DatabaseDSN     string
	SQLMaxIdleConns int
	SQLMaxOpenConns int
	SQLMaxLifetime  time.Duration
}

type ReencryptConfig struct {
	Database MaintenanceDatabaseConfig
	OldKey   []byte
	NewKey   []byte
	OldKeyID string
	NewKeyID string
}

type VerifyRestoreConfig struct {
	Database      MaintenanceDatabaseConfig
	EncryptionKey []byte
	KeyID         string
}

type ManifestPreflightConfig struct {
	EncryptionKey []byte
	KeyID         string
}

type DatabaseIdentityConfig struct {
	Database MaintenanceDatabaseConfig
}

func LoadReencrypt() (ReencryptConfig, error) {
	_ = godotenv.Load()
	return LoadReencryptFrom(os.LookupEnv)
}

func LoadReencryptFrom(lookup LookupFunc) (ReencryptConfig, error) {
	database, err := loadMaintenanceDatabase(lookup)
	if err != nil {
		return ReencryptConfig{}, err
	}
	oldKey, err := decodeBase64Secret(value(lookup, "OLD_ENCRYPTION_KEY"), "OLD_ENCRYPTION_KEY", 32, true)
	if err != nil {
		return ReencryptConfig{}, err
	}
	newKey, err := decodeBase64Secret(value(lookup, "NEW_ENCRYPTION_KEY"), "NEW_ENCRYPTION_KEY", 32, true)
	if err != nil {
		return ReencryptConfig{}, err
	}
	oldKeyID := common.KeyFingerprint(oldKey)
	newKeyID := common.KeyFingerprint(newKey)
	if oldKeyID == newKeyID {
		return ReencryptConfig{}, fmt.Errorf("OLD_ENCRYPTION_KEY and NEW_ENCRYPTION_KEY must be different")
	}
	return ReencryptConfig{
		Database: database,
		OldKey:   oldKey,
		NewKey:   newKey,
		OldKeyID: oldKeyID,
		NewKeyID: newKeyID,
	}, nil
}

func LoadVerifyRestore() (VerifyRestoreConfig, error) {
	_ = godotenv.Load()
	return LoadVerifyRestoreFrom(os.LookupEnv)
}

func LoadVerifyRestoreFrom(lookup LookupFunc) (VerifyRestoreConfig, error) {
	database, err := loadMaintenanceDatabase(lookup)
	if err != nil {
		return VerifyRestoreConfig{}, err
	}
	key, err := decodeBase64Secret(value(lookup, "ENCRYPTION_KEY"), "ENCRYPTION_KEY", 32, true)
	if err != nil {
		return VerifyRestoreConfig{}, err
	}
	return VerifyRestoreConfig{
		Database: database, EncryptionKey: key, KeyID: common.KeyFingerprint(key),
	}, nil
}

func LoadManifestPreflight() (ManifestPreflightConfig, error) {
	_ = godotenv.Load()
	return LoadManifestPreflightFrom(os.LookupEnv)
}

func LoadManifestPreflightFrom(lookup LookupFunc) (ManifestPreflightConfig, error) {
	key, err := decodeBase64Secret(value(lookup, "ENCRYPTION_KEY"), "ENCRYPTION_KEY", 32, true)
	if err != nil {
		return ManifestPreflightConfig{}, err
	}
	return ManifestPreflightConfig{EncryptionKey: key, KeyID: common.KeyFingerprint(key)}, nil
}

func LoadDatabaseIdentity() (DatabaseIdentityConfig, error) {
	_ = godotenv.Load()
	return LoadDatabaseIdentityFrom(os.LookupEnv)
}

func LoadDatabaseIdentityFrom(lookup LookupFunc) (DatabaseIdentityConfig, error) {
	database, err := loadMaintenanceDatabase(lookup)
	if err != nil {
		return DatabaseIdentityConfig{}, err
	}
	return DatabaseIdentityConfig{Database: database}, nil
}

func loadMaintenanceDatabase(lookup LookupFunc) (MaintenanceDatabaseConfig, error) {
	dsn := value(lookup, "DATABASE_DSN")
	if err := validateDatabaseDSN(dsn); err != nil {
		return MaintenanceDatabaseConfig{}, err
	}
	maxIdle, err := positiveInt(lookup, "SQL_MAX_IDLE_CONNS", 2)
	if err != nil {
		return MaintenanceDatabaseConfig{}, err
	}
	maxOpen, err := positiveInt(lookup, "SQL_MAX_OPEN_CONNS", 4)
	if err != nil {
		return MaintenanceDatabaseConfig{}, err
	}
	if maxOpen < maxIdle {
		return MaintenanceDatabaseConfig{}, fmt.Errorf("SQL_MAX_OPEN_CONNS must be greater than or equal to SQL_MAX_IDLE_CONNS")
	}
	lifetimeSeconds, err := positiveInt(lookup, "SQL_MAX_LIFETIME_SECONDS", 60)
	if err != nil {
		return MaintenanceDatabaseConfig{}, err
	}
	return MaintenanceDatabaseConfig{
		DatabaseDSN: dsn, SQLMaxIdleConns: maxIdle, SQLMaxOpenConns: maxOpen,
		SQLMaxLifetime: time.Duration(lifetimeSeconds) * time.Second,
	}, nil
}
