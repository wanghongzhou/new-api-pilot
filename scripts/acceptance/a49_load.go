package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const a49MaximumResponseBytes int64 = 64 * 1024 * 1024

type a49Envelope struct {
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	Code      string          `json:"code"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

type a49VirtualUser struct {
	Number   int
	ID       string
	Username string
	Client   *http.Client
}

type a49RawRecord struct {
	SchemaVersion  int    `json:"schema_version"`
	Kind           string `json:"kind"`
	Phase          string `json:"phase"`
	Scenario       string `json:"scenario"`
	Endpoint       string `json:"endpoint"`
	RequestID      string `json:"request_id,omitempty"`
	BatchID        string `json:"batch_id,omitempty"`
	ViewerNumber   int    `json:"viewer_number"`
	StatusCode     int    `json:"status_code"`
	Success        bool   `json:"success"`
	DurationNanos  int64  `json:"duration_nanos"`
	ResponseBytes  int64  `json:"response_bytes,omitempty"`
	ErrorClass     string `json:"error_class,omitempty"`
	ObservedAtUnix int64  `json:"observed_at_unix"`
}

type a49LoadMetadata struct {
	SchemaVersion        int                    `json:"schema_version"`
	AcceptanceID         string                 `json:"acceptance_id"`
	Status               string                 `json:"status"`
	Mode                 string                 `json:"mode"`
	EvidenceClass        string                 `json:"evidence_class"`
	AcceptanceEligible   bool                   `json:"acceptance_eligible"`
	FixturePath          string                 `json:"fixture_path"`
	FixtureSHA256        string                 `json:"fixture_sha256"`
	FixedNowUnix         int64                  `json:"fixed_now_unix"`
	ViewerIDs            []string               `json:"viewer_ids"`
	RawResultsPath       string                 `json:"raw_results_path"`
	StartedAt            string                 `json:"started_at"`
	FinishedAt           string                 `json:"finished_at"`
	DurationMilliseconds int64                  `json:"duration_milliseconds"`
	Scenarios            []string               `json:"scenarios"`
	Phases               []a49LoadPhaseMetadata `json:"phases"`
}

type a49LoadPhaseMetadata struct {
	Scenario             string `json:"scenario"`
	Phase                string `json:"phase"`
	ConfiguredSeconds    int    `json:"configured_seconds"`
	StartedAt            string `json:"started_at"`
	FinishedAt           string `json:"finished_at"`
	DurationMilliseconds int64  `json:"duration_milliseconds"`
}

type a49RawRecorder struct {
	mutex sync.Mutex
	file  *os.File
	err   error
}

func newA49RawRecorder(path string) (*a49RawRecorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	return &a49RawRecorder{file: file}, nil
}

func (recorder *a49RawRecorder) write(record a49RawRecord) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if recorder.err != nil {
		return
	}
	payload, err := json.Marshal(record)
	if err == nil {
		payload = append(payload, '\n')
		_, err = recorder.file.Write(payload)
	}
	if err != nil {
		recorder.err = err
	}
}

func (recorder *a49RawRecorder) close() error {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if recorder.file != nil {
		if err := recorder.file.Sync(); recorder.err == nil && err != nil {
			recorder.err = err
		}
		if err := recorder.file.Close(); recorder.err == nil && err != nil {
			recorder.err = err
		}
		recorder.file = nil
	}
	return recorder.err
}

type a49LoadRunner struct {
	profile  a49RunProfile
	baseURL  *url.URL
	users    []a49VirtualUser
	recorder *a49RawRecorder
	counter  atomic.Uint64
	started  time.Time
}

func runA49Load(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a49-load", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fixturePath := flags.String("fixture", "testdata/design/f05-ops-capacity.yaml", "F05 fixture path")
	mode := flags.String("mode", a49FullMode, "full or smoke")
	baseURL := flags.String("base-url", "http://app-a49:3000", "internal application URL")
	rawPath := flags.String("raw", "a49-load-results.jsonl", "immutable JSONL request records")
	metadataPath := flags.String("metadata", "a49-load-metadata.json", "load metadata report")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	profile, err := loadA49RunProfile(*fixturePath, *mode)
	if err != nil {
		fmt.Fprintf(stderr, "load A49 profile: %v\n", err)
		return 2
	}
	if err := validateA49LoadEnvironment(profile); err != nil {
		fmt.Fprintf(stderr, "A49 load guard: %v\n", err)
		return 2
	}
	parsedBaseURL, err := validateA49InternalBaseURL(*baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "A49 base URL: %v\n", err)
		return 2
	}
	recorder, err := newA49RawRecorder(*rawPath)
	if err != nil {
		fmt.Fprintf(stderr, "create A49 raw results: %v\n", err)
		return 1
	}
	started := time.Now()
	runner := &a49LoadRunner{profile: profile, baseURL: parsedBaseURL, recorder: recorder, started: started}
	if err := runner.loginViewers(stdout); err != nil {
		_ = recorder.close()
		fmt.Fprintf(stderr, "login A49 viewers: %v\n", err)
		return 1
	}
	if err := runner.preflight(stdout); err != nil {
		_ = recorder.close()
		fmt.Fprintf(stderr, "A49 data/readiness preflight: %v\n", err)
		return 1
	}
	phases := make([]a49LoadPhaseMetadata, 0, 6)
	for _, scenario := range []string{"list", "hourly", "dashboard"} {
		fmt.Fprintf(stdout, "A49 %s warmup: %ds\n", scenario, profile.Capacity.WarmupSeconds)
		phases = append(phases, runner.runPhase(scenario, "warmup", profile.Capacity.WarmupSeconds))
		fmt.Fprintf(stdout, "A49 %s sample: %ds\n", scenario, profile.Capacity.SampleSeconds)
		phases = append(phases, runner.runPhase(scenario, "sample", profile.Capacity.SampleSeconds))
	}
	if err := recorder.close(); err != nil {
		fmt.Fprintf(stderr, "finalize A49 raw results: %v\n", err)
		return 1
	}
	totalElapsed := time.Since(started)
	finished := projectA49LoadTime(started, totalElapsed)
	viewerIDs := make([]string, len(runner.users))
	for index := range runner.users {
		viewerIDs[index] = runner.users[index].ID
	}
	metadata := a49LoadMetadata{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a49AcceptanceID, Status: "completed",
		Mode: profile.Mode, EvidenceClass: profile.evidenceClass(), AcceptanceEligible: profile.AcceptanceEligible,
		FixturePath: profile.FixturePath, FixtureSHA256: profile.FixtureSHA256,
		FixedNowUnix: profile.Fixture.Clock.NowUnix, ViewerIDs: viewerIDs, RawResultsPath: *rawPath,
		StartedAt: projectA49LoadTime(started, 0).Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano),
		DurationMilliseconds: totalElapsed.Milliseconds(), Scenarios: []string{"list", "hourly", "dashboard"},
		Phases: phases,
	}
	if err := writeJSONAtomic(*metadataPath, metadata); err != nil {
		fmt.Fprintf(stderr, "write A49 load metadata: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "A49 %s load completed; raw=%s metadata=%s\n", profile.Mode, *rawPath, *metadataPath)
	return 0
}

func validateA49LoadEnvironment(profile a49RunProfile) error {
	if os.Getenv("ACCEPTANCE_ID") != a49AcceptanceID || os.Getenv("A49_ISOLATED_LOAD") != "true" {
		return errors.New("ACCEPTANCE_ID=A49 and A49_ISOLATED_LOAD=true are required")
	}
	if os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != profile.evidenceClass() {
		return errors.New("A49 evidence class does not match the load profile mode")
	}
	if len(os.Getenv("A49_VIEWER_PASSWORD")) < 16 {
		return errors.New("ephemeral viewer credential is missing")
	}
	if os.Getenv("A49_FIXED_NOW_UNIX") != strconv.FormatInt(profile.Fixture.Clock.NowUnix, 10) {
		return errors.New("A49_FIXED_NOW_UNIX does not match F05")
	}
	return nil
}

func (runner *a49LoadRunner) loginViewers(stdout io.Writer) error {
	password := os.Getenv("A49_VIEWER_PASSWORD")
	for index := 1; index <= runner.profile.Capacity.ConcurrentReadUsers; index++ {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return err
		}
		transport := &http.Transport{
			Proxy: nil, MaxIdleConns: 64, MaxIdleConnsPerHost: 32,
			IdleConnTimeout: 90 * time.Second, ResponseHeaderTimeout: time.Duration(runner.profile.Capacity.RequestTimeoutSeconds) * time.Second,
		}
		client := &http.Client{Transport: transport, Jar: jar, Timeout: time.Duration(runner.profile.Capacity.RequestTimeoutSeconds) * time.Second}
		username := fmt.Sprintf("%s%02d", runner.profile.Capacity.ViewerUsernamePrefix, index)
		requestID := fmt.Sprintf("a49_login_%02d", index)
		body, _ := json.Marshal(map[string]string{"username": username, "password": password})
		request, err := http.NewRequest(http.MethodPost, runner.resolve("/api/user/login"), bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", requestID)
		started := time.Now()
		response, err := client.Do(request)
		duration := time.Since(started)
		record := a49RawRecord{SchemaVersion: evidenceSchemaVersion, Kind: "request", Phase: "login", Scenario: "login",
			Endpoint: "login", RequestID: requestID, ViewerNumber: index, DurationNanos: duration.Nanoseconds(), ObservedAtUnix: time.Now().Unix()}
		if err != nil {
			record.ErrorClass = "transport"
			runner.recorder.write(record)
			return errors.New("viewer login transport failed")
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, a49MaximumResponseBytes+1))
		response.Body.Close()
		record.StatusCode = response.StatusCode
		record.ResponseBytes = int64(len(payload))
		if readErr != nil || int64(len(payload)) > a49MaximumResponseBytes {
			record.ErrorClass = "body"
			runner.recorder.write(record)
			return errors.New("viewer login response body failed")
		}
		var envelope a49Envelope
		if response.StatusCode != http.StatusOK || json.Unmarshal(payload, &envelope) != nil || !validA49SuccessEnvelope(envelope, requestID) {
			record.ErrorClass = "contract"
			runner.recorder.write(record)
			return errors.New("viewer login contract failed")
		}
		var login struct {
			ID                 string `json:"id"`
			Username           string `json:"username"`
			Role               string `json:"role"`
			Status             int    `json:"status"`
			MustChangePassword bool   `json:"must_change_password"`
		}
		if json.Unmarshal(envelope.Data, &login) != nil || !canonicalPositiveA49ID(login.ID) || login.Username != username ||
			login.Role != "viewer" || login.Status != 1 || login.MustChangePassword || len(jar.Cookies(runner.baseURL)) == 0 {
			record.ErrorClass = "dto"
			runner.recorder.write(record)
			return errors.New("viewer login DTO/session failed")
		}
		record.Success = true
		runner.recorder.write(record)
		runner.users = append(runner.users, a49VirtualUser{Number: index, ID: login.ID, Username: username, Client: client})
	}
	fmt.Fprintf(stdout, "A49 authenticated %d distinct viewer sessions\n", len(runner.users))
	return nil
}

func validateA49InternalBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Host != "app-a49:3000" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("must be exactly the internal http://app-a49:3000 origin")
	}
	return parsed, nil
}

func (runner *a49LoadRunner) preflight(stdout io.Writer) error {
	if len(runner.users) != runner.profile.Capacity.ConcurrentReadUsers {
		return errors.New("viewer session cardinality mismatch")
	}
	seen := make(map[string]struct{}, len(runner.users))
	for _, user := range runner.users {
		if _, exists := seen[user.ID]; exists {
			return errors.New("viewer identities are not distinct")
		}
		seen[user.ID] = struct{}{}
	}
	for _, endpoint := range runner.profile.Capacity.Endpoints {
		requestID := "a49_preflight_" + strings.TrimPrefix(endpoint.Name, "dashboard_")
		if len(requestID) > 64 {
			return errors.New("preflight request ID exceeds middleware contract")
		}
		record := runner.perform(runner.users[0], "preflight", endpoint, requestID, "")
		runner.recorder.write(record)
		if !record.Success {
			return fmt.Errorf("endpoint %s failed (%s)", endpoint.Name, record.ErrorClass)
		}
	}
	fmt.Fprintln(stdout, "A49 fixed-clock/non-empty DTO preflight passed")
	return nil
}

func (runner *a49LoadRunner) runPhase(scenario, phase string, configuredSeconds int) a49LoadPhaseMetadata {
	endpoints := runner.scenarioEndpoints(scenario)
	phaseStarted := time.Now()
	startOffset := phaseStarted.Sub(runner.started)
	duration := time.Duration(configuredSeconds) * time.Second
	deadline := phaseStarted.Add(duration)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for _, user := range runner.users {
		user := user
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			iteration := uint64(0)
			for time.Now().Before(deadline) {
				iteration++
				if scenario == "dashboard" {
					runner.performDashboardBatch(user, phase, endpoints, iteration)
				} else {
					endpoint := endpoints[int(iteration+uint64(user.Number))%len(endpoints)]
					requestID := runner.nextRequestID(phase, scenario, user.Number, iteration, "")
					runner.recorder.write(runner.perform(user, phase, endpoint, requestID, ""))
				}
				runner.think(deadline)
			}
		}()
	}
	close(start)
	wait.Wait()
	elapsed := time.Since(phaseStarted)
	evidenceStarted := projectA49LoadTime(runner.started, startOffset)
	evidenceFinished := projectA49LoadTime(runner.started, startOffset+elapsed)
	return a49LoadPhaseMetadata{
		Scenario: scenario, Phase: phase, ConfiguredSeconds: configuredSeconds,
		StartedAt: evidenceStarted.Format(time.RFC3339Nano), FinishedAt: evidenceFinished.Format(time.RFC3339Nano),
		DurationMilliseconds: elapsed.Milliseconds(),
	}
}

func projectA49LoadTime(anchor time.Time, monotonicOffset time.Duration) time.Time {
	return anchor.Add(monotonicOffset).UTC()
}

func (runner *a49LoadRunner) performDashboardBatch(user a49VirtualUser, phase string, endpoints []a49Endpoint, iteration uint64) {
	batchSequence := runner.counter.Add(1)
	batchID := fmt.Sprintf("a49_%s_dash_%02d_%06d", phaseShortA49(phase), user.Number, batchSequence)
	started := time.Now()
	records := make([]a49RawRecord, len(endpoints))
	var wait sync.WaitGroup
	for index, endpoint := range endpoints {
		index, endpoint := index, endpoint
		wait.Add(1)
		go func() {
			defer wait.Done()
			suffix := strings.TrimPrefix(endpoint.Name, "dashboard_")
			requestID := runner.nextRequestID(phase, "dash", user.Number, iteration, suffix)
			records[index] = runner.perform(user, phase, endpoint, requestID, batchID)
		}()
	}
	wait.Wait()
	success := true
	for _, record := range records {
		runner.recorder.write(record)
		success = success && record.Success
	}
	composite := a49RawRecord{
		SchemaVersion: evidenceSchemaVersion, Kind: "composite", Phase: phase, Scenario: "dashboard",
		Endpoint: "dashboard_composite", BatchID: batchID, ViewerNumber: user.Number,
		Success: success, DurationNanos: time.Since(started).Nanoseconds(), ObservedAtUnix: time.Now().Unix(),
	}
	if !success {
		composite.ErrorClass = "child_request"
	}
	runner.recorder.write(composite)
}

func (runner *a49LoadRunner) perform(
	user a49VirtualUser,
	phase string,
	endpoint a49Endpoint,
	requestID string,
	batchID string,
) a49RawRecord {
	record := a49RawRecord{
		SchemaVersion: evidenceSchemaVersion, Kind: "request", Phase: phase, Scenario: endpoint.Scenario,
		Endpoint: endpoint.Name, RequestID: requestID, BatchID: batchID, ViewerNumber: user.Number,
		ObservedAtUnix: time.Now().Unix(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(runner.profile.Capacity.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, runner.resolve(endpoint.Path), nil)
	if err != nil {
		record.ErrorClass = "request"
		return record
	}
	request.Header.Set("New-Api-User", user.ID)
	request.Header.Set("X-Request-ID", requestID)
	started := time.Now()
	response, err := user.Client.Do(request)
	if err != nil {
		record.DurationNanos = time.Since(started).Nanoseconds()
		record.ErrorClass = "transport"
		return record
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, a49MaximumResponseBytes+1))
	response.Body.Close()
	record.DurationNanos = time.Since(started).Nanoseconds()
	record.StatusCode = response.StatusCode
	record.ResponseBytes = int64(len(payload))
	if readErr != nil || int64(len(payload)) > a49MaximumResponseBytes {
		record.ErrorClass = "body"
		return record
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "application/json") {
		record.ErrorClass = "status"
		return record
	}
	var envelope a49Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil || !validA49SuccessEnvelope(envelope, requestID) {
		record.ErrorClass = "envelope"
		return record
	}
	if err := validateA49EndpointDTO(endpoint.Name, envelope.Data, runner.profile); err != nil {
		record.ErrorClass = "dto"
		return record
	}
	record.Success = true
	return record
}

func validA49SuccessEnvelope(envelope a49Envelope, requestID string) bool {
	return envelope.Success && envelope.Message == "" && envelope.Code == "" && envelope.RequestID == requestID &&
		len(envelope.Data) > 0 && string(envelope.Data) != "null"
}

func validateA49EndpointDTO(name string, payload json.RawMessage, profile a49RunProfile) error {
	capacity := profile.Capacity
	switch name {
	case "list_sites":
		var page struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Items    []struct {
				ID               string `json:"id"`
				Name             string `json:"name"`
				ManagementStatus string `json:"management_status"`
				StatisticsStatus string `json:"statistics_status"`
			} `json:"items"`
		}
		if json.Unmarshal(payload, &page) != nil || page.Page != 1 || page.PageSize != 20 ||
			page.Total != capacity.Sites || len(page.Items) == 0 || len(page.Items) > 20 {
			return errors.New("site list page contract")
		}
		for _, item := range page.Items {
			if !canonicalPositiveA49ID(item.ID) || item.Name == "" || item.ManagementStatus == "" || item.StatisticsStatus == "" {
				return errors.New("site list item contract")
			}
		}
		return nil
	case "list_customers":
		var page struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Items    []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				Status       string `json:"status"`
				AccountCount int    `json:"account_count"`
			} `json:"items"`
		}
		if json.Unmarshal(payload, &page) != nil || page.Page != 1 || page.PageSize != 20 ||
			page.Total != capacity.Customers || len(page.Items) == 0 || len(page.Items) > 20 {
			return errors.New("customer list page contract")
		}
		for _, item := range page.Items {
			if !canonicalPositiveA49ID(item.ID) || item.Name == "" || item.Status == "" || item.AccountCount < 0 {
				return errors.New("customer list item contract")
			}
		}
		return nil
	case "list_accounts":
		var page struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Items    []struct {
				ID           string `json:"id"`
				SiteID       string `json:"site_id"`
				CustomerID   string `json:"customer_id"`
				RemoteUserID string `json:"remote_user_id"`
				Username     string `json:"username"`
				Quota        string `json:"quota"`
			} `json:"items"`
		}
		if json.Unmarshal(payload, &page) != nil || page.Page != 1 || page.PageSize != 20 ||
			page.Total != capacity.ManagedAccounts || len(page.Items) == 0 || len(page.Items) > 20 {
			return errors.New("account list page contract")
		}
		for _, item := range page.Items {
			if !canonicalPositiveA49ID(item.ID) || !canonicalPositiveA49ID(item.SiteID) ||
				!canonicalPositiveA49ID(item.CustomerID) || !canonicalPositiveA49ID(item.RemoteUserID) ||
				item.Username == "" || !canonicalNonNegativeA49Int(item.Quota) {
				return errors.New("account list item contract")
			}
		}
		return nil
	case "hourly_global_31d":
		var statistics struct {
			Scope       string `json:"scope"`
			Granularity string `json:"granularity"`
			Range       struct {
				StartTimestamp int64  `json:"start_timestamp"`
				EndTimestamp   int64  `json:"end_timestamp"`
				Timezone       string `json:"timezone"`
			} `json:"range"`
			Summary struct {
				RequestCount *string `json:"request_count"`
				DataStatus   string  `json:"data_status"`
			} `json:"summary"`
			Trend []struct {
				BucketStart int64  `json:"bucket_start"`
				DataStatus  string `json:"data_status"`
			} `json:"trend"`
		}
		expectedPoints := int((capacity.HourlyQueryEndUnix - capacity.HourlyQueryStartUnix) / 3600)
		if json.Unmarshal(payload, &statistics) != nil || statistics.Scope != "global" || statistics.Granularity != "hour" ||
			statistics.Range.StartTimestamp != capacity.HourlyQueryStartUnix || statistics.Range.EndTimestamp != capacity.HourlyQueryEndUnix ||
			statistics.Range.Timezone != "Asia/Shanghai" || statistics.Summary.RequestCount == nil ||
			!canonicalNonNegativeA49Int(*statistics.Summary.RequestCount) || statistics.Summary.DataStatus == "" ||
			len(statistics.Trend) != expectedPoints || statistics.Trend[0].BucketStart != capacity.HourlyQueryStartUnix {
			return errors.New("31-day hourly statistics contract")
		}
		return nil
	case "dashboard_summary":
		var summary struct {
			Today struct {
				RequestCount *string `json:"request_count"`
				AsOf         *int64  `json:"as_of"`
			} `json:"today"`
			ActiveAccountsToday *string `json:"active_accounts_today"`
			SiteCount           int     `json:"site_count"`
			CustomerCount       int     `json:"customer_count"`
			ManagedAccountCount int     `json:"managed_account_count"`
			RealtimeAsOf        *int64  `json:"realtime_as_of"`
			ResourceAsOf        *int64  `json:"resource_as_of"`
		}
		if json.Unmarshal(payload, &summary) != nil {
			return errors.New("dashboard summary JSON contract")
		}
		if summary.SiteCount != capacity.Sites || summary.CustomerCount != capacity.Customers ||
			summary.ManagedAccountCount != capacity.ManagedAccounts {
			return errors.New("dashboard summary entity cardinality contract")
		}
		if summary.Today.RequestCount == nil || !canonicalNonNegativeA49Int(*summary.Today.RequestCount) ||
			summary.Today.AsOf == nil || *summary.Today.AsOf != capacity.HourlyQueryEndUnix {
			return errors.New("dashboard summary last-complete-hour contract")
		}
		if summary.ActiveAccountsToday == nil || !canonicalNonNegativeA49Int(*summary.ActiveAccountsToday) {
			return errors.New("dashboard summary active-account contract")
		}
		if summary.RealtimeAsOf == nil || *summary.RealtimeAsOf != profile.Fixture.Clock.NowUnix {
			return errors.New("dashboard summary realtime fixed-clock contract")
		}
		if summary.ResourceAsOf == nil || *summary.ResourceAsOf != profile.Fixture.Clock.NowUnix-profile.Fixture.Clock.NowUnix%60 {
			return errors.New("dashboard summary resource fixed-clock contract")
		}
		return nil
	case "dashboard_trend":
		var trend []struct {
			BucketStart int64  `json:"bucket_start"`
			DataStatus  string `json:"data_status"`
		}
		if json.Unmarshal(payload, &trend) != nil || len(trend) != 30 || trend[0].BucketStart <= 0 {
			return errors.New("dashboard trend contract")
		}
		return nil
	case "dashboard_top_site", "dashboard_top_customer", "dashboard_top_model", "dashboard_top_channel":
		var items []struct {
			DimensionType string  `json:"dimension_type"`
			DimensionID   string  `json:"dimension_id"`
			Value         *string `json:"value"`
			DataStatus    string  `json:"data_status"`
		}
		expectedType := strings.TrimPrefix(name, "dashboard_top_")
		if json.Unmarshal(payload, &items) != nil || len(items) == 0 || len(items) > 5 {
			return errors.New("dashboard ranking cardinality")
		}
		for _, item := range items {
			if item.DimensionType != expectedType || item.DimensionID == "" || item.Value == nil ||
				!canonicalNonNegativeA49Int(*item.Value) || item.DataStatus == "" {
				return errors.New("dashboard ranking item contract")
			}
		}
		return nil
	case "dashboard_health":
		var health struct {
			FiringAlertCount          int    `json:"firing_alert_count"`
			CriticalAlertCount        int    `json:"critical_alert_count"`
			WarningAlertCount         int    `json:"warning_alert_count"`
			YesterdayValidationStatus string `json:"yesterday_validation_status"`
			Sites                     []struct {
				SiteID           string `json:"site_id"`
				ManagementStatus string `json:"management_status"`
			} `json:"sites"`
		}
		if json.Unmarshal(payload, &health) != nil || health.FiringAlertCount < 0 || health.CriticalAlertCount < 0 ||
			health.WarningAlertCount < 0 || health.YesterdayValidationStatus == "" || len(health.Sites) != capacity.Sites {
			return errors.New("dashboard health contract")
		}
		for _, site := range health.Sites {
			if !canonicalPositiveA49ID(site.SiteID) || site.ManagementStatus == "" {
				return errors.New("dashboard health site contract")
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown A49 endpoint contract %q", name)
	}
}

func (runner *a49LoadRunner) scenarioEndpoints(scenario string) []a49Endpoint {
	result := make([]a49Endpoint, 0, 7)
	for _, endpoint := range runner.profile.Capacity.Endpoints {
		if endpoint.Scenario == scenario {
			result = append(result, endpoint)
		}
	}
	return result
}

func (runner *a49LoadRunner) nextRequestID(phase, scenario string, viewer int, iteration uint64, suffix string) string {
	sequence := runner.counter.Add(1)
	result := fmt.Sprintf("a49_%s_%s_%02d_%06d_%06d", phaseShortA49(phase), scenario, viewer, iteration%1000000, sequence%1000000)
	if suffix != "" {
		short := strings.NewReplacer("customer", "cust", "channel", "chan", "summary", "sum", "health", "hlth").Replace(suffix)
		result += "_" + short
	}
	if len(result) > 64 {
		result = result[:64]
	}
	return result
}

func phaseShortA49(phase string) string {
	switch phase {
	case "warmup":
		return "warm"
	case "sample":
		return "sample"
	default:
		return phase
	}
}

func (runner *a49LoadRunner) think(deadline time.Time) {
	duration := time.Duration(runner.profile.Capacity.ThinkTimeMilliseconds) * time.Millisecond
	if duration <= 0 {
		return
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return
	}
	if duration > remaining {
		duration = remaining
	}
	timer := time.NewTimer(duration)
	<-timer.C
}

func (runner *a49LoadRunner) resolve(path string) string {
	return strings.TrimRight(runner.baseURL.String(), "/") + path
}

func canonicalPositiveA49ID(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func canonicalNonNegativeA49Int(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed >= 0 && strconv.FormatInt(parsed, 10) == value
}
