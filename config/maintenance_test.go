package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadReencryptFromUsesDedicatedKeys(t *testing.T) {
	values := validMaintenanceEnvironment()
	loaded, err := LoadReencryptFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadReencryptFrom() error = %v", err)
	}
	if len(loaded.OldKey) != 32 || len(loaded.NewKey) != 32 || loaded.OldKeyID == loaded.NewKeyID {
		t.Fatalf("unexpected re-encryption config: %#v", loaded)
	}
	if loaded.Database.SQLMaxIdleConns != 2 || loaded.Database.SQLMaxOpenConns != 4 {
		t.Fatalf("unexpected maintenance pool defaults: %#v", loaded.Database)
	}
}

func TestLoadReencryptFromRejectsSameKeys(t *testing.T) {
	values := validMaintenanceEnvironment()
	values["NEW_ENCRYPTION_KEY"] = values["OLD_ENCRYPTION_KEY"]
	_, err := LoadReencryptFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "must be different") {
		t.Fatalf("same-key error = %v", err)
	}
}

func TestLoadReencryptFromDoesNotUseRuntimeKey(t *testing.T) {
	values := validMaintenanceEnvironment()
	delete(values, "OLD_ENCRYPTION_KEY")
	values["ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	_, err := LoadReencryptFrom(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "OLD_ENCRYPTION_KEY") {
		t.Fatalf("missing old-key error = %v", err)
	}
}

func TestLoadVerifyRestoreFromUsesRuntimeKeyOnly(t *testing.T) {
	values := validMaintenanceEnvironment()
	delete(values, "OLD_ENCRYPTION_KEY")
	delete(values, "NEW_ENCRYPTION_KEY")
	values["ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	loaded, err := LoadVerifyRestoreFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadVerifyRestoreFrom() error = %v", err)
	}
	if len(loaded.EncryptionKey) != 32 || loaded.KeyID == "" {
		t.Fatalf("unexpected verify config: %#v", loaded)
	}
}

func TestLoadManifestPreflightDoesNotRequireDatabase(t *testing.T) {
	values := validMaintenanceEnvironment()
	delete(values, "DATABASE_DSN")
	delete(values, "OLD_ENCRYPTION_KEY")
	delete(values, "NEW_ENCRYPTION_KEY")
	values["ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	loaded, err := LoadManifestPreflightFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadManifestPreflightFrom() error = %v", err)
	}
	if len(loaded.EncryptionKey) != 32 || loaded.KeyID == "" {
		t.Fatalf("manifest preflight key = %#v", loaded)
	}
}

func TestLoadDatabaseIdentityRequiresOnlyDatabaseConfiguration(t *testing.T) {
	values := validMaintenanceEnvironment()
	delete(values, "OLD_ENCRYPTION_KEY")
	delete(values, "NEW_ENCRYPTION_KEY")
	loaded, err := LoadDatabaseIdentityFrom(mapLookup(values))
	if err != nil {
		t.Fatalf("LoadDatabaseIdentityFrom() error = %v", err)
	}
	if loaded.Database.DatabaseDSN != values["DATABASE_DSN"] ||
		loaded.Database.SQLMaxIdleConns != 2 || loaded.Database.SQLMaxOpenConns != 4 {
		t.Fatalf("unexpected database identity config: %#v", loaded)
	}
	delete(values, "DATABASE_DSN")
	if _, err := LoadDatabaseIdentityFrom(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "DATABASE_DSN") {
		t.Fatalf("missing database identity DSN error = %v", err)
	}
}

func TestLoadReencryptUsesProcessEnvironment(t *testing.T) {
	values := validMaintenanceEnvironment()
	for name, setting := range values {
		t.Setenv(name, setting)
	}
	loaded, err := LoadReencrypt()
	if err != nil {
		t.Fatalf("LoadReencrypt() error = %v", err)
	}
	if loaded.OldKeyID == "" || loaded.NewKeyID == "" || loaded.OldKeyID == loaded.NewKeyID {
		t.Fatalf("unexpected process environment config: %#v", loaded)
	}
}

func validMaintenanceEnvironment() map[string]string {
	return map[string]string{
		"DATABASE_DSN":       "pilot:pilot@tcp(localhost:3306)/pilot?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai",
		"OLD_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyz123456")),
		"NEW_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")),
	}
}
