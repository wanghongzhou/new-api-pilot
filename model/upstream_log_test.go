package model

import (
	"context"
	"errors"
	"fmt"
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
	query := dto.LogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}, StartTimestamp: now - 3600, EndTimestamp: now}
	rows, total, err := repository.Query(context.Background(), query)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].SiteName != site.Name || rows[0].IP != "" {
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
