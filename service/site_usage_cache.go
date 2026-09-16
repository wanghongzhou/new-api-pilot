package service

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"new-api-pilot/model"
)

const (
	siteUsageOverviewCacheTTL        = 30 * time.Second
	siteUsageOverviewCacheMaxEntries = 128
)

type siteUsageOverviewCacheEntry struct {
	value     map[int64]model.SiteUsageOverview
	expiresAt time.Time
}

type siteUsageOverviewFlight struct {
	done  chan struct{}
	value map[int64]model.SiteUsageOverview
	err   error
}

type siteUsageOverviewCache struct {
	mu      sync.Mutex
	entries map[string]siteUsageOverviewCacheEntry
	flights map[string]*siteUsageOverviewFlight
	now     func() time.Time
}

func newSiteUsageOverviewCache() *siteUsageOverviewCache {
	return &siteUsageOverviewCache{
		entries: make(map[string]siteUsageOverviewCacheEntry),
		flights: make(map[string]*siteUsageOverviewFlight),
		now:     time.Now,
	}
}

func siteUsageOverviewCacheKey(siteIDs []int64, startTimestamp, endTimestamp int64) (string, bool) {
	if len(siteIDs) == 0 || startTimestamp <= 0 || endTimestamp <= startTimestamp {
		return "", false
	}
	normalized := append([]int64(nil), siteIDs...)
	sort.Slice(normalized, func(left, right int) bool { return normalized[left] < normalized[right] })
	deduplicated := normalized[:0]
	for _, siteID := range normalized {
		if siteID <= 0 || len(deduplicated) > 0 && deduplicated[len(deduplicated)-1] == siteID {
			continue
		}
		deduplicated = append(deduplicated, siteID)
	}
	if len(deduplicated) == 0 {
		return "", false
	}
	payload, err := json.Marshal(struct {
		SiteIDs []int64 `json:"site_ids"`
		Start   int64   `json:"start"`
		End     int64   `json:"end"`
	}{SiteIDs: deduplicated, Start: startTimestamp, End: endTimestamp})
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func (cache *siteUsageOverviewCache) load(
	ctx context.Context,
	key string,
	loader func() (map[int64]model.SiteUsageOverview, error),
) (map[int64]model.SiteUsageOverview, error) {
	if cache == nil || key == "" {
		return loader()
	}
	now := cache.now()
	cache.mu.Lock()
	for cachedKey, entry := range cache.entries {
		if !entry.expiresAt.After(now) {
			delete(cache.entries, cachedKey)
		}
	}
	if entry, ok := cache.entries[key]; ok {
		cache.mu.Unlock()
		return cloneSiteUsageOverviews(entry.value), nil
	}
	if flight, ok := cache.flights[key]; ok {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return cloneSiteUsageOverviews(flight.value), flight.err
		}
	}
	flight := &siteUsageOverviewFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	cache.mu.Unlock()

	flight.value, flight.err = loader()
	cache.mu.Lock()
	if flight.err == nil {
		if len(cache.entries) >= siteUsageOverviewCacheMaxEntries {
			var oldestKey string
			var oldestExpiry time.Time
			for cachedKey, entry := range cache.entries {
				if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
					oldestKey, oldestExpiry = cachedKey, entry.expiresAt
				}
			}
			delete(cache.entries, oldestKey)
		}
		flight.value = cloneSiteUsageOverviews(flight.value)
		cache.entries[key] = siteUsageOverviewCacheEntry{
			value: cloneSiteUsageOverviews(flight.value), expiresAt: cache.now().Add(siteUsageOverviewCacheTTL),
		}
	}
	delete(cache.flights, key)
	close(flight.done)
	cache.mu.Unlock()
	return cloneSiteUsageOverviews(flight.value), flight.err
}

func cloneSiteUsageOverviews(source map[int64]model.SiteUsageOverview) map[int64]model.SiteUsageOverview {
	if source == nil {
		return nil
	}
	result := make(map[int64]model.SiteUsageOverview, len(source))
	for siteID, overview := range source {
		result[siteID] = overview
	}
	return result
}
