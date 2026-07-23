package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
)

type aggregationSpec struct {
	Name     string
	Actual   string
	Expected string
}

type metricHash struct {
	Rows   int64
	SHA256 string
}

const beijingDateKeySQL = `CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, f.hour_ts + 28800, '1970-01-01'), '%Y%m%d') AS UNSIGNED)`

func verifyAggregations(ctx context.Context, queryer queryContext) (map[string]any, error) {
	if err := verifyAggregationScalarInvariants(ctx, queryer); err != nil {
		return nil, err
	}
	results := make(map[string]any, len(aggregationSpecs))
	for _, spec := range aggregationSpecs {
		actual, err := streamMetricHash(ctx, queryer, spec.Actual)
		if err != nil {
			return nil, fmt.Errorf("read %s aggregate: %w", spec.Name, err)
		}
		expected, err := streamMetricHash(ctx, queryer, spec.Expected)
		if err != nil {
			return nil, fmt.Errorf("recompute %s aggregate: %w", spec.Name, err)
		}
		if actual != expected {
			return nil, fmt.Errorf("%s aggregate mismatch", spec.Name)
		}
		results[spec.Name] = map[string]any{"rows": actual.Rows, "sha256": actual.SHA256}
	}
	return results, nil
}

func streamMetricHash(ctx context.Context, queryer queryContext, query string) (metricHash, error) {
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return metricHash{}, err
	}
	defer func() { _ = rows.Close() }()
	digest := sha256.New()
	var count int64
	for rows.Next() {
		var key, requests, quota, tokens, active, dataStatus, isFinal string
		if err := rows.Scan(&key, &requests, &quota, &tokens, &active, &dataStatus, &isFinal); err != nil {
			return metricHash{}, err
		}
		writeHashField(digest, key)
		writeHashField(digest, requests)
		writeHashField(digest, quota)
		writeHashField(digest, tokens)
		writeHashField(digest, active)
		writeHashField(digest, dataStatus)
		writeHashField(digest, isFinal)
		_, _ = io.WriteString(digest, "\n")
		count++
	}
	if err := rows.Err(); err != nil {
		return metricHash{}, err
	}
	return metricHash{Rows: count, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func writeHashField(digest hash.Hash, value string) {
	_, _ = fmt.Fprintf(digest, "%d:", len(value))
	_, _ = io.WriteString(digest, value)
}

func verifyAggregationScalarInvariants(ctx context.Context, queryer queryContext) error {
	queries := []string{
		`SELECT COUNT(*) FROM usage_fact_hourly f
LEFT JOIN collection_window w ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts
LEFT JOIN site s ON s.id = f.site_id
WHERE f.request_count < 0 OR f.quota < 0 OR f.token_used < 0 OR MOD(f.hour_ts, 3600) <> 0
   OR w.site_id IS NULL OR w.status <> 'complete' OR s.id IS NULL OR s.statistics_start_at IS NULL
   OR s.statistics_start_at >= f.hour_ts + 3600
   OR (s.statistics_end_at IS NOT NULL AND s.statistics_end_at <= f.hour_ts)`,
		`SELECT COUNT(*) FROM usage_fact_daily
WHERE request_count < 0 OR quota < 0 OR token_used < 0 OR is_final NOT IN (0, 1)`,
		aggregationViolationQuery("account_stat_hourly", false, false),
		aggregationViolationQuery("account_stat_daily", false, true),
		aggregationViolationQuery("customer_stat_hourly", true, false),
		aggregationViolationQuery("customer_stat_daily", true, true),
		aggregationViolationQuery("site_stat_hourly", true, false),
		aggregationViolationQuery("site_stat_daily", true, true),
		aggregationViolationQuery("global_stat_hourly", true, false),
		aggregationViolationQuery("global_stat_daily", true, true),
		aggregationViolationQuery("model_stat_hourly", true, false),
		aggregationViolationQuery("model_stat_daily", true, true),
		aggregationViolationQuery("channel_stat_hourly", true, false),
		aggregationViolationQuery("channel_stat_daily", true, true),
	}
	for _, query := range queries {
		var violations int64
		if err := queryer.QueryRowContext(ctx, query).Scan(&violations); err != nil {
			return err
		}
		if violations != 0 {
			return errors.New("aggregation scalar invariant violation")
		}
	}
	return nil
}

func aggregationViolationQuery(table string, activeUsers, daily bool) string {
	conditions := "request_count < 0 OR quota < 0 OR token_used < 0 OR data_status NOT IN ('complete','partial')"
	if activeUsers {
		conditions += " OR active_users < 0"
	}
	if daily {
		conditions += " OR is_final NOT IN (0, 1)"
	}
	return "SELECT COUNT(*) FROM " + quoteIdentifier(table) + " WHERE " + conditions
}

func beijingDateKey(column string) string {
	return strings.ReplaceAll(beijingDateKeySQL, "f.hour_ts", column)
}

func actualAggregationQuery(table, keys, active, status, final, order string) string {
	return `SELECT CAST(JSON_ARRAY(` + keys + `) AS CHAR), CAST(request_count AS CHAR),
CAST(quota AS CHAR), CAST(token_used AS CHAR), CAST(` + active + ` AS CHAR),
CAST(` + status + ` AS CHAR), CAST(` + final + ` AS CHAR)
FROM ` + table + ` ORDER BY ` + order
}

func qualifyAggregationColumns(alias, columns string) string {
	items := strings.Split(columns, ",")
	for index := range items {
		items[index] = alias + "." + strings.TrimSpace(items[index])
	}
	return strings.Join(items, ", ")
}

var dailyCoverageCTE = `WITH RECURSIVE
hour_offsets AS (
  SELECT 0 AS hour_offset
  UNION ALL SELECT hour_offset + 3600 FROM hour_offsets WHERE hour_offset < 23 * 3600
),
date_candidates AS (
  SELECT ` + beijingDateKey("source.hour_ts") + ` AS date_key
  FROM (
    SELECT hour_ts FROM collection_window
    UNION SELECT hour_ts FROM usage_fact_hourly
  ) AS source
  UNION SELECT date_key FROM usage_fact_daily
  UNION SELECT date_key FROM account_stat_daily
  UNION SELECT date_key FROM customer_stat_daily
  UNION SELECT date_key FROM site_stat_daily
  UNION SELECT date_key FROM global_stat_daily
  UNION SELECT date_key FROM model_stat_daily
  UNION SELECT date_key FROM channel_stat_daily
),
date_bounds AS (
  SELECT date_key,
    TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00',
      STR_TO_DATE(CAST(date_key AS CHAR), '%Y%m%d')) - 28800 AS date_start,
    TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00',
      STR_TO_DATE(CAST(date_key AS CHAR), '%Y%m%d')) - 28800 + 86400 AS date_end
  FROM date_candidates
  WHERE STR_TO_DATE(CAST(date_key AS CHAR), '%Y%m%d') IS NOT NULL
),
site_hours AS (
  SELECT d.date_key, d.date_start, d.date_end, s.id AS site_id,
    d.date_start + o.hour_offset AS hour_ts
  FROM date_bounds AS d
  CROSS JOIN hour_offsets AS o
  JOIN site AS s
    ON s.statistics_start_at IS NOT NULL
   AND s.statistics_start_at < d.date_start + o.hour_offset + 3600
   AND (s.statistics_end_at IS NULL OR s.statistics_end_at > d.date_start + o.hour_offset)
),
site_coverage AS (
  SELECT h.date_key, h.date_end, h.site_id, COUNT(*) AS expected_count,
    SUM(CASE WHEN w.status = 'complete' THEN 1 ELSE 0 END) AS complete_count,
    SUM(CASE WHEN w.status = 'complete' AND w.verified_at >= h.date_end THEN 1 ELSE 0 END) AS verified_count
  FROM site_hours AS h
  LEFT JOIN collection_window AS w ON w.site_id = h.site_id AND w.hour_ts = h.hour_ts
  GROUP BY h.date_key, h.date_end, h.site_id
)`

var accountCoverageCTE = dailyCoverageCTE + `,
account_hours AS (
  SELECT h.date_key, h.date_end, a.id AS account_id, h.hour_ts
  FROM site_hours AS h
  JOIN account AS a ON a.site_id = h.site_id
  WHERE a.remote_created_at < h.hour_ts + 3600
    AND (a.statistics_paused_at IS NULL OR h.hour_ts < a.statistics_paused_at)
),
account_coverage AS (
  SELECT h.date_key, h.date_end, h.account_id, COUNT(*) AS expected_count,
    SUM(CASE WHEN w.status = 'complete' THEN 1 ELSE 0 END) AS complete_count,
    SUM(CASE WHEN w.status = 'complete' AND w.verified_at >= h.date_end THEN 1 ELSE 0 END) AS verified_count
  FROM account_hours AS h
  LEFT JOIN account AS a ON a.id = h.account_id
  LEFT JOIN collection_window AS w ON w.site_id = a.site_id AND w.hour_ts = h.hour_ts
  GROUP BY h.date_key, h.date_end, h.account_id
)`

var customerCoverageCTE = dailyCoverageCTE + `,
customer_hours AS (
  SELECT DISTINCT h.date_key, h.date_end, a.customer_id, a.site_id, h.hour_ts
  FROM site_hours AS h
  JOIN account AS a ON a.site_id = h.site_id
  JOIN customer AS c ON c.id = a.customer_id
  WHERE a.remote_created_at < h.hour_ts + 3600
    AND (a.statistics_paused_at IS NULL OR h.hour_ts < a.statistics_paused_at)
    AND (c.statistics_paused_at IS NULL OR h.hour_ts < c.statistics_paused_at)
),
customer_coverage AS (
  SELECT h.date_key, h.date_end, h.customer_id, h.site_id, COUNT(*) AS expected_count,
    SUM(CASE WHEN w.status = 'complete' THEN 1 ELSE 0 END) AS complete_count,
    SUM(CASE WHEN w.status = 'complete' AND w.verified_at >= h.date_end THEN 1 ELSE 0 END) AS verified_count
  FROM customer_hours AS h
  LEFT JOIN collection_window AS w ON w.site_id = h.site_id AND w.hour_ts = h.hour_ts
  GROUP BY h.date_key, h.date_end, h.customer_id, h.site_id
)`

var globalDailyCoverageCTE = dailyCoverageCTE + `,
global_coverage AS (
  SELECT date_key, date_end, SUM(expected_count) AS expected_count,
    SUM(complete_count) AS complete_count, SUM(verified_count) AS verified_count
  FROM site_coverage GROUP BY date_key, date_end
)`

var aggregationSpecs = []aggregationSpec{
	usageFactDailyAggregation(),
	hourlyAccountAggregation(),
	hourlyCustomerAggregation(),
	hourlySimpleAggregation("site", "site_stat_hourly", "site_id, hour_ts", "f.site_id, f.hour_ts", "COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END)"),
	hourlyGlobalAggregation(),
	hourlySimpleAggregation("model", "model_stat_hourly", "site_id, model_name, hour_ts", "f.site_id, f.model_name, f.hour_ts", "COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END)"),
	hourlySimpleAggregation("channel", "channel_stat_hourly", "site_id, channel_id, hour_ts", "f.site_id, f.channel_id, f.hour_ts", "COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END)"),
	dailyAccountAggregation(),
	dailyCustomerAggregation(),
	dailySiteAggregation(),
	dailyGlobalAggregation(),
	dailySimpleAggregation("model", "model_stat_daily", "site_id, model_name, date_key", "f.site_id, f.model_name", "COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END)"),
	dailySimpleAggregation("channel", "channel_stat_daily", "site_id, channel_id, date_key", "f.site_id, f.channel_id", "COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END)"),
}

func usageFactDailyAggregation() aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	return aggregationSpec{
		Name:   "usage_fact_daily",
		Actual: actualAggregationQuery("usage_fact_daily", "site_id, remote_user_id, username_snapshot, model_name, channel_id, date_key", "0", "''", "is_final", "site_id, remote_user_id, model_name, channel_id, date_key"),
		Expected: dailyCoverageCTE + `
SELECT CAST(JSON_ARRAY(f.site_id, f.remote_user_id,
  COALESCE(MIN(NULLIF(f.username_snapshot, '') COLLATE utf8mb4_bin), ''),
  f.model_name, f.channel_id, MIN(` + dateKey + `)) AS CHAR),
CAST(SUM(f.request_count) AS CHAR), CAST(SUM(f.quota) AS CHAR), CAST(SUM(f.token_used) AS CHAR),
'0', '', CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
JOIN site AS s ON s.id = f.site_id
JOIN site_coverage AS c ON c.site_id = f.site_id AND c.date_key = ` + dateKey + `
WHERE s.statistics_start_at IS NOT NULL AND s.statistics_start_at < f.hour_ts + 3600
  AND (s.statistics_end_at IS NULL OR s.statistics_end_at > f.hour_ts)
GROUP BY f.site_id, f.remote_user_id, f.model_name, f.channel_id, ` + dateKey + `,
  c.date_end, c.expected_count, c.complete_count, c.verified_count
ORDER BY f.site_id, f.remote_user_id, f.model_name, f.channel_id, ` + dateKey,
	}
}

