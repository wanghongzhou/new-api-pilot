package service

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"new-api-pilot/dto"
)

const (
	statisticsReadCacheTTL        = 30 * time.Second
	statisticsReadCacheMaxEntries = 128
	statisticsReadCacheMinRange   = 7 * 24 * time.Hour
)

type statisticsReadCacheEntry struct {
	response  dto.StatisticsResponse
	expiresAt time.Time
}

type statisticsReadFlight struct {
	done     chan struct{}
	response dto.StatisticsResponse
	err      error
}

type statisticsReadCache struct {
	mu      sync.Mutex
	entries map[string]statisticsReadCacheEntry
	flights map[string]*statisticsReadFlight
	now     func() time.Time
}

func newStatisticsReadCache() *statisticsReadCache {
	return &statisticsReadCache{
		entries: make(map[string]statisticsReadCacheEntry),
		flights: make(map[string]*statisticsReadFlight),
		now:     time.Now,
	}
}

func statisticsReadCacheKey(scope string, query dto.StatisticsQuery) (string, bool) {
	if scope != dto.StatisticsScopeGlobal || query.Granularity != dto.StatisticsGranularityHour ||
		query.EndTimestamp-query.StartTimestamp < int64(statisticsReadCacheMinRange/time.Second) {
		return "", false
	}
	query = normalizeStatisticsCacheQuery(query)
	payload, err := json.Marshal(struct {
		Scope string              `json:"scope"`
		Query dto.StatisticsQuery `json:"query"`
	}{Scope: scope, Query: query})
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func statisticsDashboardCacheKey(scope string, query dto.StatisticsQuery) (string, bool) {
	query = normalizeStatisticsCacheQuery(query)
	payload, err := json.Marshal(struct {
		Projection string              `json:"projection"`
		Scope      string              `json:"scope"`
		Query      dto.StatisticsQuery `json:"query"`
	}{Projection: "dashboard_metrics", Scope: scope, Query: query})
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func normalizeStatisticsCacheQuery(query dto.StatisticsQuery) dto.StatisticsQuery {
	query.Normalize()
	query.SiteIDs = append([]int64(nil), query.SiteIDs...)
	query.CustomerIDs = append([]int64(nil), query.CustomerIDs...)
	query.AccountIDs = append([]int64(nil), query.AccountIDs...)
	query.ModelNames = append([]string(nil), query.ModelNames...)
	query.ChannelKeys = append([]string(nil), query.ChannelKeys...)
	query.UseGroups = append([]string(nil), query.UseGroups...)
	query.TokenKeys = append([]string(nil), query.TokenKeys...)
	query.NodeNames = append([]string(nil), query.NodeNames...)
	sort.Slice(query.SiteIDs, func(i, j int) bool { return query.SiteIDs[i] < query.SiteIDs[j] })
	sort.Slice(query.CustomerIDs, func(i, j int) bool { return query.CustomerIDs[i] < query.CustomerIDs[j] })
	sort.Slice(query.AccountIDs, func(i, j int) bool { return query.AccountIDs[i] < query.AccountIDs[j] })
	sort.Strings(query.ModelNames)
	sort.Strings(query.ChannelKeys)
	sort.Strings(query.UseGroups)
	sort.Strings(query.TokenKeys)
	sort.Strings(query.NodeNames)
	return query
}

func (cache *statisticsReadCache) load(
	ctx context.Context,
	key string,
	loader func() (dto.StatisticsResponse, error),
) (dto.StatisticsResponse, error) {
	if cache == nil {
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
		return entry.response, nil
	}
	if flight, ok := cache.flights[key]; ok {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return dto.StatisticsResponse{}, ctx.Err()
		case <-flight.done:
			return flight.response, flight.err
		}
	}
	flight := &statisticsReadFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	cache.mu.Unlock()

	flight.response, flight.err = loader()
	cache.mu.Lock()
	if flight.err == nil {
		if len(cache.entries) >= statisticsReadCacheMaxEntries {
			var oldestKey string
			var oldestExpiry time.Time
			for cachedKey, entry := range cache.entries {
				if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
					oldestKey, oldestExpiry = cachedKey, entry.expiresAt
				}
			}
			delete(cache.entries, oldestKey)
		}
		cache.entries[key] = statisticsReadCacheEntry{
			response: flight.response, expiresAt: cache.now().Add(statisticsReadCacheTTL),
		}
	}
	delete(cache.flights, key)
	close(flight.done)
	cache.mu.Unlock()
	return flight.response, flight.err
}
