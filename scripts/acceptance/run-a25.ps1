$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = 'A25'
$targetTest = 'TestA25MigrationAcceptance'
$goImage = 'golang:1.25.1'
$mysqlImage = 'mysql:8.4'
$legacyMySQLImage = 'mysql:5.7'
$mariaDBImage = 'mariadb:10.11'
$currentDatabase = 'pilot_a25'
$legacyDatabase = 'pilot_a25_legacy'
$mariaDBDatabase = 'pilot_a25_mariadb'
$innerCommand = @(
    'go', 'test', '-json', './tests/integration', '-run', '^TestA25MigrationAcceptance$',
    '-count=1', '-timeout=10m'
)

function Get-A25ImageIdentity {
    param([Parameter(Mandatory = $true)][string]$Reference)

    $result = Invoke-OpsProcess -FileName 'docker' -Arguments @(
        'image', 'inspect', $Reference, '--format', '{{.Id}}|{{index .RepoDigests 0}}'
    ) -TimeoutSeconds 30
    if ($result.TimedOut -or $result.ExitCode -ne 0) {
        throw "A25 required image is unavailable: $Reference"
    }
    $fields = $result.Stdout.Trim().Split('|')
    if ($fields.Count -ne 2 -or $fields[0] -notmatch '^sha256:[0-9a-f]{64}$' -or
        $fields[1] -notmatch '^[^|]+@sha256:[0-9a-f]{64}$') {
        throw "A25 image identity is invalid: $Reference"
    }
    return [ordered]@{ reference = $Reference; id = $fields[0]; digest = $fields[1] }
}

function Wait-A25HealthyContainer {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][ValidateRange(1, 600)][int]$TimeoutSeconds
    )

    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    while ($true) {
        $result = Invoke-OpsProcess -FileName 'docker' -Arguments @(
            'inspect', '--format', '{{.State.Status}}|{{.State.Health.Status}}', $Container
        ) -TimeoutSeconds 15
        if (-not $result.TimedOut -and $result.ExitCode -eq 0) {
            $state = $result.Stdout.Trim()
            if ($state -ceq 'running|healthy') { return }
            if ($state.StartsWith('exited|') -or $state.StartsWith('dead|') -or $state.EndsWith('|unhealthy')) {
                throw "A25 database container stopped before becoming healthy: $Container ($state)"
            }
        }
        if ([DateTimeOffset]::UtcNow -ge $deadline) {
            throw "A25 database health wait timed out: $Container"
        }
        Start-Sleep -Seconds 2
    }
}

function Get-A25MySQLFields {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][string]$Client,
        [Parameter(Mandatory = $true)][string]$Query,
        [Parameter(Mandatory = $true)][ValidateRange(1, 10)][int]$ExpectedFields
    )

    $result = Invoke-OpsDocker -Arguments @(
        'exec', $Container, $Client, '--batch', '--skip-column-names', '--user=root', '--execute', $Query
    ) -TimeoutSeconds 30
    $fields = $result.Stdout.Trim().Split("`t")
    if ($fields.Count -ne $ExpectedFields) {
        throw "A25 database contract query returned an unexpected shape: $Container"
    }
    return $fields
}

function Assert-A25TestJSON {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $lines = 0
    $passes = 0
    $failures = 0
    $skips = 0
    $noTests = $false
    foreach ($rawLine in ($Payload -split "`r?`n")) {
        $line = $rawLine.Trim()
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $lines++
        try { $event = $line | ConvertFrom-Json -ErrorAction Stop }
        catch { throw "A25 go test stdout line $lines is not valid JSON." }
        $action = [string]$event.Action
        $test = if ($null -ne $event.PSObject.Properties['Test']) { [string]$event.Test } else { '' }
        $output = if ($null -ne $event.PSObject.Properties['Output']) { [string]$event.Output } else { '' }
        if ($action -ceq 'skip') { $skips++ }
        if ($action -ceq 'fail') { $failures++ }
        if ($test -ceq $targetTest -and $action -ceq 'pass') { $passes++ }
        if ($output -match '(?i)no tests to run|\[no test files\]') { $noTests = $true }
    }
    if ($lines -le 0 -or $passes -ne 1 -or $failures -ne 0 -or $skips -ne 0 -or $noTests) {
        throw 'A25 go test stream did not prove one unskipped passing target test.'
    }
    return [ordered]@{
        schema_version = 1
        acceptance_id = 'A25'
        status = 'passed'
        target_test = $targetTest
        package = 'new-api-pilot/tests/integration'
        pass_events = $passes
        fail_events = $failures
        skip_events = $skips
        no_tests = $noTests
        json_lines = $lines
        json_path = 'a25-test.jsonl'
        stderr_path = 'a25-test.stderr.log'
    }
}