func hourlyAccountAggregation() aggregationSpec {
	return aggregationSpec{
		Name:   "account_hourly",
		Actual: actualAggregationQuery("account_stat_hourly", "account_id, hour_ts", "0", "data_status", "0", "account_id, hour_ts"),
		Expected: `SELECT CAST(JSON_ARRAY(a.id, f.hour_ts) AS CHAR), CAST(SUM(f.request_count) AS CHAR),
CAST(SUM(f.quota) AS CHAR), CAST(SUM(f.token_used) AS CHAR), '0', 'complete', '0'
FROM account AS a
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)
GROUP BY a.id, f.hour_ts ORDER BY a.id, f.hour_ts`,
	}
}

func hourlyCustomerAggregation() aggregationSpec {
	return aggregationSpec{
		Name:   "customer_hourly",
		Actual: actualAggregationQuery("customer_stat_hourly", "customer_id, site_id, hour_ts", "active_users", "data_status", "0", "customer_id, site_id, hour_ts"),
		Expected: `SELECT CAST(JSON_ARRAY(c.id, a.site_id, f.hour_ts) AS CHAR), CAST(SUM(f.request_count) AS CHAR),
CAST(SUM(f.quota) AS CHAR), CAST(SUM(f.token_used) AS CHAR), CAST(COUNT(DISTINCT a.id) AS CHAR),
'complete', '0'
FROM account AS a
JOIN customer AS c ON c.id = a.customer_id
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)
  AND (c.statistics_paused_at IS NULL OR f.hour_ts < c.statistics_paused_at)
GROUP BY c.id, a.site_id, f.hour_ts ORDER BY c.id, a.site_id, f.hour_ts`,
	}
}

