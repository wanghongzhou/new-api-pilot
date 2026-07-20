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
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":88,"user_id":7,"created_at":150,"type":2,"content":"consume","username":"alice","token_name":"key","model_name":"gpt","quota":9,"prompt_tokens":3,"completion_tokens":4,"use_time":2,"is_stream":true,"channel":5,"token_id":6,"group":"vip","ip":"203.0.113.1"}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	page, err := client.LogPage(context.Background(), "req-log", 100, 199, 1)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].RequestID != "" || page.Items[0].TokenID != 6 {
		t.Fatalf("log page = %+v, %v", page, err)
	}
}

func TestCanonicalUpstreamLogFactIgnoresDisplayIDAndRedactsSecrets(t *testing.T) {
	row := upstreamLogRowForTest()
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
	if firstKey != secondKey || first.ContentRedacted != "[redacted]" || second.IP != "" {
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

type overlappingLogClient struct {
	*testSiteClient
	pages map[int]dto.UpstreamLogPage
	calls int
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
