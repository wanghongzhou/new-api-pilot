ALTER TABLE upstream_log_fact
  ADD COLUMN first_response_time_ms BIGINT NULL AFTER is_stream,
  ADD COLUMN stream_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER first_response_time_ms,
  ADD COLUMN stream_end_reason VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '' AFTER stream_status,
  ADD COLUMN stream_error_count BIGINT NOT NULL DEFAULT 0 AFTER stream_end_reason;
