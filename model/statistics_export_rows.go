package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const StatisticsExportMaximumPageSize = 5000

// Bound the candidate CTE before execution. This is essential for metric sorts,
// which must rank the complete result set, while still permitting the documented
// one-million-bucket, single-dimension export.
const StatisticsExportMaximumCandidateRows int64 = 1_000_000

var ErrStatisticsExportCapacity = errors.New("statistics export candidate set exceeds capacity")

type StatisticsExportRowQuery struct {
	Request           StatisticsReadRequest
	SortBy            string
	SortOrder         string
	Now               int64
	UsageDelayMinutes int
	Limit             int
	Cursor            StatisticsExportRowCursor
}

type StatisticsExportRowCursor struct {
	Initialized     bool
	EntityID        int64
	DimensionSiteID int64
	DimensionValue  string
	DimensionKey    string
	BucketStart     int64
	SortKnown       int
	SortNumber      int64
	SortText        string
	BreakdownSiteID int64
}

type StatisticsExportRow struct {
	EntityID        int64  `gorm:"column:entity_id"`
	DimensionSiteID int64  `gorm:"column:dimension_site_id"`
	DimensionValue  string `gorm:"column:dimension_value"`
	DimensionKey    string `gorm:"column:dimension_key"`
	DimensionID     string `gorm:"column:dimension_id"`
	DimensionName   string `gorm:"column:dimension_name"`

	BreakdownSiteID   int64  `gorm:"column:breakdown_site_id"`
	BreakdownSiteName string `gorm:"column:breakdown_site_name"`
	BucketStart       int64  `gorm:"column:bucket_start"`
	BucketEnd         int64  `gorm:"column:bucket_end"`

	RequestCount *string `gorm:"column:request_count"`
	Quota        *string `gorm:"column:quota"`
	TokenUsed    *string `gorm:"column:token_used"`
	ActiveUsers  *string `gorm:"column:active_users"`
	SiteQuota    *string `gorm:"column:site_quota"`
	DataStatus   string  `gorm:"column:data_status"`
	IsFinal      bool    `gorm:"column:is_final"`
	AsOf         *int64  `gorm:"column:as_of"`

	SortKnown  int    `gorm:"column:sort_known"`
	SortNumber int64  `gorm:"column:sort_number"`
	SortText   string `gorm:"column:sort_text"`
}

func (row StatisticsExportRow) Cursor() StatisticsExportRowCursor {
	return StatisticsExportRowCursor{
		Initialized: true, EntityID: row.EntityID, DimensionSiteID: row.DimensionSiteID,
		DimensionValue: row.DimensionValue, DimensionKey: row.DimensionKey, BucketStart: row.BucketStart,
		SortKnown: row.SortKnown, SortNumber: row.SortNumber, SortText: row.SortText,
		BreakdownSiteID: row.BreakdownSiteID,
	}
}

func (repository *StatisticsRepository) LoadExportRows(
	ctx context.Context,
	query StatisticsExportRowQuery,
) ([]StatisticsExportRow, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrStatisticsReadContract
	}
	if err := repository.validateStatisticsExportCapacity(ctx, query.Request); err != nil {
		return nil, err
	}
	statement, args, err := statisticsExportRowsStatement(query)
	if err != nil {
		return nil, err
	}
	var rows []StatisticsExportRow
	if err := repository.db.WithContext(ctx).Raw(statement, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load final statistics export rows: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) validateStatisticsExportCapacity(
	ctx context.Context,
	request StatisticsReadRequest,
) error {
	bucketCount, err := statisticsExportBucketCount(request)
	if err != nil || bucketCount < 1 {
		return ErrStatisticsReadContract
	}
	spaceSQL, args, err := statisticsExportRowSpaceSQL(request)
	if err != nil {
		return err
	}
	var rowCount int64
	if err := repository.db.WithContext(ctx).Raw(
		"WITH "+spaceSQL+" SELECT COUNT(*) FROM row_space", args...,
	).Scan(&rowCount).Error; err != nil {
		return fmt.Errorf("count statistics export candidates: %w", err)
	}
	candidateRows, valid := checkedStatisticsExportCandidateRows(rowCount, bucketCount)
	if !valid || candidateRows > StatisticsExportMaximumCandidateRows {
		return ErrStatisticsExportCapacity
	}
	return nil
}

func checkedStatisticsExportCandidateRows(rowCount, bucketCount int64) (int64, bool) {
	const maximumInt64 = int64(^uint64(0) >> 1)
	if rowCount < 0 || bucketCount < 0 || rowCount > 0 && bucketCount > maximumInt64/rowCount {
		return 0, false
	}
	return rowCount * bucketCount, true
}

