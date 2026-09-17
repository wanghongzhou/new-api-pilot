package model

import (
	"context"
	"strings"
)

type SiteUsageOverview struct {
	SiteID          int64  `gorm:"column:site_id"`
	RequestCount    string `gorm:"column:request_count"`
	Quota           string `gorm:"column:quota"`
	TokenUsed       string `gorm:"column:token_used"`
	ActiveUsers     int64  `gorm:"column:active_users"`
	AvgRPM          string `gorm:"column:avg_rpm"`
	AvgTPM          string `gorm:"column:avg_tpm"`
	AsOf            *int64 `gorm:"column:as_of"`
	CompleteWindows int64  `gorm:"column:complete_windows"`
}

const siteUsageOverviewQuery = `SELECT w.site_id,
  CAST(COALESCE(SUM(s.request_count), 0) AS CHAR) AS request_count,
  CAST(COALESCE(SUM(s.quota), 0) AS CHAR) AS quota,
  CAST(COALESCE(SUM(s.token_used), 0) AS CHAR) AS token_used,
  COALESCE(a.active_users, 0) AS active_users,
  CAST(COALESCE(SUM(s.request_count) / NULLIF(?, 0), 0) AS CHAR) AS avg_rpm,
  CAST(COALESCE(SUM(s.token_used) / NULLIF(?, 0), 0) AS CHAR) AS avg_tpm,
  MAX(s.last_calculated_at) AS as_of,
  COUNT(DISTINCT w.hour_ts) AS complete_windows
FROM collection_window AS w
LEFT JOIN site_stat_hourly AS s
  ON s.site_id = w.site_id AND s.hour_ts = w.hour_ts
LEFT JOIN (
  SELECT f.site_id, COUNT(DISTINCT f.remote_user_id) AS active_users
  FROM usage_fact_hourly AS f FORCE INDEX (idx_usage_fact_hourly_site_time)
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  WHERE f.site_id IN ? AND f.hour_ts >= ? AND f.hour_ts < ?
  GROUP BY f.site_id
) AS a ON a.site_id = w.site_id
WHERE w.site_id IN ? AND w.hour_ts >= ? AND w.hour_ts < ? AND w.status = 'complete'
GROUP BY w.site_id, a.active_users`

func (repository *SiteRepository) ListCollectionWindowCompleteness(
	ctx context.Context,
	sites []Site,
	now int64,
) (map[int64]float64, error) {
	result := make(map[int64]float64, len(sites))
	if len(sites) == 0 {
		return result, nil
	}
	type completenessScope struct {
		siteID   int64
		start    int64
		end      int64
		expected int64
	}
	currentHour := now - now%3600
	scopes := make([]completenessScope, 0, len(sites))
	for _, site := range sites {
		result[site.ID] = 0
		if site.StatisticsStartAt == nil || *site.StatisticsStartAt <= 0 || *site.StatisticsStartAt%3600 != 0 {
			continue
		}
		end := currentHour
		if site.StatisticsEndAt != nil && *site.StatisticsEndAt < end {
			end = *site.StatisticsEndAt
		}
		end -= end % 3600
		if end <= *site.StatisticsStartAt {
			continue
		}
		expected := (end - *site.StatisticsStartAt) / 3600
		if expected <= 0 {
			continue
		}
		scopes = append(scopes, completenessScope{
			siteID: site.ID, start: *site.StatisticsStartAt, end: end, expected: expected,
		})
	}
	if len(scopes) == 0 {
		return result, nil
	}
	var rows []struct {
		SiteID   int64 `gorm:"column:site_id"`
		Complete int64 `gorm:"column:complete_windows"`
	}
	conditions := make([]string, 0, len(scopes))
	args := make([]any, 0, len(scopes)*3)
	for _, scope := range scopes {
		conditions = append(conditions, "(site_id = ? AND hour_ts >= ? AND hour_ts < ?)")
		args = append(args, scope.siteID, scope.start, scope.end)
	}
	err := repository.db.WithContext(ctx).Table("collection_window").
		Select("site_id, COUNT(*) AS complete_windows").
		Where("status = ?", CollectionWindowStatusComplete).
		Where(strings.Join(conditions, " OR "), args...).
		Group("site_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	completeBySite := make(map[int64]int64, len(rows))
	for _, row := range rows {
		completeBySite[row.SiteID] = row.Complete
	}
	for _, scope := range scopes {
		complete := completeBySite[scope.siteID]
		if complete < 0 {
			complete = 0
		}
		if complete > scope.expected {
			complete = scope.expected
		}
		result[scope.siteID] = float64(complete) / float64(scope.expected)
	}
	return result, nil
}

func (repository *SiteRepository) ListUsageOverviews(
	ctx context.Context,
	siteIDs []int64,
	startTimestamp, endTimestamp int64,
) (map[int64]SiteUsageOverview, error) {
	result := make(map[int64]SiteUsageOverview, len(siteIDs))
	if len(siteIDs) == 0 {
		return result, nil
	}
	rangeMinutes := (endTimestamp - startTimestamp) / 60
	if rangeMinutes < 1 {
		return result, nil
	}
	var rows []SiteUsageOverview
	err := repository.db.WithContext(ctx).Raw(
		siteUsageOverviewQuery,
		rangeMinutes, rangeMinutes, siteIDs, startTimestamp, endTimestamp, siteIDs, startTimestamp, endTimestamp,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.SiteID] = row
	}
	return result, nil
}
