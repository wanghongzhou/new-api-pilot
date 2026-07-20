package main

type seedReport struct {
	SchemaVersion       int              `json:"schema_version"`
	AcceptanceID        string           `json:"acceptance_id"`
	Status              string           `json:"status"`
	FixtureID           string           `json:"fixture_id"`
	FixtureSHA256       string           `json:"fixture_sha256"`
	Database            string           `json:"database"`
	FixedHourUnix       int64            `json:"fixed_hour_unix"`
	DateKey             int              `json:"date_key"`
	LastBusinessTime    int64            `json:"last_business_time_unix"`
	SiteTokenEncrypted  bool             `json:"site_token_encrypted"`
	SecretEncrypted     bool             `json:"secret_setting_encrypted"`
	TaskStatuses        map[string]int64 `json:"task_statuses"`
	WindowStatuses      map[string]int64 `json:"run_window_statuses"`
	CollectionStatuses  map[string]int64 `json:"collection_window_statuses"`
	AggregateRows       map[string]int64 `json:"aggregate_rows"`
	ProductionReleaseOK bool             `json:"production_release_authorized"`
}

type aggregateMetric struct {
	Rows        int64  `json:"rows"`
	Requests    string `json:"request_count"`
	Quota       string `json:"quota"`
	Tokens      string `json:"token_used"`
	ActiveUsers string `json:"active_users"`
	DataStatus  string `json:"data_status"`
	IsFinal     bool   `json:"is_final"`
}

type scopeAggregates struct {
	Hourly aggregateMetric `json:"hourly"`
	Daily  aggregateMetric `json:"daily"`
}

type snapshotReport struct {
	SchemaVersion          int                        `json:"schema_version"`
	AcceptanceID           string                     `json:"acceptance_id"`
	Status                 string                     `json:"status"`
	Role                   string                     `json:"role"`
	Database               string                     `json:"database"`
	ServerUUID             string                     `json:"server_uuid"`
	ServerUUIDFingerprint  string                     `json:"server_uuid_fingerprint"`
	MySQLVersion           string                     `json:"mysql_version"`
	SnapshotSHA256         string                     `json:"snapshot_sha256"`
	TableCounts            map[string]int64           `json:"table_counts"`
	TaskStatuses           map[string]int64           `json:"task_statuses"`
	RunWindowStatuses      map[string]int64           `json:"run_window_statuses"`
	CollectionWindowStates map[string]int64           `json:"collection_window_statuses"`
	ActiveKeys             map[string]int64           `json:"active_keys"`
	Aggregates             map[string]scopeAggregates `json:"aggregates"`
	SiteTokenDecrypted     bool                       `json:"site_token_decrypted"`
	SecretSettingDecrypted bool                       `json:"secret_setting_decrypted"`
	LastBusinessTime       int64                      `json:"last_business_time_unix"`
	ProductionReleaseOK    bool                       `json:"production_release_authorized"`
}

type simpleStatusReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
}

type rpoRTOReport struct {
	SchemaVersion       int    `json:"schema_version"`
	AcceptanceID        string `json:"acceptance_id"`
	Status              string `json:"status"`
	BackupCreatedAtUnix int64  `json:"backup_created_at_unix"`
	LastBusinessTime    int64  `json:"last_business_time_unix"`
	RecoverableAge      int64  `json:"recoverable_age_seconds"`
	ActualDataLoss      int64  `json:"actual_data_loss_seconds"`
	RPOSeconds          int64  `json:"rpo_seconds"`
	RTOSeconds          int64  `json:"rto_seconds"`
	RPOLimitSeconds     int64  `json:"rpo_limit_seconds"`
	RTOLimitSeconds     int64  `json:"rto_limit_seconds"`
	RPOPassed           bool   `json:"rpo_passed"`
	RTOPassed           bool   `json:"rto_passed"`
}

type finalReport struct {
	SchemaVersion               int             `json:"schema_version"`
	AcceptanceID                string          `json:"acceptance_id"`
	Status                      string          `json:"status"`
	Passed                      bool            `json:"passed"`
	EvidenceClass               string          `json:"evidence_class"`
	AcceptanceEligible          bool            `json:"acceptance_eligible"`
	Scope                       string          `json:"scope"`
	ProductionReleaseAuthorized bool            `json:"production_release_authorized"`
	RPOSeconds                  int64           `json:"rpo_seconds"`
	RTOSeconds                  int64           `json:"rto_seconds"`
	Checks                      map[string]bool `json:"checks"`
	Violations                  []string        `json:"violations"`
}