func hourlySimpleAggregation(name, table, actualKeys, expectedKeys, active string) aggregationSpec {
	return aggregationSpec{
		Name:   name + "_hourly",
		Actual: actualAggregationQuery(table, actualKeys, "active_users", "data_status", "0", actualKeys),
		Expected: `SELECT CAST(JSON_ARRAY(` + expectedKeys + `) AS CHAR), CAST(SUM(f.request_count) AS CHAR),
CAST(SUM(f.quota) AS CHAR), CAST(SUM(f.token_used) AS CHAR), CAST(` + active + ` AS CHAR),
'complete', '0'
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
GROUP BY ` + expectedKeys + ` ORDER BY ` + expectedKeys,
	}
}

func hourlyGlobalAggregation() aggregationSpec {
	return aggregationSpec{
		Name:   "global_hourly",
		Actual: actualAggregationQuery("global_stat_hourly", "hour_ts", "active_users", "data_status", "0", "hour_ts"),
		Expected: `WITH
hour_candidates AS (
  SELECT hour_ts FROM collection_window
  UNION SELECT hour_ts FROM usage_fact_hourly
),
hour_coverage AS (
  SELECT h.hour_ts, COUNT(*) AS expected_count,
    SUM(CASE WHEN w.status = 'complete' THEN 1 ELSE 0 END) AS complete_count
  FROM hour_candidates AS h
  JOIN site AS s
    ON s.statistics_start_at IS NOT NULL AND s.statistics_start_at < h.hour_ts + 3600
   AND (s.statistics_end_at IS NULL OR s.statistics_end_at > h.hour_ts)
  LEFT JOIN collection_window AS w ON w.site_id = s.id AND w.hour_ts = h.hour_ts
  GROUP BY h.hour_ts
),
metrics AS (
  SELECT f.hour_ts, SUM(f.request_count) AS request_count, SUM(f.quota) AS quota,
    SUM(f.token_used) AS token_used,
    COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN CONCAT(f.site_id, ':', f.remote_user_id) END) AS active_users
  FROM usage_fact_hourly AS f
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY f.hour_ts
)
SELECT CAST(JSON_ARRAY(c.hour_ts) AS CHAR), CAST(COALESCE(m.request_count, 0) AS CHAR),
CAST(COALESCE(m.quota, 0) AS CHAR), CAST(COALESCE(m.token_used, 0) AS CHAR),
CAST(COALESCE(m.active_users, 0) AS CHAR),
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
'0'
FROM hour_coverage AS c LEFT JOIN metrics AS m ON m.hour_ts = c.hour_ts
WHERE c.complete_count > 0 AND (m.hour_ts IS NOT NULL OR c.complete_count < c.expected_count)
ORDER BY c.hour_ts`,
	}
}

