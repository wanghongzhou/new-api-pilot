package model

import (
	"context"
)

type SiteUsageOverview struct {
	SiteID       int64  `gorm:"column:site_id"`
	RequestCount string `gorm:"column:request_count"`
	Quota        string `gorm:"column:quota"`
	TokenUsed    string `gorm:"column:token_used"`
	ActiveUsers  int64  `gorm:"column:active_users"`
	AvgRPM       string `gorm:"column:avg_rpm"`
	AvgTPM       string `gorm:"column:avg_tpm"`
	AsOf         *int64 `gorm:"column:as_of"`
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
	err := repository.db.WithContext(ctx).Raw(`SELECT w.site_id,
  CAST(COALESCE(SUM(s.request_count), 0) AS CHAR) AS request_count,
  CAST(COALESCE(SUM(s.quota), 0) AS CHAR) AS quota,
  CAST(COALESCE(SUM(s.token_used), 0) AS CHAR) AS token_used,
  COALESCE(a.active_users, 0) AS active_users,
  CAST(COALESCE(SUM(s.request_count) / NULLIF(?, 0), 0) AS CHAR) AS avg_rpm,
  CAST(COALESCE(SUM(s.token_used) / NULLIF(?, 0), 0) AS CHAR) AS avg_tpm,
  MAX(s.last_calculated_at) AS as_of
FROM collection_window AS w
LEFT JOIN site_stat_hourly AS s
  ON s.site_id = w.site_id AND s.hour_ts = w.hour_ts
LEFT JOIN (
  SELECT f.site_id, COUNT(DISTINCT f.remote_user_id) AS active_users
  FROM usage_fact_hourly AS f
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  WHERE f.site_id IN ? AND f.hour_ts >= ? AND f.hour_ts < ?
  GROUP BY f.site_id
) AS a ON a.site_id = w.site_id
WHERE w.site_id IN ? AND w.hour_ts >= ? AND w.hour_ts < ? AND w.status = 'complete'
GROUP BY w.site_id, a.active_users`, rangeMinutes, rangeMinutes, siteIDs, startTimestamp, endTimestamp, siteIDs, startTimestamp, endTimestamp).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.SiteID] = row
	}
	return result, nil
}
