package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

var (
	ErrMigrationSourceInvalid   = errors.New("migration source invalid")
	ErrMigrationChecksumInvalid = errors.New("migration checksum invalid")
)

type MigrationVersion struct {
	Version  string
	Checksum string
}

type MigrationQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type migrationProgressState struct {
	Version        string
	Checksum       string
	StatementIndex int
	State          string
}

type migrationTableInventory struct {
	Tables                  int
	NonMetadataTables       int
	SchemaMigration         bool
	SchemaMigrationProgress bool
}

func LoadMigrationVersions(source fs.FS) ([]MigrationVersion, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: repository filesystem is required", ErrMigrationSourceInvalid)
	}
	paths, err := fs.Glob(source, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("%w: list repository migrations: %v", ErrMigrationSourceInvalid, err)
	}
	sort.Strings(paths)
	versions := make([]MigrationVersion, 0, len(paths))
	for _, path := range paths {
		payload, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, fmt.Errorf("%w: read repository migration %s: %v", ErrMigrationSourceInvalid, path, err)
		}
		versions = append(versions, MigrationVersion{
			Version: strings.TrimSuffix(path, ".sql"), Checksum: migrationChecksum(payload),
		})
	}
	if err := validateMigrationSequence("repository", versions, false); err != nil {
		return nil, err
	}
	return versions, nil
}