func statisticsExportRowsStatement(query StatisticsExportRowQuery) (string, []any, error) {
	if query.Now <= 0 || query.Limit < 1 ||
		query.Limit > StatisticsExportMaximumPageSize ||
		(query.SortOrder != "asc" && query.SortOrder != "desc") {
		return "", nil, ErrStatisticsReadContract
	}
	bucketCount, err := statisticsExportBucketCount(query.Request)
	if err != nil || bucketCount < 1 || bucketCount > 1_000_000 {
		return "", nil, ErrStatisticsReadContract
	}
	bucketSQL, bucketArgs := statisticsExportBucketsSQL(query.Request, bucketCount)
	spaceSQL, spaceArgs, err := statisticsExportRowSpaceSQL(query.Request)
	if err != nil {
		return "", nil, err
	}
	metricTable, metricTimeColumn, metricJoin, err := statisticsExportMetricSQL(query.Request)
	if err != nil {
		return "", nil, err
	}
	activeExpression, err := statisticsExportActiveExpression(query.Request)
	if err != nil {
		return "", nil, err
	}
	entityActivitySQL := statisticsExportEntityActivitySQL(query.Request.Scope)
	windowEntityActive := "TRUE"
	runEntityActive := "TRUE"
	if query.Request.Scope == "customer" {
		windowEntityActive = statisticsExportCustomerEntityActive("cw.hour_ts")
		runEntityActive = statisticsExportCustomerEntityActive("rw.hour_ts")
	}
	orderTerms, err := statisticsExportOrderTerms(query.Request.Scope, query.SortBy, query.SortOrder, query.Cursor)
	if err != nil {
		return "", nil, err
	}
	cursorWhere, cursorArgs := statisticsExportAfter(orderTerms, query.Cursor.Initialized)
	orderBy := make([]string, 0, len(orderTerms))
	for _, term := range orderTerms {
		orderBy = append(orderBy, term.Expression+" "+term.Direction)
	}
	selectedItemsSQL, selectedArgs, err := statisticsExportSelectedItemsSQL(
		query.Request.Scope, query.SortBy, query.SortOrder, query.Cursor, query.Limit,
	)
	if err != nil {
		return "", nil, err
	}

	metricSort := "0"
	switch query.SortBy {
	case "request_count":
		metricSort = "total_request_count"
	case "quota":
		metricSort = "total_quota"
	case "token_used":
		metricSort = "total_token_used"
	case "active_users":
		metricSort = "total_active_users"
	case "bucket_start", "name":
	default:
		return "", nil, ErrStatisticsReadContract
	}
	sortKnown := "1"
	if query.SortBy == "request_count" || query.SortBy == "quota" ||
		query.SortBy == "token_used" || query.SortBy == "active_users" {
		sortKnown = "total_complete_count > 0"
	}
	sortNumber := "0"
	if query.SortBy == "bucket_start" {
		sortNumber = "bucket_start"
	} else if metricSort != "0" {
		sortNumber = metricSort
	}
	sortText := "dimension_name"
	activeUsersVisible := "w.total_complete_count > 0"
	if query.Request.Scope == "account" {
		activeUsersVisible += " AND w.total_complete_count = w.total_expected_count"
	}
	deadlineMinutes := query.UsageDelayMinutes + 10
	if deadlineMinutes < 15 {
		deadlineMinutes = 15
	}
	nowHour := query.Now - query.Now%3600
	oldCutoff := query.Now - int64(deadlineMinutes)*60
	oldCutoff -= oldCutoff % 3600
	nonHourlyFinal := "FALSE"
	if query.Request.Granularity != "hour" {
		nonHourlyFinal = "TRUE"
	}
	verifiedWindow := "TRUE"
	if query.Request.Granularity != "hour" {
		verifiedWindow = `cw.verified_at IS NOT NULL AND cw.verified_at >= (
        TIMESTAMPDIFF(SECOND, CAST('1970-01-01 00:00:00' AS DATETIME),
          DATE_ADD(
            DATE(TIMESTAMPADD(SECOND, cw.hour_ts + 28800, CAST('1970-01-01 00:00:00' AS DATETIME))),
            INTERVAL 1 DAY
          )
        ) - 28800
      )`
	}

	statement := `WITH
` + bucketSQL + `,
` + spaceSQL + `,
runtime_values AS (
  SELECT ? AS now_ts, ? AS now_hour, ? AS old_cutoff
),
` + selectedItemsSQL + `,
candidates AS (
  SELECT r.*, i.bucket_start, i.bucket_end, i.metric_start, i.metric_end,
         i.now_ts, i.now_hour, i.old_cutoff
  FROM selected_items AS i
  JOIN row_space AS r
    ON r.entity_id = i.entity_id
   AND r.dimension_site_id = i.dimension_site_id
   AND r.dimension_value COLLATE utf8mb4_bin = i.dimension_value COLLATE utf8mb4_bin
   AND r.dimension_key COLLATE utf8mb4_bin = i.dimension_key COLLATE utf8mb4_bin
),
candidate_bounds AS (
  SELECT c.*,
         CASE
           WHEN c.breakdown_site_id = 0 OR c.statistics_start_at IS NULL THEN c.bucket_end
           ELSE GREATEST(
             c.bucket_start,
             FLOOR(c.statistics_start_at / 3600) * 3600,
             FLOOR(COALESCE(c.entity_start_at, c.statistics_start_at) / 3600) * 3600
           )
         END AS effective_start,
         CASE
           WHEN c.breakdown_site_id = 0 OR c.statistics_start_at IS NULL THEN c.bucket_start
           ELSE LEAST(
             c.bucket_end,
             c.now_hour,
             COALESCE(((c.statistics_end_at + 3599) DIV 3600) * 3600, c.bucket_end)
           )
         END AS effective_end,
         CASE
           WHEN c.entity_paused_at IS NOT NULL THEN ((c.entity_paused_at + 3599) DIV 3600) * 3600
         END AS entity_pause_at,
         CASE
           WHEN c.management_status = 'disabled' AND c.statistics_end_at IS NULL AND c.disabled_at IS NOT NULL
             THEN FLOOR(c.disabled_at / 3600) * 3600
         END AS site_pause_at
  FROM candidates AS c
),
bounded_candidates AS (
  SELECT c.*,
         GREATEST(0, (c.effective_end - c.effective_start) DIV 3600) AS expected_count,
         GREATEST(0, (LEAST(c.effective_end, c.old_cutoff) - c.effective_start) DIV 3600) AS old_expected_count,
         CASE WHEN c.entity_pause_at IS NULL THEN 0 ELSE
           GREATEST(0, (c.effective_end - GREATEST(c.effective_start, c.entity_pause_at)) DIV 3600)
         END AS entity_paused_count,
         CASE WHEN c.site_pause_at IS NULL THEN 0 ELSE
           GREATEST(0, (
             LEAST(c.effective_end, COALESCE(c.entity_pause_at, c.effective_end)) -
             GREATEST(c.effective_start, c.site_pause_at)
           ) DIV 3600)
         END AS raw_site_paused_count
  FROM candidate_bounds AS c
),
` + entityActivitySQL + `,
window_counts AS (
  SELECT c.entity_id, c.dimension_site_id, c.dimension_value,
         c.bucket_start, c.breakdown_site_id,
         COALESCE(SUM(cw.status = 'complete' AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `)), 0) AS complete_count,
         COALESCE(SUM(cw.status = 'complete' AND cw.hour_ts < c.old_cutoff AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `)), 0) AS old_complete_count,
         COALESCE(SUM(cw.status = 'unavailable' AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `)), 0) AS unavailable_count,
         COALESCE(SUM(cw.status = 'missing' AND
                      cw.hour_ts < LEAST(
                        COALESCE(c.entity_pause_at, c.effective_end),
                        COALESCE(c.site_pause_at, c.effective_end)
                      ) AND (` + windowEntityActive + `)), 0) AS explicit_missing_count,
         COALESCE(SUM(cw.status = 'complete' AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `) AND
                      (` + verifiedWindow + `)), 0) AS verified_count,
         COALESCE(SUM(cw.status = 'complete' AND c.site_pause_at IS NOT NULL AND
                      cw.hour_ts >= c.site_pause_at AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `)), 0) AS site_pause_complete_count,
         COALESCE(SUM(cw.status = 'unavailable' AND c.site_pause_at IS NOT NULL AND
                      cw.hour_ts >= c.site_pause_at AND
                      cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                      (` + windowEntityActive + `)), 0) AS site_pause_unavailable_count,
         MAX(CASE WHEN cw.status = 'complete' AND
                           cw.hour_ts < COALESCE(c.entity_pause_at, c.effective_end) AND
                           (` + windowEntityActive + `)
                  THEN cw.hour_ts + 3600 END) AS site_as_of
  FROM bounded_candidates AS c
  LEFT JOIN collection_window AS cw
    ON cw.site_id = c.breakdown_site_id
   AND cw.hour_ts >= c.effective_start AND cw.hour_ts < c.effective_end
  GROUP BY c.entity_id, c.dimension_site_id, c.dimension_value,
           c.bucket_start, c.breakdown_site_id
),
covered_rows AS (
  SELECT c.*,
         a.active_entity_count, a.active_after_site_pause_count,
         w.complete_count, w.old_complete_count, w.unavailable_count,
         w.explicit_missing_count, w.verified_count,
         w.site_pause_complete_count, w.site_pause_unavailable_count, w.site_as_of,
         EXISTS (
           SELECT 1
           FROM collection_run_window AS rw
           JOIN collection_run AS cr ON cr.id = rw.run_id
           WHERE rw.site_id = c.breakdown_site_id
             AND rw.hour_ts >= c.effective_start AND rw.hour_ts < c.effective_end
             AND rw.hour_ts < LEAST(
               COALESCE(c.entity_pause_at, c.effective_end),
               COALESCE(c.site_pause_at, c.effective_end)
             )
             AND rw.status IN ('pending', 'running')
             AND cr.status IN ('pending', 'running')
             AND cr.task_type = 'usage_backfill'
             AND cr.site_id = rw.site_id
             AND cr.target_type = 'site' AND cr.target_id = rw.site_id
             AND cr.start_timestamp IS NOT NULL AND cr.end_timestamp IS NOT NULL
             AND rw.hour_ts >= cr.start_timestamp AND rw.hour_ts < cr.end_timestamp
             AND (` + runEntityActive + `)
         ) AS has_backfill,
         EXISTS (
           SELECT 1
           FROM collection_run_window AS rw
           JOIN collection_run AS cr ON cr.id = rw.run_id
           WHERE rw.site_id = c.breakdown_site_id
             AND rw.hour_ts >= c.effective_start AND rw.hour_ts < c.effective_end
             AND rw.hour_ts < LEAST(
               COALESCE(c.entity_pause_at, c.effective_end),
               COALESCE(c.site_pause_at, c.effective_end)
             )
             AND rw.status IN ('pending', 'running')
             AND cr.status IN ('pending', 'running')
             AND cr.task_type IN ('usage_hour', 'usage_validation')
             AND cr.site_id = rw.site_id
             AND cr.target_type = 'site' AND cr.target_id = rw.site_id
             AND cr.start_timestamp IS NOT NULL AND cr.end_timestamp IS NOT NULL
             AND rw.hour_ts >= cr.start_timestamp AND rw.hour_ts < cr.end_timestamp
             AND (` + runEntityActive + `)
         ) AS has_active,
         (
           SELECT COUNT(DISTINCT rw.hour_ts)
           FROM collection_run_window AS rw
           JOIN collection_run AS cr ON cr.id = rw.run_id
           WHERE rw.site_id = c.breakdown_site_id
             AND rw.hour_ts >= c.effective_start AND rw.hour_ts < c.effective_end
             AND rw.hour_ts < LEAST(
               COALESCE(c.entity_pause_at, c.effective_end),
               COALESCE(c.site_pause_at, c.effective_end)
             )
             AND rw.status IN ('pending', 'running')
             AND cr.status IN ('pending', 'running')
             AND cr.task_type IN ('usage_hour', 'usage_backfill', 'usage_validation')
             AND cr.site_id = rw.site_id
             AND cr.target_type = 'site' AND cr.target_id = rw.site_id
             AND cr.start_timestamp IS NOT NULL AND cr.end_timestamp IS NOT NULL
             AND rw.hour_ts >= cr.start_timestamp AND rw.hour_ts < cr.end_timestamp
             AND (` + runEntityActive + `)
         ) AS active_count
  FROM bounded_candidates AS c
  JOIN entity_activity AS a
    ON a.entity_id = c.entity_id
   AND a.dimension_site_id = c.dimension_site_id
   AND a.dimension_value COLLATE utf8mb4_bin = c.dimension_value COLLATE utf8mb4_bin
   AND a.bucket_start = c.bucket_start
   AND a.breakdown_site_id = c.breakdown_site_id
  JOIN window_counts AS w
    ON w.entity_id = c.entity_id
   AND w.dimension_site_id = c.dimension_site_id
   AND w.dimension_value COLLATE utf8mb4_bin = c.dimension_value COLLATE utf8mb4_bin
   AND w.bucket_start = c.bucket_start
   AND w.breakdown_site_id = c.breakdown_site_id
),
site_rows AS (
  SELECT c.entity_id, c.dimension_site_id, c.dimension_value, c.dimension_key,
         c.dimension_id, c.dimension_name,
         c.breakdown_site_id, c.breakdown_site_name,
         c.bucket_start, c.bucket_end,
         COUNT(st.id) AS site_metric_rows,
         COALESCE(SUM(st.request_count), 0) AS site_request_count,
         COALESCE(SUM(st.quota), 0) AS site_quota_value,
         COALESCE(SUM(st.token_used), 0) AS site_token_used,
         ` + activeExpression + ` AS site_active_users
  FROM candidates AS c
  LEFT JOIN ` + metricTable + ` AS st
    ON ` + metricJoin + `
   AND st.` + metricTimeColumn + ` >= c.metric_start
   AND st.` + metricTimeColumn + ` < c.metric_end
  GROUP BY c.entity_id, c.dimension_site_id, c.dimension_value, c.dimension_key,
           c.dimension_id, c.dimension_name,
           c.breakdown_site_id, c.breakdown_site_name,
           c.bucket_start, c.bucket_end, c.metric_start, c.metric_end,
           c.statistics_start_at, c.statistics_end_at
),
windowed_rows AS (
	  SELECT s.*, c.expected_count AS site_expected_count,
	         c.complete_count AS site_complete_count,
	         c.verified_count AS site_verified_count,
	         c.site_as_of, c.now_ts,
         SUM(s.site_metric_rows) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_metric_rows,
         SUM(s.site_request_count) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_request_count,
         SUM(s.site_quota_value) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_quota,
         SUM(s.site_token_used) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_token_used,
         SUM(s.site_active_users) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_active_users,
         SUM(c.expected_count) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_expected_count,
         SUM(c.complete_count) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_complete_count,
         SUM(c.verified_count) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_verified_count,
         MAX(c.site_as_of) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS total_as_of,
         MAX(c.explicit_missing_count > 0 OR
             c.old_expected_count > c.old_complete_count + c.unavailable_count +
               c.expected_count - c.active_entity_count + c.active_after_site_pause_count -
               c.site_pause_complete_count - c.site_pause_unavailable_count + c.active_count) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS has_missing,
         MAX(c.unavailable_count > 0) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS has_unavailable,
         MAX(c.expected_count - c.active_entity_count + c.active_after_site_pause_count -
             c.site_pause_complete_count - c.site_pause_unavailable_count > 0) OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS has_paused,
         MAX(c.has_backfill OR c.statistics_status = 'backfilling') OVER (
           PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start
         ) AS has_backfilling
  FROM site_rows AS s
  JOIN covered_rows AS c
    ON c.entity_id = s.entity_id
   AND c.dimension_site_id = s.dimension_site_id
   AND c.dimension_value COLLATE utf8mb4_bin = s.dimension_value COLLATE utf8mb4_bin
   AND c.bucket_start = s.bucket_start
   AND c.breakdown_site_id = s.breakdown_site_id
),
final_rows AS (
  SELECT w.*,
         CASE WHEN w.total_complete_count > 0 THEN CAST(w.total_request_count AS CHAR) END AS request_count,
         CASE WHEN w.total_complete_count > 0 THEN CAST(w.total_quota AS CHAR) END AS quota,
         CASE WHEN w.total_complete_count > 0 THEN CAST(w.total_token_used AS CHAR) END AS token_used,
         CASE WHEN ` + activeUsersVisible + ` THEN CAST(w.total_active_users AS CHAR) END AS active_users,
         CASE WHEN w.site_complete_count > 0 THEN CAST(w.site_quota_value AS CHAR) END AS site_quota,
         CASE
           WHEN w.total_expected_count = 0 OR w.total_complete_count = w.total_expected_count THEN 'complete'
           WHEN w.total_complete_count > 0 THEN 'partial'
           WHEN w.has_missing THEN 'missing'
           WHEN w.has_unavailable THEN 'unavailable'
           WHEN w.has_paused THEN 'paused'
           WHEN w.has_backfilling THEN 'backfilling'
           ELSE 'pending'
         END AS data_status,
         (w.total_expected_count > 0 AND w.total_complete_count = w.total_expected_count AND
          (NOT ` + nonHourlyFinal + ` OR (w.bucket_end <= w.now_ts AND w.total_verified_count = w.total_expected_count))) AS is_final,
         CASE WHEN w.total_as_of > 0 THEN w.total_as_of END AS as_of,
         (` + sortKnown + `) AS sort_known,
         (` + sortNumber + `) AS sort_number,
         (` + sortText + `) AS sort_text
  FROM windowed_rows AS w
)
SELECT entity_id, dimension_site_id, dimension_value,
       dimension_key, dimension_id, dimension_name,
       breakdown_site_id, breakdown_site_name,
       bucket_start, bucket_end,
       request_count, quota, token_used, active_users, site_quota,
       data_status, is_final, as_of,
       sort_known, sort_number, sort_text
