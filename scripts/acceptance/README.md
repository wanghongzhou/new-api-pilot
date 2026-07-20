# Acceptance Harness

The harness writes one immutable run directory per command. Every directory
contains `evidence.json`, `stdout.log`, and `stderr.log`; case-specific commands
may add structured reports to the same directory.

Run a case from the repository root:

```bash
go run ./scripts/acceptance run -case A83 -- \
  go run ./scripts/acceptance docs-negative -root .
```

Run the formal A22 backup and isolated-restore drill through the canonical
wrapper:

```powershell
go run ./scripts/acceptance run -case A22 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a22.ps1
```

The A22 runner creates two MySQL 8.4 instances with the same database name but
different server UUIDs on a unique internal network, with independent data and
work volumes and no host ports. It seeds the deterministic F05 representative
fixture, including encrypted values, task/window/cursor state, alerts, exports,
and all six hourly/daily aggregate levels. It then performs a real backup,
manifest preflight, two mandatory pre-import failure branches (manifest tamper
and target identity mismatch), restore, full verification, release-gate check,
exact whole-database snapshot comparison, and authenticated health/ready/login/
self/site application smoke against the restored target. The application smoke
requires both the session cookie and the matching `New-Api-User` header and
proves the observed database UUID fingerprint is the target rather than the
source.

Formal evidence is accepted only for the exact command above, all 19 inner
artifacts plus their SHA-256 inventory, zero secret-scan findings, and a
post-cleanup sweep with no residual containers, networks, volumes, or images.
The report scope is always `controlled_technical_drill`; even a formal passing
run always records `production_release_authorized=false` and does not authorize
a production switch.

For a development-only drill, invoke `run-a22.ps1` directly with no acceptance
environment variables. It creates a unique `artifacts/smoke/A22-dev-*`
directory and records `evidence_class=development` and
`acceptance_eligible=false`. Development output cannot remove A22's `planned:`
manifest marker.

Run the A85 alert fixture and delivery drill through the same harness:

```powershell
go run ./scripts/acceptance run -case A85 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a85.ps1
```

`run-a85.ps1` refuses direct invocation: the harness must provide exactly
`ACCEPTANCE_ID=A85` and an existing absolute `ACCEPTANCE_EVIDENCE_DIR`. It
creates a uniquely named, per-run internal Docker network and an ephemeral
`mysql:8.4` container without a host port. The repository is mounted read-only
in the `golang:1.25.1` test container and the harness evidence directory is
mounted at `/evidence`. A separate container on the default network first
warms the shared Go module cache without receiving the database DSN or evidence
mount; the test container then runs only on the internal network with module
and checksum lookups disabled, and receives the `A85_ISOLATED_MYSQL=true`
acceptance guard. Only the exact containers and network created for that run
are removed; shared Go module and build-cache volumes are retained.

Run the formal A25 migration acceptance through the canonical wrapper:

```powershell
go run ./scripts/acceptance run -case A25 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a25.ps1
```

`run-a25.ps1` uses real MySQL 8.4, MySQL 5.7, and MariaDB 10.11 containers on
one unique internal network without host ports. The repository mount is
read-only, the evidence mount is separate, and both Go cache volumes are
unique to the run and removed afterward. The target test proves empty-schema
migration, historical-data upgrade, idempotency, checksum/source/unknown
version rejection, transactional recovery, both DDL dirty-checkpoint paths,
authoritative schema equality, and fail-closed version gates with zero tables
and no migration lock on unsupported servers. Formal evidence is accepted only
for the exact command above, one unskipped passing JSON event, the complete
report contract, an exact SHA-256 artifact inventory, and a zero-residual
container/network/volume/image sweep. The runner never invokes Docker prune.

For development only, create a unique `artifacts/smoke/A25-dev-*` directory,
set `ACCEPTANCE_ID=A25`, `ACCEPTANCE_EVIDENCE_DIR` to its absolute path, and
`A25_DEVELOPMENT=true`, then invoke `run-a25.ps1` directly. Development output
is not formal evidence and cannot remove A25's `planned:` manifest marker.

Run an A45 development smoke through the canonical wrapper while the evidence
implementation is under review:

```powershell
go run ./scripts/acceptance run -case A45 -evidence-root artifacts/smoke -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a45.ps1
```

After independent approval, the formal command is the same except that it uses
the default `artifacts/acceptance` root:

```powershell
go run ./scripts/acceptance run -case A45 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a45.ps1
```

`run-a45.ps1` runs only `TestA45SecurityBoundaryAcceptance` in a read-only Go
container attached to one unique internal Docker network. It publishes no host
ports, mounts the repository read-only, does not mount the evidence directory
into the test container, uses a read-only root filesystem, drops all Linux
capabilities, enables `no-new-privileges`, disables Go network lookup, and
explicitly clears every upper- and lower-case HTTP proxy variable. The four
required subtests prove the Origin, trusted-proxy, DNS/TLS/address and old-Token
credential boundaries plus response/log redaction. Both F01 and F02 are bound
to their fixed checksums.