function Assert-A25Report {
    param([Parameter(Mandatory = $true)][string]$EvidenceDirectory)

    $path = Join-Path $EvidenceDirectory 'a25-report.json'
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw 'A25 report was not produced.'
    }
    $report = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json -ErrorAction Stop
    if ([int]$report.schema_version -ne 1 -or [string]$report.acceptance_id -cne 'A25' -or
        [string]$report.status -cne 'passed' -or [int64]$report.fixed_now_unix -ne 1768665599 -or
        [string]$report.authoritative_schema_sha256 -notmatch '^[0-9a-f]{64}$' -or
        -not [bool]$report.version_gate.current_accepted -or
        -not [bool]$report.version_gate.legacy_mysql_rejected -or
        -not [bool]$report.version_gate.mariadb_rejected -or
        [int64]$report.version_gate.legacy_tables_before -ne 0 -or
        [int64]$report.version_gate.legacy_tables_after -ne 0 -or
        [int64]$report.version_gate.mariadb_tables_before -ne 0 -or
        [int64]$report.version_gate.mariadb_tables_after -ne 0 -or
        -not [bool]$report.empty_database.applied_at_stable -or
        -not [bool]$report.empty_database.idempotent_schema_stable -or
        -not [bool]$report.upgrade.historical_preserved -or
        -not [bool]$report.upgrade.foreign_keys_preserved -or
        -not [bool]$report.upgrade.backfill_scope_migrated -or
        -not [bool]$report.tamper.database_checksum_rejected -or
        -not [bool]$report.tamper.repository_source_rejected -or
        -not [bool]$report.tamper.unknown_version_rejected -or
        -not [bool]$report.tamper.no_schema_mutation -or
        -not [bool]$report.dml_failure.initial_failure_observed -or
        -not [bool]$report.dml_failure.checkpoint_ready -or
        -not [bool]$report.dml_failure.resume_completed -or
        -not [bool]$report.ddl_recovery.dirty_without_ddl_replayed -or
        -not [bool]$report.ddl_recovery.dirty_with_ddl_recognized) {
        throw 'A25 report did not satisfy the migration acceptance contract.'
    }
    return $report
}

function Write-A25ArtifactInventory {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $required = @(
        'a25-test.jsonl', 'a25-test.stderr.log', 'a25-test-summary.json', 'a25-command.json',
        'a25-environment.json', 'a25-fixture.json', 'a25-report.json', 'a25-cleanup.json'
    )
    $files = @()
    foreach ($relative in $required) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "A25 required evidence artifact is missing: $relative"
        }
        $info = Get-Item -LiteralPath $path
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A25'
        evidence_class = $EvidenceClass
        files = $files
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a25-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

