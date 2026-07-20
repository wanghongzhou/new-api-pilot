package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	a49AcceptanceID = "A49"
	a49FullMode     = "full"
	a49SmokeMode    = "smoke"
)

type a49Endpoint struct {
	Name     string `yaml:"name" json:"name"`
	Scenario string `yaml:"scenario" json:"scenario"`
	Path     string `yaml:"path" json:"path"`
}

var a49RequiredEndpoints = []a49Endpoint{
	{Name: "list_sites", Scenario: "list", Path: "/api/sites?p=1&page_size=20&sort_by=updated_at&sort_order=desc"},
	{Name: "list_customers", Scenario: "list", Path: "/api/customers?p=1&page_size=20&sort_by=updated_at&sort_order=desc"},
	{Name: "list_accounts", Scenario: "list", Path: "/api/accounts?p=1&page_size=20&managed_status=active&sort_by=updated_at&sort_order=desc"},
	{Name: "hourly_global_31d", Scenario: "hourly", Path: "/api/statistics/global?start_timestamp=1765983600&end_timestamp=1768662000&granularity=hour&p=1&page_size=100&sort_by=bucket_start&sort_order=desc"},
	{Name: "dashboard_summary", Scenario: "dashboard", Path: "/api/dashboard/summary"},
	{Name: "dashboard_trend", Scenario: "dashboard", Path: "/api/dashboard/trend?days=30"},
	{Name: "dashboard_top_site", Scenario: "dashboard", Path: "/api/dashboard/top?type=site&metric=request_count&limit=5"},
	{Name: "dashboard_top_customer", Scenario: "dashboard", Path: "/api/dashboard/top?type=customer&metric=request_count&limit=5"},
	{Name: "dashboard_top_model", Scenario: "dashboard", Path: "/api/dashboard/top?type=model&metric=request_count&limit=5"},
	{Name: "dashboard_top_channel", Scenario: "dashboard", Path: "/api/dashboard/top?type=channel&metric=request_count&limit=5"},
	{Name: "dashboard_health", Scenario: "dashboard", Path: "/api/dashboard/health"},
}

type a49CapacityFixture struct {
	Seed                                 int64         `yaml:"seed"`
	Customers                            int           `yaml:"customers"`
	Sites                                int           `yaml:"sites"`
	RemoteUsers                          int           `yaml:"remote_users"`
	RemoteUsersPerSite                   int           `yaml:"remote_users_per_site"`
	ManagedAccounts                      int           `yaml:"managed_accounts"`
	ManagedAccountsPerSite               int           `yaml:"managed_accounts_per_site"`
	UsageFactHourlyRows30D               int64         `yaml:"usage_fact_hourly_rows_30d"`
	FactDays                             int           `yaml:"fact_days"`
	FactHoursLocal                       []int         `yaml:"fact_hours_local"`
	ActiveChannelsPerSite                int           `yaml:"active_channels_per_site"`
	ActiveModelsPerSite                  int           `yaml:"active_models_per_site"`
	ConcurrentReadUsers                  int           `yaml:"concurrent_read_users"`
	ViewerUsernamePrefix                 string        `yaml:"viewer_username_prefix"`
	CollectionWindowStartUnix            int64         `yaml:"collection_window_start_unix"`
	CollectionWindowEndUnix              int64         `yaml:"collection_window_end_unix"`
	HourlyQueryStartUnix                 int64         `yaml:"hourly_query_start_unix"`
	HourlyQueryEndUnix                   int64         `yaml:"hourly_query_end_unix"`
	UpstreamPreviousHourP95Seconds       int           `yaml:"upstream_previous_hour_p95_seconds"`
	WarmupSeconds                        int           `yaml:"warmup_seconds"`
	SampleSeconds                        int           `yaml:"sample_seconds"`
	ThinkTimeMilliseconds                int           `yaml:"think_time_milliseconds"`
	RequestTimeoutSeconds                int           `yaml:"request_timeout_seconds"`
	MinimumSuccessfulRequestsPerEndpoint int           `yaml:"minimum_successful_requests_per_endpoint"`
	MaximumErrorRate                     float64       `yaml:"maximum_error_rate"`
	Endpoints                            []a49Endpoint `yaml:"endpoints"`
	PopulatedSummaryTables               []string      `yaml:"populated_summary_tables"`
	Targets                              struct {
		ListP95Seconds               float64 `yaml:"list_p95_seconds"`
		Hourly31DP95Seconds          float64 `yaml:"hourly_31d_p95_seconds"`
		DashboardP95Seconds          float64 `yaml:"dashboard_p95_seconds"`
		PreviousHourCompleteByMinute int     `yaml:"previous_hour_complete_by_minute"`
	} `yaml:"targets"`
	Resources struct {
		MinimumCPUs            int   `yaml:"minimum_cpus"`
		MinimumMemoryBytes     int64 `yaml:"minimum_memory_bytes"`
		MinimumDockerFreeBytes int64 `yaml:"minimum_docker_free_bytes"`
		MinimumEvidenceBytes   int64 `yaml:"minimum_evidence_free_bytes"`
	} `yaml:"resources"`
}

