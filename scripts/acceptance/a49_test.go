package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestA49FullAndSmokeProfiles(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	full, err := loadA49RunProfile(fixture, a49FullMode)
	if err != nil {
		t.Fatalf("load full profile: %v", err)
	}
	if !full.AcceptanceEligible || full.Capacity.Sites != 50 || full.Capacity.RemoteUsers != 100000 ||
		full.Capacity.ManagedAccounts != 10000 || full.Capacity.UsageFactHourlyRows30D != 15000000 ||
		len(full.Capacity.Endpoints) != 11 {
		t.Fatalf("unexpected full profile: %#v", full.Capacity)
	}
	counts := map[string]int{}
	for _, endpoint := range full.Capacity.Endpoints {
		counts[endpoint.Scenario]++
	}
	if counts["list"] != 3 || counts["hourly"] != 1 || counts["dashboard"] != 7 {
		t.Fatalf("endpoint matrix = %v", counts)
	}
	smoke, err := loadA49RunProfile(fixture, a49SmokeMode)
	if err != nil {
		t.Fatalf("load smoke profile: %v", err)
	}
	if smoke.AcceptanceEligible || smoke.Capacity.Sites != 2 || smoke.Capacity.ConcurrentReadUsers != 2 ||
		smoke.Capacity.UsageFactHourlyRows30D != 160 || smoke.Capacity.WarmupSeconds != 1 || smoke.Capacity.SampleSeconds != 3 {
		t.Fatalf("unexpected smoke profile: %#v", smoke.Capacity)
	}
}

func TestA49RunnerStaticSafetyContract(t *testing.T) {
	repositoryRoot := acceptanceRepositoryRoot(t)
	if _, err := os.Stat(filepath.Join(repositoryRoot, "scripts", "acceptance", "run-a49.ps1")); err != nil {
		t.Skipf("acceptance scripts are not present in this production test image: %v", err)
	}
	runnerPath := filepath.Join(repositoryRoot, "scripts", "acceptance", "run-a49.ps1")
	payload, err := os.ReadFile(runnerPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	pathGuard, err := os.ReadFile(filepath.Join(repositoryRoot, "scripts", "acceptance", "a49-path-guard.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	gitState, err := os.ReadFile(filepath.Join(repositoryRoot, "scripts", "acceptance", "a49-git-state.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	staticContract := text + string(pathGuard) + string(gitState)
	for _, required := range []string{
		"'network', 'create', '--internal'", "capacity-serve", "A49_FIXED_NOW_UNIX",
		"A49_ISOLATED_MYSQL=true", "A49_ISOLATED_LOAD=true", "A49_ISOLATED_REPORT=true",
		"a49-load-results.jsonl", "a49-app.log", "a49-report.json", "Remove-RunDockerResource",
		"Start-Job", "docker stats --no-stream", "a49-docker-stats.tsv",
		"--default-time-zone=+08:00", "@@global.time_zone",
		"Get-A49RepositoryRelativePath", "GetFullPath", "OrdinalIgnoreCase",
		"Get-A49GitState", "Invoke-A49GitProcess", "rev-list", "--all", "--count",
	} {
		if !strings.Contains(staticContract, required) {
			t.Fatalf("runner is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"docker system prune", "docker volume prune", "docker network prune", "--publish", "{{json .}}", "GetRelativePath", "& git",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("runner contains forbidden global/network mutation %q", forbidden)
		}
	}
	manifestPath := filepath.Join(acceptanceRepositoryRoot(t), "docs", "acceptance", "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("acceptance docs are not present in this production test image: %v", err)
	}
	if !strings.Contains(string(manifest), "acceptance_id: A49") ||
		!strings.Contains(string(manifest), `evidence_path: "planned:artifacts/acceptance/A49/"`) {
		t.Fatal("A49 planned evidence path was removed without a formal passing run")
	}
	fixturePath := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(fixture))
	fixtureManifest, err := os.ReadFile(filepath.Join("..", "..", "testdata", "design", "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fixtureManifest), digest+"  testdata/design/f05-ops-capacity.yaml") {
		t.Fatal("F05 fixture checksum was not updated")
	}
}

func acceptanceRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve acceptance test source")
	}
	directory := filepath.Dir(source)
	for _, candidate := range []string{"/build", "/src"} {
		if _, err := os.Stat(filepath.Join(candidate, "docs", "acceptance", "manifest.yaml")); err == nil {
			return candidate
		}
	}
	for {
		candidate := filepath.Join(directory, "docs", "acceptance", "manifest.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return directory
		}
		directory = parent
	}
}

func TestA49EvidenceGuardRequiresCanonicalRunner(t *testing.T) {
	formal := []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a49.ps1",
	}
	smoke := append(append([]string(nil), formal...), "-Smoke")
	if class, err := classifyA49Evidence(a49AcceptanceID, "artifacts/acceptance", formal); err != nil || class != "formal" {
		t.Fatalf("canonical formal command rejected: class=%q err=%v", class, err)
	}
	if class, err := classifyA49Evidence(a49AcceptanceID, "artifacts/smoke", smoke); err != nil || class != "smoke" {
		t.Fatalf("canonical smoke command rejected: class=%q err=%v", class, err)
	}
	for name, test := range map[string]struct {
		root    string
		command []string
	}{
		"arbitrary exit zero":  {"artifacts/acceptance", []string{"powershell.exe", "-Command", "exit 0"}},
		"smoke in formal root": {"artifacts/acceptance", smoke},
		"formal in smoke root": {"artifacts/smoke", formal},
		"extra argument":       {"artifacts/acceptance", append(append([]string(nil), formal...), "-Verbose")},
		"noncanonical root":    {"artifacts/acceptance-other", formal},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := classifyA49Evidence(a49AcceptanceID, test.root, test.command); err == nil {
				t.Fatal("unsafe A49 command/root combination was accepted")
			}
		})
	}
}

