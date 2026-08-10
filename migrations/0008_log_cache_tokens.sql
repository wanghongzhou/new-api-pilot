ALTER TABLE upstream_log_fact
  ADD COLUMN cache_read_tokens BIGINT NOT NULL DEFAULT 0 AFTER completion_tokens,
  ADD COLUMN cache_creation_tokens BIGINT NOT NULL DEFAULT 0 AFTER cache_read_tokens,
  ADD COLUMN cache_creation_tokens_5m BIGINT NOT NULL DEFAULT 0 AFTER cache_creation_tokens,
  ADD COLUMN cache_creation_tokens_1h BIGINT NOT NULL DEFAULT 0 AFTER cache_creation_tokens_5m;
