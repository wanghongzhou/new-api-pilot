#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

fail() {
  printf 'restore failed: %s\n' "$1" >&2
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

require_regular_absolute_file() {
  local path="$1"
  [[ "$path" = /* ]] || fail "path must be absolute: $path"
  reject_symlink_path "$path"
  [[ -f "$path" && ! -L "$path" ]] || fail "path must be a regular file: $path"
}

verify_sidecar() {
  local file="$1"
  local sidecar="$2"
  local expected_name="$3"
  require_regular_absolute_file "$file"
  require_regular_absolute_file "$sidecar"
  local expected_hash listed_name extra actual_hash
  read -r expected_hash listed_name extra <"$sidecar" || fail "cannot read checksum sidecar: $sidecar"
  listed_name="${listed_name#\*}"
  [[ -z "${extra:-}" && "$expected_hash" =~ ^[0-9a-f]{64}$ && "$listed_name" == "$expected_name" ]] \
    || fail "checksum sidecar is invalid: $sidecar"
  actual_hash="$(sha256sum "$file" | awk '{print $1}')"
  [[ "$actual_hash" == "$expected_hash" ]] || fail "checksum mismatch: $file"
}

key_fingerprint() {
  local decoded_size
  decoded_size="$(printf '%s' "$ENCRYPTION_KEY" | base64 --decode 2>/dev/null | wc -c | tr -d '[:space:]')"
  [[ "$decoded_size" == "32" ]] || fail "ENCRYPTION_KEY must be Base64-encoded 32 bytes"
  printf '%s' "$ENCRYPTION_KEY" | base64 --decode 2>/dev/null | sha256sum | awk '{print $1}'
}

[[ "$#" -eq 0 ]] || fail "this script does not accept command-line arguments"

for command_name in base64 gzip jq mysql realpath sha256sum stat; do
  require_command "$command_name"
done

: "${RESTORE_MANIFEST:?RESTORE_MANIFEST is required}"
: "${RESTORE_RELEASE_ROOT:?RESTORE_RELEASE_ROOT is required}"
: "${MYSQL_DATABASE:?MYSQL_DATABASE is required}"
: "${DATABASE_DSN:?DATABASE_DSN is required for full verification}"
: "${ENCRYPTION_KEY:?ENCRYPTION_KEY is required}"

MYSQL_DEFAULTS_FILE="${MYSQL_DEFAULTS_FILE:-/run/secrets/mysql-client.cnf}"
NEW_API_PILOT_BIN="${NEW_API_PILOT_BIN:-/usr/local/bin/new-api-pilot}"

[[ "$MYSQL_DATABASE" =~ ^[A-Za-z0-9_]+$ ]] || fail "MYSQL_DATABASE is invalid"
case "$DATABASE_DSN" in
  *"/${MYSQL_DATABASE}?"* | *"/${MYSQL_DATABASE}") ;;
  *) fail "DATABASE_DSN must select MYSQL_DATABASE" ;;
esac

require_regular_absolute_file "$MYSQL_DEFAULTS_FILE"
defaults_mode="$(stat -c '%a' "$MYSQL_DEFAULTS_FILE")"
[[ "$defaults_mode" == "600" || "$defaults_mode" == "400" ]] \
  || fail "MYSQL_DEFAULTS_FILE must have mode 0600 or 0400"
require_regular_absolute_file "$NEW_API_PILOT_BIN"
[[ -x "$NEW_API_PILOT_BIN" ]] || fail "NEW_API_PILOT_BIN must be executable"

require_regular_absolute_file "$RESTORE_MANIFEST"
[[ "$(basename "$RESTORE_MANIFEST")" == "manifest.json" ]] || fail "manifest filename must be manifest.json"
manifest_path="$(realpath -e "$RESTORE_MANIFEST")"
manifest_directory="$(dirname "$manifest_path")"
preflight_report=""
if ! preflight_report="$("$NEW_API_PILOT_BIN" verify-backup \
  --mode=manifest-only --manifest="$manifest_path")"; then
  fail "backup manifest preflight failed"
fi
printf '%s\n' "$preflight_report" \
  | jq -e '.status == "success" and .summary.failed == 0' >/dev/null \
  || fail "backup manifest preflight did not pass"
verify_sidecar "$manifest_path" "${manifest_path}.sha256" 'manifest.json'

jq -e '
  .schema_version == 1
  and (.backup_id | type == "string" and test("^backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8,64}$"))
  and (.dump_file | type == "string" and test("^[A-Za-z0-9][A-Za-z0-9._-]*$"))
  and (.dump_sha256 | type == "string" and test("^[0-9a-f]{64}$"))
  and (.dump_size_bytes | type == "number" and . > 0)
  and (.encryption_key_id | type == "string" and test("^[0-9a-f]{64}$"))
' "$manifest_path" >/dev/null || fail "manifest fields are invalid"

backup_id="$(jq -r '.backup_id' "$manifest_path")"
dump_name="$(jq -r '.dump_file' "$manifest_path")"
dump_path="${manifest_directory}/${dump_name}"
require_regular_absolute_file "$dump_path"
dump_path="$(realpath -e "$dump_path")"
[[ "$(dirname "$dump_path")" == "$manifest_directory" ]] || fail "dump must remain inside the manifest directory"
verify_sidecar "$dump_path" "${dump_path}.sha256" "$dump_name"
manifest_dump_hash="$(jq -r '.dump_sha256' "$manifest_path")"
manifest_dump_size="$(jq -r '.dump_size_bytes' "$manifest_path")"
actual_dump_hash="$(sha256sum "$dump_path" | awk '{print $1}')"
actual_dump_size="$(stat -c '%s' "$dump_path")"
[[ "$actual_dump_hash" == "$manifest_dump_hash" && "$actual_dump_size" == "$manifest_dump_size" ]] \
  || fail "dump hash or size does not match manifest"
gzip -t "$dump_path"

expected_key_id="$(jq -r '.encryption_key_id' "$manifest_path")"
actual_key_id="$(key_fingerprint)"
[[ "$actual_key_id" == "$expected_key_id" ]] || fail "ENCRYPTION_KEY does not match manifest"

defaults_identity="$(
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names \
    "$MYSQL_DATABASE" --execute='SELECT @@server_uuid, DATABASE()'
)"
defaults_server_uuid=""
defaults_database=""
defaults_extra=""
IFS=$'\t' read -r defaults_server_uuid defaults_database defaults_extra <<<"$defaults_identity" || true
[[ -z "${defaults_extra:-}" && "$defaults_server_uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ \
  && "$defaults_database" == "$MYSQL_DATABASE" ]] || fail "MYSQL_DEFAULTS_FILE target identity is invalid"

dsn_identity_report=""
if ! dsn_identity_report="$("$NEW_API_PILOT_BIN" database-identity)"; then
  fail "DATABASE_DSN target identity query failed"
fi
printf '%s\n' "$dsn_identity_report" \
  | jq -e --arg database "$MYSQL_DATABASE" '
      .schema_version == 1
      and .command == "database-identity"
      and .status == "success"
      and .database == $database
      and (.server_uuid | type == "string" and test("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"))
    ' >/dev/null || fail "DATABASE_DSN target identity report is invalid"
dsn_server_uuid="$(printf '%s\n' "$dsn_identity_report" | jq -r '.server_uuid')"
[[ "$defaults_server_uuid" == "$dsn_server_uuid" ]] \
  || fail "MYSQL_DEFAULTS_FILE and DATABASE_DSN must identify the same MySQL server"

database_exists="$(
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names \
    --execute="SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = '${MYSQL_DATABASE}'"
)"
[[ "$database_exists" == "1" ]] || fail "target database does not exist"
table_count="$(
  mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --batch --skip-column-names \
    --execute="SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '${MYSQL_DATABASE}'"
)"
[[ "$table_count" == "0" ]] || fail "target database must be empty"

[[ "$RESTORE_RELEASE_ROOT" = /* ]] || fail "RESTORE_RELEASE_ROOT must be absolute"
reject_symlink_path "$RESTORE_RELEASE_ROOT"
mkdir -p "$RESTORE_RELEASE_ROOT"
reject_symlink_path "$RESTORE_RELEASE_ROOT"
RESTORE_RELEASE_ROOT="$(realpath -e "$RESTORE_RELEASE_ROOT")"
release_directory="${RESTORE_RELEASE_ROOT}/${backup_id}"
staging_release="${RESTORE_RELEASE_ROOT}/.${backup_id}.tmp.$$"
[[ ! -e "$release_directory" && ! -e "$staging_release" ]] || fail "restore release already exists"
mkdir "$staging_release"
published=0
cleanup() {
  if [[ "$published" -eq 0 ]]; then
    rm -rf -- "$staging_release"
  fi
}
trap cleanup EXIT

gzip -cd "$dump_path" \
  | mysql --defaults-extra-file="$MYSQL_DEFAULTS_FILE" "$MYSQL_DATABASE"

verify_report="${staging_release}/verify-report.json"
"$NEW_API_PILOT_BIN" verify-restore --mode=full --manifest="$manifest_path" >"$verify_report"
jq -e '.status == "success" and .summary.failed == 0' "$verify_report" >/dev/null \
  || fail "full restore verification did not pass"

manifest_sha256="$(sha256sum "$manifest_path" | awk '{print $1}')"
jq -n \
  --arg backup_id "$backup_id" \
  --arg source_manifest "$manifest_path" \
  --arg manifest_sha256 "$manifest_sha256" \
  --arg encryption_key_id "${actual_key_id:0:12}" \
  --arg verified_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{status:"verified", backup_id:$backup_id, source_manifest:$source_manifest,
    manifest_sha256:$manifest_sha256, encryption_key_id:$encryption_key_id,
    verified_at_utc:$verified_at_utc}' >"${staging_release}/release.json"

mv -- "$staging_release" "$release_directory"
published=1
trap - EXIT

jq -n \
  --arg backup_id "$backup_id" \
  --arg release_directory "$release_directory" \
  --arg manifest_sha256 "$manifest_sha256" \
  --arg encryption_key_id "${actual_key_id:0:12}" \
  '{status:"success", backup_id:$backup_id, release_directory:$release_directory,
    manifest_sha256:$manifest_sha256, encryption_key_id:$encryption_key_id}'
