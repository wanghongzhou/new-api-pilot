package service

import (
	"context"
	"sync"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const (
	sitePerformanceCacheTTL        = time.Hour
	sitePerformanceFailureCacheTTL = 5 * time.Minute
)

type sitePerformanceCacheEntry struct {
	configVersion int
	summary       dto.SitePerformanceSummary
	expiresAt     int64
}

type sitePerformanceCache struct {
	mu         sync.Mutex
	entries    map[int64]sitePerformanceCacheEntry
	refreshing map[int64]bool
}

func newSitePerformanceCache() *sitePerformanceCache {
	return &sitePerformanceCache{entries: map[int64]sitePerformanceCacheEntry{}, refreshing: map[int64]bool{}}
}

func (cache *sitePerformanceCache) Get(siteID int64, configVersion int, now int64) (dto.SitePerformanceSummary, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, exists := cache.entries[siteID]
	if !exists || entry.configVersion != configVersion || entry.expiresAt <= now {
		return dto.SitePerformanceSummary{}, false
	}
	return entry.summary, true
}

func (cache *sitePerformanceCache) StartRefresh(siteID int64) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.refreshing[siteID] {
		return false
	}
	cache.refreshing[siteID] = true
	return true
}

func (cache *sitePerformanceCache) Store(siteID int64, configVersion int, summary dto.SitePerformanceSummary, now int64) {
	cache.store(siteID, configVersion, summary, now+int64(sitePerformanceCacheTTL/time.Second))
}

func (cache *sitePerformanceCache) StoreFailure(siteID int64, configVersion int, now int64) {
	cache.store(siteID, configVersion, unavailableSitePerformanceSummary(24), now+int64(sitePerformanceFailureCacheTTL/time.Second))
}

func (cache *sitePerformanceCache) store(siteID int64, configVersion int, summary dto.SitePerformanceSummary, expiresAt int64) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.entries[siteID] = sitePerformanceCacheEntry{configVersion: configVersion, summary: summary, expiresAt: expiresAt}
	delete(cache.refreshing, siteID)
}

func (service *SiteService) listPerformanceSummaries(sites []model.Site, now int64) map[int64]dto.SitePerformanceSummary {
	results := make(map[int64]dto.SitePerformanceSummary, len(sites))
	for _, site := range sites {
		if summary, ok := service.performanceCache.Get(site.ID, site.ConfigVersion, now); ok {
			results[site.ID] = summary
			continue
		}
		results[site.ID] = unavailableSitePerformanceSummary(24)
		if site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized ||
			site.RootUserID == nil || site.AccessTokenEncrypted == nil || !service.performanceCache.StartRefresh(site.ID) {
			continue
		}
		go service.refreshListPerformance(site.ID, site.ConfigVersion)
	}
	return results
}

func (service *SiteService) refreshListPerformance(siteID int64, configVersion int) {
	service.performanceRefreshes <- struct{}{}
	defer func() { <-service.performanceRefreshes }()
	ctx, cancel := context.WithTimeout(context.Background(), sitePerformanceCacheTTL)
	defer cancel()
	_, err := service.PerformanceSummary(ctx, siteID, 24, performanceCacheRequestID(siteID))
	if err != nil {
		service.performanceCache.StoreFailure(siteID, configVersion, service.clock.Now().Unix())
	}
}
