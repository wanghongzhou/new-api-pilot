WEB_DIR = ./web
COMPOSE_FILE = docker-compose.yml
DEV_WEB_PORT ?= 5173
GO_PACKAGES = . ./cmd/... ./common/... ./config/... ./constant/... ./controller/... ./dto/... ./internal/... ./middleware/... ./migrations/... ./model/... ./router/... ./service/... ./tests/... ./webui/... ./worker/...
GO_TEST_FLAGS = -count=1
BUN_IMAGE ?= oven/bun:1.3.13-alpine
GO_IMAGE ?= golang:1.25-alpine
RUNTIME_IMAGE ?= alpine:3.22
GO_MODULE_PROXY ?= https://proxy.golang.org,direct
GO_SUM_DATABASE ?= sum.golang.org
ALPINE_MIRROR ?=
DOCKER_BUILD_ARGS = --build-arg BUN_IMAGE=$(BUN_IMAGE) --build-arg GO_IMAGE=$(GO_IMAGE) --build-arg RUNTIME_IMAGE=$(RUNTIME_IMAGE) --build-arg GO_MODULE_PROXY=$(GO_MODULE_PROXY) --build-arg GO_SUM_DATABASE=$(GO_SUM_DATABASE) --build-arg ALPINE_MIRROR=$(ALPINE_MIRROR)

ifneq ($(strip $(TEST_DATABASE_DSN)),)
GO_TEST_FLAGS += -p 1
endif

.PHONY: dev dev-api dev-api-rebuild dev-web down logs build-web check-web test-api test-api-docker test-support check-prometheus docs-check docs-check-final docs-check-docker docs-check-final-docker contract-generate acceptance

dev: dev-api dev-web

dev-api:
	docker compose -f $(COMPOSE_FILE) up -d

dev-api-rebuild:
	docker compose -f $(COMPOSE_FILE) up -d --build api

dev-web:
	@echo "Frontend: http://localhost:$(DEV_WEB_PORT)"
	cd $(WEB_DIR) && bun install && bun run dev -- --host 0.0.0.0 --port $(DEV_WEB_PORT)

down:
	docker compose -f $(COMPOSE_FILE) down

logs:
	docker compose -f $(COMPOSE_FILE) logs -f api mysql

build-web:
	cd $(WEB_DIR) && bun install --frozen-lockfile && bun run build

check-web:
	cd $(WEB_DIR) && bun run check

test-api:
	go test $(GO_TEST_FLAGS) $(GO_PACKAGES)

test-api-docker:
	docker build --target go-test-runner $(DOCKER_BUILD_ARGS) -t new-api-pilot-go-test:latest .
	COMPOSE_FILE='$(COMPOSE_FILE)' GO_PACKAGES='$(GO_PACKAGES)' sh scripts/test-api-docker.sh

test-support:
	go test ./internal/docscheck ./tests/support

check-prometheus:
	docker run --rm --entrypoint /bin/promtool -v "$(CURDIR):/workspace:ro" prom/prometheus:v3.5.0 check rules /workspace/deploy/prometheus/recording-rules.yaml /workspace/deploy/prometheus/alert-rules.yaml

docs-check:
	go run ./cmd/docscheck -root .

docs-check-final:
	go run ./cmd/docscheck -root . -final

docs-check-docker:
	docker build --target go-test-runner $(DOCKER_BUILD_ARGS) -t new-api-pilot-go-test:latest .
	docker run --rm -v "$(CURDIR):/workspace:ro" -w /workspace new-api-pilot-go-test:latest go run ./cmd/docscheck -root .

docs-check-final-docker:
	docker build --target go-test-runner $(DOCKER_BUILD_ARGS) -t new-api-pilot-go-test:latest .
	docker run --rm -v "$(CURDIR):/workspace:ro" -w /workspace new-api-pilot-go-test:latest go run ./cmd/docscheck -root . -final

contract-generate:
	go run ./cmd/contractgen -root .

acceptance:
	$(MAKE) docs-check-final-docker
	$(MAKE) test-api-docker
	$(MAKE) check-prometheus
	cd $(WEB_DIR) && bun install --frozen-lockfile
	$(MAKE) check-web
	cd $(WEB_DIR) && bun run test:unit
	cd $(WEB_DIR) && bun run test:e2e
