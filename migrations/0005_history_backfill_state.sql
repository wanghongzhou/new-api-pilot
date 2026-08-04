ALTER TABLE site_performance_collection_state
  ADD COLUMN backfill_completed_at BIGINT NULL AFTER last_success_at;

ALTER TABLE upstream_log_collection_state
  ADD COLUMN history_start_at BIGINT NULL AFTER window_end,
  ADD COLUMN backfill_completed_at BIGINT NULL AFTER last_success_at;

ALTER TABLE site_upstream_task_collection_state
  ADD COLUMN backfill_completed_at BIGINT NULL AFTER last_success_at;