The A45 inner contract contains exactly nine payload artifacts: the raw JSONL
and stderr logs, test summary, command, environment, fixture, authoritative
four-scenario report, cleanup report, and secret-scan report. A tenth inner
file inventories the nine payloads by exact path, size, and SHA-256. Together
with the wrapper's `stdout.log`, `stderr.log`, and `evidence.json`, that is the
entire allowed run directory; extra files, skips, no-tests output, arbitrary
commands, tampering, secret patterns, or residual labeled Docker resources are
fail-closed. A development run is never sufficient to remove A45's `planned:`
manifest markers.

Run the A50 statistics-state E2E evidence in development mode through its
canonical wrapper:

```powershell
go run ./scripts/acceptance run -case A50 -evidence-root artifacts/smoke -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a50.ps1
```

After independent review, omit `-evidence-root artifacts/smoke` for the formal
run. `run-a50.ps1` first executes the full `bun run check` gate and proves that
the only locale is `zh-CN`. It then starts a hidden Rsbuild process on a newly
reserved `127.0.0.1` port that is explicitly not 5173; the configured base URL
disables Playwright's shared/default server. The run fixes two workers, zero
retries, `--forbid-only`, and the exact desktop/mobile projects. Test results
and the HTML reporter use unique temporary directories, while JSON is written
directly into the immutable evidence directory. The standalone HTML index is
copied into evidence before its temporary report directory is removed.

The validator parses the Playwright JSON itself and requires exactly nine
statistics route titles for each of the two projects: 18 expected results,
zero unexpected/flaky/skipped results, one passing attempt per test, retry 0,
and no annotations, errors, attachments, stdout, or stderr. It binds the frozen
spec, package, Playwright config, and F03 fixture checksums. The inner contract
contains exactly 15 payload artifacts plus one exact path/size/SHA-256
inventory. After the wrapper adds its two logs and `evidence.json`, no other
file or directory is allowed. Cleanup must stop the entire server PID tree,
prove the dynamic port is no longer connectable, and remove both temporary
directories; development evidence cannot remove A50's `planned:` markers.

Run the A62 resource-minute retention acceptance through the harness:

```powershell
go run ./scripts/acceptance run -case A62 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a62.ps1
```

`run-a62.ps1` creates a unique internal Docker network and an ephemeral MySQL
8.4 database without publishing a host port. It runs only
`TestA62ResourceMinuteRetention` against the F05 fixture and requires exactly
one passing JSON test event with no skips. The report proves the strict
90-day boundary, multi-batch deletion, hourly/daily finalization guards,
starvation-free progress past a blocked prefix, restart continuation,
idempotent retries, and preservation of all resource aggregates and business
facts. The runner stores the raw JSON stream, stderr, test summary, canonical
inner command, environment, fixture identity, report, and post-cleanup result;
an exact path/size/SHA-256 inventory covers all eight inner artifacts. The
outer acceptance wrapper accepts only the canonical command and evidence root,
scans its closed stdout/stderr logs, validates the inner inventory and report,
and only then may publish a passing `evidence.json`. The script removes only
resources bearing its unique run label and sweeps containers, networks,
volumes, and images before it can pass.

Run an A49 development smoke (never acceptance evidence):

```powershell
go run ./scripts/acceptance run -case A49 -evidence-root artifacts/smoke -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a49.ps1 -Smoke
```

Run the formal A49 profile only in a controlled environment with at least 8
Docker CPUs, 16 GiB memory, 35 GiB Docker free space, and 5 GiB evidence free
space:

```powershell
go run ./scripts/acceptance run -case A49 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a49.ps1
```

The A49 runner creates a unique internal network and isolated MySQL 8.4 data
volume without publishing host ports. `a49-seed` enforces the F05 formal shape
(50 sites, 100,000 remote users, 10,000 accounts, 15,000,000 hourly facts),
and `a49-load` performs real logins for 20 distinct viewers. Three scenarios
each run for 120 seconds of warmup plus 600 seconds of sampling (36 minutes in
total). `a49-report` applies nearest-rank P50/P95/P99 to all attempts, reconciles
server access-log durations by request ID, and independently gates three list
endpoints, one hourly endpoint, seven Dashboard endpoints, and the Dashboard
composite. Smoke reports always set `acceptance_eligible=false`; neither smoke
nor a failed full run permits removal of A49's `planned:` evidence path.
The generic harness accepts only the exact commands shown above for A49. After
the script exits successfully, it verifies the report mode and evidence class,
the size and SHA-256 of every inventoried artifact, and the post-cleanup Docker
sweep before it can write a passing `evidence.json`.

Use `-cwd web` for frontend commands. A failed command still writes evidence,
but its manifest entry must remain `planned:` until a later run passes.

The A83 checker copies the repository into five independent temporary trees,
injects exactly one traceability, manifest, locale, Markdown, or fixture
checksum defect into each tree, and confirms the normal tree passes before and
after all negative checks.