func TestA49SeedDoesNotReopenOneTemporaryDigitTable(t *testing.T) {
	payload, err := os.ReadFile("a49_seed.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "FROM a49_digit d0 CROSS JOIN a49_digit d1") {
		t.Fatal("MySQL 8.4 cannot reopen one TEMPORARY digit table through multiple aliases")
	}
	crossJoin := regexp.MustCompile("(?s)FROM\\s+(a49_[a-z0-9_]*seq)\\s+[a-z]+([^`]*)CROSS JOIN\\s+(a49_[a-z0-9_]*seq)\\s+[a-z]+")
	for _, match := range crossJoin.FindAllStringSubmatch(text, -1) {
		if match[1] == match[3] {
			t.Fatalf("MySQL 8.4 cannot reopen TEMPORARY sequence %s through multiple aliases", match[1])
		}
	}
	for index := 0; index < 5; index++ {
		if !strings.Contains(text, fmt.Sprintf("a49_digit%d", index)) {
			t.Fatalf("seed sequence is missing independent temporary digit table %d", index)
		}
	}
	for _, table := range []string{"a49_site_seq", "a49_user_seq", "a49_aux_seq"} {
		if !strings.Contains(text, table) {
			t.Fatalf("seed sequence is missing independent dimension table %s", table)
		}
	}
}

