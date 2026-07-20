-- F03 deterministic statistics fixture.
-- Preconditions: an empty MySQL 8.4 schema migrated from the authoritative DDL.
-- Fixed range: [2026-01-15 00:00, 2026-01-17 00:00) Asia/Shanghai.
-- The fixture deliberately leaves aggregate tables empty so rebuild tests must
-- derive all six scopes from usage_fact_hourly + complete collection_window.

START TRANSACTION;

SET @fixture_now := 1768622400;
SET @base_hour := 1768406400;

INSERT INTO site (
  id, name, base_url, config_version, remark, management_status,
  online_status, auth_status, statistics_status, health_status,
  root_user_id, root_created_at, access_token_encrypted, version, system_name,
  quota_per_unit, usd_exchange_rate, last_rate_at, data_export_enabled,
  current_rpm, current_tpm, last_realtime_stat_at, probe_fail_count,
  last_probe_at, last_probe_success_at, statistics_start_at,
  statistics_start_source, statistics_end_at, disabled_at,
  monitoring_start_at, created_at, updated_at
) VALUES
  (
    101, 'Fixture Alpha', 'https://alpha.fixture.invalid', 3, '', 'active',
    'online', 'authorized', 'partial', 'warning',
    1, 1735660800, 'v1:fixture-nonce:fixture-ciphertext',
    'v0.6.11-fixture', 'Fixture Alpha', 500000.0000000000,
    7.2000000000, @fixture_now, 1, 120, 24000, @fixture_now, 0,
    @fixture_now, @fixture_now, @base_hour, 'root_created_at', NULL, NULL,
    @base_hour, @base_hour, @fixture_now
  ),
  (
    102, 'Fixture Beta', 'https://beta.fixture.invalid', 5, '', 'disabled',
    'online', 'authorized', 'paused', 'unavailable',
    1, 1735660800, 'v1:fixture-nonce:fixture-ciphertext',
    'v0.6.11-fixture', 'Fixture Beta', 1000000.0000000000,
    7.1000000000, @fixture_now, 1, 80, 16000, @fixture_now, 0,
    @fixture_now, @fixture_now, @base_hour, 'root_created_at', NULL,
    @base_hour + 44 * 3600, @base_hour, @base_hour, @fixture_now
  );

INSERT INTO site_channel (
  id, site_id, remote_channel_id, name, last_synced_at, remote_missing,
  created_at, updated_at
) VALUES
  (401, 101, 1, 'Alpha Primary', @fixture_now, 0, @base_hour, @fixture_now),
  (402, 102, 1, 'Beta Primary', @fixture_now, 0, @base_hour, @fixture_now),
  (403, 101, 9, 'Historical Channel', @fixture_now, 1, @base_hour, @fixture_now);

INSERT INTO customer (
  id, name, contact, remark, status, statistics_paused_at,
  statistics_backfill_status, created_at, updated_at
) VALUES
  (201, 'Fixture Customer One', '', '', 'using', NULL, 'none', @base_hour, @fixture_now),
  (202, 'Fixture Customer Two', '', '', 'disabled', @base_hour + 44 * 3600,
   'failed', @base_hour, @fixture_now);

INSERT INTO account (
  id, site_id, customer_id, remote_user_id, remote_created_at, username,
  display_name, remote_group, remote_status, remote_state,
  remote_missing_count, last_remote_seen_at, quota, used_quota,
  request_count, managed_status, statistics_paused_at,
  statistics_backfill_status, last_synced_at, remark, created_at, updated_at
) VALUES
  (301, 101, 201, 1001, @base_hour - 86400, 'alpha-user-1', 'Alpha User 1',
   'default', 1, 'normal', 0, @fixture_now, 9007199254740993,
   9007199254740994, 9007199254740995, 'active', NULL, 'none',
   @fixture_now, '', @base_hour, @fixture_now),
  (302, 101, 201, 1002, @base_hour + 6 * 3600, 'alpha-user-2', 'Alpha User 2',
   'default', 1, 'normal', 0, @fixture_now, 500000, 250000, 250,
   'active', NULL, 'none', @fixture_now, '', @base_hour, @fixture_now),
  (303, 102, 202, 2001, @base_hour - 86400, 'beta-user-1', 'Beta User 1',
   'default', 1, 'normal', 0, @fixture_now, 700000, 300000, 300,
   'active', @base_hour + 44 * 3600, 'failed', @fixture_now, '',
   @base_hour, @fixture_now),
  (304, 102, 202, 2002, @base_hour + 12 * 3600, 'beta-user-2', 'Beta User 2',
   'special', 2, 'missing', 2, @base_hour + 40 * 3600, 0, 1000000, 1000,
   'archived', @base_hour + 42 * 3600, 'none', @fixture_now, '',
   @base_hour, @fixture_now);