func dailyAccountAggregation() aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	return aggregationSpec{
		Name:   "account_daily",
		Actual: actualAggregationQuery("account_stat_daily", "account_id, date_key", "0", "data_status", "is_final", "account_id, date_key"),
		Expected: accountCoverageCTE + `,
metrics AS (
  SELECT a.id AS account_id, ` + dateKey + ` AS date_key,
    SUM(f.request_count) AS request_count, SUM(f.quota) AS quota, SUM(f.token_used) AS token_used
  FROM account AS a
  JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  WHERE a.remote_created_at < f.hour_ts + 3600
    AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)
  GROUP BY a.id, ` + dateKey + `
)
SELECT CAST(JSON_ARRAY(c.account_id, c.date_key) AS CHAR), CAST(COALESCE(m.request_count, 0) AS CHAR),
CAST(COALESCE(m.quota, 0) AS CHAR), CAST(COALESCE(m.token_used, 0) AS CHAR), '0',
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM account_coverage AS c
LEFT JOIN metrics AS m ON m.account_id = c.account_id AND m.date_key = c.date_key
WHERE c.complete_count > 0 AND (m.account_id IS NOT NULL OR c.complete_count < c.expected_count)
ORDER BY c.account_id, c.date_key`,
	}
}

func dailyCustomerAggregation() aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	return aggregationSpec{
		Name:   "customer_daily",
		Actual: actualAggregationQuery("customer_stat_daily", "customer_id, site_id, date_key", "active_users", "data_status", "is_final", "customer_id, site_id, date_key"),
		Expected: customerCoverageCTE + `,
metrics AS (
  SELECT c.id AS customer_id, a.site_id, ` + dateKey + ` AS date_key,
    SUM(f.request_count) AS request_count, SUM(f.quota) AS quota, SUM(f.token_used) AS token_used,
    COUNT(DISTINCT a.id) AS active_users
  FROM account AS a
  JOIN customer AS c ON c.id = a.customer_id
  JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  WHERE a.remote_created_at < f.hour_ts + 3600
    AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)
    AND (c.statistics_paused_at IS NULL OR f.hour_ts < c.statistics_paused_at)
  GROUP BY c.id, a.site_id, ` + dateKey + `
)
SELECT CAST(JSON_ARRAY(c.customer_id, c.site_id, c.date_key) AS CHAR),
CAST(COALESCE(m.request_count, 0) AS CHAR), CAST(COALESCE(m.quota, 0) AS CHAR),
CAST(COALESCE(m.token_used, 0) AS CHAR), CAST(COALESCE(m.active_users, 0) AS CHAR),
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM customer_coverage AS c
LEFT JOIN metrics AS m ON m.customer_id = c.customer_id AND m.site_id = c.site_id AND m.date_key = c.date_key
WHERE c.complete_count > 0 AND (m.customer_id IS NOT NULL OR c.complete_count < c.expected_count)
ORDER BY c.customer_id, c.site_id, c.date_key`,
	}
}

