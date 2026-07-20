package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

var _ func(UsageCollectionRequest, int64, []model.UsageFactInput, model.UsageFactMutation) (model.UsageAggregationCommit, error) = bindUsageAggregationCommit

func TestUsageCollectionCallsFlowAndDataExactlyOnceForRootCurrentHour(t *testing.T) {
	now := time.Date(2026, 7, 13, 6, 5, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	client := &usageCollectionTestClient{
		flow: []dto.UpstreamFlowRow{{
			UserID: 1, Username: "", ModelName: "Model-A", ChannelID: 0,
			RequestCount: 2, Quota: 20, TokenUsed: 200,
		}},
		data: []dto.UpstreamDataRow{{
			ModelName: "Model-A", CreatedAt: hour, RequestCount: 2, Quota: 20, TokenUsed: 200,
		}},
	}
	service, run, window := newUsageCollectionTestService(t, now, hour, constant.TaskTypeUsageHour, client)
	result, err := service.CollectHour(context.Background(), UsageCollectionRequest{
		Run: run, Window: window, RequestID: run.LastRequestID,
	})
	if err != nil || result.Failure != nil || !result.Commit.Valid() ||
		result.Planned.WrittenRows != 1 ||
		len(result.Planned.SourceHash) != 64 || result.SourceRequestID != run.LastRequestID {
		t.Fatalf("collect root current hour = %#v, %v", result, err)
	}
	if client.flowCalls != 1 || client.dataCalls != 1 || client.closeCalls != 1 {
		t.Fatalf("usage calls flow=%d data=%d close=%d", client.flowCalls, client.dataCalls, client.closeCalls)
	}
}

func TestUsageCollectionResultExposesOnlyOneAuthoritativeCommitHook(t *testing.T) {
	resultType := reflect.TypeOf(UsageCollectionResult{})
	commitType := reflect.TypeOf(model.UsageAggregationCommit{})
	factType := reflect.TypeOf(model.UsageFactMutation{})
	hooks := 0
	for index := 0; index < resultType.NumField(); index++ {
		field := resultType.Field(index)
		if field.Type == commitType || field.Type == factType || field.Type.Kind() == reflect.Func {
			hooks++
			if field.Name != "Commit" || field.Type != commitType {
				t.Fatalf("unexpected executable usage collection field %s %s", field.Name, field.Type)
			}
		}
	}
	if hooks != 1 {
		t.Fatalf("usage collection executable hook count = %d, want 1", hooks)
	}
	for _, removed := range []string{"Mutation", "Aggregation"} {
		if _, exists := resultType.FieldByName(removed); exists {
			t.Fatalf("legacy executable hook %s is still exposed", removed)
		}
	}
}

func TestUsageCollectionStillCallsBothEndpointsOnPartialFailure(t *testing.T) {
	now := time.Date(2026, 7, 13, 7, 5, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	client := &usageCollectionTestClient{
		flowErr: ErrUpstreamUnavailable,
		data:    []dto.UpstreamDataRow{{ModelName: "m", CreatedAt: hour, RequestCount: 1, Quota: 2, TokenUsed: 3}},
	}
	service, run, window := newUsageCollectionTestService(t, now, hour, constant.TaskTypeUsageBackfill, client)
	result, err := service.CollectHour(context.Background(), UsageCollectionRequest{
		Run: run, Window: window, RequestID: run.LastRequestID,
	})
	if err != nil || result.Failure == nil || !errors.Is(result.Failure.Cause, ErrUpstreamUnavailable) ||
		!result.Commit.Valid() {
		t.Fatalf("collect partial failure = %#v, %v", result, err)
	}
	if result.Failure.Code != constant.CodeUpstreamUnavailable {
		t.Fatalf("partial failure code = %q", result.Failure.Code)
	}
	if client.flowCalls != 1 || client.dataCalls != 1 {
		t.Fatalf("partial failure calls flow=%d data=%d", client.flowCalls, client.dataCalls)
	}
}

func TestUsageAuthorizationExpirationIsIdempotentAndKeepsDisabledSitePaused(t *testing.T) {
	now := time.Date(2026, 7, 13, 7, 35, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	collector, run, _ := newUsageCollectionTestService(
		t, now, hour, constant.TaskTypeUsageHour, &usageCollectionTestClient{},
	)
	site, err := collector.sites.FindByID(context.Background(), *run.SiteID)
	if err != nil {
		t.Fatalf("read disabled expiration site: %v", err)
	}
	site.ManagementStatus = constant.SiteManagementDisabled
	site.StatisticsStatus = constant.SiteStatisticsPaused
	if err := collector.sites.Save(context.Background(), &site); err != nil {
		t.Fatalf("disable expiration site: %v", err)
	}
	if err := collector.expireUsageAuthorization(context.Background(), site.ID, site.ConfigVersion); err != nil {
		t.Fatalf("expire disabled usage authorization: %v", err)
	}
	if err := collector.expireUsageAuthorization(context.Background(), site.ID, site.ConfigVersion); err != nil {
		t.Fatalf("repeat disabled usage expiration: %v", err)
	}
	persisted, err := collector.sites.FindByID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("read expired disabled site: %v", err)
	}
	if persisted.ConfigVersion != site.ConfigVersion+1 || persisted.AuthStatus != constant.SiteAuthExpired ||
		persisted.StatisticsStatus != constant.SiteStatisticsPaused {
		t.Fatalf("expired disabled site = %#v", persisted)
	}
}

func TestUsageCollectionExpiresAuthorizationForRemoteAndDecryptFailures(t *testing.T) {
	now := time.Date(2026, 7, 13, 7, 35, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	t.Run("remote_unauthorized", func(t *testing.T) {
		client := &usageCollectionTestClient{flowErr: ErrUpstreamAuthExpired}
		collector, run, window := newUsageCollectionTestService(t, now, hour, constant.TaskTypeUsageHour, client)
		_, err := collector.CollectHour(context.Background(), UsageCollectionRequest{
			Run: run, Window: window, RequestID: run.LastRequestID,
		})
		if !errors.Is(err, ErrUpstreamAuthExpired) {
			t.Fatalf("usage authorization error = %v", err)
		}
		persisted, readErr := collector.sites.FindByID(context.Background(), *run.SiteID)
		if readErr != nil || persisted.AuthStatus != constant.SiteAuthExpired ||
			persisted.ConfigVersion != run.SiteConfigVersion+1 {
			t.Fatalf("expired usage site = %#v, %v", persisted, readErr)
		}
		if client.flowCalls != 1 || client.dataCalls != 1 {
			t.Fatalf("usage auth calls flow=%d data=%d", client.flowCalls, client.dataCalls)
		}
	})

	t.Run("credential_decrypt", func(t *testing.T) {
		collector, run, window := newUsageCollectionTestService(
			t, now.Add(time.Second), hour, constant.TaskTypeUsageHour, &usageCollectionTestClient{},
		)
		site, err := collector.sites.FindByID(context.Background(), *run.SiteID)
		if err != nil {
			t.Fatalf("read usage site: %v", err)
		}
		invalid := "invalid-ciphertext"
		site.AccessTokenEncrypted = &invalid
		if err := collector.sites.Save(context.Background(), &site); err != nil {
			t.Fatalf("corrupt usage credential: %v", err)
		}
		_, err = collector.CollectHour(context.Background(), UsageCollectionRequest{
			Run: run, Window: window, RequestID: run.LastRequestID,
		})
		if !errors.Is(err, ErrUpstreamAuthExpired) {
			t.Fatalf("usage decrypt error = %v", err)
		}
		persisted, readErr := collector.sites.FindByID(context.Background(), site.ID)
		if readErr != nil || persisted.AuthStatus != constant.SiteAuthExpired ||
			persisted.ConfigVersion != run.SiteConfigVersion+1 {
			t.Fatalf("expired decrypt usage site = %#v, %v", persisted, readErr)
		}
	})
}

func TestUsageCollectionRejectsPerModelMismatchWhenWholeHourMatches(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 5, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	client := &usageCollectionTestClient{
		flow: []dto.UpstreamFlowRow{
			{UserID: 1, Username: "root", ModelName: "a", RequestCount: 1, Quota: 10, TokenUsed: 100},
			{UserID: 1, Username: "root", ModelName: "b", RequestCount: 2, Quota: 20, TokenUsed: 200},
		},
		data: []dto.UpstreamDataRow{
			{ModelName: "a", CreatedAt: hour, RequestCount: 2, Quota: 20, TokenUsed: 200},
			{ModelName: "b", CreatedAt: hour, RequestCount: 1, Quota: 10, TokenUsed: 100},
		},
	}
	service, run, window := newUsageCollectionTestService(t, now, hour, constant.TaskTypeUsageValidation, client)
	result, err := service.CollectHour(context.Background(), UsageCollectionRequest{
		Run: run, Window: window, RequestID: run.LastRequestID,
	})
	if err != nil || result.Failure == nil || !errors.Is(result.Failure.Cause, ErrUpstreamDataMismatch) ||
		result.Failure.Code != string(constant.MessageDataValidationMismatch) || !result.Commit.Valid() {
		t.Fatalf("collect model mismatch = %#v, %v", result, err)
	}
	if client.flowCalls != 1 || client.dataCalls != 1 {
		t.Fatalf("mismatch calls flow=%d data=%d", client.flowCalls, client.dataCalls)
	}
}

func TestUsageCollectionRejectsHourBeforeStatisticsStartWithoutUpstreamCalls(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 5, 0, 0, time.UTC)
	hour := now.Unix() - now.Unix()%3600 - 3600
	client := &usageCollectionTestClient{}
	service, run, window := newUsageCollectionTestServiceWithStatisticsStart(
		t, now, hour, hour+3600, constant.TaskTypeUsageBackfill, client,
	)
	_, err := service.CollectHour(context.Background(), UsageCollectionRequest{
		Run: run, Window: window, RequestID: run.LastRequestID,
	})
	if !errors.Is(err, model.ErrSiteRunConfigChanged) {
		t.Fatalf("collect before statistics start error = %v", err)
	}
	if client.flowCalls != 0 || client.dataCalls != 0 || client.closeCalls != 1 {
		t.Fatalf("out-of-range calls flow=%d data=%d close=%d", client.flowCalls, client.dataCalls, client.closeCalls)
	}
}

func TestUsageCollectionRejectsCurrentAndFutureHoursWithoutUpstreamCalls(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 5, 0, 0, time.UTC)
	currentHour := now.Unix() - now.Unix()%3600
	for _, hour := range []int64{currentHour, currentHour + 3600} {
		t.Run(fmt.Sprintf("hour_%d", hour), func(t *testing.T) {
			client := &usageCollectionTestClient{}
			service, run, window := newUsageCollectionTestService(t, now, hour, constant.TaskTypeUsageBackfill, client)
			_, err := service.CollectHour(context.Background(), UsageCollectionRequest{
				Run: run, Window: window, RequestID: run.LastRequestID,
			})
			if !errors.Is(err, model.ErrCollectionRunContract) {
				t.Fatalf("collect unfinished hour %d error = %v", hour, err)
			}
			if client.flowCalls != 0 || client.dataCalls != 0 || client.closeCalls != 0 {
				t.Fatalf("unfinished calls flow=%d data=%d close=%d", client.flowCalls, client.dataCalls, client.closeCalls)
			}
		})
	}
}

func TestUsageWindowFailureReasonPreservesStableCause(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		dataMismatch bool
		want         constant.MessageCode
	}{
		{name: "network", err: ErrUpstreamUnavailable, want: constant.MessageDataUpstreamUnavailable},
		{name: "timeout", err: context.DeadlineExceeded, want: constant.MessageDataUpstreamUnavailable},
		{name: "remote", err: ErrUpstreamRemote, want: constant.MessageDataUpstreamUnavailable},
		{name: "invalid", err: ErrUpstreamResponseInvalid, want: constant.MessageUpstreamResponseInvalid},
		{name: "too_large", err: newUpstreamResponseTooLargeError(129, 128), want: constant.MessageUpstreamResponseTooLarge},
		{name: "mismatch", err: ErrUpstreamDataMismatch, dataMismatch: true, want: constant.MessageDataValidationMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, encoded := usageWindowFailureReason(test.err, test.dataMismatch, 7, 3600)
			if code != string(test.want) {
				t.Fatalf("usage window reason = %s, want %s", code, test.want)
			}
			params := map[string]any{}
			if err := common.Unmarshal(encoded, &params); err != nil {
				t.Fatalf("decode usage window reason params: %v", err)
			}
			if err := dto.ValidateMessageParams(test.want, params); err != nil {
				t.Fatalf("validate usage window reason params %#v: %v", params, err)
			}
		})
	}
	if got := usageFailureCode(ErrUpstreamRemote); got != constant.CodeUpstreamError {
		t.Fatalf("remote upstream failure code = %s, want %s", got, constant.CodeUpstreamError)
	}
}

