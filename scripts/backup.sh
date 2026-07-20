#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
BACKUP_METRICS_HELPER="${SCRIPT_DIRECTORY}/backup_metrics.sh"

fail() {
  printf 'backup failed: %s\n' "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command is unavailable: $1"
}

reject_symlink_path() {
  local current="$1"
  while [[ "$current" != "/" ]]; do
    [[ ! -L "$current" ]] || fail "path contains a symbolic link: $current"
    current="$(dirname "$current")"
  done
}

require_private_defaults_file() {
  local path="$1"
  [[ "$path" = /* ]] || fail "MYSQL_DEFAULTS_FILE must be absolute"
  reject_symlink_path "$path"
  [[ -f "$path" && ! -L "$path" ]] || fail "MYSQL_DEFAULTS_FILE must be a regular file"
  local mode
  mode="$(stat -c '%a' "$path")"
  [[ "$mode" == "600" || "$mode" == "400" ]] || fail "MYSQL_DEFAULTS_FILE must have mode 0600 or 0400"
}

key_fingerprint() {
  local decoded_size
  decoded_size="$(printf '%s' "$ENCRYPTION_KEY" | base64 --decode 2>/dev/null | wc -c | tr -d '[:space:]')"
  [[ "$decoded_size" == "32" ]] || fail "ENCRYPTION_KEY must be Base64-encoded 32 bytes"
  printf '%s' "$ENCRYPTION_KEY" | base64 --decode 2>/dev/null | sha256sum | awk '{print $1}'
}

migration_lock_pid=""
migration_lock_output=""

release_migration_lock() {
  if [[ -n "$migration_lock_pid" ]]; then
    kill "$migration_lock_pid" 2>/dev/null || true
    wait "$migration_lock_pid" 2>/dev/null || true
    migration_lock_pid=""
  fi
  if [[ -n "$migration_lock_output" ]]; then
    rm -f -- "$migration_lock_output"
  fi
}

acquire_migration_lock() {
  : >"$migration_lock_output"
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names --unbuffered \
    --execute="
SELECT GET_LOCK('new-api-pilot:migration-runner', 60);
SELECT SLEEP(86400)
WHERE IS_USED_LOCK('new-api-pilot:migration-runner') = CONNECTION_ID();
" >"$migration_lock_output" &
  migration_lock_pid=$!

  local attempt=0
  while [[ ! -s "$migration_lock_output" && "$attempt" -lt 650 ]]; do
    if ! kill -0 "$migration_lock_pid" 2>/dev/null; then
      wait "$migration_lock_pid" 2>/dev/null || true
      migration_lock_pid=""
      fail "migration advisory lock connection exited before acquisition"
    fi
    sleep 0.1
    attempt=$((attempt + 1))
  done
  [[ -s "$migration_lock_output" ]] || fail "timed out waiting for migration advisory lock result"
  local acquired=""
  IFS= read -r acquired <"$migration_lock_output" || true
  [[ "$acquired" == "1" ]] || fail "could not acquire migration advisory lock"
}

[[ "$#" -eq 0 ]] || fail "this script does not accept command-line arguments"

for command_name in base64 gzip jq mysql mysqldump realpath sed sha256sum stat; do
  require_command "$command_name"
done

: "${BACKUP_ROOT:?BACKUP_ROOT is required}"
: "${MYSQL_DATABASE:?MYSQL_DATABASE is required}"
: "${IMAGE_DIGEST:?IMAGE_DIGEST is required}"
: "${ENCRYPTION_KEY:?ENCRYPTION_KEY is required}"

MYSQL_DEFAULTS_FILE="${MYSQL_DEFAULTS_FILE:-/run/secrets/mysql-client.cnf}"
[[ "$BACKUP_ROOT" = /* ]] || fail "BACKUP_ROOT must be absolute"
[[ "$MYSQL_DATABASE" =~ ^[A-Za-z0-9_]+$ ]] || fail "MYSQL_DATABASE is invalid"
[[ "$IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "IMAGE_DIGEST must be a sha256 image digest"
require_private_defaults_file "$MYSQL_DEFAULTS_FILE"

reject_symlink_path "$BACKUP_ROOT"
mkdir -p "$BACKUP_ROOT"
reject_symlink_path "$BACKUP_ROOT"
BACKUP_ROOT="$(realpath -e "$BACKUP_ROOT")"
[[ -d "$BACKUP_ROOT" ]] || fail "BACKUP_ROOT must be a directory"

utc_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
random_suffix="$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
backup_id="backup-${utc_timestamp}-${random_suffix}"
staging_directory="${BACKUP_ROOT}/.${backup_id}.tmp.$$"
final_directory="${BACKUP_ROOT}/${backup_id}"
[[ ! -e "$staging_directory" && ! -e "$final_directory" ]] || fail "backup destination already exists"
mkdir "$staging_directory"
published=0
backup_started_at="$(date +%s)"
backup_metric_size=0
backup_metrics_reported=0

report_backup_metrics() {
  local outcome="$1"
  local finished_at
  finished_at="$(date +%s)"
  if ! PROMETHEUS_TEXTFILE_DIR="${PROMETHEUS_TEXTFILE_DIR:-}" \
    bash "$BACKUP_METRICS_HELPER" "$outcome" "$backup_started_at" "$finished_at" "$backup_metric_size"; then
    printf 'backup metrics write failed\n' >&2
  fi
  backup_metrics_reported=1
}

cleanup() {
  release_migration_lock
  if [[ "$published" -eq 0 ]]; then
    rm -rf -- "$staging_directory"
  fi
}

on_exit() {
  local exit_status="$?"
  cleanup
  if [[ "$exit_status" -ne 0 && "$backup_metrics_reported" -eq 0 ]]; then
    report_backup_metrics failure
  fi
  trap - EXIT
  exit "$exit_status"
}
trap on_exit EXIT

migration_lock_output="$staging_directory/.migration-lock"
acquire_migration_lock

dump_name="database.sql.gz"
dump_path="${staging_directory}/${dump_name}"
mysqldump --defaults-extra-file="$MYSQL_DEFAULTS_FILE" \
  --single-transaction --quick --routines --events --hex-blob \
  --source-data=2 --set-gtid-purged=OFF "$MYSQL_DATABASE" \
  | gzip -c >"$dump_path"
[[ -s "$dump_path" ]] || fail "mysqldump produced an empty file"
gzip -t "$dump_path"

source_record="$(
  gzip -cd "$dump_path" \
    | sed -n "s/.*SOURCE_LOG_FILE='\([^']*\)', SOURCE_LOG_POS=\([0-9][0-9]*\).*/\1\t\2/p" \
    | sed -n '1p'
)"
[[ "$source_record" == *$'\t'* ]] || fail "dump does not contain a source file/position coordinate"
source_log_file="${source_record%%$'\t'*}"
source_log_position="${source_record#*$'\t'}"
[[ "$source_log_file" =~ ^[A-Za-z0-9._-]+$ && "$source_log_position" =~ ^[1-9][0-9]*$ ]] \
  || fail "dump source coordinate is invalid"

dump_sha256="$(sha256sum "$dump_path" | awk '{print $1}')"
dump_size_bytes="$(stat -c '%s' "$dump_path")"
printf '%s  %s\n' "$dump_sha256" "$dump_name" >"${dump_path}.sha256"

server_record="$(
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names \
    "$MYSQL_DATABASE" --execute='SELECT VERSION(), @@server_uuid'
)"
mysql_version="${server_record%%$'\t'*}"
server_uuid="${server_record#*$'\t'}"
[[ -n "$mysql_version" && -n "$server_uuid" && "$server_record" == *$'\t'* ]] \
  || fail "could not read MySQL identity"

migration_rows="$(
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names \
    "$MYSQL_DATABASE" --execute='SELECT version, checksum FROM schema_migration ORDER BY version ASC'
)"
migrations_json="$(
  printf '%s\n' "$migration_rows" \
    | jq -R -s -e '
        split("\n")
        | map(select(length > 0) | split("\t"))
        | if length == 0 or any(.[]; length != 2 or (.[1] | test("^[0-9a-f]{64}$") | not))
          then error("invalid schema_migration rows")
          else map({version: .[0], checksum: .[1]})
          end
      '
)" || fail "schema_migration rows are missing or invalid"
release_migration_lock