func dailySiteAggregation() aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	return aggregationSpec{
		Name:   "site_daily",
		Actual: actualAggregationQuery("site_stat_daily", "site_id, date_key", "active_users", "data_status", "is_final", "site_id, date_key"),
		Expected: dailyCoverageCTE + `,
metrics AS (
  SELECT f.site_id, ` + dateKey + ` AS date_key, SUM(f.request_count) AS request_count,
    SUM(f.quota) AS quota, SUM(f.token_used) AS token_used,
    COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END) AS active_users
  FROM usage_fact_hourly AS f
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY f.site_id, ` + dateKey + `
)
SELECT CAST(JSON_ARRAY(c.site_id, c.date_key) AS CHAR), CAST(COALESCE(m.request_count, 0) AS CHAR),
CAST(COALESCE(m.quota, 0) AS CHAR), CAST(COALESCE(m.token_used, 0) AS CHAR),
CAST(COALESCE(m.active_users, 0) AS CHAR),
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM site_coverage AS c LEFT JOIN metrics AS m ON m.site_id = c.site_id AND m.date_key = c.date_key
WHERE c.complete_count > 0 AND (m.site_id IS NOT NULL OR c.complete_count < c.expected_count)
ORDER BY c.site_id, c.date_key`,
	}
}

