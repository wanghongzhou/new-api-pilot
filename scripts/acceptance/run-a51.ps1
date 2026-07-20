[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = 'A51'
$mysqlImage = 'mysql:8.4'
$goImage = 'golang:1.25.1'
$databaseName = 'pilot_a51'
$testDatabaseName = 'pilot_a51_tests'
$failure = $null
$cleanupFailed = $false
$containerCleanupFailed = $false
$networkCleanupFailed = $false
$volumeCleanupFailed = $false
$networkName = $null
$mysqlVolumeName = $null
$runLabel = $null
$evidenceDirectory = $null
$containers = @{}
$createdContainers = @{}
$containerResourcesCreated = $false
$networkCreated = $false
$networkWasCreated = $false
$mysqlVolumeCreated = $false
$mysqlVolumeWasCreated = $false

function Save-A51ContainerOutput {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][string]$Path,
        [switch]$StdoutOnly
    )
    $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $Container) -TimeoutSeconds 60
    $payload = if ($StdoutOnly.IsPresent) { $logs.Stdout } else { $logs.Stdout + $logs.Stderr }
    Write-OpsUtf8NoBom -Path $Path -Payload (Protect-OpsDiagnostic -Payload $payload)
    return $logs
}

function ConvertTo-A51EnvironmentMap {
    param([Parameter(Mandatory = $true)][string[]]$Environment)

    $result = @{}
    foreach ($entry in $Environment) {
        $separator = $entry.IndexOf('=')
        if ($separator -le 0) { throw 'A51 preflight environment entry is invalid' }
        $name = $entry.Substring(0, $separator)
        $value = $entry.Substring($separator + 1)
        if ($result.ContainsKey($name) -and [string]$result[$name] -cne $value) {
            throw "A51 preflight environment contains conflicting $name values"
        }
        $result[$name] = $value
    }
    return $result
}

function Invoke-A51ConfigurationPreflight {
    param(
        [Parameter(Mandatory = $true)][string[]]$Environment,
        [Parameter(Mandatory = $true)][string]$LogPath
    )

    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_RUNNER_EXE -PathType Leaf)) {
        throw 'A51 acceptance runner executable is unavailable for configuration preflight'
    }
    $processEnvironment = ConvertTo-A51EnvironmentMap -Environment $Environment
    $processEnvironment['ACCEPTANCE_ID'] = 'A51'
    $processEnvironment['ACCEPTANCE_EVIDENCE_CLASS'] = 'formal'
    $processEnvironment['A51_CONFIG_PREFLIGHT'] = 'true'
    $result = Invoke-OpsProcess -FileName $env:ACCEPTANCE_RUNNER_EXE -Arguments @('a51-preflight') `
        -TimeoutSeconds 30 -Environment $processEnvironment
    $payload = Protect-OpsDiagnostic -Payload ($result.Stdout + $result.Stderr)
    if ([string]::IsNullOrWhiteSpace($payload)) { $payload = "A51 configuration preflight exit_code=$($result.ExitCode)`n" }
    Write-OpsUtf8NoBom -Path $LogPath -Payload $payload
    if ($result.TimedOut) { throw 'A51 configuration preflight timed out' }
    if ($result.ExitCode -ne 0) {
        $diagnostic = (Protect-OpsDiagnostic -Payload $result.Stderr).Trim()
        if ([string]::IsNullOrWhiteSpace($diagnostic)) { $diagnostic = 'configuration contract rejected' }
        throw "A51 configuration preflight failed: $diagnostic"
    }
}

