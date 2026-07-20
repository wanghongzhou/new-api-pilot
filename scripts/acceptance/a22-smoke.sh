#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

for command_name in curl jq sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'A22 smoke prerequisite unavailable\n' >&2
    exit 1
  }
done

: "${A22_ADMIN_PASSWORD:?A22_ADMIN_PASSWORD is required}"
: "${A22_EXPECTED_DATABASE:?A22_EXPECTED_DATABASE is required}"
: "${A22_SOURCE_UUID_FINGERPRINT:?A22_SOURCE_UUID_FINGERPRINT is required}"
: "${A22_TARGET_UUID_FINGERPRINT:?A22_TARGET_UUID_FINGERPRINT is required}"
: "${A22_RELEASE_GATE:?A22_RELEASE_GATE is required}"

[[ -f "$A22_RELEASE_GATE" ]] || {
  printf 'A22 release gate is absent\n' >&2
  exit 1
}

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf -- "$temporary_directory"
}
trap cleanup EXIT

identity="$(/work/new-api-pilot database-identity)"
database="$(printf '%s\n' "$identity" | jq -r 'select(.status == "success") | .database')"
server_uuid="$(printf '%s\n' "$identity" | jq -r 'select(.status == "success") | .server_uuid')"
observed_fingerprint="$(printf '%s' "$server_uuid" | sha256sum | awk '{print substr($1,1,12)}')"
connected_to_target=false
if [[ "$database" == "$A22_EXPECTED_DATABASE" && "$observed_fingerprint" == "$A22_TARGET_UUID_FINGERPRINT" \
  && "$observed_fingerprint" != "$A22_SOURCE_UUID_FINGERPRINT" ]]; then
  connected_to_target=true
fi

health_status=0
ready_status=0
deadline=$((SECONDS + 120))
while (( SECONDS < deadline )); do
  health_status="$(curl --silent --show-error --output "$temporary_directory/health.json" \
    --write-out '%{http_code}' http://127.0.0.1:3000/healthz || true)"
  ready_status="$(curl --silent --show-error --output "$temporary_directory/ready.json" \
    --write-out '%{http_code}' http://127.0.0.1:3000/readyz || true)"
  if [[ "$health_status" == "200" && "$ready_status" == "200" ]]; then
    break
  fi
  sleep 1
done

login_payload="$(jq -n --arg username admin --arg password "$A22_ADMIN_PASSWORD" \
  '{username:$username,password:$password}')"
login_status="$(curl --silent --show-error --output "$temporary_directory/login.json" \
  --write-out '%{http_code}' --cookie-jar "$temporary_directory/cookies" \
  --header 'Content-Type: application/json' --data "$login_payload" \
  http://127.0.0.1:3000/api/user/login || true)"
self_user_id="$(jq -r 'select(.success == true) | .data.id // empty' "$temporary_directory/login.json")"
self_status="$(curl --silent --show-error --output "$temporary_directory/self.json" \
  --write-out '%{http_code}' --cookie "$temporary_directory/cookies" \
  --header "New-Api-User: $self_user_id" \
  http://127.0.0.1:3000/api/user/self || true)"
sites_status="$(curl --silent --show-error --output "$temporary_directory/sites.json" \
  --write-out '%{http_code}' --cookie "$temporary_directory/cookies" \
  --header "New-Api-User: $self_user_id" \
  'http://127.0.0.1:3000/api/sites?p=1&page_size=20' || true)"

fixture_site_found=false
if [[ "$sites_status" == "200" ]] && jq -e --arg name 'A22 恢复演练站点' \
  '.. | objects | select(.name? == $name)' "$temporary_directory/sites.json" >/dev/null; then
  fixture_site_found=true
fi

status=failed
if [[ "$health_status" == "200" && "$ready_status" == "200" && "$login_status" == "200" \
  && "$self_user_id" =~ ^[1-9][0-9]*$ \
  && "$self_status" == "200" && "$sites_status" == "200" && "$fixture_site_found" == "true" \
  && "$connected_to_target" == "true" ]]; then
  status=passed
fi

jq -n \
  --arg status "$status" \
  --argjson health_status "${health_status:-0}" \
  --argjson ready_status "${ready_status:-0}" \
  --argjson login_status "${login_status:-0}" \
  --argjson self_status "${self_status:-0}" \
  --argjson sites_status "${sites_status:-0}" \
  --argjson fixture_site_found "$fixture_site_found" \
  --argjson connected_to_target "$connected_to_target" \
  --arg database "$database" \
  --arg source_uuid_fingerprint "$A22_SOURCE_UUID_FINGERPRINT" \
  --arg target_uuid_fingerprint "$A22_TARGET_UUID_FINGERPRINT" \
  --arg observed_uuid_fingerprint "$observed_fingerprint" \
  '{schema_version:1,acceptance_id:"A22",status:$status,
    health_status:$health_status,ready_status:$ready_status,login_status:$login_status,
    self_status:$self_status,sites_status:$sites_status,fixture_site_found:$fixture_site_found,
    connected_to_target:$connected_to_target,database:$database,
    source_uuid_fingerprint:$source_uuid_fingerprint,target_uuid_fingerprint:$target_uuid_fingerprint,
    observed_uuid_fingerprint:$observed_uuid_fingerprint,release_gate_required:true,
    production_release_authorized:false}'

[[ "$status" == "passed" ]]
