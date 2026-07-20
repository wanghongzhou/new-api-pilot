package ops

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
)

var mysqlServerUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func RunDatabaseIdentity(ctx context.Context, database *sql.DB) (DatabaseIdentityReport, error) {
	report := newDatabaseIdentityReport()
	if database == nil {
		report.Error = &OperationError{Code: "DATABASE_IDENTITY_CONFIG_INVALID"}
		return report, errors.New("database identity configuration is invalid")
	}
	if err := database.QueryRowContext(ctx, "SELECT @@server_uuid, DATABASE(), VERSION()").Scan(
		&report.ServerUUID,
		&report.Database,
		&report.MySQLVersion,
	); err != nil {
		report.Error = &OperationError{Code: "DATABASE_IDENTITY_DATABASE_ERROR"}
		return report, err
	}
	if !mysqlServerUUIDPattern.MatchString(report.ServerUUID) || report.Database == "" || report.MySQLVersion == "" {
		report.Error = &OperationError{Code: "DATABASE_IDENTITY_INVALID"}
		return report, errors.New("database identity is invalid")
	}
	report.Status = "success"
	report.Error = nil
	return report, nil
}