type a49Fixture struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Clock         struct {
		Timezone string `yaml:"timezone"`
		Now      string `yaml:"now"`
		NowUnix  int64  `yaml:"now_unix"`
	} `yaml:"clock"`
	Capacity a49CapacityFixture `yaml:"capacity"`
}

type a49RunProfile struct {
	Mode               string             `json:"mode"`
	AcceptanceEligible bool               `json:"acceptance_eligible"`
	FixturePath        string             `json:"fixture_path"`
	FixtureSHA256      string             `json:"fixture_sha256"`
	Fixture            a49Fixture         `json:"-"`
	Capacity           a49CapacityFixture `json:"capacity"`
}

func loadA49RunProfile(path, mode string) (a49RunProfile, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return a49RunProfile{}, fmt.Errorf("read F05 fixture: %w", err)
	}
	var fixture a49Fixture
	if err := yaml.Unmarshal(payload, &fixture); err != nil {
		return a49RunProfile{}, fmt.Errorf("parse F05 fixture: %w", err)
	}
	digest := sha256.Sum256(payload)
	profile := a49RunProfile{
		Mode: mode, AcceptanceEligible: mode == a49FullMode, FixturePath: path,
		FixtureSHA256: hex.EncodeToString(digest[:]), Fixture: fixture, Capacity: fixture.Capacity,
	}
	if err := validateA49Fixture(fixture); err != nil {
		return a49RunProfile{}, err
	}
	switch mode {
	case a49FullMode:
		if err := validateA49FormalCapacity(profile.Capacity); err != nil {
			return a49RunProfile{}, err
		}
	case a49SmokeMode:
		profile.AcceptanceEligible = false
		profile.Capacity = a49SmokeCapacity(profile.Capacity)
	default:
		return a49RunProfile{}, fmt.Errorf("A49 mode must be %q or %q", a49FullMode, a49SmokeMode)
	}
	if err := validateA49EffectiveCapacity(profile.Capacity); err != nil {
		return a49RunProfile{}, err
	}
	return profile, nil
}

func validateA49Fixture(fixture a49Fixture) error {
	if fixture.SchemaVersion != 2 || fixture.FixtureID != "F05" {
		return errors.New("F05 fixture must use schema_version 2 and fixture_id F05")
	}
	if fixture.Clock.Timezone != "Asia/Shanghai" || fixture.Clock.NowUnix <= 0 {
		return errors.New("F05 clock must be a positive Asia/Shanghai instant")
	}
	parsed, err := time.Parse(time.RFC3339, fixture.Clock.Now)
	if err != nil || parsed.Unix() != fixture.Clock.NowUnix || parsed.Format("-07:00") != "+08:00" ||
		fixture.Clock.Now != "2026-01-17T23:59:59+08:00" || fixture.Clock.NowUnix != 1768665599 {
		return errors.New("F05 clock.now and clock.now_unix must identify the same +08:00 instant")
	}
	return nil
}

