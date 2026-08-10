package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestNewAPIClientLogPageContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/log/" || request.URL.Query().Get("p") != "1" || request.URL.Query().Get("page_size") != "100" ||
			request.URL.Query().Get("start_timestamp") != "100" || request.URL.Query().Get("end_timestamp") != "199" {
			t.Fatalf("log request = %s", request.URL.String())
		}
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":88,"user_id":7,"created_at":150,"type":2,"content":"consume","username":"alice","token_name":"key","model_name":"gpt","quota":9,"prompt_tokens":3,"completion_tokens":4,"use_time":2,"is_stream":true,"channel":5,"token_id":6,"group":"vip","ip":"203.0.113.1","other":"{\"frt\":450,\"cache_tokens\":2,\"cache_creation_tokens\":3,\"cache_creation_tokens_5m\":4,\"cache_creation_tokens_1h\":5,\"stream_status\":{\"status\":\"ok\",\"end_reason\":\"done\",\"error_count\":0}}"}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	page, err := client.LogPage(context.Background(), "req-log", 100, 199, 1)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].RequestID != "" || page.Items[0].TokenID != 6 ||
		page.Items[0].FirstResponseTimeMs == nil || *page.Items[0].FirstResponseTimeMs != 450 || page.Items[0].StreamStatus != "ok" ||
		page.Items[0].StreamEndReason != "done" || page.Items[0].StreamErrorCount != 0 || page.Items[0].CacheReadTokens != 2 ||
		page.Items[0].CacheCreationTokens != 3 || page.Items[0].CacheCreation5m != 4 || page.Items[0].CacheCreation1h != 5 {
		t.Fatalf("log page = %+v, %v", page, err)
	}
}

func TestSafeUpstreamLogOtherIgnoresUnsafeTimingDiagnostics(t *testing.T) {
	for _, test := range []struct {
		raw        string
		wantStatus string
	}{
		{raw: `{"frt":-1}`},
		{raw: `{"frt":1.5}`},
		{raw: `{"stream_status":{"status":"ok","error_count":-1}}`, wantStatus: "ok"},
		{raw: `{"stream_status":{"status":"status-name-that-is-too-long"}}`},
	} {
		firstResponseTimeMs, status, endReason, errorCount, cacheRead, cacheCreation, cache5m, cache1h := safeUpstreamLogOther(&test.raw)
		if firstResponseTimeMs != nil || status != test.wantStatus || endReason != "" || errorCount != 0 {
			t.Fatalf("unsafe log other was not safely normalized: %s", test.raw)
		}
		if cacheRead != 0 || cacheCreation != 0 || cache5m != 0 || cache1h != 0 {
			t.Fatalf("unsafe cache diagnostics were not normalized: %s", test.raw)
		}
	}
}

func TestResolveLogRateUsesSiteThenFallback(t *testing.T) {
	quotaPerUnit, exchangeRate, updatedAt := "500000", "7.2", int64(123)
	site := resolveLogRate(&quotaPerUnit, &exchangeRate, &updatedAt, "400000", "6.8")
	if site.Source != "site" || site.UpdatedAt == nil || *site.UpdatedAt != updatedAt {
		t.Fatalf("site rate = %+v", site)
	}
	fallback := resolveLogRate(nil, nil, nil, "400000", "6.8")
	if fallback.Source != "fallback" || fallback.QuotaPerUnit == nil || *fallback.QuotaPerUnit != "400000" {
		t.Fatalf("fallback rate = %+v", fallback)
	}
	unavailable := resolveLogRate(nil, nil, &updatedAt, "400000", "6.8")
	if unavailable.Source != "unavailable" {
		t.Fatalf("unavailable rate = %+v", unavailable)
	}
}

func TestCanonicalUpstreamLogFactIgnoresDisplayIDAndRedactsSecrets(t *testing.T) {
	row := upstreamLogRowForTest()
	firstResponseTimeMs := int64(345)
	row.FirstResponseTimeMs = &firstResponseTimeMs
	row.StreamStatus = "ok"
	row.StreamEndReason = "done"
	row.CacheReadTokens = 10
	row.CacheCreationTokens = 20
	row.Content = "Authorization: Bearer secret-value"
	row.ID = 1
	first, firstKey, err := canonicalUpstreamLogFact(row)
	if err != nil {
		t.Fatal(err)
	}
	row.ID = 999
	second, secondKey, err := canonicalUpstreamLogFact(row)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey != secondKey || first.ContentRedacted != "[redacted]" || second.IP != "" ||
		first.FirstResponseTimeMs == nil || *first.FirstResponseTimeMs != firstResponseTimeMs || first.StreamStatus != "ok" || first.CacheReadTokens != 10 {
		t.Fatalf("canonical log key/redaction = %s/%s %+v %+v", firstKey, secondKey, first, second)
	}
	if strings.Contains(first.ContentRedacted, "secret-value") {
		t.Fatal("secret remained in content")
	}
	row.Content = "Authorization: Bearer another-secret"
	third, thirdKey, err := canonicalUpstreamLogFact(row)
	if err != nil {
		t.Fatal(err)
	}
	if third.ContentRedacted != "[redacted]" || thirdKey == firstKey {
		t.Fatalf("distinct secret-bearing log contents collapsed: first=%s third=%s", firstKey, thirdKey)
	}
}

