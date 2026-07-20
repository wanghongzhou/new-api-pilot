#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

fail() {
  printf 'backup metrics failed: %s\n' "$1" >&2
  exit 1
}

[[ "$#" -eq 4 ]] || fail "expected outcome, start timestamp, finish timestamp, and size"

outcome="$1"
started_at="$2"
finished_at="$3"
size_bytes="$4"
textfile_directory="${PROMETHEUS_TEXTFILE_DIR:-}"

[[ "$outcome" == "success" || "$outcome" == "failure" ]] || fail "outcome is invalid"
[[ "$started_at" =~ ^[0-9]+$ && "$finished_at" =~ ^[0-9]+$ && "$size_bytes" =~ ^[0-9]+$ ]] \
  || fail "metric values must be non-negative integers"
[[ "$finished_at" -ge "$started_at" ]] || fail "finish timestamp precedes start timestamp"

# Metrics are optional for development and mandatory only when the deployment
# configures the node-exporter textfile directory.
[[ -n "$textfile_directory" ]] || exit 0
[[ "$textfile_directory" = /* ]] || fail "PROMETHEUS_TEXTFILE_DIR must be absolute"
[[ -d "$textfile_directory" && ! -L "$textfile_directory" ]] \
  || fail "PROMETHEUS_TEXTFILE_DIR must be a real directory"

textfile_directory="$(realpath "$textfile_directory")"
metrics_file="${textfile_directory}/new_api_pilot_backup.prom"
[[ ! -L "$metrics_file" ]] || fail "backup metrics file must not be a symbolic link"

metric_value() {
  local name="$1"
  local fallback="$2"
  if [[ ! -f "$metrics_file" ]]; then
    printf '%s\n' "$fallback"
    return
  fi
  awk -v metric="$name" -v fallback="$fallback" '
    $1 == metric && $2 ~ /^[0-9]+$/ { value = $2 }
    END { print value == "" ? fallback : value }
  ' "$metrics_file"
}

last_success="$(metric_value new_api_pilot_backup_last_success_timestamp_seconds 0)"
last_failure="$(metric_value new_api_pilot_backup_last_failure_timestamp_seconds 0)"
failures_total="$(metric_value new_api_pilot_backup_failures_total 0)"
last_size="$(metric_value new_api_pilot_backup_last_size_bytes 0)"
duration_seconds=$((finished_at - started_at))

if [[ "$outcome" == "success" ]]; then
  last_success="$finished_at"
  last_size="$size_bytes"
else
  last_failure="$finished_at"
  failures_total=$((failures_total + 1))
fi

temporary_file="$(mktemp "${textfile_directory}/.new_api_pilot_backup.prom.XXXXXX")"
cleanup() {
  if [[ -n "${temporary_file:-}" ]]; then
    rm -f -- "$temporary_file"
  fi
}
trap cleanup EXIT

{
  printf '# HELP new_api_pilot_backup_last_success_timestamp_seconds Unix timestamp of the latest successful database backup.\n'
  printf '# TYPE new_api_pilot_backup_last_success_timestamp_seconds gauge\n'
  printf 'new_api_pilot_backup_last_success_timestamp_seconds %s\n' "$last_success"
  printf '# HELP new_api_pilot_backup_last_failure_timestamp_seconds Unix timestamp of the latest failed database backup attempt.\n'
  printf '# TYPE new_api_pilot_backup_last_failure_timestamp_seconds gauge\n'
  printf 'new_api_pilot_backup_last_failure_timestamp_seconds %s\n' "$last_failure"
  printf '# HELP new_api_pilot_backup_failures_total Total failed database backup attempts.\n'
  printf '# TYPE new_api_pilot_backup_failures_total counter\n'
  printf 'new_api_pilot_backup_failures_total %s\n' "$failures_total"
  printf '# HELP new_api_pilot_backup_last_duration_seconds Duration of the latest database backup attempt.\n'
  printf '# TYPE new_api_pilot_backup_last_duration_seconds gauge\n'
  printf 'new_api_pilot_backup_last_duration_seconds %s\n' "$duration_seconds"
  printf '# HELP new_api_pilot_backup_last_size_bytes Compressed size of the latest successful database backup.\n'
  printf '# TYPE new_api_pilot_backup_last_size_bytes gauge\n'
  printf 'new_api_pilot_backup_last_size_bytes %s\n' "$last_size"
} >"$temporary_file"

chmod 0644 "$temporary_file"
mv -f -- "$temporary_file" "$metrics_file"
temporary_file=""
trap - EXIT
