package ops

import (
	"context"
	"testing"
)

func TestRunDatabaseIdentityRejectsMissingDatabase(t *testing.T) {
	report, err := RunDatabaseIdentity(context.Background(), nil)
	if err == nil || report.Status != "failed" || report.Error == nil ||
		report.Error.Code != "DATABASE_IDENTITY_CONFIG_INVALID" || report.Command != "database-identity" {
		t.Fatalf("database identity report = %#v, error = %v", report, err)
	}
}