func validateA49FormalCapacity(capacity a49CapacityFixture) error {
	expected := []struct {
		name string
		got  int64
		want int64
	}{
		{"sites", int64(capacity.Sites), 50},
		{"seed", capacity.Seed, 49050117},
		{"customers", int64(capacity.Customers), 1000},
		{"remote_users", int64(capacity.RemoteUsers), 100000},
		{"remote_users_per_site", int64(capacity.RemoteUsersPerSite), 2000},
		{"managed_accounts", int64(capacity.ManagedAccounts), 10000},
		{"managed_accounts_per_site", int64(capacity.ManagedAccountsPerSite), 200},
		{"usage_fact_hourly_rows_30d", capacity.UsageFactHourlyRows30D, 15000000},
		{"active_channels_per_site", int64(capacity.ActiveChannelsPerSite), 100},
		{"active_models_per_site", int64(capacity.ActiveModelsPerSite), 200},
		{"concurrent_read_users", int64(capacity.ConcurrentReadUsers), 20},
		{"warmup_seconds", int64(capacity.WarmupSeconds), 120},
		{"sample_seconds", int64(capacity.SampleSeconds), 600},
		{"think_time_milliseconds", int64(capacity.ThinkTimeMilliseconds), 200},
		{"request_timeout_seconds", int64(capacity.RequestTimeoutSeconds), 10},
		{"minimum_successful_requests_per_endpoint", int64(capacity.MinimumSuccessfulRequestsPerEndpoint), 1000},
		{"collection_window_start_unix", capacity.CollectionWindowStartUnix, 1765983600},
		{"collection_window_end_unix", capacity.CollectionWindowEndUnix, 1768665600},
		{"hourly_query_start_unix", capacity.HourlyQueryStartUnix, 1765983600},
		{"hourly_query_end_unix", capacity.HourlyQueryEndUnix, 1768662000},
		{"upstream_previous_hour_p95_seconds", int64(capacity.UpstreamPreviousHourP95Seconds), 60},
		{"previous_hour_complete_by_minute", int64(capacity.Targets.PreviousHourCompleteByMinute), 15},
		{"minimum_cpus", int64(capacity.Resources.MinimumCPUs), 8},
		{"minimum_memory_bytes", capacity.Resources.MinimumMemoryBytes, 17179869184},
		{"minimum_docker_free_bytes", capacity.Resources.MinimumDockerFreeBytes, 37580963840},
		{"minimum_evidence_free_bytes", capacity.Resources.MinimumEvidenceBytes, 5368709120},
	}
	for _, item := range expected {
		if item.got != item.want {
			return fmt.Errorf("formal F05 %s=%d, want %d", item.name, item.got, item.want)
		}
	}
	if capacity.MaximumErrorRate != 0.001 || capacity.Targets.ListP95Seconds != 1 ||
		capacity.Targets.Hourly31DP95Seconds != 3 || capacity.Targets.DashboardP95Seconds != 3 {
		return errors.New("formal F05 latency/error thresholds differ from A49")
	}
	if capacity.FactDays != 30 || !intSlicesEqualA49(capacity.FactHoursLocal, []int{0, 4, 8, 12, 16}) {
		return errors.New("formal F05 facts must cover 30 days with five deterministic hours per day")
	}
	if capacity.ViewerUsernamePrefix != "a49-viewer-" {
		return errors.New("formal F05 viewer username prefix differs from A49")
	}
	if !stringSlicesEqualA49Profile(capacity.PopulatedSummaryTables, []string{
		"usage_fact_daily", "account_stat_daily", "customer_stat_daily", "site_stat_hourly", "site_stat_daily",
		"global_stat_hourly", "global_stat_daily", "model_stat_daily", "channel_stat_daily",
	}) {
		return errors.New("formal F05 populated summary tables differ from A49")
	}
	if err := validateA49Endpoints(capacity.Endpoints); err != nil {
		return err
	}
	return nil
}