$failure = $null
$cleanupFailed = $false
$containerCleanupFailed = $false
$networkCleanupFailed = $false
$volumeCleanupFailed = $false
$containersCreated = $false
$networkCreated = $false
$volumesCreated = $false
$runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
$runLabel = "new-api-pilot.acceptance-run=$runToken"
$networkName = "new-api-pilot-a25-$runToken-network"
$moduleVolumeName = "new-api-pilot-a25-$runToken-gomod"
$buildVolumeName = "new-api-pilot-a25-$runToken-gobuild"
$warmContainerName = "new-api-pilot-a25-$runToken-warm"
$currentContainerName = "new-api-pilot-a25-$runToken-mysql84"
$legacyContainerName = "new-api-pilot-a25-$runToken-mysql57"
$mariaDBContainerName = "new-api-pilot-a25-$runToken-mariadb"
$testContainerName = "new-api-pilot-a25-$runToken-test"
$warmContainerMayExist = $false
$currentContainerMayExist = $false
$legacyContainerMayExist = $false
$mariaDBContainerMayExist = $false
$testContainerMayExist = $false
$networkMayExist = $false
$moduleVolumeMayExist = $false
$buildVolumeMayExist = $false
$evidenceDirectory = $null
$developmentMode = $env:A25_DEVELOPMENT -ceq 'true'
$evidenceClass = if ($developmentMode) { 'development' } else { 'formal' }

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID) {
        throw 'This script must be invoked with ACCEPTANCE_ID=A25.'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory.'
    }
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $relativeEvidence = Get-OpsRepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    if ($developmentMode) {
        if (-not [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_CLASS) -or
            -not $relativeEvidence.StartsWith('artifacts/smoke/A25-dev-', [System.StringComparison]::Ordinal)) {
            throw 'A25 development evidence must use artifacts/smoke/A25-dev-* without a formal class.'
        }
    }
    elseif ($env:ACCEPTANCE_EVIDENCE_CLASS -cne 'formal' -or
        -not $relativeEvidence.StartsWith('artifacts/acceptance/A25/', [System.StringComparison]::Ordinal) -or
        [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE)) {
        throw 'A25 formal evidence must be invoked by the canonical wrapper under artifacts/acceptance/A25/.'
    }

    [void](Invoke-OpsDocker -Arguments @('version', '--format', '{{.Client.Version}}') -TimeoutSeconds 30)
    $imageGo = Get-A25ImageIdentity -Reference $goImage
    $imageCurrent = Get-A25ImageIdentity -Reference $mysqlImage
    $imageLegacy = Get-A25ImageIdentity -Reference $legacyMySQLImage
    $imageMariaDB = Get-A25ImageIdentity -Reference $mariaDBImage
    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"

    $moduleVolumeMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'volume', 'create', '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel, $moduleVolumeName
    ) -TimeoutSeconds 30)
    $buildVolumeMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'volume', 'create', '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel, $buildVolumeName
    ) -TimeoutSeconds 30)
    $volumesCreated = $true

    $warmContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'create', '--name', $warmContainerName,
        '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount,
        '--mount', "type=volume,source=$moduleVolumeName,target=/go/pkg/mod",
        '--mount', "type=volume,source=$buildVolumeName,target=/root/.cache/go-build",
        $goImage, 'go', 'mod', 'download'
    ) -TimeoutSeconds 60)
    $containersCreated = $true
    [void](Invoke-OpsDocker -Arguments @('start', $warmContainerName) -TimeoutSeconds 30)
    $warmExit = Wait-OpsContainer -Container $warmContainerName -TimeoutSeconds 600
    if ($warmExit -ne 0) { throw 'A25 Go dependency warm-up failed.' }
    if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $warmContainerName))) {
        throw 'A25 warm-up container cleanup failed.'
    }
    $warmContainerMayExist = $false

    $networkMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'network', 'create', '--internal', '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel, $networkName
    ) -TimeoutSeconds 30)
    $networkCreated = $true

    $currentContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'run', '--detach', '--name', $currentContainerName, '--network', $networkName, '--network-alias', 'mysql-a25',
        '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel,
        '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$currentDatabase",
        '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '60', '--health-start-period', '5s',
        $mysqlImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci',
        '--transaction-isolation=READ-COMMITTED', '--default-time-zone=+08:00'
    ) -TimeoutSeconds 60)

    $legacyContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'run', '--detach', '--name', $legacyContainerName, '--network', $networkName, '--network-alias', 'mysql57-a25',
        '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel,
        '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$legacyDatabase",
        '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '60', '--health-start-period', '5s',
        $legacyMySQLImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci'
    ) -TimeoutSeconds 60)

    $mariaDBContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'run', '--detach', '--name', $mariaDBContainerName, '--network', $networkName, '--network-alias', 'mariadb-a25',
        '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel,
        '--env', 'MARIADB_ALLOW_EMPTY_ROOT_PASSWORD=1', '--env', "MARIADB_DATABASE=$mariaDBDatabase",
        '--health-cmd', 'mariadb-admin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '60', '--health-start-period', '5s',
        $mariaDBImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci'
    ) -TimeoutSeconds 60)

    Wait-A25HealthyContainer -Container $currentContainerName -TimeoutSeconds 180
    Wait-A25HealthyContainer -Container $legacyContainerName -TimeoutSeconds 180
    Wait-A25HealthyContainer -Container $mariaDBContainerName -TimeoutSeconds 180

    $currentFields = @(Get-A25MySQLFields -Container $currentContainerName -Client 'mysql' -ExpectedFields 5 `
        -Query 'SELECT VERSION(), @@transaction_isolation, @@character_set_server, @@collation_server, @@time_zone')
    $legacyFields = @(Get-A25MySQLFields -Container $legacyContainerName -Client 'mysql' -ExpectedFields 1 -Query 'SELECT VERSION()')
    $mariaDBFields = @(Get-A25MySQLFields -Container $mariaDBContainerName -Client 'mariadb' -ExpectedFields 1 -Query 'SELECT VERSION()')
    if ($currentFields[0] -notmatch '^8\.4\.' -or $legacyFields[0] -notmatch '^5\.7\.' -or
        $mariaDBFields[0] -notmatch '(?i)mariadb') {
        throw 'A25 database image versions do not satisfy the real version-gate matrix.'
    }

    $gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot
    $commandReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A25'
        evidence_class = $evidenceClass
        target_test = $targetTest
        working_directory = '/workspace'
        command = $innerCommand
        go_image = $goImage
        mysql_image = $mysqlImage
        legacy_mysql_image = $legacyMySQLImage
        mariadb_image = $mariaDBImage
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-command.json') -Payload ($commandReport | ConvertTo-Json -Depth 5)
    $environmentReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A25'
        evidence_class = $evidenceClass
        commit = $gitState.Commit
        worktree_dirty = $gitState.WorktreeDirty
        isolated_guard = $true
        images = [ordered]@{
            go = $imageGo
            current_mysql = $imageCurrent
            legacy_mysql = $imageLegacy
            mariadb = $imageMariaDB
        }
        servers = [ordered]@{
            current = [ordered]@{
                version = $currentFields[0]
                transaction_isolation = $currentFields[1]
                character_set_server = $currentFields[2]
                collation_server = $currentFields[3]
                time_zone = $currentFields[4]
            }
            legacy_mysql = [ordered]@{ version = $legacyFields[0] }
            mariadb = [ordered]@{ version = $mariaDBFields[0] }
        }
        network = [ordered]@{ internal = $true; host_ports = @() }
        databases = [ordered]@{
            current = $currentDatabase
            legacy_mysql = $legacyDatabase
            mariadb = $mariaDBDatabase
        }
        resources = [ordered]@{
            network = $networkName
            module_cache = $moduleVolumeName
            build_cache = $buildVolumeName
        }
        repository_read_only = $true
        evidence_writable = $true
        offline_test_network = $true
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 8)

    $currentDSN = "root:@tcp(mysql-a25:3306)/${currentDatabase}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $legacyDSN = "root:@tcp(mysql57-a25:3306)/${legacyDatabase}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $mariaDBDSN = "root:@tcp(mariadb-a25:3306)/${mariaDBDatabase}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $testContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'create', '--name', $testContainerName, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A25', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', "type=volume,source=$moduleVolumeName,target=/go/pkg/mod",
        '--mount', "type=volume,source=$buildVolumeName,target=/root/.cache/go-build",
        '--env', "TEST_DATABASE_DSN=$currentDSN", '--env', "A25_LEGACY_DATABASE_DSN=$legacyDSN",
        '--env', "A25_MARIADB_DATABASE_DSN=$mariaDBDSN", '--env', 'ACCEPTANCE_ID=A25',
        '--env', 'ACCEPTANCE_EVIDENCE_DIR=/evidence', '--env', 'A25_ISOLATED_MYSQL=true',
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
        $goImage, 'go', 'test', '-json', './tests/integration', '-run', '^TestA25MigrationAcceptance$',
        '-count=1', '-timeout=10m'
    ) -TimeoutSeconds 60)
    [void](Invoke-OpsDocker -Arguments @('start', $testContainerName) -TimeoutSeconds 30)
    $testExit = Wait-OpsContainer -Container $testContainerName -TimeoutSeconds 660
    if ($testExit -eq -1) {
        [void](Remove-OpsDockerResource -Arguments @('kill', $testContainerName))
    }
    $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $testContainerName) -TimeoutSeconds 30
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-test.jsonl') -Payload $logs.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-test.stderr.log') -Payload $logs.Stderr
    if (-not [string]::IsNullOrEmpty($logs.Stdout)) { [Console]::Out.Write($logs.Stdout) }
    if (-not [string]::IsNullOrEmpty($logs.Stderr)) { [Console]::Error.Write($logs.Stderr) }
    if ($testExit -ne 0 -or $logs.TimedOut -or $logs.ExitCode -ne 0) {
        throw 'A25 go test did not complete successfully.'
    }
    $testSummary = Assert-A25TestJSON -Payload $logs.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-test-summary.json') -Payload ($testSummary | ConvertTo-Json -Depth 4)
    $report = Assert-A25Report -EvidenceDirectory $evidenceDirectory
    $fixtureReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A25'
        fixture_id = 'F05'
        path = [string]$report.fixture_path
        sha256 = [string]$report.fixture_sha256
        fixed_now_unix = [int64]$report.fixed_now_unix
        migration_count = @($report.repository_migrations).Count
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-fixture.json') -Payload ($fixtureReport | ConvertTo-Json -Depth 4)
}
catch {
    $failure = $_.Exception.Message
}
finally {
    foreach ($container in @(
        [pscustomobject]@{ MayExist = $testContainerMayExist; Name = $testContainerName },
        [pscustomobject]@{ MayExist = $warmContainerMayExist; Name = $warmContainerName },
        [pscustomobject]@{ MayExist = $currentContainerMayExist; Name = $currentContainerName },
        [pscustomobject]@{ MayExist = $legacyContainerMayExist; Name = $legacyContainerName },
        [pscustomobject]@{ MayExist = $mariaDBContainerMayExist; Name = $mariaDBContainerName }
    )) {
        if ($container.MayExist -and -not (Remove-OpsDockerResource -Arguments @('rm', '--force', $container.Name))) {
            $containerCleanupFailed = $true
        }
    }
    if ($networkMayExist -and -not (Remove-OpsDockerResource -Arguments @('network', 'rm', $networkName))) {
        $networkCleanupFailed = $true
    }
    foreach ($volume in @(
        [pscustomobject]@{ MayExist = $moduleVolumeMayExist; Name = $moduleVolumeName },
        [pscustomobject]@{ MayExist = $buildVolumeMayExist; Name = $buildVolumeName }
    )) {
        if ($volume.MayExist -and -not (Remove-OpsDockerResource -Arguments @('volume', 'rm', $volume.Name))) {
            $volumeCleanupFailed = $true
        }
    }
    $cleanupFailed = $containerCleanupFailed -or $networkCleanupFailed -or $volumeCleanupFailed
    if ($null -ne $evidenceDirectory) {
        $containerSweep = Get-OpsResidualSweep -Arguments @('ps', '-a', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Names}}')
        $networkSweep = Get-OpsResidualSweep -Arguments @('network', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Name}}')
        $volumeSweep = Get-OpsResidualSweep -Arguments @('volume', 'ls', '--filter', "label=$runLabel", '--format', '{{.Name}}')
        $imageSweep = Get-OpsResidualSweep -Arguments @('image', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Repository}}:{{.Tag}}')
        $sweepsSucceeded = $containerSweep.Succeeded -and $networkSweep.Succeeded -and
            $volumeSweep.Succeeded -and $imageSweep.Succeeded
        $noResiduals = $containerSweep.Items.Count -eq 0 -and $networkSweep.Items.Count -eq 0 -and
            $volumeSweep.Items.Count -eq 0 -and $imageSweep.Items.Count -eq 0
        $cleanupPassed = (-not $cleanupFailed) -and $sweepsSucceeded -and $noResiduals -and
            $containersCreated -and $networkCreated -and $volumesCreated
        $cleanupReport = [ordered]@{
            schema_version = 1
            acceptance_id = 'A25'
            evidence_class = $evidenceClass
            passed = $cleanupPassed
            sweeps_succeeded = $sweepsSucceeded
            lifecycle = [ordered]@{
                containers = if ($containersCreated -and -not $containerCleanupFailed -and $containerSweep.Items.Count -eq 0) { 'created_and_removed' } elseif ($containersCreated) { 'cleanup_failed' } else { 'not_created' }
                networks = if ($networkCreated -and -not $networkCleanupFailed -and $networkSweep.Items.Count -eq 0) { 'created_and_removed' } elseif ($networkCreated) { 'cleanup_failed' } else { 'not_created' }
                volumes = if ($volumesCreated -and -not $volumeCleanupFailed -and $volumeSweep.Items.Count -eq 0) { 'created_and_removed' } elseif ($volumesCreated) { 'cleanup_failed' } else { 'not_created' }
            }
            residuals = [ordered]@{
                containers = @($containerSweep.Items)
                networks = @($networkSweep.Items)
                volumes = @($volumeSweep.Items)
                images = @($imageSweep.Items)
            }
        }
        try {
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6)
        }
        catch { $cleanupPassed = $false }
        if (-not $cleanupPassed) { $cleanupFailed = $true }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A25 exact-resource Docker cleanup failed.')
    exit 1
}
try {
    Write-A25ArtifactInventory -Directory $evidenceDirectory -EvidenceClass $evidenceClass
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
exit 0
