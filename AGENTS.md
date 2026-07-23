# Project conventions

## Overview

This repository is an independent multi-site operations platform. The sibling
`new-api` repository is a reference for architecture, tooling, and visual
conventions only; do not import its business packages or register routes in it.

## Tech stack

- Backend: Go 1.25+, Gin, GORM, MySQL 8.0+
- Frontend: React 19, TypeScript, Rsbuild, TanStack Router and Query
- UI: Base UI, shadcn base-nova conventions, Tailwind CSS v4, Hugeicons
- Package manager: Bun
- Quality tools: gofmt, go test, tsgo, oxlint, oxfmt

## Architecture

Keep backend dependencies flowing in one direction:

`router -> controller -> service -> model`

- `router/`: HTTP route registration and middleware composition
- `controller/`: request parsing and response mapping
- `service/`: business rules and orchestration
- `model/`: GORM models and database access
- `dto/`: request and response contracts
- `common/`: shared infrastructure helpers
- `constant/`: stable enums and context keys
- `worker/`: schedulers and background workers

Frontend features live under `web/src/features/<feature>/`; file-based routes
live under `web/src/routes/`.

## Feature change guardrail

- First classify the work as a feature/contract change or a bug fix.
- Treat every feature as a complete, traceable vertical slice across its
  authoritative documentation, implementation, and tests. A feature addition
  or externally visible contract change must update all three together; partial
  delivery is not acceptable.
- For a feature addition or externally visible contract change, update the
  relevant existing authoritative detailed-design document under `docs/`
  before changing implementation and tests. Do not create a standalone
  requirement document unless the user asks for one or no suitable existing
  document exists.
- For a bug fix that preserves the documented contract, update implementation
  and regression tests without creating documentation. Correct the existing
  detailed design only when it is inaccurate or lacks the affected boundary.
- When removing a feature or externally visible contract, remove or update its
  entire vertical slice in the same change: authoritative documentation,
  backend and frontend implementation, routes and navigation, API contracts,
  permissions, configuration and feature flags, jobs or hooks, translations,
  tests, fixtures, mocks, and any other references that exist only for that
  feature.
- After a removal, search the repository for the removed feature's symbols,
  routes, configuration keys, permission keys, and user-visible strings. Do
  not leave dead code, unreachable UI, inactive routes, unused configuration,
  obsolete tests, compatibility shims, or a documented feature that no longer
  exists. Preserve historical migrations or audit records only when they are
  still required for upgrade or data-history correctness.
- Do not consider a feature or contract change complete until its detailed
  design, implementation, tests, and exposed integration points are consistent
  and the relevant validation commands pass.

## Backend rules

- Use the response envelope from `common/response.go`.
- Keep MySQL access behind GORM unless a measured query requires bound raw SQL.
- Use `int64` for quota, token, and request counts. Expose bigint values as JSON
  strings to the frontend.
- Never log passwords, site access tokens, webhook URLs, or encryption keys.
- All external requests must carry a request ID and a bounded timeout.
- Write deterministic tests for API contracts and business invariants.
- Run backend tests and checks in Docker containers; do not rely on a local Go
  installation. Integration tests must target the dedicated
  `new_api_pilot_test` database, never the development database.

## Frontend rules

- Use Bun for install and scripts.
- User-visible strings must go through i18next and exist in every locale file.
- Use TanStack Query for server state and the shared Axios client for HTTP.
- Use React Hook Form with Zod for forms.
- Use semantic Tailwind tokens instead of fixed component-level colors.
- Run `bun run check` after TypeScript or TSX changes.

## AI Agent completion workflow

Every task must finish as a coherent, locally reviewable change. Apply this
sequence without waiting for the user to repeat it:

1. Classify the request as a feature/contract change or a contract-preserving
   bug fix, and inspect the existing dirty worktree without reverting unrelated
   user changes.
2. For a feature/contract change, update the authoritative detailed design
   first. For a bug fix, update the design only when the documented boundary is
   inaccurate or incomplete.
3. Implement the complete vertical slice, including backend, frontend, API
   contracts, configuration, permissions, jobs, translations, fixtures and
   exposed integration points that are actually affected.
4. Add or update deterministic tests that prove the changed behavior. Do not
   weaken existing assertions merely to make a change pass.
5. Run validation proportionate to the touched areas: `gofmt` and Docker Go
   tests for backend changes; `bun run check` plus relevant unit tests for
   frontend changes; docs checks for authoritative documentation changes.
6. Refresh the isolated local development stack, verify health, and leave the
   browser-review URL ready before handoff.

Do not call a feature/contract change complete while documentation,
implementation, tests, generated contracts or the running development stack
disagree. If a validation failure is caused by unrelated pre-existing work,
report it explicitly while still running every safe in-scope validation.

## Development stack and local refresh

- `docker-compose.dev.yml` is the only Compose file authorized for automatic
  local refresh. It has an isolated Compose project, persistent MySQL/Redis and
  export volumes, a backend-only development image, and a persistent Bun/
  Rsbuild web service with source bind mounts and HMR.
- Never start, rebuild or recreate the production `docker-compose.yml` merely
  to validate a code change. Production Compose is used only when the user
  explicitly asks for production deployment or production-mode verification.
- After a backend, Go dependency, Dockerfile, Compose or runtime configuration
  change, run:
  `docker compose -f docker-compose.dev.yml up -d --build api web`.
- For frontend-only TS/TSX/CSS/i18n changes, do not rebuild the API. HMR should
  apply the change; ensure the stack is running with:
  `docker compose -f docker-compose.dev.yml up -d web`.
- Before handoff, require both `new-api-pilot-dev-api` and
  `new-api-pilot-dev-web` to be healthy. Verify `http://localhost:3000/healthz`
  and that `http://localhost:5173` returns successfully. For a user-visible
  change, smoke-test the affected browser route when practical.
- If refresh or health checks fail, inspect `docker compose -f
  docker-compose.dev.yml logs --tail=200 api web mysql redis`, fix the in-scope
  problem, and retry. Do not leave the user with a broken local stack.
- The stable user-facing development URL is `http://localhost:5173`; port 3000
  is the backend API and is not the frontend entry point.
- Use `make test-api-docker` for backend validation. If GNU Make is unavailable,
  execute the equivalent Docker commands from the Makefile; tests must still
  use the isolated `new_api_pilot_test_*` databases and never the development
  database.