func TestWholeHourUsageTotalsRejectCrossModelOverflow(t *testing.T) {
	err := validateWholeHourUsageTotals([]dto.UpstreamFlowRow{
		{ModelName: "first", RequestCount: math.MaxInt64},
		{ModelName: "second", RequestCount: 1},
	}, nil)
	if !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("whole-hour overflow error = %v", err)
	}
}

type usageCollectionTestClient struct {
	SiteUpstreamClient
	flow       []dto.UpstreamFlowRow
	data       []dto.UpstreamDataRow
	flowErr    error
	dataErr    error
	flowCalls  int
	dataCalls  int
	closeCalls int
}

func (client *usageCollectionTestClient) FlowHour(context.Context, string, int64) ([]dto.UpstreamFlowRow, error) {
	client.flowCalls++
	return append([]dto.UpstreamFlowRow(nil), client.flow...), client.flowErr
}

func (client *usageCollectionTestClient) DataHour(context.Context, string, int64) ([]dto.UpstreamDataRow, error) {
	client.dataCalls++
	return append([]dto.UpstreamDataRow(nil), client.data...), client.dataErr
}

func (client *usageCollectionTestClient) CloseIdleConnections() { client.closeCalls++ }

func newUsageCollectionTestService(
	t *testing.T,
	now time.Time,
	hour int64,
	taskType string,
	client SiteUpstreamClient,
) (*UsageCollectionService, model.CollectionRun, model.CollectionRunWindow) {
	return newUsageCollectionTestServiceWithStatisticsStart(t, now, hour, hour-24*3600, taskType, client)
}