encryption_key_id="$(key_fingerprint)"
manifest_path="${staging_directory}/manifest.json"
jq -n \
  --arg backup_id "$backup_id" \
  --arg created_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg database "$MYSQL_DATABASE" \
  --arg dump_file "$dump_name" \
  --arg dump_sha256 "$dump_sha256" \
  --argjson dump_size_bytes "$dump_size_bytes" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg encryption_key_id "$encryption_key_id" \
  --arg mysql_version "$mysql_version" \
  --arg server_uuid "$server_uuid" \
  --arg log_file "$source_log_file" \
  --argjson log_position "$source_log_position" \
  --argjson schema_migrations "$migrations_json" \
  '{
    schema_version: 1,
    backup_id: $backup_id,
    created_at_utc: $created_at_utc,
    database: $database,
    dump_file: $dump_file,
    dump_sha256: $dump_sha256,
    dump_size_bytes: $dump_size_bytes,
    image_digest: $image_digest,
    encryption_key_id: $encryption_key_id,
    mysql_version: $mysql_version,
    server_uuid: $server_uuid,
    source: {log_file: $log_file, log_position: $log_position},
    schema_migrations: $schema_migrations,
    export_files: "excluded_regenerable"
  }' >"$manifest_path"

manifest_sha256="$(sha256sum "$manifest_path" | awk '{print $1}')"
printf '%s  %s\n' "$manifest_sha256" 'manifest.json' >"${manifest_path}.sha256"

mv -- "$staging_directory" "$final_directory"
published=1
backup_metric_size="$dump_size_bytes"
report_backup_metrics success

jq -n \
  --arg backup_id "$backup_id" \
  --arg manifest "${final_directory}/manifest.json" \
  --arg manifest_sha256 "$manifest_sha256" \
  --arg dump_sha256 "$dump_sha256" \
  --arg encryption_key_id "${encryption_key_id:0:12}" \
  '{status:"success", backup_id:$backup_id, manifest:$manifest,
    manifest_sha256:$manifest_sha256, dump_sha256:$dump_sha256,
    encryption_key_id:$encryption_key_id}'
