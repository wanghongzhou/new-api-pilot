package model

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"new-api-pilot/migrations"
	"os"
	"strings"
	"testing"
)

func TestReleasedMigrationDiffersOnlyInLineEndings(t *testing.T) {
	raw, e := os.ReadFile("../testdata/migrations/initial-mixed-eol.sql.gz")
	if e != nil {
		t.Fatal(e)
	}
	reader, e := gzip.NewReader(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	old, e := io.ReadAll(io.LimitReader(reader, 200000))
	if e != nil {
		t.Fatal(e)
	}
	current, e := migrations.Files.ReadFile("0001_initial_schema.sql")
	if e != nil {
		t.Fatal(e)
	}
	if migrationChecksum(old) != releasedMixedEOLInitialChecksum || migrationChecksum(current) != canonicalInitialChecksum {
		t.Fatal("released migration fixture/source checksum changed")
	}
	if !bytes.Equal(bytes.ReplaceAll(old, []byte("\r\n"), []byte("\n")), current) {
		t.Fatal("legacy SQL is not identical after newline normalization")
	}
	if !completedMigrationChecksumMatches("0001_initial_schema", canonicalInitialChecksum, releasedMixedEOLInitialChecksum) {
		t.Fatal("released hash rejected")
	}
	if completedMigrationChecksumMatches("0002_balance_monitor", canonicalInitialChecksum, releasedMixedEOLInitialChecksum) || completedMigrationChecksumMatches("0001_initial_schema", strings.Repeat("a", 64), releasedMixedEOLInitialChecksum) {
		t.Fatal("unrelated source accepted")
	}
}

func TestCompletedLegacyMigrationRemainsUnmodified(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	_, e := database.SQL.ExecContext(ctx, "UPDATE schema_migration SET checksum=? WHERE version='0001_initial_schema'", releasedMixedEOLInitialChecksum)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "UPDATE schema_migration SET checksum=? WHERE version='0001_initial_schema'", canonicalInitialChecksum)
	})
	if e := NewMigrationRunner(database.SQL).Run(ctx); e != nil {
		t.Fatal(e)
	}
	var got string
	if e := database.SQL.QueryRowContext(ctx, "SELECT checksum FROM schema_migration WHERE version='0001_initial_schema'").Scan(&got); e != nil {
		t.Fatal(e)
	}
	if got != releasedMixedEOLInitialChecksum {
		t.Fatal("release history was rewritten")
	}
	repository, e := LoadMigrationVersions(migrations.Files)
	if e != nil {
		t.Fatal(e)
	}
	applied, e := ReadMigrationVersions(ctx, database.SQL)
	if e != nil {
		t.Fatal(e)
	}
	if e := ValidateMigrationVersionPrefix(repository, applied, true); e != nil {
		t.Fatal(e)
	}
}
