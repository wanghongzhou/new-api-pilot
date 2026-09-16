package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func TestStatisticsReadCacheCoalescesLongGlobalHourlyReadsAndExpires(t *testing.T) {
	query := dto.StatisticsQuery{
		StartTimestamp: 1_750_000_000,
		EndTimestamp:   1_750_000_000 + int64(8*24*time.Hour/time.Second),
		Granularity:    dto.StatisticsGranularityHour,
		SiteIDs:        []int64{2, 1, 2},
	}
	query.Normalize()
	key, ok := statisticsReadCacheKey(dto.StatisticsScopeGlobal, query)
	if !ok || key == "" {
		t.Fatal("long global hourly query was not cache eligible")
	}
	cache := newStatisticsReadCache()
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }
	var loads atomic.Int64
	release := make(chan struct{})
	loader := func() (dto.StatisticsResponse, error) {
		loads.Add(1)
		<-release
		return dto.StatisticsResponse{}, nil
	}

	const readers = 20
	var wait sync.WaitGroup
	wait.Add(readers)
	errorsSeen := make(chan error, readers)
	for range readers {
		go func() {
			defer wait.Done()
			_, err := cache.load(context.Background(), key, loader)
			errorsSeen <- err
		}()
	}
	for loads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("coalesced read failed: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("coalesced loader calls = %d, want 1", loads.Load())
	}
	if _, err := cache.load(context.Background(), key, loader); err != nil || loads.Load() != 1 {
		t.Fatalf("cached read err=%v loads=%d", err, loads.Load())
	}

	now = now.Add(statisticsReadCacheTTL + time.Second)
	release = make(chan struct{})
	close(release)
	if _, err := cache.load(context.Background(), key, loader); err != nil || loads.Load() != 2 {
		t.Fatalf("expired read err=%v loads=%d", err, loads.Load())
	}
}

func TestStatisticsReadCacheRejectsShortOrNonGlobalReadsAndDoesNotCacheErrors(t *testing.T) {
	short := dto.StatisticsQuery{
		StartTimestamp: 1_750_000_000, EndTimestamp: 1_750_000_000 + 3600,
		Granularity: dto.StatisticsGranularityHour,
	}
	if _, ok := statisticsReadCacheKey(dto.StatisticsScopeGlobal, short); ok {
		t.Fatal("short global hourly query became cache eligible")
	}
	long := short
	long.EndTimestamp = long.StartTimestamp + int64(8*24*time.Hour/time.Second)
	if _, ok := statisticsReadCacheKey(dto.StatisticsScopeSite, long); ok {
		t.Fatal("site query became cache eligible")
	}

	key, ok := statisticsReadCacheKey(dto.StatisticsScopeGlobal, long)
	if !ok {
		t.Fatal("long global query is not cache eligible")
	}
	cache := newStatisticsReadCache()
	want := errors.New("read failed")
	var loads atomic.Int64
	loader := func() (dto.StatisticsResponse, error) {
		loads.Add(1)
		return dto.StatisticsResponse{}, want
	}
	for range 2 {
		if _, err := cache.load(context.Background(), key, loader); !errors.Is(err, want) {
			t.Fatalf("cached error = %v, want %v", err, want)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("failed loader calls = %d, want 2", loads.Load())
	}
}

func TestStatisticsDashboardCacheKeyCoversProjectionScopeAndNormalizedQuery(t *testing.T) {
	query := dto.StatisticsQuery{
		StartTimestamp: 1_750_000_000, EndTimestamp: 1_750_086_400,
		Granularity: dto.StatisticsGranularityDay,
		SiteIDs:     []int64{2, 1, 2}, CustomerIDs: []int64{4, 3}, AccountIDs: []int64{6, 5},
		ModelNames: []string{"z", "a"}, ChannelKeys: []string{"2:2", "1:1"},
		UseGroups: []string{"vip", "default"}, TokenKeys: []string{"2:20", "1:10"},
		NodeNames: []string{"node-z", "node-a"},
	}
	global, ok := statisticsDashboardCacheKey(dto.StatisticsScopeGlobal, query)
	if !ok || global == "" {
		t.Fatal("dashboard cache key was not generated")
	}
	site, ok := statisticsDashboardCacheKey(dto.StatisticsScopeSite, query)
	if !ok || site == global {
		t.Fatal("dashboard cache key did not include scope")
	}
	ordinary, ok := statisticsReadCacheKey(dto.StatisticsScopeGlobal, query)
	if ok || ordinary != "" {
		t.Fatal("dashboard daily projection collided with the ordinary long-hour cache")
	}
	reordered := query
	reordered.SiteIDs = []int64{1, 2}
	reordered.CustomerIDs = []int64{3, 4}
	reordered.AccountIDs = []int64{5, 6}
	reordered.ModelNames = []string{"a", "z"}
	reordered.ChannelKeys = []string{"1:1", "2:2"}
	reordered.UseGroups = []string{"default", "vip"}
	reordered.TokenKeys = []string{"1:10", "2:20"}
	reordered.NodeNames = []string{"node-a", "node-z"}
	if other, ok := statisticsDashboardCacheKey(dto.StatisticsScopeGlobal, reordered); !ok || other != global {
		t.Fatal("equivalent site filter order produced a different dashboard key")
	}
	changed := query
	changed.EndTimestamp++
	if other, ok := statisticsDashboardCacheKey(dto.StatisticsScopeGlobal, changed); !ok || other == global {
		t.Fatal("changed query produced the same dashboard key")
	}
}