func TestCollectWindowContinuesPastOverlappingPagesUntilAllStableRowsArrive(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatal(err)
	}
	rootID := int64(1)
	site := model.Site{
		Name: "Log overlap", BaseURL: "https://logs.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		OnlineStatus: constant.SiteOnlineOnline, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, RootUserID: &rootID,
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create log overlap site: %v", err)
	}
	start, end := int64(1_752_391_200), int64(1_752_398_400)
	rows := make([]dto.UpstreamLogRow, 4)
	for index := range rows {
		rows[index] = upstreamLogRowForTest()
		rows[index].ID = int64(index + 1)
		rows[index].CreatedAt = start + int64(index+1)
		rows[index].RequestID = "req-" + strconv.Itoa(index+1)
	}
	client := &overlappingLogClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix()), pages: map[int]dto.UpstreamLogPage{
		1: {Page: 1, PageSize: 2, Total: 4, Items: []dto.UpstreamLogRow{rows[0], rows[1]}},
		2: {Page: 2, PageSize: 2, Total: 4, Items: []dto.UpstreamLogRow{rows[1], rows[2]}},
		3: {Page: 3, PageSize: 2, Total: 4, Items: []dto.UpstreamLogRow{rows[3]}},
	}}
	service, err := NewUpstreamLogService(UpstreamLogServiceOptions{
		Database: tx, SiteRepository: model.NewSiteRepository(tx),
		ClientFactory: &testSiteClientFactory{authenticated: client, public: client}, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Re-encrypt after GORM has confirmed the explicit site identity used by AAD.
	token, err := cipher.Encrypt([]byte("log-secret"), siteTokenAAD(site.ID))
	if err != nil || tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", token).Error != nil {
		t.Fatalf("store log token: %v", err)
	}
	fetched, written, err := service.collectWindow(context.Background(), site.ID, site.ConfigVersion, start, end, "log-overlap")
	if err != nil || fetched != 5 || written != 4 || client.calls != 3 {
		t.Fatalf("overlap collection fetched=%d written=%d calls=%d err=%v", fetched, written, client.calls, err)
	}
	var count int64
	if err := tx.Model(&model.UpstreamLogFact{}).Where("site_id = ?", site.ID).Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("overlap fact count=%d err=%v", count, err)
	}
	client.calls = 0
	client.pages = map[int]dto.UpstreamLogPage{
		1: {Page: 1, PageSize: 2, Total: 4, Items: []dto.UpstreamLogRow{rows[0], rows[1]}},
		2: {Page: 2, PageSize: 2, Total: 5, Items: []dto.UpstreamLogRow{rows[2], rows[3]}},
	}
	if _, _, err := service.collectWindow(context.Background(), site.ID, site.ConfigVersion, start, end, "log-drift"); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("total drift error=%v", err)
	}
	states, err := model.NewUpstreamLogRepository(tx).LoadStates(context.Background(), []int64{site.ID})
	if err != nil || len(states) != 1 || states[0].Status != dto.LogCollectionUnavailable ||
		states[0].LastErrorCode != string(constant.MessageUpstreamResponseInvalid) {
		t.Fatalf("total drift state=%+v err=%v", states, err)
	}
	if err := tx.Model(&model.UpstreamLogFact{}).Where("site_id = ?", site.ID).Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("failed overlap collection changed facts count=%d err=%v", count, err)
	}
}