func dailyGlobalAggregation() aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	return aggregationSpec{
		Name:   "global_daily",
		Actual: actualAggregationQuery("global_stat_daily", "date_key", "active_users", "data_status", "is_final", "date_key"),
		Expected: globalDailyCoverageCTE + `,
metrics AS (
  SELECT ` + dateKey + ` AS date_key, SUM(f.request_count) AS request_count,
    SUM(f.quota) AS quota, SUM(f.token_used) AS token_used,
    COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN CONCAT(f.site_id, ':', f.remote_user_id) END) AS active_users
  FROM usage_fact_hourly AS f
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY ` + dateKey + `
)
SELECT CAST(JSON_ARRAY(c.date_key) AS CHAR), CAST(COALESCE(m.request_count, 0) AS CHAR),
CAST(COALESCE(m.quota, 0) AS CHAR), CAST(COALESCE(m.token_used, 0) AS CHAR),
CAST(COALESCE(m.active_users, 0) AS CHAR),
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM global_coverage AS c LEFT JOIN metrics AS m ON m.date_key = c.date_key
WHERE c.complete_count > 0 AND (m.date_key IS NOT NULL OR c.complete_count < c.expected_count)
ORDER BY c.date_key`,
	}
}

func dailySimpleAggregation(name, table, actualKeys, metricKeys, active string) aggregationSpec {
	dateKey := beijingDateKey("f.hour_ts")
	expectedKeys := qualifyAggregationColumns("m", actualKeys)
	return aggregationSpec{
		Name:   name + "_daily",
		Actual: actualAggregationQuery(table, actualKeys, "active_users", "data_status", "is_final", actualKeys),
		Expected: dailyCoverageCTE + `,
metrics AS (
  SELECT ` + metricKeys + `, ` + dateKey + ` AS date_key,
    SUM(f.request_count) AS request_count, SUM(f.quota) AS quota, SUM(f.token_used) AS token_used,
    ` + active + ` AS active_users
  FROM usage_fact_hourly AS f
  JOIN collection_window AS w
    ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY ` + metricKeys + `, ` + dateKey + `
)
SELECT CAST(JSON_ARRAY(` + expectedKeys + `) AS CHAR),
CAST(m.request_count AS CHAR), CAST(m.quota AS CHAR), CAST(m.token_used AS CHAR), CAST(m.active_users AS CHAR),
CASE WHEN c.expected_count > 0 AND c.complete_count = c.expected_count THEN 'complete' ELSE 'partial' END,
CAST(CASE WHEN UNIX_TIMESTAMP() >= c.date_end AND c.expected_count > 0
  AND c.complete_count = c.expected_count AND c.verified_count = c.expected_count THEN 1 ELSE 0 END AS CHAR)
FROM metrics AS m JOIN site_coverage AS c ON c.site_id = m.site_id AND c.date_key = m.date_key
ORDER BY ` + expectedKeys,
	}
}