func TestA49FormalProfileCannotBeReduced(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	profile, err := loadA49RunProfile(fixture, a49FullMode)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*a49CapacityFixture){
		"seed":                               func(c *a49CapacityFixture) { c.Seed-- },
		"customers":                          func(c *a49CapacityFixture) { c.Customers-- },
		"sites":                              func(c *a49CapacityFixture) { c.Sites-- },
		"remote_users":                       func(c *a49CapacityFixture) { c.RemoteUsers-- },
		"remote_users_per_site":              func(c *a49CapacityFixture) { c.RemoteUsersPerSite-- },
		"managed_accounts":                   func(c *a49CapacityFixture) { c.ManagedAccounts-- },
		"managed_accounts_per_site":          func(c *a49CapacityFixture) { c.ManagedAccountsPerSite-- },
		"usage_fact_hourly_rows_30d":         func(c *a49CapacityFixture) { c.UsageFactHourlyRows30D-- },
		"fact_days":                          func(c *a49CapacityFixture) { c.FactDays-- },
		"fact_hours_local":                   func(c *a49CapacityFixture) { c.FactHoursLocal = []int{0, 4, 8, 12} },
		"active_channels_per_site":           func(c *a49CapacityFixture) { c.ActiveChannelsPerSite-- },
		"active_models_per_site":             func(c *a49CapacityFixture) { c.ActiveModelsPerSite-- },
		"concurrent_read_users":              func(c *a49CapacityFixture) { c.ConcurrentReadUsers-- },
		"viewer_username_prefix":             func(c *a49CapacityFixture) { c.ViewerUsernamePrefix = "a49-other-" },
		"collection_window_start_unix":       func(c *a49CapacityFixture) { c.CollectionWindowStartUnix += 3600 },
		"collection_window_end_unix":         func(c *a49CapacityFixture) { c.CollectionWindowEndUnix -= 3600 },
		"hourly_query_start_unix":            func(c *a49CapacityFixture) { c.HourlyQueryStartUnix += 3600 },
		"hourly_query_end_unix":              func(c *a49CapacityFixture) { c.HourlyQueryEndUnix -= 3600 },
		"upstream_previous_hour_p95_seconds": func(c *a49CapacityFixture) { c.UpstreamPreviousHourP95Seconds-- },
		"warmup_seconds":                     func(c *a49CapacityFixture) { c.WarmupSeconds-- },
		"sample_seconds":                     func(c *a49CapacityFixture) { c.SampleSeconds-- },
		"think_time_milliseconds":            func(c *a49CapacityFixture) { c.ThinkTimeMilliseconds-- },
		"request_timeout_seconds":            func(c *a49CapacityFixture) { c.RequestTimeoutSeconds-- },
		"minimum_successful_requests":        func(c *a49CapacityFixture) { c.MinimumSuccessfulRequestsPerEndpoint-- },
		"maximum_error_rate":                 func(c *a49CapacityFixture) { c.MaximumErrorRate = 0.002 },
		"populated_summary_tables":           func(c *a49CapacityFixture) { c.PopulatedSummaryTables = c.PopulatedSummaryTables[:8] },
		"list_p95_seconds":                   func(c *a49CapacityFixture) { c.Targets.ListP95Seconds = 2 },
		"hourly_31d_p95_seconds":             func(c *a49CapacityFixture) { c.Targets.Hourly31DP95Seconds = 4 },
		"dashboard_p95_seconds":              func(c *a49CapacityFixture) { c.Targets.DashboardP95Seconds = 4 },
		"previous_hour_complete_by_minute":   func(c *a49CapacityFixture) { c.Targets.PreviousHourCompleteByMinute-- },
		"minimum_cpus":                       func(c *a49CapacityFixture) { c.Resources.MinimumCPUs-- },
		"minimum_memory_bytes":               func(c *a49CapacityFixture) { c.Resources.MinimumMemoryBytes-- },
		"minimum_docker_free_bytes":          func(c *a49CapacityFixture) { c.Resources.MinimumDockerFreeBytes-- },
		"minimum_evidence_free_bytes":        func(c *a49CapacityFixture) { c.Resources.MinimumEvidenceBytes-- },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			capacity := profile.Capacity
			mutate(&capacity)
			if err := validateA49FormalCapacity(capacity); err == nil {
				t.Fatal("reduced or altered formal profile was accepted")
			}
		})
	}
}

func TestA49FormalEndpointMatrixIsExact(t *testing.T) {
	for index := range a49RequiredEndpoints {
		for field, mutate := range map[string]func(*a49Endpoint){
			"name":     func(endpoint *a49Endpoint) { endpoint.Name += "_changed" },
			"scenario": func(endpoint *a49Endpoint) { endpoint.Scenario += "_changed" },
			"path":     func(endpoint *a49Endpoint) { endpoint.Path += "&changed=true" },
		} {
			t.Run(fmt.Sprintf("%02d_%s", index, field), func(t *testing.T) {
				endpoints := append([]a49Endpoint(nil), a49RequiredEndpoints...)
				mutate(&endpoints[index])
				if err := validateA49Endpoints(endpoints); err == nil {
					t.Fatal("altered endpoint matrix was accepted")
				}
			})
		}
	}
}

func TestA49NearestRankIncludesTail(t *testing.T) {
	values := make([]int64, 100)
	for index := range values {
		values[index] = int64(index+1) * int64(time.Millisecond)
	}
	percentiles := a49Percentiles(values)
	if percentiles.P50Milliseconds != 50 || percentiles.P95Milliseconds != 95 ||
		percentiles.P99Milliseconds != 99 || percentiles.MaxMilliseconds != 100 {
		t.Fatalf("nearest-rank percentiles = %#v", percentiles)
	}
}

