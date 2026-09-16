package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/model"
)

func TestSiteUsageOverviewCacheNormalizesCoalescesAndExpires(t *testing.T) {
	left, ok := siteUsageOverviewCacheKey([]int64{2, 1, 2}, 100, 200)
	if !ok {
		t.Fatal("site usage cache key was not generated")
	}
	right, ok := siteUsageOverviewCacheKey([]int64{1, 2}, 100, 200)
	if !ok || left != right {
		t.Fatal("site usage cache key was not normalized")
	}
	cache := newSiteUsageOverviewCache()
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }
	var loads atomic.Int64
	release := make(chan struct{})
	loader := func() (map[int64]model.SiteUsageOverview, error) {
		loads.Add(1)
		<-release
		return map[int64]model.SiteUsageOverview{1: {SiteID: 1, RequestCount: "7"}}, nil
	}
	const readers = 20
	var wait sync.WaitGroup
	wait.Add(readers)
	results := make(chan map[int64]model.SiteUsageOverview, readers)
	for range readers {
		go func() {
			defer wait.Done()
			value, err := cache.load(context.Background(), left, loader)
			if err != nil {
				t.Errorf("load cached site usage: %v", err)
			}
			results <- value
		}()
	}
	for loads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wait.Wait()
	close(results)
	if loads.Load() != 1 {
		t.Fatalf("coalesced site usage loader calls = %d, want 1", loads.Load())
	}
	for result := range results {
		result[1] = model.SiteUsageOverview{SiteID: 1, RequestCount: "mutated"}
	}
	value, err := cache.load(context.Background(), right, loader)
	if err != nil || value[1].RequestCount != "7" || loads.Load() != 1 {
		t.Fatalf("cached site usage value=%#v err=%v loads=%d", value, err, loads.Load())
	}
	now = now.Add(siteUsageOverviewCacheTTL + time.Second)
	release = make(chan struct{})
	close(release)
	if _, err := cache.load(context.Background(), left, loader); err != nil || loads.Load() != 2 {
		t.Fatalf("expired site usage err=%v loads=%d", err, loads.Load())
	}
}

func TestSiteUsageOverviewCacheDoesNotCacheErrors(t *testing.T) {
	key, ok := siteUsageOverviewCacheKey([]int64{1}, 100, 200)
	if !ok {
		t.Fatal("site usage cache key was not generated")
	}
	cache := newSiteUsageOverviewCache()
	want := errors.New("read failed")
	var loads atomic.Int64
	for range 2 {
		_, err := cache.load(context.Background(), key, func() (map[int64]model.SiteUsageOverview, error) {
			loads.Add(1)
			return nil, want
		})
		if !errors.Is(err, want) {
			t.Fatalf("cached site usage error = %v, want %v", err, want)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("failed site usage loader calls = %d, want 2", loads.Load())
	}
}