function Invoke-A51Tool {
    param(
        [Parameter(Mandatory = $true)][string]$Suffix,
        [Parameter(Mandatory = $true)][string[]]$Command,
        [Parameter(Mandatory = $true)][string[]]$Environment,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds,
        [string]$OutputPath,
        [string]$LogPath
    )
    $name = "new-api-pilot-a51-$runToken-$Suffix"
    $containers[$Suffix] = $name
    $createdContainers[$Suffix] = $false
    $arguments = @(
        'create', '--name', $name, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A51', '--label', $runLabel,
        '--memory', '2g', '--cpus', '2', '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', 'ACCEPTANCE_ID=A51', '--env', 'ACCEPTANCE_EVIDENCE_CLASS=formal',
        '--env', 'A51_ISOLATED_MYSQL=true', '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off'
    )
    foreach ($entry in $Environment) { $arguments += @('--env', $entry) }
    $arguments += @($goImage) + $Command
    try {
        [void](Invoke-OpsDocker -Arguments $arguments)
        $createdContainers[$Suffix] = $true
        $script:containerResourcesCreated = $true
    }
    catch {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', $name) -TimeoutSeconds 30
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $createdContainers[$Suffix] = $true
            $script:containerResourcesCreated = $true
        }
        throw
    }
    [void](Invoke-OpsDocker -Arguments @('start', $name) -TimeoutSeconds 30)
    $exitCode = Wait-OpsContainer -Container $name -TimeoutSeconds $TimeoutSeconds
    $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $name) -TimeoutSeconds 60
    if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
        Write-OpsUtf8NoBom -Path $OutputPath -Payload (Protect-OpsDiagnostic -Payload $logs.Stdout)
    }
    if (-not [string]::IsNullOrWhiteSpace($LogPath)) {
        $payload = Protect-OpsDiagnostic -Payload ($logs.Stdout + $logs.Stderr)
        if ([string]::IsNullOrWhiteSpace($payload)) { $payload = "A51 $Suffix exit_code=$exitCode`n" }
        Write-OpsUtf8NoBom -Path $LogPath -Payload $payload
    }
    if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $name))) { throw "A51 $Suffix container cleanup failed" }
    $createdContainers[$Suffix] = $false
    $containers[$Suffix] = $null
    if ($exitCode -eq -1) { throw "A51 $Suffix timed out" }
    if ($exitCode -ne 0) { throw "A51 $Suffix failed with exit code $exitCode" }
}

function Write-A51ArtifactInventory {
    param([Parameter(Mandatory = $true)][string]$Directory)
    $required = @(
        'a51-preflight.log', 'a51-seed-report.json', 'a51-seed.log', 'a51-dry-run.json', 'a51-full.json',
        'a51-post-dry-run.json', 'a51-verify.json', 'a51-verify.log',
        'a51-integration-tests.jsonl', 'a51-integration-tests.json', 'a51-secret-scan.json',
        'a51-environment.json', 'a51-mysql-contract.tsv', 'a51-migration.log',
        'a51-report.json', 'a51-report.log'
    )
    $files = @()
    foreach ($relative in $required) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "A51 artifact missing: $relative" }
        $info = Get-Item -LiteralPath $path
        if ($info.Length -le 0) { throw "A51 artifact empty: $relative" }
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A51'
        evidence_class = 'formal'
        generated_at = [DateTimeOffset]::UtcNow.ToString('o')
        files = $files
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a51-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