func TestA49EndpointContracts(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	profile, err := loadA49RunProfile(fixture, a49SmokeMode)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]any{
		"list_sites": map[string]any{"page": 1, "page_size": 20, "total": 2,
			"items": []any{map[string]any{"id": "1", "name": "站点", "management_status": "active", "statistics_status": "ready"}}},
		"list_customers": map[string]any{"page": 1, "page_size": 20, "total": 10,
			"items": []any{map[string]any{"id": "1", "name": "客户", "status": "cooperating", "account_count": 1}}},
		"list_accounts": map[string]any{"page": 1, "page_size": 20, "total": 10,
			"items": []any{map[string]any{"id": "1", "site_id": "1", "customer_id": "1", "remote_user_id": "1000001", "username": "remote", "quota": "1"}}},
		"dashboard_summary": map[string]any{
			"today":                 map[string]any{"request_count": "1", "as_of": profile.Capacity.HourlyQueryEndUnix},
			"active_accounts_today": "10", "site_count": 2, "customer_count": 10, "managed_account_count": 10,
			"realtime_as_of": profile.Fixture.Clock.NowUnix,
			"resource_as_of": profile.Fixture.Clock.NowUnix - profile.Fixture.Clock.NowUnix%60,
		},
	}
	for name, value := range tests {
		payload, _ := json.Marshal(value)
		if err := validateA49EndpointDTO(name, payload, profile); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if profile.Capacity.HourlyQueryEndUnix != profile.Fixture.Clock.NowUnix-profile.Fixture.Clock.NowUnix%3600 ||
		profile.Capacity.CollectionWindowEndUnix-profile.Capacity.HourlyQueryEndUnix != 3600 {
		t.Fatal("F05 must keep the current hour pending after the last complete hour")
	}
	wrongSummary := tests["dashboard_summary"].(map[string]any)
	wrongSummary["today"].(map[string]any)["as_of"] = profile.Fixture.Clock.NowUnix
	wrongPayload, _ := json.Marshal(wrongSummary)
	if err := validateA49EndpointDTO("dashboard_summary", wrongPayload, profile); err == nil {
		t.Fatal("dashboard statistics as_of incorrectly accepted Clock.Now instead of the last complete hour")
	}
}

func TestA49LoadOnlyAcceptsInternalApplicationOrigin(t *testing.T) {
	accepted, err := validateA49InternalBaseURL("http://app-a49:3000")
	if err != nil || accepted.Host != "app-a49:3000" {
		t.Fatalf("internal origin rejected: %#v %v", accepted, err)
	}
	for _, raw := range []string{
		"https://app-a49:3000", "http://example.com", "http://app-a49:3000/api",
		"http://user@app-a49:3000", "http://app-a49:3000?x=1",
	} {
		if _, err := validateA49InternalBaseURL(raw); err == nil {
			t.Fatalf("unsafe/non-internal origin %q was accepted", raw)
		}
	}
}

func TestA49ReportGatesEveryEndpointAndComposite(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	profile, err := loadA49RunProfile(fixture, a49SmokeMode)
	if err != nil {
		t.Fatal(err)
	}
	inputs := newPassingA49ReportInputs(profile)
	report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
	if !report.Passed || len(report.Endpoints) != 12 || report.Status != "smoke_passed_not_acceptance_evidence" {
		t.Fatalf("report did not pass: status=%s violations=%v endpoints=%d", report.Status, report.Violations, len(report.Endpoints))
	}
	metrics := report.Endpoints["dashboard_top_customer"]
	if !metrics.Passed || metrics.Sample.Successful != 2 {
		t.Fatalf("top-customer was not independently gated: %#v", metrics)
	}

	records := inputs.dataset.records["list_sites"]["sample"]
	records[0].DurationNanos = int64(time.Second)
	records[1].DurationNanos = int64(time.Second)
	inputs.dataset.records["list_sites"]["sample"] = records
	failed := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
	if failed.Passed || failed.Endpoints["list_sites"].Checks["client_p95"] {
		t.Fatal("strict list P95 threshold did not fail at exactly 1s")
	}
}

