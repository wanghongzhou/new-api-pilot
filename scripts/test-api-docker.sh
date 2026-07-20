#!/bin/sh

set -eu

compose_file=${COMPOSE_FILE:-docker-compose.yml}
test_image=${TEST_API_DOCKER_IMAGE:-new-api-pilot-go-test:latest}
test_network=${TEST_API_DOCKER_NETWORK:-new-api-pilot_pilot-network}
test_build_cache=${TEST_API_DOCKER_BUILD_CACHE:-new-api-pilot-go-test-cache}
test_go_proxy=${TEST_API_DOCKER_GOPROXY:-off}
test_go_sum_database=${TEST_API_DOCKER_GOSUMDB:-off}
go_packages=${GO_PACKAGES:-. ./cmd/... ./common/... ./config/... ./constant/... ./controller/... ./dto/... ./internal/... ./middleware/... ./migrations/... ./model/... ./router/... ./service/... ./tests/... ./webui/... ./worker/...}
test_database_prefix=${TEST_DATABASE_NAME:-new_api_pilot_test_$(date +%s)_$$}

case "$test_database_prefix" in
  new_api_pilot_test_*) ;;
  *)
    echo "refusing unsafe test database name: $test_database_prefix" >&2
    exit 2
    ;;
esac
case "$test_database_prefix" in
  *[!A-Za-z0-9_]*)
    echo "refusing non-identifier test database name: $test_database_prefix" >&2
    exit 2
    ;;
esac
if [ "$test_database_prefix" = "new_api_pilot" ] || [ "$test_database_prefix" = "new_api_pilot_test" ]; then
  echo "refusing protected database name: $test_database_prefix" >&2
  exit 2
fi
if [ "${#test_database_prefix}" -gt 48 ]; then
  echo "refusing overlong test database prefix: $test_database_prefix" >&2
  exit 2
fi

if [ "${TEST_API_DOCKER_VALIDATE_ONLY:-}" = "1" ]; then
  printf '%s\n' "$test_database_prefix"
  exit 0
fi

mysql_root() {
  docker compose -f "$compose_file" exec -T -e MYSQL_PWD=root mysql mysql -uroot "$@"
}

current_database=
cleanup() {
  if [ -n "$current_database" ]; then
    mysql_root -e "DROP DATABASE IF EXISTS \`$current_database\`" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT HUP INT TERM

docker compose -f "$compose_file" up -d mysql redis
packages=$(docker run --rm \
  -e "GOPROXY=$test_go_proxy" \
  -e "GOSUMDB=$test_go_sum_database" \
  "$test_image" go list $go_packages)
status=0
index=0
for package in $packages; do
  index=$((index + 1))
  current_database="${test_database_prefix}_${index}"
  mysql_root -e "DROP DATABASE IF EXISTS \`$current_database\`; CREATE DATABASE \`$current_database\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; GRANT ALL PRIVILEGES ON \`$current_database\`.* TO 'pilot'@'%';"
  if ! docker run --rm \
    --network "$test_network" \
    --mount "type=volume,source=$test_build_cache,target=/root/.cache/go-build" \
    -e "GOPROXY=$test_go_proxy" \
    -e "GOSUMDB=$test_go_sum_database" \
    -e "TEST_DATABASE_DSN=pilot:pilot@tcp(mysql:3306)/$current_database?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai" \
    -e 'TEST_DATABASE_ADMIN_DSN=root:root@tcp(mysql:3306)/?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai' \
    "$test_image" go test -count=1 -p 1 "$package"; then
    status=1
  fi
  cleanup
  current_database=
done
exit "$status"
