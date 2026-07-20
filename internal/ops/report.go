package ops

const ReportSchemaVersion = 1

type OperationError struct {
	Code    string `json:"code"`
	RowType string `json:"row_type,omitempty"`
	RowID   string `json:"row_id,omitempty"`
}

type ReencryptCounts struct {
	Total      int64 `json:"total"`
	SiteTokens int64 `json:"site_tokens"`
	Settings   int64 `json:"secret_settings"`
	OldKey     int64 `json:"old_key"`
	NewKey     int64 `json:"new_key"`
	Staged     int64 `json:"staged"`
	Updated    int64 `json:"updated"`
}

type ReencryptReport struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	Status        string          `json:"status"`
	DryRun        bool            `json:"dry_run"`
	Resumed       bool            `json:"resumed"`
	OldKeyID      string          `json:"old_key_id"`
	NewKeyID      string          `json:"new_key_id"`
	Counts        ReencryptCounts `json:"counts"`
	Error         *OperationError `json:"error,omitempty"`
}

type DatabaseIdentityReport struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	Status        string          `json:"status"`
	ServerUUID    string          `json:"server_uuid,omitempty"`
	Database      string          `json:"database,omitempty"`
	MySQLVersion  string          `json:"mysql_version,omitempty"`
	Error         *OperationError `json:"error,omitempty"`
}

type VerifyCheck struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Code    string         `json:"code,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type VerifySummary struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type VerifyReport struct {
	SchemaVersion   int             `json:"schema_version"`
	Command         string          `json:"command"`
	Mode            string          `json:"mode"`
	Status          string          `json:"status"`
	EncryptionKeyID string          `json:"encryption_key_id"`
	BackupID        string          `json:"backup_id,omitempty"`
	Checks          []VerifyCheck   `json:"checks"`
	Summary         VerifySummary   `json:"summary"`
	Error           *OperationError `json:"error,omitempty"`
}

func newVerifyReport(keyID string) VerifyReport {
	return VerifyReport{
		SchemaVersion:   ReportSchemaVersion,
		Command:         "verify-restore",
		Mode:            "full",
		Status:          "failed",
		EncryptionKeyID: ShortFingerprint(keyID),
		Checks:          make([]VerifyCheck, 0, 10),
	}
}

func newDatabaseIdentityReport() DatabaseIdentityReport {
	return DatabaseIdentityReport{
		SchemaVersion: ReportSchemaVersion,
		Command:       "database-identity",
		Status:        "failed",
	}
}

func (report *VerifyReport) addCheck(check VerifyCheck) {
	report.Checks = append(report.Checks, check)
	if check.Status == "passed" {
		report.Summary.Passed++
		return
	}
	report.Summary.Failed++
	if report.Error == nil {
		report.Error = &OperationError{Code: check.Code}
	}
}

func newReencryptReport(oldKeyID, newKeyID string, dryRun bool) ReencryptReport {
	return ReencryptReport{
		SchemaVersion: ReportSchemaVersion,
		Command:       "secrets reencrypt",
		Status:        "failed",
		DryRun:        dryRun,
		OldKeyID:      ShortFingerprint(oldKeyID),
		NewKeyID:      ShortFingerprint(newKeyID),
	}
}

func ShortFingerprint(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