func TestA49ReportRejectsIncompleteEvidence(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	profile, err := loadA49RunProfile(fixture, a49SmokeMode)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("shortened phase", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		inputs.load.Phases[0].DurationMilliseconds--
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
		if report.Passed || !hasA49Violation(report, "load_timing_contract") {
			t.Fatalf("shortened phase was accepted: %v", report.Violations)
		}
	})
	t.Run("empty warmup", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		inputs.dataset.records["list_sites"]["warmup"] = nil
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
		if report.Passed || report.Endpoints["list_sites"].Checks["warmup_nonempty"] {
			t.Fatal("empty warmup was accepted")
		}
	})
	for name, mutate := range map[string]func(*a49ReportTestInputs, string){
		"missing warmup access record": func(inputs *a49ReportTestInputs, requestID string) {
			delete(inputs.access, requestID)
		},
		"duplicate warmup access record": func(inputs *a49ReportTestInputs, requestID string) {
			record := inputs.access[requestID]
			record.Count = 2
			inputs.access[requestID] = record
		},
		"mismatched warmup status": func(inputs *a49ReportTestInputs, requestID string) {
			record := inputs.access[requestID]
			record.Status = 500
			inputs.access[requestID] = record
		},
	} {
		t.Run(name, func(t *testing.T) {
			inputs := newPassingA49ReportInputs(profile)
			requestID := inputs.dataset.records["list_sites"]["warmup"][0].RequestID
			mutate(&inputs, requestID)
			report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
			if report.Passed || report.Endpoints["list_sites"].Checks["warmup_complete_access_log"] {
				t.Fatalf("invalid warmup access log was accepted: %#v", report.Endpoints["list_sites"].Warmup)
			}
		})
	}
	t.Run("empty environment", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, a49EnvironmentReport{}, true)
		if report.Passed || !hasA49Violation(report, "environment_contract") {
			t.Fatalf("empty environment was accepted: %v", report.Violations)
		}
	})
	t.Run("evidence class mismatch", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		inputs.seed.EvidenceClass = "formal"
		inputs.load.EvidenceClass = "formal"
		inputs.environment.EvidenceClass = "formal"
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
		if report.Passed || !hasA49Violation(report, "seed_report_contract") ||
			!hasA49Violation(report, "load_metadata_contract") || !hasA49Violation(report, "environment_contract") {
			t.Fatalf("evidence class mismatch was accepted: %v", report.Violations)
		}
	})
	t.Run("missing stats timeline", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, false)
		if report.Passed || !hasA49Violation(report, "stats_timeline_contract") {
			t.Fatalf("missing stats timeline was accepted: %v", report.Violations)
		}
	})
	t.Run("error rate exactly threshold", func(t *testing.T) {
		inputs := newPassingA49ReportInputs(profile)
		records := make([]a49RawRecord, 1000)
		for index := range records {
			requestID := fmt.Sprintf("a49_error_threshold_%04d", index)
			success := index != 0
			status := 200
			errorClass := ""
			if !success {
				status = 500
				errorClass = "http_status"
			}
			records[index] = a49RawRecord{
				SchemaVersion: evidenceSchemaVersion, Kind: "request", Phase: "sample", Scenario: "list",
				Endpoint: "list_sites", RequestID: requestID, ViewerNumber: index%2 + 1, StatusCode: status,
				Success: success, DurationNanos: int64(10 * time.Millisecond), ErrorClass: errorClass,
			}
			inputs.access[requestID] = a49AccessRecord{Status: status, Duration: 5, Count: 1}
		}
		inputs.dataset.records["list_sites"]["sample"] = records
		report := buildA49FinalReport(profile, inputs.dataset, inputs.access, inputs.seed, inputs.load, inputs.environment, true)
		metrics := report.Endpoints["list_sites"]
		if report.Passed || metrics.Checks["maximum_error_rate"] || metrics.Sample.ErrorRate != 0.001 {
			t.Fatalf("error rate at exact threshold was accepted: %#v", metrics)
		}
	})
}