func newUsageCollectionTestServiceWithStatisticsStart(
	t *testing.T,
	now time.Time,
	hour int64,
	statisticsStart int64,
	taskType string,
	client SiteUpstreamClient,
) (*UsageCollectionService, model.CollectionRun, model.CollectionRunWindow) {
	t.Helper()
	tx := openSiteTestTransaction(t)
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create usage test cipher: %v", err)
	}
	rootID := int64(1)
	site := model.Site{
		Name:    "Usage Service " + fmt.Sprintf("%d", now.UnixNano()),
		BaseURL: "https://usage-service.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true,
		RootUserID: &rootID, StatisticsStartAt: &statisticsStart, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create usage service site: %v", err)
	}
	encrypted, err := cipher.Encrypt([]byte("usage-secret"), siteTokenAAD(site.ID))
	if err != nil {
		t.Fatalf("encrypt usage credential: %v", err)
	}
	if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", encrypted).Error; err != nil {
		t.Fatalf("store usage credential: %v", err)
	}
	service, err := NewUsageCollectionService(UsageCollectionServiceOptions{
		Repository:    model.NewSiteRepository(tx),
		ClientFactory: &usageCollectionTestFactory{client: client},
		Cipher:        cipher, Clock: testsupport.NewFakeClock(now),
	})
	if err != nil {
		t.Fatalf("create usage collection service: %v", err)
	}
	end := hour + 3600
	started := now.Unix()
	run := model.CollectionRun{
		ID: 9001, SiteID: &site.ID, SiteConfigVersion: 1, TaskType: taskType,
		TargetType: "site", TargetID: site.ID, StartTimestamp: &hour, EndTimestamp: &end,
		Status: model.CollectionTaskStatusRunning, LastRequestID: "req_usage_service",
	}
	window := model.CollectionRunWindow{
		ID: 9002, RunID: run.ID, SiteID: site.ID, HourTS: hour,
		Status: model.CollectionTaskStatusRunning, AttemptCount: 1, StartedAt: &started,
	}
	return service, run, window
}

type usageCollectionTestFactory struct{ client SiteUpstreamClient }

func (factory *usageCollectionTestFactory) NewPublic(string) (SiteUpstreamClient, error) {
	return factory.client, nil
}

func (factory *usageCollectionTestFactory) NewAuthenticated(string, string, string, int64) (SiteUpstreamClient, error) {
	return factory.client, nil
}