CREATE TEMPORARY TABLE fixture_hour (
  hour_offset INT NOT NULL PRIMARY KEY,
  hour_ts BIGINT NOT NULL
);

INSERT INTO fixture_hour (hour_offset, hour_ts)
WITH RECURSIVE fixture_sequence(n) AS (
  SELECT 0
  UNION ALL
  SELECT n + 1 FROM fixture_sequence WHERE n < 47
)
SELECT n, @base_hour + n * 3600 FROM fixture_sequence;

INSERT INTO collection_window (
  site_id, hour_ts, status, fetched_rows, source_hash, last_fact_run_id,
  verified_at, last_error_code, last_error_params, last_error_message,
  updated_at
)
SELECT
  101,
  hour_ts,
  CASE hour_offset
    WHEN 45 THEN 'missing'
    WHEN 46 THEN 'pending'
    WHEN 47 THEN 'unavailable'
    ELSE 'complete'
  END,
  CASE WHEN hour_offset IN (45, 46, 47) THEN 0 ELSE 2 END,
  CASE
    WHEN hour_offset IN (45, 46, 47) THEN ''
    ELSE SHA2(CONCAT('F03:101:', hour_ts), 256)
  END,
  NULL,
  CASE WHEN hour_offset < 45 THEN @fixture_now ELSE NULL END,
  CASE
    WHEN hour_offset = 45 THEN 'DATA_VALIDATION_MISMATCH'
    WHEN hour_offset = 47 THEN 'DATA_UPSTREAM_UNAVAILABLE'
    ELSE ''
  END,
  NULL,
  NULL,
  @fixture_now
FROM fixture_hour;

INSERT INTO collection_window (
  site_id, hour_ts, status, fetched_rows, source_hash, last_fact_run_id,
  verified_at, last_error_code, last_error_params, last_error_message,
  updated_at
)
SELECT
  102,
  hour_ts,
  'complete',
  2,
  SHA2(CONCAT('F03:102:', hour_ts), 256),
  NULL,
  @fixture_now,
  '',
  NULL,
  NULL,
  @fixture_now
FROM fixture_hour
WHERE hour_offset < 44;

-- Case-sensitive model rows and channel 0 exercise the documented identity keys.
-- Offset 12 for site 101 and offset 13 for site 102 are complete zero windows.
INSERT INTO usage_fact_hourly (
  site_id, remote_user_id, username_snapshot, model_name, channel_id,
  hour_ts, request_count, quota, token_used, collected_at
)
SELECT
  101,
  1001,
  'alpha-user-1',
  'Model-A',
  1,
  h.hour_ts,
  10 + h.hour_offset,
  1000 + h.hour_offset * 10,
  100 + h.hour_offset,
  @fixture_now
FROM fixture_hour h
JOIN collection_window w
  ON w.site_id = 101 AND w.hour_ts = h.hour_ts AND w.status = 'complete'
WHERE h.hour_offset <> 12;

INSERT INTO usage_fact_hourly (
  site_id, remote_user_id, username_snapshot, model_name, channel_id,
  hour_ts, request_count, quota, token_used, collected_at
)
SELECT
  101,
  1002,
  'alpha-user-2',
  'model-a',
  0,
  h.hour_ts,
  1,
  100,
  10,
  @fixture_now
FROM fixture_hour h
JOIN collection_window w
  ON w.site_id = 101 AND w.hour_ts = h.hour_ts AND w.status = 'complete'
WHERE MOD(h.hour_offset, 2) = 0 AND h.hour_offset >= 6 AND h.hour_offset <> 12;

INSERT INTO usage_fact_hourly (
  site_id, remote_user_id, username_snapshot, model_name, channel_id,
  hour_ts, request_count, quota, token_used, collected_at
)
SELECT
  102,
  2001,
  'beta-user-1',
  'Model-A',
  1,
  h.hour_ts,
  5 + h.hour_offset,
  500 + h.hour_offset * 10,
  50 + h.hour_offset,
  @fixture_now
FROM fixture_hour h
WHERE h.hour_offset < 44 AND h.hour_offset <> 13;

INSERT INTO usage_fact_hourly (
  site_id, remote_user_id, username_snapshot, model_name, channel_id,
  hour_ts, request_count, quota, token_used, collected_at
)
SELECT
  102,
  2002,
  'beta-user-2',
  'model-a',
  0,
  h.hour_ts,
  2,
  200,
  20,
  @fixture_now
FROM fixture_hour h
WHERE h.hour_offset BETWEEN 12 AND 41
  AND MOD(h.hour_offset, 3) = 0
  AND h.hour_offset <> 13;

DROP TEMPORARY TABLE fixture_hour;

COMMIT;