func TestA49StatsTimelineContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a49-docker-stats.tsv")
	valid := strings.Join([]string{
		"2026-01-17T00:00:00Z|new-api-pilot-a49-token-mysql|1.20%|128MiB / 1.5GiB|1kB / 2kB|3kB / 4kB|12",
		"2026-01-17T00:00:00Z|new-api-pilot-a49-token-app|0.20%|64MiB / 768MiB|1kB / 2kB|3kB / 4kB|8",
		"2026-01-17T00:00:00Z|new-api-pilot-a49-token-load|2.00%|32MiB / 1GiB|1kB / 2kB|3kB / 4kB|6",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(valid), 0o640); err != nil {
		t.Fatal(err)
	}
	if !validateA49StatsTimeline(path) {
		t.Fatal("valid stats timeline was rejected")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(valid, "|6\n", "\n", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	if validateA49StatsTimeline(path) {
		t.Fatal("malformed stats timeline was accepted")
	}
}

func TestA49MonotonicTimelineIgnoresWallClockJumps(t *testing.T) {
	anchor := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)
	rollbackWallRead := anchor.Add(-5 * time.Minute)
	forwardWallRead := anchor.Add(2 * time.Hour)
	if !rollbackWallRead.Before(anchor) || !forwardWallRead.After(anchor) {
		t.Fatal("test wall-clock jumps were not constructed")
	}

	type interval struct {
		offset  time.Duration
		elapsed time.Duration
	}
	intervals := []interval{
		{500 * time.Millisecond, time.Second},
		{1500 * time.Millisecond, 3 * time.Second},
		{4500 * time.Millisecond, time.Second},
		{5500 * time.Millisecond, 3 * time.Second},
		{8500 * time.Millisecond, time.Second},
		{9500 * time.Millisecond, 3 * time.Second},
	}
	previousFinished := projectA49LoadTime(anchor, 0)
	for index, current := range intervals {
		started := projectA49LoadTime(anchor, current.offset)
		finished := projectA49LoadTime(anchor, current.offset+current.elapsed)
		serializedStarted, err := time.Parse(time.RFC3339Nano, started.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
		serializedFinished, err := time.Parse(time.RFC3339Nano, finished.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
		if serializedStarted.Before(previousFinished) || serializedFinished.Sub(serializedStarted) != current.elapsed {
			t.Fatalf("projected phase %d lost order/duration across wall-clock jump", index)
		}
		previousFinished = serializedFinished
	}
	totalElapsed := 13 * time.Second
	if got := projectA49LoadTime(anchor, totalElapsed).Sub(projectA49LoadTime(anchor, 0)); got != totalElapsed ||
		previousFinished.After(projectA49LoadTime(anchor, totalElapsed)) {
		t.Fatalf("projected total duration/order invalid: %s", got)
	}
}

type a49ReportTestInputs struct {
	dataset     a49RawDataset
	access      map[string]a49AccessRecord
	seed        a49SeedReport
	load        a49LoadMetadata
	environment a49EnvironmentReport
}

func newPassingA49ReportInputs(profile a49RunProfile) a49ReportTestInputs {
	result := a49ReportTestInputs{
		dataset: a49RawDataset{records: make(map[string]map[string][]a49RawRecord), seenIDs: make(map[string]struct{})},
		access:  make(map[string]a49AccessRecord),
	}
	definitions := append([]a49Endpoint(nil), profile.Capacity.Endpoints...)
	definitions = append(definitions, a49Endpoint{Name: "dashboard_composite", Scenario: "dashboard"})
	sequence := 0
	for _, endpoint := range definitions {
		result.dataset.records[endpoint.Name] = make(map[string][]a49RawRecord)
		for _, phase := range []string{"warmup", "sample"} {
			for viewer := 1; viewer <= profile.Capacity.ConcurrentReadUsers; viewer++ {
				sequence++
				requestID := fmt.Sprintf("a49_test_%d", sequence)
				record := a49RawRecord{
					SchemaVersion: evidenceSchemaVersion, Kind: "request", Phase: phase, Scenario: endpoint.Scenario,
					Endpoint: endpoint.Name, RequestID: requestID, ViewerNumber: viewer, StatusCode: 200,
					Success: true, DurationNanos: int64(10 * time.Millisecond),
				}
				if endpoint.Name == "dashboard_composite" {
					record.Kind, record.RequestID = "composite", ""
				} else {
					result.access[requestID] = a49AccessRecord{Status: 200, Duration: 5, Count: 1}
				}
				result.dataset.records[endpoint.Name][phase] = append(result.dataset.records[endpoint.Name][phase], record)
			}
		}
	}
	result.seed = a49SeedReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a49AcceptanceID, Status: "passed", Mode: profile.Mode,
		EvidenceClass: profile.evidenceClass(), AcceptanceEligible: profile.AcceptanceEligible,
		FixturePath: profile.FixturePath, FixtureSHA256: profile.FixtureSHA256, FixedNowUnix: profile.Fixture.Clock.NowUnix,
	}
	started := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)
	cursor := started
	phases := make([]a49LoadPhaseMetadata, 0, 6)
	for _, scenario := range []string{"list", "hourly", "dashboard"} {
		for _, phase := range []string{"warmup", "sample"} {
			seconds := profile.Capacity.WarmupSeconds
			if phase == "sample" {
				seconds = profile.Capacity.SampleSeconds
			}
			finished := cursor.Add(time.Duration(seconds) * time.Second)
			phases = append(phases, a49LoadPhaseMetadata{
				Scenario: scenario, Phase: phase, ConfiguredSeconds: seconds,
				StartedAt: cursor.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano),
				DurationMilliseconds: finished.Sub(cursor).Milliseconds(),
			})
			cursor = finished
		}
	}
	viewerIDs := make([]string, profile.Capacity.ConcurrentReadUsers)
	for index := range viewerIDs {
		viewerIDs[index] = fmt.Sprintf("%d", index+1)
	}
	result.load = a49LoadMetadata{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a49AcceptanceID, Status: "completed", Mode: profile.Mode,
		EvidenceClass: profile.evidenceClass(), AcceptanceEligible: profile.AcceptanceEligible,
		FixturePath: profile.FixturePath, FixtureSHA256: profile.FixtureSHA256, FixedNowUnix: profile.Fixture.Clock.NowUnix,
		ViewerIDs: viewerIDs, RawResultsPath: "/evidence/a49-load-results.jsonl",
		StartedAt: started.Format(time.RFC3339Nano), FinishedAt: cursor.Format(time.RFC3339Nano),
		DurationMilliseconds: cursor.Sub(started).Milliseconds(), Scenarios: []string{"list", "hourly", "dashboard"}, Phases: phases,
	}
	result.environment.SchemaVersion = evidenceSchemaVersion
	result.environment.AcceptanceID = a49AcceptanceID
	result.environment.Mode = profile.Mode
	result.environment.EvidenceClass = profile.evidenceClass()
	result.environment.AcceptanceEligible = profile.AcceptanceEligible
	result.environment.FixedNowUnix = profile.Fixture.Clock.NowUnix
	result.environment.Commit = "test-commit"
	result.environment.Docker.ClientVersion = "28.0.0"
	result.environment.Docker.ServerVersion = "28.0.0"
	result.environment.Docker.CPUs = profile.Capacity.Resources.MinimumCPUs
	result.environment.Docker.MemoryBytes = profile.Capacity.Resources.MinimumMemoryBytes
	result.environment.Docker.StorageDriver = "overlay2"
	result.environment.Docker.DockerRootDir = "/var/lib/docker"
	result.environment.Docker.DockerFreeBytesBefore = profile.Capacity.Resources.MinimumDockerFreeBytes
	result.environment.Docker.EvidenceFreeBytesBefore = profile.Capacity.Resources.MinimumEvidenceBytes
	result.environment.Images.Application = "sha256:" + strings.Repeat("a", 64)
	result.environment.Images.MySQL = "sha256:" + strings.Repeat("b", 64)
	result.environment.Images.Go = "sha256:" + strings.Repeat("c", 64)
	result.environment.MySQL.Version = "8.4.6"
	result.environment.MySQL.TransactionIsolation = "READ-COMMITTED"
	result.environment.MySQL.CharacterSetServer = "utf8mb4"
	result.environment.MySQL.CollationServer = "utf8mb4_unicode_ci"
	result.environment.MySQL.TimeZone = "+08:00"
	result.environment.MySQL.Database = "pilot_a49"
	if profile.Mode == a49SmokeMode {
		result.environment.Limits.MySQLMemory = "1536m"
		result.environment.Limits.MySQLCPUs = "2"
		result.environment.Limits.MySQLBufferPool = "256M"
		result.environment.Limits.AppMemory = "768m"
		result.environment.Limits.AppCPUs = "1"
		result.environment.Limits.LoadMemory = "1g"
		result.environment.Limits.LoadCPUs = "2"
	} else {
		result.environment.Limits.MySQLMemory = "6g"
		result.environment.Limits.MySQLCPUs = "4"
		result.environment.Limits.MySQLBufferPool = "3G"
		result.environment.Limits.AppMemory = "3g"
		result.environment.Limits.AppCPUs = "2"
		result.environment.Limits.LoadMemory = "4g"
		result.environment.Limits.LoadCPUs = "4"
	}
	result.environment.Network.Internal = true
	result.environment.Network.HostPorts = []int{}
	result.environment.StatsTimeline = "a49-docker-stats.tsv"
	return result
}

func hasA49Violation(report a49FinalReport, want string) bool {
	for _, violation := range report.Violations {
		if violation == want {
			return true
		}
	}
	return false
}