function Write-A51SecretScan {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string[]]$Forbidden,
        [Parameter(Mandatory = $true)][string[]]$FullKeyIDs
    )
    $files = @(Get-ChildItem -LiteralPath $Directory -File | Where-Object {
        $_.Name -notin @('a51-secret-scan.json', 'a51-artifacts.json', 'a51-cleanup.json', 'stdout.log', 'stderr.log')
    })
    $hits = 0
    $keyLeaks = 0
    foreach ($file in $files) {
        $payload = [System.IO.File]::ReadAllText($file.FullName)
        foreach ($value in $Forbidden) {
            if (-not [string]::IsNullOrEmpty($value) -and $payload.Contains($value)) { $hits++ }
        }
        foreach ($keyID in $FullKeyIDs) {
            if ($payload.Contains($keyID)) { $keyLeaks++ }
        }
    }
    $passed = $hits -eq 0 -and $keyLeaks -eq 0
    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A51'
        status = if ($passed) { 'passed' } else { 'failed' }
        files_scanned = $files.Count
        forbidden_hits = $hits
        full_key_id_leaks = $keyLeaks
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a51-secret-scan.json') -Payload ($report | ConvertTo-Json -Depth 4)
    if (-not $passed) { throw 'A51 evidence secret scan failed' }
}

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID -or $env:ACCEPTANCE_EVIDENCE_CLASS -cne 'formal') {
        throw 'A51 must be launched by the formal acceptance runner'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory'
    }
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $relativeEvidence = Get-OpsRepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    if (-not $relativeEvidence.StartsWith('artifacts/acceptance/A51/', [System.StringComparison]::Ordinal)) {
        throw 'A51 evidence must remain under artifacts/acceptance/A51/'
    }

    $runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
    $runLabel = "new-api-pilot.acceptance-run=$runToken"
    $oldKey = New-OpsBase64Key
    $newKey = New-OpsBase64Key
    $alternateKey = New-OpsBase64Key
    $oldKeyID = Get-OpsKeyFingerprint -Base64Key $oldKey
    $newKeyID = Get-OpsKeyFingerprint -Base64Key $newKey
    $alternateKeyID = Get-OpsKeyFingerprint -Base64Key $alternateKey
    if ($oldKeyID -eq $newKeyID -or $oldKeyID -eq $alternateKeyID -or $newKeyID -eq $alternateKeyID) {
        throw 'A51 generated duplicate key fingerprints'
    }
    $databaseDSN = "root:@tcp(mysql-a51:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $testDatabaseDSN = "root:@tcp(mysql-a51:3306)/${testDatabaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"

    $maintenanceEnvironment = @(
        "DATABASE_DSN=$databaseDSN", "OLD_ENCRYPTION_KEY=$oldKey", "NEW_ENCRYPTION_KEY=$newKey",
        'SQL_MAX_IDLE_CONNS=2', 'SQL_MAX_OPEN_CONNS=4', 'SQL_MAX_LIFETIME_SECONDS=60'
    )
    $migrationEnvironment = @(
        'APP_ENV=test', 'PORT=3000', "DATABASE_DSN=$databaseDSN",
        'SESSION_SECRET=YTUxLXNlc3Npb24tc2VjcmV0LWV4YWN0bHktMzItYnl0ZXMh', "ENCRYPTION_KEY=$oldKey",
        'SESSION_COOKIE_SECURE=false', 'EXPORT_DIR=/tmp/a51-exports', 'PUBLIC_ORIGIN=http://a51.invalid',
        'TRUSTED_PROXIES=', 'UPSTREAM_ALLOWED_HOST_SUFFIXES=', 'UPSTREAM_ALLOWED_CIDRS=172.16.0.0/12',
        'UPSTREAM_CA_FILE=',
        'UPSTREAM_CONNECT_TIMEOUT_SECONDS=5', 'UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS=15',
        'UPSTREAM_REQUEST_TIMEOUT_SECONDS=30', 'UPSTREAM_EXPORT_TIMEOUT_SECONDS=120',
        'DINGTALK_ALLOWED_HOSTS=', 'METRICS_ALLOWED_CIDRS=127.0.0.0/8', 'TZ=Asia/Shanghai',
        'SQL_MAX_IDLE_CONNS=2', 'SQL_MAX_OPEN_CONNS=4', 'SQL_MAX_LIFETIME_SECONDS=60'
    )
    Invoke-A51ConfigurationPreflight -Environment @($migrationEnvironment + $maintenanceEnvironment) `
        -LogPath (Join-Path $evidenceDirectory 'a51-preflight.log')

    $networkName = "new-api-pilot-a51-$runToken-network"
    $mysqlVolumeName = "new-api-pilot-a51-$runToken-mysql"
    $containers.mysql = "new-api-pilot-a51-$runToken-mysql"

    [void](Invoke-OpsDocker -Arguments @('version', '--format', '{{.Client.Version}}|{{.Server.Version}}') -TimeoutSeconds 60)
    foreach ($image in @($mysqlImage, $goImage)) {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('image', 'inspect', $image) -TimeoutSeconds 30
        if ($inspect.ExitCode -ne 0) { [void](Invoke-OpsDocker -Arguments @('pull', $image) -TimeoutSeconds 1200) }
    }
    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"

    $containers.warm = "new-api-pilot-a51-$runToken-warm"
    $createdContainers.warm = $false
    try {
        [void](Invoke-OpsDocker -Arguments @(
            'create', '--name', $containers.warm,
            '--label', 'new-api-pilot.acceptance=A51', '--label', $runLabel,
            '--workdir', '/workspace', '--mount', $repositoryMount,
            '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
            $goImage, 'go', 'mod', 'download'
        ))
        $createdContainers.warm = $true
        $containerResourcesCreated = $true
    }
    catch {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', $containers.warm) -TimeoutSeconds 30
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $createdContainers.warm = $true
            $containerResourcesCreated = $true
        }
        throw
    }
    [void](Invoke-OpsDocker -Arguments @('start', $containers.warm) -TimeoutSeconds 30)
    if ((Wait-OpsContainer -Container $containers.warm -TimeoutSeconds 600) -ne 0) { throw 'A51 module warm-up failed' }
    if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $containers.warm))) { throw 'A51 warm-up cleanup failed' }
    $createdContainers.warm = $false
    $containers.warm = $null

    try {
        [void](Invoke-OpsDocker -Arguments @('network', 'create', '--internal', '--label', 'new-api-pilot.acceptance=A51', '--label', $runLabel, $networkName))
        $networkCreated = $true
        $networkWasCreated = $true
    }
    catch {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('network', 'inspect', $networkName) -TimeoutSeconds 30
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $networkCreated = $true
            $networkWasCreated = $true
        }
        throw
    }
    try {
        [void](Invoke-OpsDocker -Arguments @('volume', 'create', '--label', 'new-api-pilot.acceptance=A51', '--label', $runLabel, $mysqlVolumeName))
        $mysqlVolumeCreated = $true
        $mysqlVolumeWasCreated = $true
    }
    catch {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('volume', 'inspect', $mysqlVolumeName) -TimeoutSeconds 30
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $mysqlVolumeCreated = $true
            $mysqlVolumeWasCreated = $true
        }
        throw
    }
    $createdContainers.mysql = $false
    try {
        [void](Invoke-OpsDocker -Arguments @(
            'run', '--detach', '--name', $containers.mysql, '--network', $networkName, '--network-alias', 'mysql-a51',
            '--label', 'new-api-pilot.acceptance=A51', '--label', $runLabel,
            '--memory', '2g', '--cpus', '2', '--mount', "type=volume,source=$mysqlVolumeName,target=/var/lib/mysql",
            '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$databaseName", '--env', 'TZ=Asia/Shanghai',
            '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
            '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '75', '--health-start-period', '10s',
            $mysqlImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci',
            '--transaction-isolation=READ-COMMITTED', '--default-time-zone=+08:00', '--skip-log-bin'
        ))
        $createdContainers.mysql = $true
        $containerResourcesCreated = $true
    }
    catch {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', $containers.mysql) -TimeoutSeconds 30
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $createdContainers.mysql = $true
            $containerResourcesCreated = $true
        }
        throw
    }
    $healthDeadline = [DateTimeOffset]::UtcNow.AddSeconds(180)
    while ($true) {
        $state = (Invoke-OpsDocker -Arguments @('inspect', '--format', '{{.State.Status}} {{.State.Health.Status}}', $containers.mysql) -TimeoutSeconds 15).Stdout.Trim()
        if ($state -ceq 'running healthy') { break }
        if ($state -match '^(exited|dead) ' -or $state -match ' unhealthy$') { throw 'A51 MySQL did not become healthy' }
        if ([DateTimeOffset]::UtcNow -ge $healthDeadline) { throw 'A51 MySQL health wait timed out' }
        Start-Sleep -Seconds 2
    }
    $mysqlContract = Invoke-OpsDocker -Arguments @(
        'exec', $containers.mysql, 'mysql', '-uroot', '-N', '-B', '-e',
        "SELECT VERSION(), @@transaction_isolation, @@character_set_server, @@collation_server, @@global.time_zone;"
    ) -TimeoutSeconds 60
    $mysqlFields = $mysqlContract.Stdout.Trim() -split "`t"
    if ($mysqlFields.Count -ne 5 -or -not $mysqlFields[0].StartsWith('8.4.') -or $mysqlFields[1] -cne 'READ-COMMITTED' -or
        $mysqlFields[2] -cne 'utf8mb4' -or $mysqlFields[3] -cne 'utf8mb4_unicode_ci' -or $mysqlFields[4] -cne '+08:00') {
        throw 'A51 MySQL runtime contract failed'
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a51-mysql-contract.tsv') -Payload ($mysqlContract.Stdout.Trim() + "`n")
    [void](Invoke-OpsDocker -Arguments @(
        'exec', $containers.mysql, 'mysql', '-uroot', '-e',
        "CREATE DATABASE ${testDatabaseName} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    ) -TimeoutSeconds 60)

    Invoke-A51Tool -Suffix 'migration' -Command @('go', 'run', '.', 'migrate') -Environment $migrationEnvironment `
        -TimeoutSeconds 600 -LogPath (Join-Path $evidenceDirectory 'a51-migration.log')
    Invoke-A51Tool -Suffix 'seed' -Command @(
        'go', 'run', './scripts/acceptance', 'a51-seed', '-fixture', '/workspace/testdata/design/f05-ops-capacity.yaml',
        '-report', '/evidence/a51-seed-report.json'
    ) -Environment $maintenanceEnvironment -TimeoutSeconds 300 -LogPath (Join-Path $evidenceDirectory 'a51-seed.log')
    Invoke-A51Tool -Suffix 'dry' -Command @('go', 'run', '.', 'secrets', 'reencrypt', '--dry-run', '--batch-size=2') `
        -Environment $maintenanceEnvironment -TimeoutSeconds 300 -OutputPath (Join-Path $evidenceDirectory 'a51-dry-run.json')
    Invoke-A51Tool -Suffix 'full' -Command @('go', 'run', '.', 'secrets', 'reencrypt', '--batch-size=2') `
        -Environment $maintenanceEnvironment -TimeoutSeconds 300 -OutputPath (Join-Path $evidenceDirectory 'a51-full.json')
    Invoke-A51Tool -Suffix 'post' -Command @('go', 'run', '.', 'secrets', 'reencrypt', '--dry-run', '--batch-size=2') `
        -Environment $maintenanceEnvironment -TimeoutSeconds 300 -OutputPath (Join-Path $evidenceDirectory 'a51-post-dry-run.json')
    Invoke-A51Tool -Suffix 'verify' -Command @(
        'go', 'run', './scripts/acceptance', 'a51-verify', '-report', '/evidence/a51-verify.json'
    ) -Environment $maintenanceEnvironment -TimeoutSeconds 300 -LogPath (Join-Path $evidenceDirectory 'a51-verify.log')

    Invoke-A51Tool -Suffix 'integration' -Command @(
        'go', 'test', '-count=1', '-run', '^TestMySQLReencrypt', '-json', './internal/ops'
    ) -Environment @("TEST_DATABASE_DSN=$testDatabaseDSN") -TimeoutSeconds 900 `
        -OutputPath (Join-Path $evidenceDirectory 'a51-integration-tests.jsonl')
    $expectedTests = @(
        'TestMySQLReencryptDryRunAndSuccess',
        'TestMySQLReencryptResumesAndRejectsDifferentKeyPair',
        'TestMySQLReencryptCASFailureRollsBackAllUpdates',
        'TestMySQLReencryptRejectsBadCiphertextWithoutWrites'
    )
    $passedTests = @()
    $failedTests = @()
    $skippedTests = @()
    foreach ($line in Get-Content -Encoding UTF8 (Join-Path $evidenceDirectory 'a51-integration-tests.jsonl')) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $event = $line | ConvertFrom-Json
        $testProperty = $event.PSObject.Properties['Test']
        if ($null -eq $testProperty -or [string]::IsNullOrWhiteSpace([string]$testProperty.Value)) { continue }
        $testName = [string]$testProperty.Value
        if ($event.Action -ceq 'pass') { $passedTests += $testName }
        elseif ($event.Action -ceq 'fail') { $failedTests += $testName }
        elseif ($event.Action -ceq 'skip') { $skippedTests += $testName }
    }
    $passedTests = @($passedTests | Sort-Object -Unique)
    $failedTests = @($failedTests | Sort-Object -Unique)
    $skippedTests = @($skippedTests | Sort-Object -Unique)
    if ($failedTests.Count -ne 0 -or $skippedTests.Count -ne 0 -or $passedTests.Count -ne $expectedTests.Count) {
        throw 'A51 fault-injection test matrix was incomplete'
    }
    foreach ($test in $expectedTests) {
        if ($test -notin $passedTests) { throw "A51 required fault-injection test missing: $test" }
    }
    $integrationSummary = [ordered]@{
        schema_version = 1
        acceptance_id = 'A51'
        status = 'passed'
        tests_passed = $passedTests.Count
        tests_failed = $failedTests.Count
        tests_skipped = $skippedTests.Count
        test_names = $passedTests
        log_path = 'a51-integration-tests.jsonl'
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a51-integration-tests.json') -Payload ($integrationSummary | ConvertTo-Json -Depth 4)

    $gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot
    $environment = [ordered]@{
        schema_version = 1
        acceptance_id = 'A51'
        evidence_class = 'formal'
        commit = $gitState.Commit
        worktree_dirty = $gitState.WorktreeDirty
        mysql = [ordered]@{
            version = $mysqlFields[0]
            transaction_isolation = $mysqlFields[1]
            character_set_server = $mysqlFields[2]
            collation_server = $mysqlFields[3]
            time_zone = $mysqlFields[4]
        }
        network = [ordered]@{ internal = $true; host_ports = @() }
        databases = @($databaseName, $testDatabaseName)
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a51-environment.json') -Payload ($environment | ConvertTo-Json -Depth 6)

    $forbidden = @(
        $oldKey, $newKey, $alternateKey, $databaseDSN, $testDatabaseDSN,
        'a51-site-token-alpha-never-log', 'a51-site-token-beta-never-log',
        'https://oapi.dingtalk.com/robot/send?access_token=a51-never-log', 'a51-signing-secret-never-log'
    )
    Write-A51SecretScan -Directory $evidenceDirectory -Forbidden $forbidden -FullKeyIDs @($oldKeyID, $newKeyID, $alternateKeyID)

    Invoke-A51Tool -Suffix 'report' -Command @(
        'go', 'run', './scripts/acceptance', 'a51-report',
        '-seed', '/evidence/a51-seed-report.json', '-dry-run', '/evidence/a51-dry-run.json',
        '-full-run', '/evidence/a51-full.json', '-post-dry-run', '/evidence/a51-post-dry-run.json',
        '-verify', '/evidence/a51-verify.json', '-integration', '/evidence/a51-integration-tests.json',
        '-scan', '/evidence/a51-secret-scan.json', '-environment', '/evidence/a51-environment.json',
        '-output', '/evidence/a51-report.json'
    ) -Environment @('A51_ISOLATED_REPORT=true') -TimeoutSeconds 300 -LogPath (Join-Path $evidenceDirectory 'a51-report.log')
    Write-A51SecretScan -Directory $evidenceDirectory -Forbidden $forbidden -FullKeyIDs @($oldKeyID, $newKeyID, $alternateKeyID)
    Write-A51ArtifactInventory -Directory $evidenceDirectory
    Write-Output 'A51 formal isolated key re-encryption acceptance passed.'
}
catch {
    $failure = $_
    if ($null -ne $evidenceDirectory -and (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
        foreach ($entry in $containers.GetEnumerator()) {
            $container = [string]$entry.Value
            if ([string]::IsNullOrWhiteSpace($container)) { continue }
            $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', $container) -TimeoutSeconds 30
            if ($inspect.ExitCode -eq 0) {
                Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ("$($entry.Key)-inspect.json")) `
                    -Payload (Protect-OpsDiagnostic -Payload $inspect.Stdout)
                $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $container) -TimeoutSeconds 30
                Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ("$($entry.Key).log")) `
                    -Payload (Protect-OpsDiagnostic -Payload ($logs.Stdout + $logs.Stderr))
            }
        }
    }
}
finally {
    foreach ($entry in @($containers.GetEnumerator())) {
        $key = [string]$entry.Key
        $container = [string]$entry.Value
        $wasCreated = $createdContainers.ContainsKey($key) -and [bool]$createdContainers[$key]
        if ($wasCreated -and -not [string]::IsNullOrWhiteSpace($container)) {
            if (Remove-OpsDockerResource -Arguments @('rm', '--force', $container)) {
                $createdContainers[$key] = $false
            }
            else {
                $containerCleanupFailed = $true
            }
        }
    }
    if ($networkCreated -and -not [string]::IsNullOrWhiteSpace($networkName)) {
        if (Remove-OpsDockerResource -Arguments @('network', 'rm', $networkName)) {
            $networkCreated = $false
        }
        else {
            $networkCleanupFailed = $true
        }
    }
    if ($mysqlVolumeCreated -and -not [string]::IsNullOrWhiteSpace($mysqlVolumeName)) {
        if (Remove-OpsDockerResource -Arguments @('volume', 'rm', $mysqlVolumeName)) {
            $mysqlVolumeCreated = $false
        }
        else {
            $volumeCleanupFailed = $true
        }
    }
    $cleanupFailed = $containerCleanupFailed -or $networkCleanupFailed -or $volumeCleanupFailed
    if (-not [string]::IsNullOrWhiteSpace($runLabel) -and -not [string]::IsNullOrWhiteSpace($evidenceDirectory) -and
        (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
        $containerSweep = Get-OpsResidualSweep -Arguments @('ps', '-a', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Names}}')
        $networkSweep = Get-OpsResidualSweep -Arguments @('network', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Name}}')
        $volumeSweep = Get-OpsResidualSweep -Arguments @('volume', 'ls', '--filter', "label=$runLabel", '--format', '{{.Name}}')
        $imageSweep = Get-OpsResidualSweep -Arguments @('image', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Repository}}:{{.Tag}}')
        $sweepsSucceeded = $containerSweep.Succeeded -and $networkSweep.Succeeded -and $volumeSweep.Succeeded -and $imageSweep.Succeeded
        $noResiduals = $containerSweep.Items.Count -eq 0 -and $networkSweep.Items.Count -eq 0 -and
            $volumeSweep.Items.Count -eq 0 -and $imageSweep.Items.Count -eq 0
        $cleanupPassed = (-not $cleanupFailed) -and $sweepsSucceeded -and $noResiduals
        $containerLifecycle = if (-not $containerResourcesCreated) { 'not_created' }
            elseif (-not $containerCleanupFailed -and $containerSweep.Items.Count -eq 0) { 'created_and_removed' }
            else { 'cleanup_failed' }
        $networkLifecycle = if (-not $networkWasCreated) { 'not_created' }
            elseif (-not $networkCleanupFailed -and $networkSweep.Items.Count -eq 0) { 'created_and_removed' }
            else { 'cleanup_failed' }
        $volumeLifecycle = if (-not $mysqlVolumeWasCreated) { 'not_created' }
            elseif (-not $volumeCleanupFailed -and $volumeSweep.Items.Count -eq 0) { 'created_and_removed' }
            else { 'cleanup_failed' }
        $cleanup = [ordered]@{
            schema_version = 1
            acceptance_id = 'A51'
            evidence_class = 'formal'
            passed = $cleanupPassed
            generated_at = [DateTimeOffset]::UtcNow.ToString('o')
            sweeps_succeeded = $sweepsSucceeded
            lifecycle = [ordered]@{
                containers = $containerLifecycle
                networks = $networkLifecycle
                volumes = $volumeLifecycle
            }
            residuals = [ordered]@{
                containers = @($containerSweep.Items)
                networks = @($networkSweep.Items)
                volumes = @($volumeSweep.Items)
                images = @($imageSweep.Items)
            }
        }
        try { Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a51-cleanup.json') -Payload ($cleanup | ConvertTo-Json -Depth 6) }
        catch { $cleanupPassed = $false }
        if (-not $cleanupPassed) { $cleanupFailed = $true }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure.Exception.Message)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A51 exact-resource cleanup failed.')
    exit 1
}
exit 0