func TestScheduledLogTaskBackfillsToStatisticsStartThenStaysIncremental(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	now := clock.Now().Unix()
	currentHour := now - now%3600
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatal(err)
	}
	rootID := int64(1)
	statisticsStart := currentHour - 50*3600
	site := model.Site{
		Name: "Log history", BaseURL: "https://log-history.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		OnlineStatus: constant.SiteOnlineOnline, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, RootUserID: &rootID, StatisticsStartAt: &statisticsStart,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create log history site: %v", err)
	}
	token, err := cipher.Encrypt([]byte("log-history-secret"), siteTokenAAD(site.ID))
	if err != nil || tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", token).Error != nil {
		t.Fatalf("store log history token: %v", err)
	}
	client := &historyLogClient{testSiteClient: authorizedTestSiteClient(now)}
	collector, err := NewUpstreamLogService(UpstreamLogServiceOptions{
		Database: tx, SiteRepository: model.NewSiteRepository(tx),
		ClientFactory: &testSiteClientFactory{authenticated: client, public: client}, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetched, written, err := collector.ExecuteScheduledLogTask(context.Background(), site.ID, site.ConfigVersion, "log-scheduled-first"); err != nil || fetched != 0 || written != 0 {
		t.Fatalf("first scheduled log collection fetched=%d written=%d err=%v", fetched, written, err)
	}
	wantWindows := [][2]int64{
		{currentHour - 2*3600, currentHour - 1},
		{currentHour - 26*3600, currentHour - 2*3600 - 1},
		{statisticsStart, currentHour - 26*3600 - 1},
	}
	if len(client.windows) != len(wantWindows) {
		t.Fatalf("log backfill windows=%v want=%v", client.windows, wantWindows)
	}
	for index := range wantWindows {
		if client.windows[index] != wantWindows[index] {
			t.Fatalf("log backfill window[%d]=%v want=%v", index, client.windows[index], wantWindows[index])
		}
	}
	state, err := model.NewUpstreamLogRepository(tx).LoadState(context.Background(), site.ID)
	if err != nil || state.HistoryStartAt == nil || *state.HistoryStartAt != statisticsStart || state.BackfillCompletedAt == nil {
		t.Fatalf("completed log backfill state=%#v err=%v", state, err)
	}
	client.windows = nil
	if _, _, err := collector.ExecuteScheduledLogTask(context.Background(), site.ID, site.ConfigVersion, "log-scheduled-next"); err != nil {
		t.Fatalf("incremental scheduled log collection: %v", err)
	}
	if len(client.windows) != 1 || client.windows[0] != wantWindows[0] {
		t.Fatalf("incremental log windows=%v want=%v", client.windows, wantWindows[:1])
	}
}

func TestScheduledLogTaskRepairsDowntimeGapBeforeRecentOverlap(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	now := clock.Now().Unix()
	currentHour := now - now%3600
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatal(err)
	}
	rootID := int64(1)
	statisticsStart := currentHour - 4*3600
	site := model.Site{
		Name: "Log gap recovery", BaseURL: "https://log-gap.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		OnlineStatus: constant.SiteOnlineOnline, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, RootUserID: &rootID, StatisticsStartAt: &statisticsStart,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	token, err := cipher.Encrypt([]byte("log-gap-secret"), siteTokenAAD(site.ID))
	if err != nil || tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", token).Error != nil {
		t.Fatalf("store log gap token: %v", err)
	}
	client := &historyLogClient{testSiteClient: authorizedTestSiteClient(now)}
	collector, err := NewUpstreamLogService(UpstreamLogServiceOptions{
		Database: tx, SiteRepository: model.NewSiteRepository(tx),
		ClientFactory: &testSiteClientFactory{authenticated: client, public: client}, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := collector.ExecuteScheduledLogTask(context.Background(), site.ID, site.ConfigVersion, "log-gap-initial"); err != nil {
		t.Fatal(err)
	}
	client.windows = nil
	clock.Advance(30 * time.Hour)
	newCurrentHour := clock.Now().Unix()
	newCurrentHour -= newCurrentHour % 3600
	if _, _, err := collector.ExecuteScheduledLogTask(context.Background(), site.ID, site.ConfigVersion, "log-gap-recovery"); err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{
		{currentHour, currentHour + 24*3600 - 1},
		{currentHour + 24*3600, newCurrentHour - 2*3600 - 1},
		{newCurrentHour - 2*3600, newCurrentHour - 1},
	}
	if len(client.windows) != len(want) {
		t.Fatalf("gap recovery windows=%v want=%v", client.windows, want)
	}
	for index := range want {
		if client.windows[index] != want[index] {
			t.Fatalf("gap recovery window[%d]=%v want=%v", index, client.windows[index], want[index])
		}
	}
}

type overlappingLogClient struct {
	*testSiteClient
	pages map[int]dto.UpstreamLogPage
	calls int
}

type historyLogClient struct {
	*testSiteClient
	windows [][2]int64
}

func (client *historyLogClient) LogPage(_ context.Context, _ string, start, end int64, _ int) (dto.UpstreamLogPage, error) {
	client.windows = append(client.windows, [2]int64{start, end})
	return dto.UpstreamLogPage{Page: 1, PageSize: 100, Total: 0, Items: []dto.UpstreamLogRow{}}, nil
}

func (client *overlappingLogClient) LogPage(_ context.Context, _ string, _, _ int64, page int) (dto.UpstreamLogPage, error) {
	client.calls++
	return client.pages[page], nil
}

func upstreamLogRowForTest() dto.UpstreamLogRow {
	return dto.UpstreamLogRow{ID: 1, UserID: 2, CreatedAt: 100, Type: 2, Content: "ok", Username: "u", TokenName: "t",
		ModelName: "m", Quota: 3, PromptTokens: 4, CompletionTokens: 5, UseTimeSeconds: 6, ChannelID: 7, TokenID: 8,
		UseGroup: "g", RequestID: "req", UpstreamRequestID: "up"}
}