FROM final_rows
WHERE ` + cursorWhere + `
ORDER BY ` + strings.Join(orderBy, ", ") + `
LIMIT ?`
	args := make([]any, 0, len(bucketArgs)+len(spaceArgs)+len(selectedArgs)+len(cursorArgs)+4)
	args = append(args, bucketArgs...)
	args = append(args, spaceArgs...)
	args = append(args, query.Now, nowHour, oldCutoff)
	args = append(args, selectedArgs...)
	args = append(args, cursorArgs...)
	args = append(args, query.Limit+1)
	return statement, args, nil
}

func statisticsExportBucketCount(request StatisticsReadRequest) (int64, error) {
	if request.StartTimestamp <= 0 || request.EndTimestamp <= request.StartTimestamp {
		return 0, ErrStatisticsReadContract
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Unix(request.StartTimestamp, 0).In(location)
	end := time.Unix(request.EndTimestamp, 0).In(location)
	switch request.Granularity {
	case "hour":
		return (request.EndTimestamp - request.StartTimestamp) / 3600, nil
	case "day":
		return (request.EndTimestamp - request.StartTimestamp) / 86400, nil
	case "month":
		return int64((end.Year()-start.Year())*12 + int(end.Month()-start.Month())), nil
	case "year":
		return int64(end.Year() - start.Year()), nil
	default:
		return 0, ErrStatisticsReadContract
	}
}

func statisticsExportBucketsSQL(request StatisticsReadRequest, count int64) (string, []any) {
	digits := 1
	for maximum := int64(10); maximum < count; maximum *= 10 {
		digits++
	}
	aliases := make([]string, digits)
	parts := make([]string, digits)
	for index := 0; index < digits; index++ {
		aliases[index] = fmt.Sprintf("d%d", index)
		factor := int64(1)
		for power := 0; power < index; power++ {
			factor *= 10
		}
		parts[index] = fmt.Sprintf("%d * d%d.n", factor, index)
	}
	joins := make([]string, digits)
	for index, alias := range aliases {
		joins[index] = "digits AS " + alias
	}
	numbers := "digits(n) AS (SELECT 0 UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 " +
		"UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9),\n" +
		"numbers(n) AS (SELECT " + strings.Join(parts, " + ") + " FROM " + strings.Join(joins, " CROSS JOIN ") + ")"
	start := request.StartTimestamp
	var bucket string
	args := make([]any, 0, 5)
	switch request.Granularity {
	case "hour":
		bucket = `buckets AS (
  SELECT ? + n * 3600 AS bucket_start,
         ? + (n + 1) * 3600 AS bucket_end,
         ? + n * 3600 AS metric_start,
         ? + (n + 1) * 3600 AS metric_end
  FROM numbers WHERE n < ?
)`
		args = []any{start, start, start, start, count}
	case "day", "month", "year":
		unit := strings.ToUpper(request.Granularity)
		bucket = `calendar_origin(local_start) AS (
  SELECT TIMESTAMPADD(SECOND, ? + 28800, CAST('1970-01-01 00:00:00' AS DATETIME))
),
buckets AS (
  SELECT TIMESTAMPDIFF(SECOND, CAST('1970-01-01 00:00:00' AS DATETIME),
           DATE_ADD(origin.local_start, INTERVAL n ` + unit + `)) - 28800 AS bucket_start,
         TIMESTAMPDIFF(SECOND, CAST('1970-01-01 00:00:00' AS DATETIME),
           DATE_ADD(origin.local_start, INTERVAL (n + 1) ` + unit + `)) - 28800 AS bucket_end,
         CAST(DATE_FORMAT(DATE_ADD(origin.local_start, INTERVAL n ` + unit + `), '%Y%m%d') AS UNSIGNED) AS metric_start,
         CAST(DATE_FORMAT(DATE_ADD(origin.local_start, INTERVAL (n + 1) ` + unit + `), '%Y%m%d') AS UNSIGNED) AS metric_end
  FROM numbers CROSS JOIN calendar_origin AS origin WHERE n < ?
)`
		args = []any{start, count}
	}
	return numbers + ",\n" + bucket, args
}

func statisticsExportRowSpaceSQL(request StatisticsReadRequest) (string, []any, error) {
	selectedSites := `selected_sites AS (
  SELECT id, name, statistics_start_at, statistics_end_at, disabled_at,
         management_status, statistics_status
  FROM site WHERE 1 = 1`
	args := make([]any, 0)
	if len(request.SiteIDs) > 0 {
		selectedSites += " AND id IN ?"
		args = append(args, request.SiteIDs)
	}
	selectedSites += ")"
	switch request.Scope {
	case "global":
		return selectedSites + `,
row_space AS (
  SELECT 0 AS entity_id, 0 AS dimension_site_id,
         CAST('global' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CAST('global' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         'global' AS dimension_id, '全局' AS dimension_name,
         COALESCE(s.id, 0) AS breakdown_site_id,
         COALESCE(s.name, '') AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM (SELECT 1 AS singleton) AS one
  LEFT JOIN selected_sites AS s ON TRUE
)`, args, nil
	case "site":
		return selectedSites + `,
row_space AS (
  SELECT s.id AS entity_id, s.id AS dimension_site_id,
         CAST('' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CAST(s.id AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         CAST(s.id AS CHAR) AS dimension_id, s.name AS dimension_name,
         s.id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM selected_sites AS s
)`, args, nil
	case "customer":
		customers := "selected_customers AS (SELECT id, name, statistics_paused_at FROM customer WHERE 1 = 1"
		if len(request.CustomerIDs) > 0 {
			customers += " AND id IN ?"
			args = append(args, request.CustomerIDs)
		}
		customers += ")"
		return selectedSites + ",\n" + customers + `,
customer_sites AS (
  SELECT a.customer_id, a.site_id, s.name AS site_name,
         MIN(a.remote_created_at) AS entity_start_at
  FROM account AS a
  JOIN selected_sites AS s ON s.id = a.site_id
  JOIN selected_customers AS c ON c.id = a.customer_id
  GROUP BY a.customer_id, a.site_id, s.name
),
row_space AS (
  SELECT c.id AS entity_id, 0 AS dimension_site_id,
         CAST('' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CAST(c.id AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         CAST(c.id AS CHAR) AS dimension_id, c.name AS dimension_name,
         COALESCE(cs.site_id, 0) AS breakdown_site_id,
         COALESCE(cs.site_name, '') AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         cs.entity_start_at,
         c.statistics_paused_at AS entity_paused_at
  FROM selected_customers AS c
  LEFT JOIN customer_sites AS cs ON cs.customer_id = c.id
  LEFT JOIN selected_sites AS s ON s.id = cs.site_id
)`, args, nil
	case "account":
		where := ""
		if len(request.CustomerIDs) > 0 {
			where += " AND a.customer_id IN ?"
			args = append(args, request.CustomerIDs)
		}
		if len(request.AccountIDs) > 0 {
			where += " AND a.id IN ?"
			args = append(args, request.AccountIDs)
		}
		return selectedSites + `,
row_space AS (
  SELECT a.id AS entity_id, a.site_id AS dimension_site_id,
         CAST('' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CAST(a.id AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         CAST(a.id AS CHAR) AS dimension_id,
         CASE WHEN a.display_name <> '' THEN a.display_name ELSE a.username END AS dimension_name,
         a.site_id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         a.remote_created_at AS entity_start_at,
         a.statistics_paused_at AS entity_paused_at
  FROM account AS a
  JOIN selected_sites AS s ON s.id = a.site_id
  WHERE 1 = 1` + where + `
)`, args, nil
	case "model":
		if len(request.ModelNames) > 0 {
			raw, err := json.Marshal(request.ModelNames)
			if err != nil {
				return "", nil, ErrStatisticsReadContract
			}
			args = append(args, string(raw))
			return selectedSites + `,
selected_models AS (
  SELECT model_name
  FROM JSON_TABLE(?, '$[*]' COLUMNS(
    model_name VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin PATH '$'
  )) AS names
),
row_space AS (
  SELECT 0 AS entity_id, s.id AS dimension_site_id,
         m.model_name COLLATE utf8mb4_bin AS dimension_value,
         CONVERT(CONCAT(CAST(s.id AS CHAR), CHAR(0), m.model_name) USING utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         m.model_name AS dimension_id, m.model_name AS dimension_name,
         s.id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM selected_sites AS s
  CROSS JOIN selected_models AS m
)`, args, nil
		}
		table, column, start, end := statisticsExportMetricRange(request, "model")
		args = append(args, start, end)
		return selectedSites + `,
model_dimensions AS (
  SELECT DISTINCT st.site_id, st.model_name COLLATE utf8mb4_bin AS model_name
  FROM ` + table + ` AS st
  JOIN selected_sites AS s ON s.id = st.site_id
  WHERE st.` + column + ` >= ? AND st.` + column + ` < ?
),
row_space AS (
  SELECT 0 AS entity_id, m.site_id AS dimension_site_id,
         m.model_name COLLATE utf8mb4_bin AS dimension_value,
         CONVERT(CONCAT(CAST(m.site_id AS CHAR), CHAR(0), m.model_name) USING utf8mb4) COLLATE utf8mb4_bin AS dimension_key,
         m.model_name AS dimension_id, m.model_name AS dimension_name,
         m.site_id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM model_dimensions AS m
  JOIN selected_sites AS s ON s.id = m.site_id
)`, args, nil
	case "channel":
		if len(request.ChannelKeys) > 0 {
			raw, err := json.Marshal(request.ChannelKeys)
			if err != nil {
				return "", nil, ErrStatisticsReadContract
			}
			args = append(args, string(raw))
			return selectedSites + `,
selected_channels AS (
  SELECT site_id, channel_id
  FROM JSON_TABLE(?, '$[*]' COLUMNS(
    site_id BIGINT PATH '$.SiteID', channel_id BIGINT PATH '$.ChannelID'
  )) AS keys_json
),
row_space AS (
  SELECT k.channel_id AS entity_id, k.site_id AS dimension_site_id,
         CAST('' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CONCAT(k.site_id, ':', k.channel_id) COLLATE utf8mb4_bin AS dimension_key,
         CONCAT(k.site_id, ':', k.channel_id) AS dimension_id,
         COALESCE(sc.name, CASE WHEN k.channel_id = 0 THEN '未知通道' ELSE CONCAT('通道 ', k.channel_id) END) AS dimension_name,
         k.site_id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM selected_channels AS k
  JOIN selected_sites AS s ON s.id = k.site_id
  LEFT JOIN site_channel AS sc ON sc.site_id = k.site_id AND sc.remote_channel_id = k.channel_id
)`, args, nil
		}
		table, column, start, end := statisticsExportMetricRange(request, "channel")
		args = append(args, start, end)
		return selectedSites + `,
channel_dimensions AS (
  SELECT sc.site_id, sc.remote_channel_id AS channel_id, sc.name
  FROM site_channel AS sc
  JOIN selected_sites AS s ON s.id = sc.site_id
  UNION
  SELECT st.site_id, st.channel_id, '' AS name
  FROM ` + table + ` AS st
  JOIN selected_sites AS s ON s.id = st.site_id
  WHERE st.` + column + ` >= ? AND st.` + column + ` < ?
),
row_space AS (
  SELECT c.channel_id AS entity_id, c.site_id AS dimension_site_id,
         CAST('' AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_bin AS dimension_value,
         CONCAT(c.site_id, ':', c.channel_id) COLLATE utf8mb4_bin AS dimension_key,
         CONCAT(c.site_id, ':', c.channel_id) AS dimension_id,
         CASE WHEN MAX(c.name) <> '' THEN MAX(c.name)
              WHEN c.channel_id = 0 THEN '未知通道' ELSE CONCAT('通道 ', c.channel_id) END AS dimension_name,
         c.site_id AS breakdown_site_id, s.name AS breakdown_site_name,
         s.statistics_start_at, s.statistics_end_at, s.disabled_at,
         s.management_status, s.statistics_status,
         NULL AS entity_start_at, NULL AS entity_paused_at
  FROM channel_dimensions AS c
  JOIN selected_sites AS s ON s.id = c.site_id
  GROUP BY c.site_id, c.channel_id, s.name,
           s.statistics_start_at, s.statistics_end_at, s.disabled_at,
           s.management_status, s.statistics_status
)`, args, nil
	default:
		return "", nil, ErrStatisticsReadContract
	}
}

func statisticsExportMetricRange(request StatisticsReadRequest, scope string) (string, string, any, any) {
	suffix := "hourly"
	column := "hour_ts"
	start, end := any(request.StartTimestamp), any(request.EndTimestamp)
	if request.Granularity != "hour" {
		suffix = "daily"
		column = "date_key"
		start, end = request.StartDateKey, request.EndDateKey
	}
	return scope + "_stat_" + suffix, column, start, end
}

func statisticsExportMetricSQL(request StatisticsReadRequest) (string, string, string, error) {
	table, timeColumn, _, _ := statisticsExportMetricRange(request, request.Scope)
	join := ""
	switch request.Scope {
	case "global":
		table, timeColumn, _, _ = statisticsExportMetricRange(request, "site")
		join = "st.site_id = c.breakdown_site_id"
	case "site":
		join = "st.site_id = c.entity_id"
	case "customer":
		join = "st.customer_id = c.entity_id AND st.site_id = c.breakdown_site_id"
	case "account":
		join = "st.account_id = c.entity_id"
	case "model":
		join = "st.site_id = c.dimension_site_id AND st.model_name COLLATE utf8mb4_bin = c.dimension_value COLLATE utf8mb4_bin"
	case "channel":
		join = "st.site_id = c.dimension_site_id AND st.channel_id = c.entity_id"
	default:
		return "", "", "", ErrStatisticsReadContract
	}
	return table, timeColumn, join, nil
}

func statisticsExportActiveExpression(request StatisticsReadRequest) (string, error) {
	useHourly := request.Granularity == "hour" || request.Scope == "customer" || request.Scope == "account"
	from := "usage_fact_daily AS f"
	timeWhere := "f.date_key >= c.metric_start AND f.date_key < c.metric_end"
	lifecycle := ""
	if useHourly {
		from = `usage_fact_hourly AS f
    JOIN collection_window AS active_window
      ON active_window.site_id = f.site_id
     AND active_window.hour_ts = f.hour_ts
     AND active_window.status = 'complete'`
		timeWhere = "f.hour_ts >= c.bucket_start AND f.hour_ts < c.bucket_end"
		lifecycle = `
     AND c.statistics_start_at IS NOT NULL
     AND c.statistics_start_at < f.hour_ts + 3600
     AND (c.statistics_end_at IS NULL OR c.statistics_end_at > f.hour_ts)`
	}
	siteWhere := "f.site_id = c.breakdown_site_id"
	dimensionWhere := ""
	identity := "f.remote_user_id"
	switch request.Scope {
	case "global", "site":
	case "model":
		dimensionWhere = "\n     AND f.model_name COLLATE utf8mb4_bin = c.dimension_value COLLATE utf8mb4_bin"
	case "channel":
		dimensionWhere = "\n     AND f.channel_id = c.entity_id"
	case "customer":
		from += `
    JOIN account AS active_account
      ON active_account.site_id = f.site_id
     AND active_account.remote_user_id = f.remote_user_id
    JOIN customer AS active_customer ON active_customer.id = active_account.customer_id`
		dimensionWhere = `
     AND active_account.customer_id = c.entity_id
     AND active_account.remote_created_at < f.hour_ts + 3600
     AND (active_account.statistics_paused_at IS NULL OR f.hour_ts < active_account.statistics_paused_at)
     AND (active_customer.statistics_paused_at IS NULL OR f.hour_ts < active_customer.statistics_paused_at)`
		identity = "active_account.id"
	case "account":
		from += `
    JOIN account AS active_account
      ON active_account.site_id = f.site_id
     AND active_account.remote_user_id = f.remote_user_id`
		dimensionWhere = `
     AND active_account.id = c.entity_id
     AND active_account.remote_created_at < f.hour_ts + 3600
     AND (active_account.statistics_paused_at IS NULL OR f.hour_ts < active_account.statistics_paused_at)`
		identity = "active_account.id"
	default:
		return "", ErrStatisticsReadContract
	}
	return `(SELECT COUNT(DISTINCT ` + identity + `)
   FROM ` + from + `
   WHERE ` + timeWhere + `
     AND ` + siteWhere + lifecycle + dimensionWhere + `)`, nil
}

func statisticsExportCustomerEntityActive(hourExpression string) string {
	return `EXISTS (
    SELECT 1
    FROM account AS coverage_account
    WHERE coverage_account.customer_id = c.entity_id
      AND coverage_account.site_id = c.breakdown_site_id
      AND coverage_account.remote_created_at < ` + hourExpression + ` + 3600
      AND (coverage_account.statistics_paused_at IS NULL OR
           ` + hourExpression + ` < coverage_account.statistics_paused_at)
  )`
}

func statisticsExportEntityActivitySQL(scope string) string {
	if scope != "customer" {
		return `entity_activity AS (
  SELECT c.entity_id, c.dimension_site_id, c.dimension_value,
         c.bucket_start, c.breakdown_site_id,
         c.expected_count - c.entity_paused_count AS active_entity_count,
         c.raw_site_paused_count AS active_after_site_pause_count
  FROM bounded_candidates AS c
)`
	}
	return `customer_account_intervals AS (
  SELECT intervals.*
  FROM (
    SELECT c.entity_id, c.dimension_site_id, c.dimension_value,
           c.bucket_start, c.breakdown_site_id, c.site_pause_at,
           GREATEST(
             c.effective_start,
             FLOOR(a.remote_created_at / 3600) * 3600
           ) AS interval_start,
           LEAST(
             c.effective_end,
             COALESCE(((a.statistics_paused_at + 3599) DIV 3600) * 3600, c.effective_end),
             COALESCE(c.entity_pause_at, c.effective_end)
           ) AS interval_end
    FROM bounded_candidates AS c
    JOIN account AS a
      ON a.customer_id = c.entity_id AND a.site_id = c.breakdown_site_id
  ) AS intervals
  WHERE intervals.interval_start < intervals.interval_end
),
customer_interval_previous AS (
  SELECT i.*,
         MAX(i.interval_end) OVER (
           PARTITION BY i.entity_id, i.dimension_site_id, i.dimension_value,
                        i.bucket_start, i.breakdown_site_id
           ORDER BY i.interval_start, i.interval_end
           ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
         ) AS previous_end
  FROM customer_account_intervals AS i
),
customer_interval_marked AS (
  SELECT i.*,
         CASE WHEN i.previous_end IS NULL OR i.interval_start > i.previous_end
              THEN 1 ELSE 0 END AS starts_group
  FROM customer_interval_previous AS i
),
customer_interval_grouped AS (
  SELECT i.*,
         SUM(i.starts_group) OVER (
           PARTITION BY i.entity_id, i.dimension_site_id, i.dimension_value,
                        i.bucket_start, i.breakdown_site_id
           ORDER BY i.interval_start, i.interval_end
           ROWS UNBOUNDED PRECEDING
         ) AS interval_group
  FROM customer_interval_marked AS i
),
customer_active_ranges AS (
  SELECT i.entity_id, i.dimension_site_id, i.dimension_value,
         i.bucket_start, i.breakdown_site_id, i.interval_group,
         MIN(i.interval_start) AS range_start,
         MAX(i.interval_end) AS range_end,
         MAX(i.site_pause_at) AS site_pause_at
  FROM customer_interval_grouped AS i
  GROUP BY i.entity_id, i.dimension_site_id, i.dimension_value,
           i.bucket_start, i.breakdown_site_id, i.interval_group
),
entity_activity AS (
  SELECT c.entity_id, c.dimension_site_id, c.dimension_value,
         c.bucket_start, c.breakdown_site_id,
         COALESCE(SUM((r.range_end - r.range_start) DIV 3600), 0) AS active_entity_count,
         COALESCE(SUM(CASE WHEN r.site_pause_at IS NULL THEN 0 ELSE
           GREATEST(0, (r.range_end - GREATEST(r.range_start, r.site_pause_at)) DIV 3600)
         END), 0) AS active_after_site_pause_count
  FROM bounded_candidates AS c
  LEFT JOIN customer_active_ranges AS r
    ON r.entity_id = c.entity_id
   AND r.dimension_site_id = c.dimension_site_id
   AND r.dimension_value COLLATE utf8mb4_bin = c.dimension_value COLLATE utf8mb4_bin
   AND r.bucket_start = c.bucket_start
   AND r.breakdown_site_id = c.breakdown_site_id
  GROUP BY c.entity_id, c.dimension_site_id, c.dimension_value,
           c.bucket_start, c.breakdown_site_id
)`
}

type statisticsExportOrderTerm struct {
	Expression string
	Direction  string
	Value      any
}

func statisticsExportSelectedItemsSQL(
	scope, sortBy, sortOrder string,
	cursor StatisticsExportRowCursor,
	limit int,
) (string, []any, error) {
	statement := `selected_items AS (
  SELECT DISTINCT r.entity_id, r.dimension_site_id, r.dimension_value, r.dimension_key,
         r.dimension_id, r.dimension_name,
         b.bucket_start, b.bucket_end, b.metric_start, b.metric_end,
         v.now_ts, v.now_hour, v.old_cutoff
  FROM row_space AS r
  CROSS JOIN buckets AS b
  CROSS JOIN runtime_values AS v`
	if sortBy == "request_count" || sortBy == "quota" || sortBy == "token_used" || sortBy == "active_users" {
		return statement + "\n)", nil, nil
	}
	terms, err := statisticsExportLogicalSelectionTerms(scope, sortBy, sortOrder, cursor)
	if err != nil {
		return "", nil, err
	}
	where, args := statisticsExportAtOrAfter(terms, cursor.Initialized)
	orderBy := make([]string, 0, len(terms))
	for _, term := range terms {
		orderBy = append(orderBy, term.Expression+" "+term.Direction)
	}
	statement += "\n  WHERE " + where + "\n  ORDER BY " + strings.Join(orderBy, ", ") + "\n  LIMIT ?\n)"
	selectedLimit := limit + 1
	if cursor.Initialized {
		// The current logical item stays selected so a page may resume inside its
		// site breakdown. Reserve one additional item when it was fully consumed.
		selectedLimit++
	}
	args = append(args, selectedLimit)
	return statement, args, nil
}

func statisticsExportLogicalSelectionTerms(
	scope, sortBy, sortOrder string,
	cursor StatisticsExportRowCursor,
) ([]statisticsExportOrderTerm, error) {
	dimensionKey, err := statisticsExportCursorDimensionKey(scope, cursor)
	if err != nil {
		return nil, err
	}
	direction := strings.ToUpper(sortOrder)
	stableKey := statisticsExportOrderTerm{
		Expression: "r.dimension_key COLLATE utf8mb4_bin", Direction: "ASC", Value: dimensionKey,
	}
	switch sortBy {
	case "name":
		return []statisticsExportOrderTerm{
			{Expression: "r.dimension_name COLLATE utf8mb4_bin", Direction: direction, Value: cursor.SortText},
			{Expression: "b.bucket_start", Direction: direction, Value: cursor.BucketStart},
			stableKey,
		}, nil
	case "bucket_start":
		return []statisticsExportOrderTerm{
			{Expression: "b.bucket_start", Direction: direction, Value: cursor.BucketStart},
			{Expression: "r.dimension_name COLLATE utf8mb4_bin", Direction: direction, Value: cursor.SortText},
			stableKey,
		}, nil
	default:
		return nil, ErrStatisticsReadContract
	}
}

func statisticsExportOrderTerms(
	scope, sortBy, sortOrder string,
	cursor StatisticsExportRowCursor,
) ([]statisticsExportOrderTerm, error) {
	dimensionKey, err := statisticsExportCursorDimensionKey(scope, cursor)
	if err != nil {
		return nil, err
	}
	direction := strings.ToUpper(sortOrder)
	terms := make([]statisticsExportOrderTerm, 0, 6)
	switch sortBy {
	case "request_count", "quota", "token_used", "active_users":
		terms = append(terms,
			statisticsExportOrderTerm{Expression: "sort_known", Direction: "DESC", Value: cursor.SortKnown},
			statisticsExportOrderTerm{Expression: "sort_number", Direction: direction, Value: cursor.SortNumber},
			statisticsExportOrderTerm{Expression: "sort_text COLLATE utf8mb4_bin", Direction: direction, Value: cursor.SortText},
			statisticsExportOrderTerm{Expression: "bucket_start", Direction: direction, Value: cursor.BucketStart},
		)
	case "name":
		terms = append(terms,
			statisticsExportOrderTerm{Expression: "sort_text COLLATE utf8mb4_bin", Direction: direction, Value: cursor.SortText},
			statisticsExportOrderTerm{Expression: "bucket_start", Direction: direction, Value: cursor.BucketStart},
		)
	case "bucket_start":
		terms = append(terms,
			statisticsExportOrderTerm{Expression: "sort_number", Direction: direction, Value: cursor.SortNumber},
			statisticsExportOrderTerm{Expression: "sort_text COLLATE utf8mb4_bin", Direction: direction, Value: cursor.SortText},
		)
	default:
		return nil, ErrStatisticsReadContract
	}
	terms = append(terms,
		statisticsExportOrderTerm{
			Expression: "dimension_key COLLATE utf8mb4_bin", Direction: "ASC", Value: dimensionKey,
		},
		statisticsExportOrderTerm{
			Expression: "breakdown_site_id", Direction: "ASC", Value: cursor.BreakdownSiteID,
		},
	)
	return terms, nil
}

func statisticsExportCursorDimensionKey(scope string, cursor StatisticsExportRowCursor) (string, error) {
	if cursor.DimensionKey != "" {
		return cursor.DimensionKey, nil
	}
	switch scope {
	case "global":
		return "global", nil
	case "site", "customer", "account":
		return strconv.FormatInt(cursor.EntityID, 10), nil
	case "model":
		return strconv.FormatInt(cursor.DimensionSiteID, 10) + "\x00" + cursor.DimensionValue, nil
	case "channel":
		return strconv.FormatInt(cursor.DimensionSiteID, 10) + ":" + strconv.FormatInt(cursor.EntityID, 10), nil
	default:
		return "", ErrStatisticsReadContract
	}
}

func statisticsExportAfter(terms []statisticsExportOrderTerm, initialized bool) (string, []any) {
	if !initialized || len(terms) == 0 {
		return "TRUE", nil
	}
	clauses := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*(len(terms)+1)/2)
	for index, term := range terms {
		parts := make([]string, 0, index+1)
		for prefix := 0; prefix < index; prefix++ {
			parts = append(parts, terms[prefix].Expression+" = ?")
			args = append(args, terms[prefix].Value)
		}
		operator := ">"
		if term.Direction == "DESC" {
			operator = "<"
		}
		parts = append(parts, term.Expression+" "+operator+" ?")
		args = append(args, term.Value)
		clauses = append(clauses, "("+strings.Join(parts, " AND ")+")")
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func statisticsExportAtOrAfter(terms []statisticsExportOrderTerm, initialized bool) (string, []any) {
	if !initialized || len(terms) == 0 {
		return "TRUE", nil
	}
	after, args := statisticsExportAfter(terms, true)
	equal := make([]string, 0, len(terms))
	for _, term := range terms {
		equal = append(equal, term.Expression+" = ?")
		args = append(args, term.Value)
	}
	return "(" + after + " OR (" + strings.Join(equal, " AND ") + "))", args
}
