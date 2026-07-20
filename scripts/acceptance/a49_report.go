package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	a49AccessLogPattern       = regexp.MustCompile(`request_id=([A-Za-z0-9._-]+).* status=([0-9]{3}) duration_ms=([0-9]+)`)
	a49StatsCPUPercentPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?%$`)
)

type a49LatencyPercentiles struct {
	P50Milliseconds float64 `json:"p50_milliseconds"`
	P95Milliseconds float64 `json:"p95_milliseconds"`
	P99Milliseconds float64 `json:"p99_milliseconds"`
	MaxMilliseconds float64 `json:"max_milliseconds"`
}

type a49PhaseMetrics struct {
	Attempts             int64                  `json:"attempts"`
	Successful           int64                  `json:"successful"`
	Errors               int64                  `json:"errors"`
	ErrorRate            float64                `json:"error_rate"`
	ViewerCount          int                    `json:"viewer_count"`
	StatusCodes          map[string]int64       `json:"status_codes"`
	ErrorClasses         map[string]int64       `json:"error_classes"`
	Client               a49LatencyPercentiles  `json:"client"`
	Server               *a49LatencyPercentiles `json:"server,omitempty"`
	ServerLogRecords     int64                  `json:"server_log_records"`
	ServerLogMissing     int64                  `json:"server_log_missing"`
	ServerStatusMismatch int64                  `json:"server_status_mismatch"`
	ServerLogDuplicates  int64                  `json:"server_log_duplicates"`
}

type a49EndpointMetrics struct {
	Scenario              string          `json:"scenario"`
	ThresholdMilliseconds float64         `json:"threshold_milliseconds"`
	Warmup                a49PhaseMetrics `json:"warmup"`
	Sample                a49PhaseMetrics `json:"sample"`
	MinimumSuccessful     int64           `json:"minimum_successful"`
	MaximumErrorRate      float64         `json:"maximum_error_rate"`
	Checks                map[string]bool `json:"checks"`
	Passed                bool            `json:"passed"`
}

type a49FinalReport struct {
	SchemaVersion       int                           `json:"schema_version"`
	AcceptanceID        string                        `json:"acceptance_id"`
	Status              string                        `json:"status"`
	Passed              bool                          `json:"passed"`
	Mode                string                        `json:"mode"`
	EvidenceClass       string                        `json:"evidence_class"`
	AcceptanceEligible  bool                          `json:"acceptance_eligible"`
	FixturePath         string                        `json:"fixture_path"`
	FixtureSHA256       string                        `json:"fixture_sha256"`
	FixedNowUnix        int64                         `json:"fixed_now_unix"`
	PercentileAlgorithm string                        `json:"percentile_algorithm"`
	LatencyPopulation   string                        `json:"latency_population"`
	ThreePhaseDuration  string                        `json:"three_phase_duration"`
	ViewerCount         int                           `json:"viewer_count"`
	Endpoints           map[string]a49EndpointMetrics `json:"endpoints"`
	Violations          []string                      `json:"violations"`
	SeedReport          a49SeedReport                 `json:"seed_report"`
	LoadMetadata        a49LoadMetadata               `json:"load_metadata"`
	Environment         a49EnvironmentReport          `json:"environment"`
	GeneratedAt         string                        `json:"generated_at"`
}

type a49EnvironmentReport struct {
	SchemaVersion      int    `json:"schema_version"`
	AcceptanceID       string `json:"acceptance_id"`
	Mode               string `json:"mode"`
	EvidenceClass      string `json:"evidence_class"`
	AcceptanceEligible bool   `json:"acceptance_eligible"`
	FixedNowUnix       int64  `json:"fixed_now_unix"`
	Commit             string `json:"commit"`
	WorktreeDirty      bool   `json:"worktree_dirty"`
	Docker             struct {
		ClientVersion           string `json:"client_version"`
		ServerVersion           string `json:"server_version"`
		CPUs                    int    `json:"cpus"`
		MemoryBytes             int64  `json:"memory_bytes"`
		StorageDriver           string `json:"storage_driver"`
		DockerRootDir           string `json:"docker_root_dir"`
		DockerFreeBytesBefore   int64  `json:"docker_free_bytes_before"`
		EvidenceFreeBytesBefore int64  `json:"evidence_free_bytes_before"`
	} `json:"docker"`
	Images struct {
		Application string `json:"application"`
		MySQL       string `json:"mysql"`
		Go          string `json:"go"`
	} `json:"images"`
	MySQL struct {
		Version              string `json:"version"`
		TransactionIsolation string `json:"transaction_isolation"`
		CharacterSetServer   string `json:"character_set_server"`
		CollationServer      string `json:"collation_server"`
		TimeZone             string `json:"time_zone"`
		Database             string `json:"database"`
	} `json:"mysql"`
	Limits struct {
		MySQLMemory     string `json:"mysql_memory"`
		MySQLCPUs       string `json:"mysql_cpus"`
		MySQLBufferPool string `json:"mysql_buffer_pool"`
		AppMemory       string `json:"app_memory"`
		AppCPUs         string `json:"app_cpus"`
		LoadMemory      string `json:"load_memory"`
		LoadCPUs        string `json:"load_cpus"`
	} `json:"limits"`
	Network struct {
		Internal  bool  `json:"internal"`
		HostPorts []int `json:"host_ports"`
	} `json:"network"`
	StatsTimeline string `json:"stats_timeline"`
}

type a49AccessRecord struct {
	Status   int
	Duration int64
	Count    int
}

type a49MetricAccumulator struct {
	clientDurations []int64
	serverDurations []int64
	successful      int64
	statuses        map[string]int64
	errors          map[string]int64
	viewers         map[int]struct{}
	serverRecords   int64
	serverMissing   int64
	serverMismatch  int64
	serverDuplicate int64
}

type a49RawDataset struct {
	records map[string]map[string][]a49RawRecord
	seenIDs map[string]struct{}
}

func runA49Report(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a49-report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fixturePath := flags.String("fixture", "testdata/design/f05-ops-capacity.yaml", "F05 fixture path")
	mode := flags.String("mode", a49FullMode, "full or smoke")
	rawPath := flags.String("raw", "a49-load-results.jsonl", "raw load JSONL")
	appLogPath := flags.String("app-log", "a49-app.log", "complete application access log")
	seedReportPath := flags.String("seed-report", "a49-seed-report.json", "seed report")
	loadMetadataPath := flags.String("load-metadata", "a49-load-metadata.json", "load metadata")
	environmentPath := flags.String("environment", "a49-environment.json", "sanitized environment report")
	outputPath := flags.String("output", "a49-report.json", "final report")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	profile, err := loadA49RunProfile(*fixturePath, *mode)
	if err != nil {
		fmt.Fprintf(stderr, "load A49 profile: %v\n", err)
		return 2
	}
	if os.Getenv("ACCEPTANCE_ID") != a49AcceptanceID || os.Getenv("A49_ISOLATED_REPORT") != "true" {
		fmt.Fprintln(stderr, "A49 report requires ACCEPTANCE_ID=A49 and A49_ISOLATED_REPORT=true")
		return 2
	}
	if os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != profile.evidenceClass() {
		fmt.Fprintln(stderr, "A49 report evidence class does not match the profile mode")
		return 2
	}
	seedReport, err := readA49JSON[a49SeedReport](*seedReportPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A49 seed report: %v\n", err)
		return 1
	}
	loadMetadata, err := readA49JSON[a49LoadMetadata](*loadMetadataPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A49 load metadata: %v\n", err)
		return 1
	}
	environment, err := readA49JSON[a49EnvironmentReport](*environmentPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A49 environment report: %v\n", err)
		return 1
	}
	statsTimelinePresent := false
	if environment.StatsTimeline == "a49-docker-stats.tsv" {
		statsTimelinePresent = validateA49StatsTimeline(filepath.Join(filepath.Dir(*environmentPath), environment.StatsTimeline))
	}
	dataset, err := readA49RawDataset(*rawPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A49 raw results: %v\n", err)
		return 1
	}
	access, err := readA49AccessLog(*appLogPath)
	if err != nil {
		fmt.Fprintf(stderr, "read A49 app access log: %v\n", err)
		return 1
	}
	report := buildA49FinalReport(profile, dataset, access, seedReport, loadMetadata, environment, statsTimelinePresent)
	if err := writeJSONAtomic(*outputPath, report); err != nil {
		fmt.Fprintf(stderr, "write A49 final report: %v\n", err)
		return 1
	}
	if !report.Passed {
		fmt.Fprintf(stderr, "A49 %s report failed with %d violation(s); report=%s\n", profile.Mode, len(report.Violations), *outputPath)
		return 1
	}
	fmt.Fprintf(stdout, "A49 %s report passed; eligible=%t report=%s\n", profile.Mode, report.AcceptanceEligible, *outputPath)
	return 0
}

func readA49JSON[T any](path string) (T, error) {
	var result T
	payload, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func validateA49StatsTimeline(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	seen := map[string]bool{"mysql": false, "app": false, "load": false}
	lines := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.ContainsRune(line, '\x1b') {
			return false
		}
		fields := strings.Split(line, "|")
		if len(fields) != 7 {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, fields[0]); err != nil ||
			!strings.HasPrefix(fields[1], "new-api-pilot-a49-") ||
			!a49StatsCPUPercentPattern.MatchString(fields[2]) ||
			!strings.Contains(fields[3], "/") || !strings.Contains(fields[4], "/") ||
			!strings.Contains(fields[5], "/") {
			return false
		}
		if _, err := strconv.ParseUint(fields[6], 10, 32); err != nil {
			return false
		}
		for kind := range seen {
			if strings.HasSuffix(fields[1], "-"+kind) {
				seen[kind] = true
			}
		}
		lines++
	}
	return scanner.Err() == nil && lines >= 3 && seen["mysql"] && seen["app"] && seen["load"]
}

func readA49RawDataset(path string) (a49RawDataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return a49RawDataset{}, err
	}
	defer file.Close()
	dataset := a49RawDataset{records: make(map[string]map[string][]a49RawRecord), seenIDs: make(map[string]struct{})}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var record a49RawRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return a49RawDataset{}, fmt.Errorf("line %d: %w", line, err)
		}
		if record.SchemaVersion != evidenceSchemaVersion || record.DurationNanos < 0 || record.Endpoint == "" || record.Phase == "" {
			return a49RawDataset{}, fmt.Errorf("line %d violates raw record contract", line)
		}
		if record.Kind == "request" && (record.Phase == "warmup" || record.Phase == "sample") {
			if record.RequestID == "" {
				return a49RawDataset{}, fmt.Errorf("line %d has no request ID", line)
			}
			if _, exists := dataset.seenIDs[record.RequestID]; exists {
				return a49RawDataset{}, fmt.Errorf("duplicate request ID %q", record.RequestID)
			}
			dataset.seenIDs[record.RequestID] = struct{}{}
		}
		if dataset.records[record.Endpoint] == nil {
			dataset.records[record.Endpoint] = make(map[string][]a49RawRecord)
		}
		dataset.records[record.Endpoint][record.Phase] = append(dataset.records[record.Endpoint][record.Phase], record)
	}
	if err := scanner.Err(); err != nil {
		return a49RawDataset{}, err
	}
	if line == 0 {
		return a49RawDataset{}, errors.New("raw results are empty")
	}
	return dataset, nil
}

func readA49AccessLog(path string) (map[string]a49AccessRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make(map[string]a49AccessRecord)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		match := a49AccessLogPattern.FindStringSubmatch(scanner.Text())
		if len(match) != 4 || !strings.HasPrefix(match[1], "a49_") {
			continue
		}
		status, statusErr := strconv.Atoi(match[2])
		duration, durationErr := strconv.ParseInt(match[3], 10, 64)
		if statusErr != nil || durationErr != nil || duration < 0 {
			return nil, errors.New("invalid A49 access log duration/status")
		}
		record := result[match[1]]
		record.Status, record.Duration, record.Count = status, duration, record.Count+1
		result[match[1]] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func buildA49FinalReport(
	profile a49RunProfile,
	dataset a49RawDataset,
	access map[string]a49AccessRecord,
	seedReport a49SeedReport,
	loadMetadata a49LoadMetadata,
	environment a49EnvironmentReport,
	statsTimelinePresent bool,
) a49FinalReport {
	report := a49FinalReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a49AcceptanceID, Mode: profile.Mode, EvidenceClass: profile.evidenceClass(),
		AcceptanceEligible: profile.AcceptanceEligible, FixturePath: profile.FixturePath,
		FixtureSHA256: profile.FixtureSHA256, FixedNowUnix: profile.Fixture.Clock.NowUnix,
		PercentileAlgorithm: "nearest-rank: sort all attempts ascending and select ceil(p*n), with rank starting at one",
		LatencyPopulation:   "client percentiles include successful, HTTP-error, contract-error, and timeout attempts; server percentiles include every request that reached access logging",
		ThreePhaseDuration: fmt.Sprintf("3 × (%ds warmup + %ds sample) = %ds (%s)",
			profile.Capacity.WarmupSeconds, profile.Capacity.SampleSeconds,
			3*(profile.Capacity.WarmupSeconds+profile.Capacity.SampleSeconds),
			(time.Duration(3*(profile.Capacity.WarmupSeconds+profile.Capacity.SampleSeconds)) * time.Second).String()),
		ViewerCount: profile.Capacity.ConcurrentReadUsers, Endpoints: make(map[string]a49EndpointMetrics),
		Violations: []string{},
		SeedReport: seedReport, LoadMetadata: loadMetadata, Environment: environment,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	validateA49InputReports(profile, seedReport, loadMetadata, environment, statsTimelinePresent, &report)
	definitions := append([]a49Endpoint(nil), profile.Capacity.Endpoints...)
	definitions = append(definitions, a49Endpoint{Name: "dashboard_composite", Scenario: "dashboard"})
	for _, endpoint := range definitions {
		threshold := a49ThresholdMilliseconds(profile.Capacity, endpoint.Scenario)
		warmup := calculateA49PhaseMetrics(dataset.records[endpoint.Name]["warmup"], access, endpoint.Name == "dashboard_composite")
		sample := calculateA49PhaseMetrics(dataset.records[endpoint.Name]["sample"], access, endpoint.Name == "dashboard_composite")
		checks := map[string]bool{
			"minimum_successful_requests": sample.Successful >= int64(profile.Capacity.MinimumSuccessfulRequestsPerEndpoint),
			"maximum_error_rate":          sample.ErrorRate < profile.Capacity.MaximumErrorRate,
			"all_viewers_participated":    sample.ViewerCount == profile.Capacity.ConcurrentReadUsers,
			"client_p95":                  sample.Attempts > 0 && sample.Client.P95Milliseconds < threshold,
			"warmup_nonempty":             warmup.Attempts > 0,
			"warmup_all_viewers":          warmup.ViewerCount == profile.Capacity.ConcurrentReadUsers,
		}
		if endpoint.Name != "dashboard_composite" {
			checks["warmup_complete_access_log"] = warmup.ServerLogMissing == 0 && warmup.ServerStatusMismatch == 0 && warmup.ServerLogDuplicates == 0 &&
				warmup.ServerLogRecords == warmup.Attempts
			checks["complete_access_log"] = sample.ServerLogMissing == 0 && sample.ServerStatusMismatch == 0 && sample.ServerLogDuplicates == 0 &&
				sample.ServerLogRecords == sample.Attempts
			checks["server_p95"] = sample.Server != nil && sample.Server.P95Milliseconds < threshold
		}
		passed := true
		for check, value := range checks {
			if !value {
				passed = false
				report.Violations = append(report.Violations, endpoint.Name+":"+check)
			}
		}
		report.Endpoints[endpoint.Name] = a49EndpointMetrics{
			Scenario: endpoint.Scenario, ThresholdMilliseconds: threshold, Warmup: warmup, Sample: sample,
			MinimumSuccessful: int64(profile.Capacity.MinimumSuccessfulRequestsPerEndpoint),
			MaximumErrorRate:  profile.Capacity.MaximumErrorRate, Checks: checks, Passed: passed,
		}
	}
	sort.Strings(report.Violations)
	report.Passed = len(report.Violations) == 0
	switch {
	case !report.Passed:
		report.Status = "failed"
	case profile.AcceptanceEligible:
		report.Status = "passed"
	default:
		report.Status = "smoke_passed_not_acceptance_evidence"
	}
	return report
}

func validateA49InputReports(
	profile a49RunProfile,
	seed a49SeedReport,
	load a49LoadMetadata,
	environment a49EnvironmentReport,
	statsTimelinePresent bool,
	report *a49FinalReport,
) {
	if seed.SchemaVersion != evidenceSchemaVersion || seed.Status != "passed" || seed.AcceptanceID != a49AcceptanceID || seed.Mode != profile.Mode ||
		seed.EvidenceClass != profile.evidenceClass() ||
		seed.AcceptanceEligible != profile.AcceptanceEligible || seed.FixtureSHA256 != profile.FixtureSHA256 ||
		seed.FixedNowUnix != profile.Fixture.Clock.NowUnix {
		report.Violations = append(report.Violations, "seed_report_contract")
	}
	if load.SchemaVersion != evidenceSchemaVersion || load.Status != "completed" || load.AcceptanceID != a49AcceptanceID || load.Mode != profile.Mode ||
		load.EvidenceClass != profile.evidenceClass() ||
		load.AcceptanceEligible != profile.AcceptanceEligible || load.FixtureSHA256 != profile.FixtureSHA256 ||
		load.FixedNowUnix != profile.Fixture.Clock.NowUnix || len(load.ViewerIDs) != profile.Capacity.ConcurrentReadUsers ||
		load.RawResultsPath != "/evidence/a49-load-results.jsonl" {
		report.Violations = append(report.Violations, "load_metadata_contract")
	}
	validateA49LoadTiming(profile, load, report)
	viewerIDs := make(map[string]struct{}, len(load.ViewerIDs))
	for _, id := range load.ViewerIDs {
		if !canonicalPositiveA49ID(id) {
			report.Violations = append(report.Violations, "load_viewer_identity_contract")
			break
		}
		viewerIDs[id] = struct{}{}
	}
	if len(viewerIDs) != len(load.ViewerIDs) {
		report.Violations = append(report.Violations, "load_viewer_identities_not_distinct")
	}
	validateA49Environment(profile, environment, statsTimelinePresent, report)
}

func validateA49LoadTiming(profile a49RunProfile, load a49LoadMetadata, report *a49FinalReport) {
	valid := stringSlicesEqualA49(load.Scenarios, []string{"list", "hourly", "dashboard"})
	started, startErr := time.Parse(time.RFC3339Nano, load.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, load.FinishedAt)
	requiredMilliseconds := int64(3*(profile.Capacity.WarmupSeconds+profile.Capacity.SampleSeconds)) * 1000
	if startErr != nil || finishErr != nil || !finished.After(started) ||
		finished.Sub(started).Milliseconds() != load.DurationMilliseconds || load.DurationMilliseconds < requiredMilliseconds {
		valid = false
	}
	expected := []struct {
		scenario string
		phase    string
		seconds  int
	}{
		{"list", "warmup", profile.Capacity.WarmupSeconds},
		{"list", "sample", profile.Capacity.SampleSeconds},
		{"hourly", "warmup", profile.Capacity.WarmupSeconds},
		{"hourly", "sample", profile.Capacity.SampleSeconds},
		{"dashboard", "warmup", profile.Capacity.WarmupSeconds},
		{"dashboard", "sample", profile.Capacity.SampleSeconds},
	}
	if len(load.Phases) != len(expected) {
		valid = false
	} else {
		previousFinished := started
		for index, want := range expected {
			phase := load.Phases[index]
			phaseStarted, phaseStartErr := time.Parse(time.RFC3339Nano, phase.StartedAt)
			phaseFinished, phaseFinishErr := time.Parse(time.RFC3339Nano, phase.FinishedAt)
			if phase.Scenario != want.scenario || phase.Phase != want.phase || phase.ConfiguredSeconds != want.seconds ||
				phaseStartErr != nil || phaseFinishErr != nil || phaseStarted.Before(previousFinished) ||
				phaseFinished.After(finished) || !phaseFinished.After(phaseStarted) ||
				phaseFinished.Sub(phaseStarted).Milliseconds() != phase.DurationMilliseconds ||
				phase.DurationMilliseconds < int64(want.seconds)*1000 {
				valid = false
			}
			previousFinished = phaseFinished
		}
	}
	if !valid {
		report.Violations = append(report.Violations, "load_timing_contract")
	}
}

func validateA49Environment(
	profile a49RunProfile,
	environment a49EnvironmentReport,
	statsTimelinePresent bool,
	report *a49FinalReport,
) {
	valid := environment.SchemaVersion == evidenceSchemaVersion && environment.AcceptanceID == a49AcceptanceID &&
		environment.EvidenceClass == profile.evidenceClass() &&
		environment.Mode == profile.Mode && environment.AcceptanceEligible == profile.AcceptanceEligible &&
		environment.FixedNowUnix == profile.Fixture.Clock.NowUnix && environment.Commit != "" &&
		environment.Docker.ClientVersion != "" && environment.Docker.ServerVersion != "" &&
		environment.Docker.CPUs >= profile.Capacity.Resources.MinimumCPUs &&
		environment.Docker.MemoryBytes >= profile.Capacity.Resources.MinimumMemoryBytes &&
		environment.Docker.DockerFreeBytesBefore >= profile.Capacity.Resources.MinimumDockerFreeBytes &&
		environment.Docker.EvidenceFreeBytesBefore >= profile.Capacity.Resources.MinimumEvidenceBytes &&
		environment.Docker.StorageDriver != "" && environment.Docker.DockerRootDir != "" &&
		strings.HasPrefix(environment.Images.Application, "sha256:") && strings.HasPrefix(environment.Images.MySQL, "sha256:") &&
		strings.HasPrefix(environment.Images.Go, "sha256:") && strings.HasPrefix(environment.MySQL.Version, "8.4.") &&
		environment.MySQL.TransactionIsolation == "READ-COMMITTED" && environment.MySQL.CharacterSetServer == "utf8mb4" &&
		environment.MySQL.CollationServer == "utf8mb4_unicode_ci" && environment.MySQL.TimeZone == "+08:00" &&
		environment.MySQL.Database == "pilot_a49" && environment.Network.Internal &&
		environment.Network.HostPorts != nil && len(environment.Network.HostPorts) == 0 &&
		environment.StatsTimeline == "a49-docker-stats.tsv" && a49EnvironmentLimitsValid(profile.Mode, environment)
	if !valid {
		report.Violations = append(report.Violations, "environment_contract")
	}
	if !statsTimelinePresent {
		report.Violations = append(report.Violations, "stats_timeline_contract")
	}
}

func a49EnvironmentLimitsValid(mode string, environment a49EnvironmentReport) bool {
	expected := [7]string{"6g", "4", "3G", "3g", "2", "4g", "4"}
	if mode == a49SmokeMode {
		expected = [7]string{"1536m", "2", "256M", "768m", "1", "1g", "2"}
	}
	actual := [7]string{
		environment.Limits.MySQLMemory, environment.Limits.MySQLCPUs, environment.Limits.MySQLBufferPool,
		environment.Limits.AppMemory, environment.Limits.AppCPUs, environment.Limits.LoadMemory, environment.Limits.LoadCPUs,
	}
	return actual == expected
}

func stringSlicesEqualA49(left, right []string) bool {
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

func calculateA49PhaseMetrics(records []a49RawRecord, access map[string]a49AccessRecord, composite bool) a49PhaseMetrics {
	accumulator := a49MetricAccumulator{
		statuses: make(map[string]int64), errors: make(map[string]int64), viewers: make(map[int]struct{}),
	}
	for _, record := range records {
		accumulator.clientDurations = append(accumulator.clientDurations, record.DurationNanos)
		accumulator.viewers[record.ViewerNumber] = struct{}{}
		accumulator.statuses[strconv.Itoa(record.StatusCode)]++
		if record.Success {
			accumulator.successful++
		} else {
			class := record.ErrorClass
			if class == "" {
				class = "unknown"
			}
			accumulator.errors[class]++
		}
		if composite {
			continue
		}
		server, exists := access[record.RequestID]
		if !exists {
			accumulator.serverMissing++
			continue
		}
		if server.Count != 1 {
			accumulator.serverDuplicate++
		}
		if server.Status != record.StatusCode {
			accumulator.serverMismatch++
		}
		accumulator.serverRecords++
		accumulator.serverDurations = append(accumulator.serverDurations, server.Duration*int64(time.Millisecond))
	}
	attempts := int64(len(records))
	metrics := a49PhaseMetrics{
		Attempts: attempts, Successful: accumulator.successful, Errors: attempts - accumulator.successful,
		ViewerCount: len(accumulator.viewers), StatusCodes: accumulator.statuses, ErrorClasses: accumulator.errors,
		Client: a49Percentiles(accumulator.clientDurations), ServerLogRecords: accumulator.serverRecords,
		ServerLogMissing: accumulator.serverMissing, ServerStatusMismatch: accumulator.serverMismatch,
		ServerLogDuplicates: accumulator.serverDuplicate,
	}
	if attempts > 0 {
		metrics.ErrorRate = float64(metrics.Errors) / float64(attempts)
	}
	if !composite {
		server := a49Percentiles(accumulator.serverDurations)
		metrics.Server = &server
	}
	return metrics
}

func a49ThresholdMilliseconds(capacity a49CapacityFixture, scenario string) float64 {
	switch scenario {
	case "list":
		return capacity.Targets.ListP95Seconds * 1000
	case "hourly":
		return capacity.Targets.Hourly31DP95Seconds * 1000
	case "dashboard":
		return capacity.Targets.DashboardP95Seconds * 1000
	default:
		return 0
	}
}

func a49Percentiles(values []int64) a49LatencyPercentiles {
	if len(values) == 0 {
		return a49LatencyPercentiles{}
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	milliseconds := func(value int64) float64 { return float64(value) / float64(time.Millisecond) }
	return a49LatencyPercentiles{
		P50Milliseconds: milliseconds(a49NearestRank(sorted, 0.50)),
		P95Milliseconds: milliseconds(a49NearestRank(sorted, 0.95)),
		P99Milliseconds: milliseconds(a49NearestRank(sorted, 0.99)),
		MaxMilliseconds: milliseconds(sorted[len(sorted)-1]),
	}
}

func a49NearestRank(sorted []int64, percentile float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
