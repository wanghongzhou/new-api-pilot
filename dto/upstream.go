package dto

type UpstreamStatus struct {
	Version           string
	SystemName        string
	QuotaPerUnit      string
	USDExchangeRate   string
	DataExportEnabled bool
}

type UpstreamIdentity struct {
	ID          int64
	Username    string
	DisplayName string
	Role        int32
	Status      int32
	Group       string
}

type UpstreamUser struct {
	UpstreamIdentity
	Quota        int64
	UsedQuota    int64
	RequestCount int64
	CreatedAt    int64
	LastLoginAt  int64
	Deleted      bool
}

type UpstreamUserPage struct {
	Page     int
	PageSize int
	Total    int64
	Items    []UpstreamUser
}

type UpstreamUserSnapshot struct {
	Total int64
	Items []UpstreamUser
}

type UpstreamChannel struct {
	ID               int64
	Name             string
	Type             int
	Status           int32
	TestTime         int64
	ResponseTimeMS   int64
	Balance          string
	BalanceUpdatedAt int64
	Models           string
	Group            string
	UsedQuota        int64
	Priority         int64
	Weight           int64
	AutoBan          int
	Tag              string
}

type UpstreamChannelPage struct {
	Page     int
	PageSize int
	Total    int64
	Items    []UpstreamChannel
}

type UpstreamChannelSnapshot struct {
	Total int64
	Items []UpstreamChannel
}

type UpstreamTopup struct {
	ID, UserID, Amount             int64
	Money                          string
	PaymentMethod, PaymentProvider string
	CreateTime, CompleteTime       int64
	Status                         string
}
type UpstreamTopupPage struct {
	Page, PageSize int
	Total          int64
	Items          []UpstreamTopup
}
type UpstreamTopupSnapshot struct {
	Total, MaxID int64
	Items        []UpstreamTopup
}

type UpstreamRedemption struct {
	ID, UserID, Quota         int64
	Name                      string
	Status                    int
	CreatedTime, RedeemedTime int64
	UsedUserID, ExpiredTime   int64
}
type UpstreamRedemptionPage struct {
	Page, PageSize int
	Total          int64
	Items          []UpstreamRedemption
}
type UpstreamRedemptionSnapshot struct {
	Total, MaxID int64
	Items        []UpstreamRedemption
}

type UpstreamFlowRow struct {
	UserID       int64
	Username     string
	ModelName    string
	ChannelID    int64
	UseGroup     string
	TokenID      int64
	TokenName    string
	NodeName     string
	RequestCount int64
	Quota        int64
	TokenUsed    int64
}

type UpstreamDataRow struct {
	ModelName    string
	CreatedAt    int64
	RequestCount int64
	Quota        int64
	TokenUsed    int64
}

type UpstreamInstance struct {
	NodeName           string
	Status             string
	StaleAfterSeconds  int64
	StartedAt          int64
	LastSeenAt         int64
	IsMaster           *bool
	RuntimeVersion     string
	GOOS               string
	GOARCH             string
	Hostname           string
	CPUPercent         *float64
	MemoryPercent      *float64
	StorageTotalBytes  *int64
	StorageUsedBytes   *int64
	StorageFreeBytes   *int64
	StorageUsedPercent *float64
}

type UpstreamLogStat struct {
	Quota int64
	RPM   int64
	TPM   int64
}

type UpstreamPerformanceModel struct {
	ModelName    string  `json:"model_name"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTPS       float64 `json:"avg_tps"`
	RequestCount int64   `json:"request_count"`
}
type UpstreamPerformanceSummary struct {
	Models []UpstreamPerformanceModel `json:"models"`
}

type UpstreamPerformanceCounters struct{ RequestCount, SuccessCount, TotalLatencyMS, TTFTSumMS, TTFTCount, OutputTokens, GenerationMS *int64 }
type UpstreamPerformanceBucket struct {
	Timestamp                                    int64
	AvgTTFTMS, AvgLatencyMS, SuccessRate, AvgTPS string
	Counters                                     UpstreamPerformanceCounters
}
type UpstreamPerformanceGroupHistory struct {
	Group  string
	Series []UpstreamPerformanceBucket
}
type UpstreamPerformanceModelHistory struct {
	ModelName, SeriesSchema string
	Groups                  []UpstreamPerformanceGroupHistory
}
type UpstreamPerformanceHistory struct {
	Models       []UpstreamPerformanceModelHistory
	CounterReady bool
}