func ReadMigrationVersions(
	ctx context.Context,
	queryer MigrationQueryer,
) ([]MigrationVersion, error) {
	if queryer == nil {
		return nil, errors.New("migration queryer is required")
	}
	rows, err := queryer.QueryContext(ctx,
		"SELECT version, checksum FROM schema_migration ORDER BY version ASC")
	if err != nil {
		return nil, fmt.Errorf("read applied migration source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	versions := make([]MigrationVersion, 0)
	for rows.Next() {
		var version MigrationVersion
		if err := rows.Scan(&version.Version, &version.Checksum); err != nil {
			return nil, fmt.Errorf("scan applied migration source: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migration source: %w", err)
	}
	return versions, nil
}

func ValidateMigrationVersionPrefix(
	repository []MigrationVersion,
	applied []MigrationVersion,
	requireComplete bool,
) error {
	if err := validateMigrationSequence("repository", repository, false); err != nil {
		return err
	}
	if err := validateMigrationSequence("applied", applied, true); err != nil {
		return err
	}
	if len(applied) > len(repository) {
		return fmt.Errorf("%w: applied migration count %d exceeds repository count %d",
			ErrMigrationSourceInvalid, len(applied), len(repository))
	}
	for index := range applied {
		if applied[index].Version != repository[index].Version {
			return fmt.Errorf("%w: applied version %q at position %d does not match repository version %q",
				ErrMigrationSourceInvalid, applied[index].Version, index+1, repository[index].Version)
		}
		if applied[index].Checksum != repository[index].Checksum {
			return fmt.Errorf("%w: migration %s checksum mismatch: database=%s repository=%s",
				ErrMigrationChecksumInvalid, applied[index].Version,
				applied[index].Checksum, repository[index].Checksum)
		}
	}
	if requireComplete && len(applied) != len(repository) {
		return fmt.Errorf("%w: applied migration count %d does not match repository count %d",
			ErrMigrationSourceInvalid, len(applied), len(repository))
	}
	return nil
}

func validateMigrationSequence(label string, versions []MigrationVersion, allowEmpty bool) error {
	if len(versions) == 0 && !allowEmpty {
		return fmt.Errorf("%w: %s contains no migrations", ErrMigrationSourceInvalid, label)
	}
	seen := make(map[string]struct{}, len(versions))
	previous := ""
	for index, version := range versions {
		if version.Version == "" {
			return fmt.Errorf("%w: %s version at position %d is empty",
				ErrMigrationSourceInvalid, label, index+1)
		}
		if _, exists := seen[version.Version]; exists {
			return fmt.Errorf("%w: %s version %q is duplicated",
				ErrMigrationSourceInvalid, label, version.Version)
		}
		if previous != "" && version.Version <= previous {
			return fmt.Errorf("%w: %s versions are not strictly ordered at %q",
				ErrMigrationSourceInvalid, label, version.Version)
		}
		if !isStrictMigrationChecksum(version.Checksum) {
			return fmt.Errorf("%w: %s migration %s checksum must be 64 lowercase hexadecimal characters",
				ErrMigrationChecksumInvalid, label, version.Version)
		}
		seen[version.Version] = struct{}{}
		previous = version.Version
	}
	return nil
}

func isStrictMigrationChecksum(checksum string) bool {
	if len(checksum) != 64 {
		return false
	}
	for _, character := range checksum {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func migrationChecksum(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func inspectMigrationTableInventory(
	ctx context.Context,
	queryer MigrationQueryer,
) (migrationTableInventory, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
ORDER BY table_name`)
	if err != nil {
		return migrationTableInventory{}, fmt.Errorf("inspect migration tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var inventory migrationTableInventory
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return migrationTableInventory{}, fmt.Errorf("scan migration table inventory: %w", err)
		}
		inventory.Tables++
		switch table {
		case "schema_migration":
			inventory.SchemaMigration = true
		case "schema_migration_progress":
			inventory.SchemaMigrationProgress = true
		default:
			inventory.NonMetadataTables++
		}
	}
	if err := rows.Err(); err != nil {
		return migrationTableInventory{}, fmt.Errorf("iterate migration table inventory: %w", err)
	}
	return inventory, nil
}

func readMigrationProgressStates(
	ctx context.Context,
	queryer MigrationQueryer,
) ([]migrationProgressState, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT version, checksum, statement_index, state
FROM schema_migration_progress ORDER BY version ASC`)
	if err != nil {
		return nil, fmt.Errorf("read migration progress source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	states := make([]migrationProgressState, 0, 1)
	for rows.Next() {
		var state migrationProgressState
		if err := rows.Scan(&state.Version, &state.Checksum, &state.StatementIndex, &state.State); err != nil {
			return nil, fmt.Errorf("scan migration progress source: %w", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration progress source: %w", err)
	}
	return states, nil
}

func validateMigrationProgressSource(
	repository []MigrationVersion,
	applied []MigrationVersion,
	progress []migrationProgressState,
	source fs.FS,
) error {
	if len(progress) == 0 {
		return nil
	}
	if len(progress) != 1 {
		return fmt.Errorf("%w: migration progress must contain at most one version",
			ErrMigrationSourceInvalid)
	}
	state := progress[0]
	if state.Version == "" {
		return fmt.Errorf("%w: migration progress version is empty", ErrMigrationSourceInvalid)
	}
	if !isStrictMigrationChecksum(state.Checksum) {
		return fmt.Errorf("%w: migration %s checkpoint checksum must be 64 lowercase hexadecimal characters",
			ErrMigrationChecksumInvalid, state.Version)
	}
	if len(applied) >= len(repository) {
		return fmt.Errorf("%w: migration progress exists after the repository is fully applied",
			ErrMigrationSourceInvalid)
	}
	expected := repository[len(applied)]
	if state.Version != expected.Version {
		return fmt.Errorf("%w: migration progress version %q does not match next repository version %q",
			ErrMigrationSourceInvalid, state.Version, expected.Version)
	}
	if state.Checksum != expected.Checksum {
		return fmt.Errorf("%w: migration %s checkpoint checksum mismatch: database=%s repository=%s",
			ErrMigrationChecksumInvalid, state.Version, state.Checksum, expected.Checksum)
	}
	payload, err := fs.ReadFile(source, state.Version+".sql")
	if err != nil {
		return fmt.Errorf("%w: read migration progress source %s: %v",
			ErrMigrationSourceInvalid, state.Version, err)
	}
	statements, err := SplitSQLStatements(string(payload))
	if err != nil {
		return fmt.Errorf("%w: parse migration progress source %s: %v",
			ErrMigrationSourceInvalid, state.Version, err)
	}
	if state.StatementIndex < 0 || state.StatementIndex > len(statements) ||
		(state.State != migrationReady && state.State != migrationDirty) ||
		(state.State == migrationDirty && (state.StatementIndex == len(statements) ||
			!isMigrationDDL(statements[state.StatementIndex]))) {
		return fmt.Errorf("%w: migration %s checkpoint state is inconsistent: statement_index=%d state=%q count=%d",
			ErrMigrationSourceInvalid, state.Version, state.StatementIndex, state.State, len(statements))
	}
	return nil
}
