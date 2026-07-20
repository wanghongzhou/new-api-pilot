package ops

import (
	"context"
	"os"
	"testing"
	"time"

	"new-api-pilot/model"
)

func TestMySQLDatabaseIdentityDistinguishesServersWithSameDatabaseName(t *testing.T) {
	primaryDSN := os.Getenv("TEST_DATABASE_DSN")
	secondaryDSN := os.Getenv("TEST_DATABASE_DSN_SECONDARY")
	if primaryDSN == "" || secondaryDSN == "" {
		t.Skip("TEST_DATABASE_DSN and TEST_DATABASE_DSN_SECONDARY are not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	open := func(dsn string) *model.Database {
		t.Helper()
		database, err := model.Open(ctx, model.Options{
			DSN: dsn, MaxIdle: 1, MaxOpen: 2, MaxLifetime: time.Minute,
		})
		if err != nil {
			t.Fatalf("open database identity target: %v", err)
		}
		t.Cleanup(func() { _ = database.Close() })
		return database
	}
	primary, err := RunDatabaseIdentity(ctx, open(primaryDSN).SQL)
	if err != nil {
		t.Fatalf("read primary database identity: %v", err)
	}
	secondary, err := RunDatabaseIdentity(ctx, open(secondaryDSN).SQL)
	if err != nil {
		t.Fatalf("read secondary database identity: %v", err)
	}
	if primary.Database == "" || primary.Database != secondary.Database {
		t.Fatalf("database names differ: primary=%#v secondary=%#v", primary, secondary)
	}
	if primary.ServerUUID == secondary.ServerUUID {
		t.Fatalf("distinct MySQL servers returned the same UUID: %s", primary.ServerUUID)
	}
}