func intSlicesEqualA49(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringSlicesEqualA49Profile(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateA49EffectiveCapacity(capacity a49CapacityFixture) error {
	if capacity.Seed <= 0 || capacity.Customers <= 0 || capacity.Sites <= 0 || capacity.RemoteUsersPerSite <= 0 ||
		capacity.ManagedAccountsPerSite <= 0 || capacity.ManagedAccountsPerSite > capacity.RemoteUsersPerSite ||
		capacity.ActiveChannelsPerSite <= 0 || capacity.ActiveModelsPerSite <= 0 || capacity.FactDays <= 0 ||
		len(capacity.FactHoursLocal) == 0 || capacity.ConcurrentReadUsers <= 0 ||
		capacity.WarmupSeconds <= 0 || capacity.SampleSeconds <= 0 || capacity.ThinkTimeMilliseconds < 0 ||
		capacity.RequestTimeoutSeconds <= 0 || capacity.MinimumSuccessfulRequestsPerEndpoint <= 0 ||
		capacity.MaximumErrorRate < 0 || capacity.MaximumErrorRate > 1 {
		return errors.New("A49 effective capacity contains invalid positive bounds")
	}
	if capacity.Sites*capacity.RemoteUsersPerSite != capacity.RemoteUsers ||
		capacity.Sites*capacity.ManagedAccountsPerSite != capacity.ManagedAccounts {
		return errors.New("A49 site-level user/account cardinalities do not match totals")
	}
	if capacity.Customers < capacity.ManagedAccountsPerSite ||
		capacity.RemoteUsersPerSite%capacity.ActiveModelsPerSite != 0 ||
		capacity.RemoteUsersPerSite%capacity.ActiveChannelsPerSite != 0 {
		return errors.New("A49 customer/model/channel cardinalities must support uniform deterministic summaries")
	}
	expectedFacts := int64(capacity.Sites) * int64(capacity.RemoteUsersPerSite) *
		int64(capacity.FactDays) * int64(len(capacity.FactHoursLocal))
	if expectedFacts != capacity.UsageFactHourlyRows30D {
		return fmt.Errorf("A49 fact formula yields %d rows, fixture declares %d", expectedFacts, capacity.UsageFactHourlyRows30D)
	}
	hours := append([]int(nil), capacity.FactHoursLocal...)
	sort.Ints(hours)
	for index, hour := range hours {
		if hour < 0 || hour > 23 || (index > 0 && hour == hours[index-1]) {
			return errors.New("A49 fact_hours_local must contain unique values from 0 through 23")
		}
	}
	if capacity.CollectionWindowStartUnix <= 0 || capacity.CollectionWindowStartUnix%3600 != 0 ||
		capacity.CollectionWindowEndUnix <= capacity.CollectionWindowStartUnix || capacity.CollectionWindowEndUnix%3600 != 0 ||
		capacity.HourlyQueryStartUnix < capacity.CollectionWindowStartUnix ||
		capacity.HourlyQueryEndUnix > capacity.CollectionWindowEndUnix ||
		capacity.HourlyQueryStartUnix%3600 != 0 || capacity.HourlyQueryEndUnix%3600 != 0 ||
		capacity.HourlyQueryEndUnix-capacity.HourlyQueryStartUnix != int64(31*24*time.Hour/time.Second) {
		return errors.New("A49 windows must contain one aligned 31-day hourly query")
	}
	if !strings.HasPrefix(capacity.ViewerUsernamePrefix, "a49-") {
		return errors.New("A49 viewer_username_prefix must be visibly acceptance-scoped")
	}
	if err := validateA49Endpoints(capacity.Endpoints); err != nil {
		return err
	}
	return nil
}

func validateA49Endpoints(endpoints []a49Endpoint) error {
	if len(endpoints) != len(a49RequiredEndpoints) {
		return fmt.Errorf("A49 requires eleven endpoint definitions (three list, hourly, seven dashboard), got %d", len(endpoints))
	}
	for index, expected := range a49RequiredEndpoints {
		actual := endpoints[index]
		if actual != expected {
			return fmt.Errorf("A49 endpoint %d must be name=%q scenario=%q path=%q", index, expected.Name, expected.Scenario, expected.Path)
		}
	}
	return nil
}

func a49SmokeCapacity(full a49CapacityFixture) a49CapacityFixture {
	result := full
	result.Customers = 10
	result.Sites = 2
	result.RemoteUsersPerSite = 20
	result.RemoteUsers = result.Sites * result.RemoteUsersPerSite
	result.ManagedAccountsPerSite = 5
	result.ManagedAccounts = result.Sites * result.ManagedAccountsPerSite
	result.ActiveChannelsPerSite = 5
	result.ActiveModelsPerSite = 10
	result.FactDays = 2
	result.FactHoursLocal = []int{0, 4}
	result.UsageFactHourlyRows30D = int64(result.Sites * result.RemoteUsersPerSite * result.FactDays * len(result.FactHoursLocal))
	result.ConcurrentReadUsers = 2
	result.WarmupSeconds = 1
	result.SampleSeconds = 3
	result.ThinkTimeMilliseconds = 50
	result.MinimumSuccessfulRequestsPerEndpoint = 1
	result.Resources.MinimumCPUs = 2
	result.Resources.MinimumMemoryBytes = 2 * 1024 * 1024 * 1024
	result.Resources.MinimumDockerFreeBytes = 3 * 1024 * 1024 * 1024
	result.Resources.MinimumEvidenceBytes = 512 * 1024 * 1024
	return result
}

func (profile a49RunProfile) endpoint(name string) (a49Endpoint, bool) {
	for _, endpoint := range profile.Capacity.Endpoints {
		if endpoint.Name == name {
			return endpoint, true
		}
	}
	return a49Endpoint{}, false
}

func (profile a49RunProfile) evidenceClass() string {
	if profile.AcceptanceEligible {
		return "formal"
	}
	return "smoke"
}
