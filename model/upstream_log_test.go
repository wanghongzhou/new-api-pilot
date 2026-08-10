package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func TestUpstreamLogRepositoryCommitQueryFenceAndRetention(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_100_000_000)
	site := createRunnableSite(t, database, fmt.Sprintf("upstream-log-%d", time.Now().UnixNano()), now)
	repository := NewUpstreamLogRepository(database.GORM)
	fact := UpstreamLogFact{UpstreamLogKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UpstreamLogID: 99,
		CreatedAt: now - 100, Type: 2, RemoteUserID: 7, Username: "alice", ModelName: "gpt", TokenID: 8,
		TokenName: "key", ChannelID: 9, UseGroup: "vip", RequestID: "req", UpstreamRequestID: "up", Quota: 10,
		PromptTokens: 3, CompletionTokens: 4, UseTimeSeconds: 2, ContentRedacted: "safe"}
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion, now-3600, now, now,
		[]UpstreamLogFact{fact, fact}, dto.LogCollectionComplete, "", nil); err != nil {
		t.Fatalf("commit log window: %v", err)
	}
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion, now, now+3600, now+1,
		nil, dto.LogCollectionUnavailable, "UPSTREAM_UNAVAILABLE", nil); err != nil {
		t.Fatalf("commit failed log window: %v", err)
	}
	failedState, err := repository.LoadState(context.Background(), site.ID)
	if err != nil || failedState.WindowStart != now-3600 || failedState.WindowEnd != now ||
		failedState.Status != dto.LogCollectionUnavailable || failedState.LastErrorCode != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf("failed log state=%#v err=%v", failedState, err)
	}
	firstResponseTimeMs := int64(450)
	fact.FirstResponseTimeMs = &firstResponseTimeMs
	fact.StreamStatus = "ok"
	fact.StreamEndReason = "done"
	fact.CacheReadTokens = 11
	fact.CacheCreationTokens = 12
	fact.CacheCreation5m = 13
	fact.CacheCreation1h = 14
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion, now-3600, now, now,
		[]UpstreamLogFact{fact}, dto.LogCollectionComplete, "", nil); err != nil {
		t.Fatalf("enrich overlapping log window: %v", err)
	}
	historicalStart := now - 25*3600
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion, historicalStart, now-3600, now+1,
		nil, dto.LogCollectionComplete, "", nil); err != nil {
		t.Fatalf("commit historical log window: %v", err)
	}
	state, err := repository.LoadState(context.Background(), site.ID)
	if err != nil || state.WindowStart != now-3600 || state.WindowEnd != now || state.HistoryStartAt == nil ||
		*state.HistoryStartAt != historicalStart || state.LastSuccessAt == nil || *state.LastSuccessAt != now {
		t.Fatalf("historical log state=%#v err=%v", state, err)
	}
	query := dto.LogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}, StartTimestamp: now - 3600, EndTimestamp: now}
	rows, total, err := repository.Query(context.Background(), query)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].SiteName != site.Name || rows[0].IP != "" ||
		rows[0].FirstResponseTimeMs == nil || *rows[0].FirstResponseTimeMs != firstResponseTimeMs || rows[0].StreamStatus != "ok" ||
		rows[0].CacheReadTokens != 11 || rows[0].CacheCreationTokens != 12 || rows[0].CacheCreation5m != 13 || rows[0].CacheCreation1h != 14 {
		t.Fatalf("query log facts = %+v total=%d err=%v", rows, total, err)
	}
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion+1, now-3600, now, now, nil,
		dto.LogCollectionComplete, "", nil); !errors.Is(err, ErrUpstreamLogFence) {
		t.Fatalf("stale config fence error = %v", err)
	}
	deleted, err := repository.DeleteBefore(context.Background(), now, 100)
	if err != nil || deleted != 1 {
		t.Fatalf("delete retained logs = %d, %v", deleted, err)
	}
}

func TestUpstreamLogRepositoryStatsFollowFiltersAndRealtimeWindow(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_100_000_000)
	site := createRunnableSite(t, database, fmt.Sprintf("upstream-log-stat-%d", time.Now().UnixNano()), now)
	if err := database.GORM.Model(&Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"quota_per_unit": "500000", "usd_exchange_rate": "7.2", "last_rate_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewUpstreamLogRepository(database.GORM)
	facts := []UpstreamLogFact{
		{UpstreamLogKey: strings.Repeat("a", 64), CreatedAt: now - 30, Type: 2, Username: "alice", ModelName: "gpt", ChannelID: 9, UseGroup: "vip", Quota: 500000, PromptTokens: 100, CompletionTokens: 20},
		{UpstreamLogKey: strings.Repeat("b", 64), CreatedAt: now - 90, Type: 2, Username: "alice", ModelName: "gpt", ChannelID: 9, UseGroup: "vip", Quota: 250000, PromptTokens: 50, CompletionTokens: 10},
		{UpstreamLogKey: strings.Repeat("c", 64), CreatedAt: now - 20, Type: 2, Username: "bob", ModelName: "gpt", ChannelID: 9, UseGroup: "vip", Quota: 999, PromptTokens: 1, CompletionTokens: 1},
	}
	if err := repository.CommitWindow(context.Background(), site.ID, site.ConfigVersion, now-3600, now+1, now, facts, dto.LogCollectionComplete, "", nil); err != nil {
		t.Fatal(err)
	}
	query := dto.LogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}, StartTimestamp: now - 3600, EndTimestamp: now + 1, Username: "alice", ModelName: "gpt", ChannelID: int64Pointer(9), UseGroup: "vip"}
	rows, rpm, tpm, err := repository.Stats(context.Background(), query, now)
	if err != nil || len(rows) != 1 || rows[0].Quota != 750000 || rpm != 1 || tpm != 120 || rows[0].QuotaPerUnit == nil || *rows[0].QuotaPerUnit != "500000.0000000000" {
		t.Fatalf("stats rows=%+v rpm=%d tpm=%d err=%v", rows, rpm, tpm, err)
	}
	otherType := 5
	query.Type = &otherType
	rows, rpm, tpm, err = repository.Stats(context.Background(), query, now)
	if err != nil || len(rows) != 0 || rpm != 0 || tpm != 0 {
		t.Fatalf("non-consume stats rows=%+v rpm=%d tpm=%d err=%v", rows, rpm, tpm, err)
	}
}
