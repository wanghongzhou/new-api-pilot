[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = 'A22'
$databaseName = 'pilot_a22'
$goImage = 'golang:1.25.1'
$mysqlImage = 'mysql:8.4'
$canonicalCommand = @(
    'powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'scripts/acceptance/run-a22.ps1'
)
$requiredArtifacts = @(
    'a22-command.json', 'a22-environment.json', 'a22-fixture.json', 'a22-migration.log',
    'a22-seed.json', 'a22-source-snapshot.json', 'a22-backup.json', 'a22-backup.log',
    'a22-negative-manifest.json', 'a22-negative-target-mismatch.json', 'a22-restore.json',
    'a22-restore.log', 'a22-verify-restore.json', 'a22-target-snapshot.json', 'a22-app-smoke.json',
    'a22-rpo-rto.json', 'a22-secret-scan.json', 'a22-report.json', 'a22-cleanup.json'
)

$createdContainers = @{}
$containerResourcesCreated = $false
$networkCreated = $false
$volumesCreated = $false
$imageCreated = $false
$networkMayExist = $false
$sourceVolumeMayExist = $false
$targetVolumeMayExist = $false
$workVolumeMayExist = $false
$toolsImageMayExist = $false
$evidenceDirectory = $null
$networkName = $null
$sourceVolumeName = $null
$targetVolumeName = $null
$workVolumeName = $null
$toolsImageName = $null
$runLabel = $null
$repositoryMount = $null
$evidenceMount = $null
$workMount = $null

function Get-A22ImageIdentity {
    param(
        [Parameter(Mandatory = $true)][string]$Reference,
        [switch]$LocalImage
    )

    if ($LocalImage.IsPresent) {
        $result = Invoke-OpsProcess -FileName 'docker' -Arguments @(
            'image', 'inspect', $Reference, '--format', '{{.Id}}'
        ) -TimeoutSeconds 30
        if ($result.TimedOut -or $result.ExitCode -ne 0 -or $result.Stdout.Trim() -notmatch '^sha256:[0-9a-f]{64}$') {
            throw 'A22 local tools image identity is unavailable.'
        }
        $id = $result.Stdout.Trim()
        return [ordered]@{ reference = $Reference; id = $id; digest = $id }
    }

    $result = Invoke-OpsProcess -FileName 'docker' -Arguments @(
        'image', 'inspect', $Reference, '--format', '{{.Id}}|{{index .RepoDigests 0}}'
    ) -TimeoutSeconds 30
    if ($result.TimedOut -or $result.ExitCode -ne 0) {
        throw "A22 required image is unavailable: $Reference"
    }
    $fields = $result.Stdout.Trim().Split('|')
    if ($fields.Count -ne 2 -or $fields[0] -notmatch '^sha256:[0-9a-f]{64}$' -or
        $fields[1] -notmatch '^[^|]+@sha256:[0-9a-f]{64}$') {
        throw "A22 image identity is invalid: $Reference"
    }
    return [ordered]@{ reference = $Reference; id = $fields[0]; digest = $fields[1] }
}

function Get-A22Fingerprint {
    param([Parameter(Mandatory = $true)][string]$Value)

    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Value)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        return ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant().Substring(0, 12)
    }
    finally { $sha.Dispose() }
}

function Wait-A22HealthyContainer {
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
                throw "A22 database container stopped before becoming healthy: $Container"
            }
        }
        if ([DateTimeOffset]::UtcNow -ge $deadline) {
            throw "A22 database health wait timed out: $Container"
        }
        Start-Sleep -Seconds 2
    }
}

function Get-A22MySQLFields {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][string]$Query,
        [Parameter(Mandatory = $true)][ValidateRange(1, 12)][int]$ExpectedFields
    )

    $result = Invoke-OpsDocker -Arguments @(
        'exec', $Container, 'mysql', '--batch', '--skip-column-names', '--user=root',
        $databaseName, '--execute', $Query
    ) -TimeoutSeconds 30
    $fields = $result.Stdout.Trim().Split("`t")
    if ($fields.Count -ne $ExpectedFields) {
        throw 'A22 database contract query returned an unexpected shape.'
    }
    return $fields
}

function Get-A22TableCount {
    param([Parameter(Mandatory = $true)][string]$Container)

    $fields = @(Get-A22MySQLFields -Container $Container -ExpectedFields 1 -Query @'
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_type='BASE TABLE'
'@)
    return [int64]$fields[0]
}

function Invoke-A22Task {
    param(
        [Parameter(Mandatory = $true)][string]$Suffix,
        [Parameter(Mandatory = $true)][string]$Image,
        [Parameter(Mandatory = $true)][string[]]$Command,
        [string[]]$Environment = @(),
        [Parameter(Mandatory = $true)][ValidateRange(1, 20000)][int]$TimeoutSeconds,
        [string]$LogPath,
        [switch]$UseGoCache,
        [switch]$AllowFailure
    )

    $container = "new-api-pilot-a22-$runToken-$Suffix"
    $createdContainers[$container] = $false
    $arguments = @(
        'create', '--name', $container, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel,
        '--memory', '2g', '--cpus', '2', '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount, '--mount', $workMount
    )
    if ($UseGoCache.IsPresent) {
        $arguments += @(
            '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
            '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build'
        )
    }
    foreach ($entry in $Environment) { $arguments += @('--env', $entry) }
    $arguments += @($Image) + $Command
    try {
        [void](Invoke-OpsDocker -Arguments $arguments -TimeoutSeconds 120)
        $createdContainers[$container] = $true
        $script:containerResourcesCreated = $true
        [void](Invoke-OpsDocker -Arguments @('start', $container) -TimeoutSeconds 30)
        $exitCode = Wait-OpsContainer -Container $container -TimeoutSeconds $TimeoutSeconds
        if ($exitCode -eq -1) {
            [void](Remove-OpsDockerResource -Arguments @('kill', $container))
        }
        $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $container) -TimeoutSeconds 60
        if (-not [string]::IsNullOrWhiteSpace($LogPath)) {
            $payload = Protect-OpsDiagnostic -Payload ($logs.Stdout + $logs.Stderr)
            if ([string]::IsNullOrWhiteSpace($payload)) { $payload = "A22 $Suffix exit_code=$exitCode`n" }
            Write-OpsUtf8NoBom -Path $LogPath -Payload $payload
        }
        if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $container))) {
            throw "A22 task container cleanup failed: $Suffix"
        }
        $createdContainers[$container] = $false
        if ($exitCode -eq -1) { throw "A22 task timed out: $Suffix" }
        if (-not $AllowFailure.IsPresent -and ($exitCode -ne 0 -or $logs.TimedOut -or $logs.ExitCode -ne 0)) {
            throw "A22 task failed: $Suffix"
        }
        return [pscustomobject]@{
            ExitCode = $exitCode
            Stdout = $logs.Stdout
            Stderr = $logs.Stderr
        }
    }
    catch {
        if ($createdContainers[$container]) {
            [void](Remove-OpsDockerResource -Arguments @('rm', '--force', $container))
            $createdContainers[$container] = $false
        }
        throw
    }
}

function Get-A22ApplicationEnvironment {
    param([Parameter(Mandatory = $true)][string]$DatabaseDSN)

    return @(
        'APP_ENV=test', 'PORT=3000', "DATABASE_DSN=$DatabaseDSN",
        "SESSION_SECRET=$sessionSecret", "ENCRYPTION_KEY=$encryptionKey",
        "PLATFORM_BOOTSTRAP_ADMIN_PASSWORD=$adminPassword", 'SESSION_COOKIE_SECURE=false',
        'EXPORT_DIR=/work/exports', 'PUBLIC_ORIGIN=http://a22.invalid', 'TRUSTED_PROXIES=',
        'UPSTREAM_ALLOWED_HOST_SUFFIXES=', 'UPSTREAM_ALLOWED_CIDRS=172.16.0.0/12',
        'UPSTREAM_CA_FILE=',
        'UPSTREAM_CONNECT_TIMEOUT_SECONDS=5', 'UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS=15',
        'UPSTREAM_REQUEST_TIMEOUT_SECONDS=30', 'UPSTREAM_EXPORT_TIMEOUT_SECONDS=120',
        'DINGTALK_ALLOWED_HOSTS=', 'METRICS_ALLOWED_CIDRS=127.0.0.0/8', 'TZ=Asia/Shanghai',
        'SQL_MAX_IDLE_CONNS=2', 'SQL_MAX_OPEN_CONNS=4', 'SQL_MAX_LIFETIME_SECONDS=60'
    )
}

function Get-A22SecretEnvironment {
    param([Parameter(Mandatory = $true)][string]$DatabaseDSN)

    return @(
        "DATABASE_DSN=$DatabaseDSN", "ENCRYPTION_KEY=$encryptionKey",
        "A22_ADMIN_PASSWORD=$adminPassword", "A22_SITE_TOKEN=$siteToken",
        "A22_SECRET_SETTING=$secretSetting", 'SQL_MAX_IDLE_CONNS=2',
        'SQL_MAX_OPEN_CONNS=4', 'SQL_MAX_LIFETIME_SECONDS=60'
    )
}

function Get-A22RestoreEnvironment {
    param(
        [Parameter(Mandatory = $true)][string]$Manifest,
        [Parameter(Mandatory = $true)][string]$ReleaseRoot,
        [Parameter(Mandatory = $true)][string]$DefaultsFile,
        [Parameter(Mandatory = $true)][string]$DatabaseDSN
    )

    return @(
        "RESTORE_MANIFEST=$Manifest", "RESTORE_RELEASE_ROOT=$ReleaseRoot", "MYSQL_DEFAULTS_FILE=$DefaultsFile",
        "MYSQL_DATABASE=$databaseName", "DATABASE_DSN=$DatabaseDSN", "ENCRYPTION_KEY=$encryptionKey",
        'NEW_API_PILOT_BIN=/work/new-api-pilot', 'SQL_MAX_IDLE_CONNS=2',
        'SQL_MAX_OPEN_CONNS=4', 'SQL_MAX_LIFETIME_SECONDS=60'
    )
}

function Get-A22SnapshotHash {
    param(
        [Parameter(Mandatory = $true)][string]$Suffix,
        [Parameter(Mandatory = $true)][string]$DatabaseDSN,
        [Parameter(Mandatory = $true)][string]$Role
    )

    $path = "/work/$Suffix.json"
    [void](Invoke-A22Task -Suffix $Suffix -Image $toolsImageName -TimeoutSeconds 120 `
        -Environment (Get-A22SecretEnvironment -DatabaseDSN $DatabaseDSN) `
        -Command @('/work/a22tool', 'snapshot', '--role', $Role, '--report', $path))
    $read = Invoke-A22Task -Suffix "$Suffix-read" -Image $toolsImageName -TimeoutSeconds 30 `
        -Command @('jq', '-r', '.snapshot_sha256', $path)
    $hash = $read.Stdout.Trim()
    if ($hash -notmatch '^[0-9a-f]{64}$') { throw 'A22 diagnostic snapshot hash is invalid.' }
    return $hash
}

function Write-A22ArtifactInventory {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $files = @()
    foreach ($relative in $requiredArtifacts) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "A22 required artifact is missing: $relative"
        }
        $info = Get-Item -LiteralPath $path
        if ($info.Length -le 0 -and $relative -notlike '*.log') {
            throw "A22 required artifact is empty: $relative"
        }
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A22'
        evidence_class = $EvidenceClass
        files = $files
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a22-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

function Write-A22SecretScan {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string[]]$Files
    )

    $forbiddenHits = 0
    $dsnLeaks = 0
    $keyLeaks = 0
    $urlLeaks = 0
    $forbidden = @($adminPassword, $siteToken, $secretSetting)
    $keys = @($encryptionKey, $sessionSecret)
    $dsns = @($sourceDSN, $targetDSN)
    foreach ($relative in $Files) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "A22 scan input missing: $relative" }
        $payload = [System.IO.File]::ReadAllText($path)
        foreach ($value in $forbidden) {
            if (-not [string]::IsNullOrEmpty($value) -and $payload.Contains($value)) { $forbiddenHits++ }
        }
        foreach ($value in $keys) {
            if (-not [string]::IsNullOrEmpty($value) -and $payload.Contains($value)) { $keyLeaks++ }
        }
        foreach ($value in $dsns) {
            if (-not [string]::IsNullOrEmpty($value) -and $payload.Contains($value)) { $dsnLeaks++ }
        }
        if ($payload -match '(?i)https?://[^\s]*(?:access_token|token|secret|key|signature)=[^&\s]+') { $urlLeaks++ }
    }
    $passed = $forbiddenHits -eq 0 -and $dsnLeaks -eq 0 -and $keyLeaks -eq 0 -and $urlLeaks -eq 0
    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A22'
        status = if ($passed) { 'passed' } else { 'failed' }
        files_scanned = $Files.Count
        forbidden_hits = $forbiddenHits
        dsn_leaks = $dsnLeaks
        key_leaks = $keyLeaks
        url_credential_leaks = $urlLeaks
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a22-secret-scan.json') -Payload ($report | ConvertTo-Json -Depth 4)
    if (-not $passed) { throw 'A22 evidence secret scan failed.' }
}

$failure = $null
$cleanupFailed = $false
$sourceName = $null
$targetName = $null
$appName = $null
$developmentMode = $false
$evidenceClass = 'formal'

try {
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_ID) -and
        [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -and
        [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_CLASS)) {
        $developmentMode = $true
        $evidenceClass = 'development'
        $stamp = [DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmss.fffffffZ')
        $evidenceDirectory = Join-Path $repositoryRoot "artifacts\smoke\A22-dev-$stamp-$PID"
        [void](New-Item -ItemType Directory -Path $evidenceDirectory -Force)
    }
    elseif ($env:ACCEPTANCE_ID -ceq 'A22' -and $env:A22_DEVELOPMENT -ceq 'true' -and
        -not [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR)) {
        $developmentMode = $true
        $evidenceClass = 'development'
        if (-not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
            -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
            throw 'A22 development evidence directory must be an existing absolute directory.'
        }
        $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    }
    else {
        if ($env:ACCEPTANCE_ID -cne 'A22' -or $env:ACCEPTANCE_EVIDENCE_CLASS -cne 'formal' -or
            [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE) -or
            [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
            -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
            -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
            throw 'A22 formal evidence must be invoked by the canonical acceptance wrapper.'
        }
        $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    }

    $relativeEvidence = Get-OpsRepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    if ($developmentMode) {
        if (-not $relativeEvidence.StartsWith('artifacts/smoke/A22-dev-', [System.StringComparison]::Ordinal)) {
            throw 'A22 development evidence must remain under artifacts/smoke/A22-dev-*.'
        }
    }
    elseif (-not $relativeEvidence.StartsWith('artifacts/acceptance/A22/', [System.StringComparison]::Ordinal)) {
        throw 'A22 formal evidence must remain under artifacts/acceptance/A22/.'
    }

    $runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
    $runLabel = "new-api-pilot.acceptance-run=$runToken"
    $networkName = "new-api-pilot-a22-$runToken-network"
    $sourceVolumeName = "new-api-pilot-a22-$runToken-source"
    $targetVolumeName = "new-api-pilot-a22-$runToken-target"
    $workVolumeName = "new-api-pilot-a22-$runToken-work"
    $toolsImageName = "new-api-pilot-a22-tools:$runToken"
    $sourceName = "new-api-pilot-a22-$runToken-source"
    $targetName = "new-api-pilot-a22-$runToken-target"
    $appName = "new-api-pilot-a22-$runToken-app"

    $encryptionKey = New-OpsBase64Key
    $sessionSecret = New-OpsBase64Key
    $adminPassword = 'a22-admin-password-never-log'
    $siteToken = 'a22-site-token-never-log'
    $secretSetting = 'https://oapi.dingtalk.com/robot/send?access_token=a22-never-log'
    $sourceDSN = "root:@tcp(a22-source:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $targetDSN = "root:@tcp(a22-target:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"

    [void](Invoke-OpsDocker -Arguments @('version', '--format', '{{.Client.Version}}|{{.Server.Version}}') -TimeoutSeconds 60)
    $goIdentity = Get-A22ImageIdentity -Reference $goImage
    $mysqlIdentity = Get-A22ImageIdentity -Reference $mysqlImage
    $gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot

    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"
    $workMount = "type=volume,source=$workVolumeName,target=/work"

    foreach ($volume in @($sourceVolumeName, $targetVolumeName, $workVolumeName)) {
        if ($volume -ceq $sourceVolumeName) { $sourceVolumeMayExist = $true }
        elseif ($volume -ceq $targetVolumeName) { $targetVolumeMayExist = $true }
        else { $workVolumeMayExist = $true }
        [void](Invoke-OpsDocker -Arguments @(
            'volume', 'create', '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel, $volume
        ) -TimeoutSeconds 30)
    }
    $volumesCreated = $true

    $networkMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'network', 'create', '--internal', '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel, $networkName
    ) -TimeoutSeconds 30)
    $networkCreated = $true

    foreach ($databaseContainer in @(
        [pscustomobject]@{ Name = $sourceName; Alias = 'a22-source'; Volume = $sourceVolumeName; ServerID = '2201' },
        [pscustomobject]@{ Name = $targetName; Alias = 'a22-target'; Volume = $targetVolumeName; ServerID = '2202' }
    )) {
        $createdContainers[$databaseContainer.Name] = $false
        [void](Invoke-OpsDocker -Arguments @(
            'run', '--detach', '--name', $databaseContainer.Name, '--network', $networkName,
            '--network-alias', $databaseContainer.Alias,
            '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel,
            '--mount', "type=volume,source=$($databaseContainer.Volume),target=/var/lib/mysql",
            '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$databaseName",
            '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
            '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '60', '--health-start-period', '5s',
            $mysqlImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci',
            '--transaction-isolation=READ-COMMITTED', '--default-time-zone=+08:00',
            "--server-id=$($databaseContainer.ServerID)", '--log-bin=mysql-bin', '--binlog-format=ROW'
        ) -TimeoutSeconds 60)
        $createdContainers[$databaseContainer.Name] = $true
        $containerResourcesCreated = $true
    }
    Wait-A22HealthyContainer -Container $sourceName -TimeoutSeconds 180
    Wait-A22HealthyContainer -Container $targetName -TimeoutSeconds 180

    $buildResult = Invoke-OpsProcess -FileName 'docker' -Arguments @(
        'build', '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel,
        '--tag', $toolsImageName, '--file', (Join-Path $repositoryRoot 'scripts\acceptance\a22-tools.Dockerfile'), $repositoryRoot
    ) -TimeoutSeconds 600
    if ($buildResult.TimedOut -or $buildResult.ExitCode -ne 0) { throw 'A22 tools image build failed.' }
    $toolsImageMayExist = $true
    $imageCreated = $true
    $toolsIdentity = Get-A22ImageIdentity -Reference $toolsImageName -LocalImage

    [void](Invoke-A22Task -Suffix 'build' -Image $goImage -TimeoutSeconds 600 -UseGoCache `
        -Environment @('GOPROXY=off', 'GOSUMDB=off', 'CGO_ENABLED=0') `
        -Command @('sh', '-c', @'
set -eu
/usr/local/go/bin/go build -trimpath -o /work/new-api-pilot .
/usr/local/go/bin/go build -trimpath -o /work/a22tool ./scripts/acceptance/a22tool
chmod 0500 /work/new-api-pilot /work/a22tool
'@))
    [void](Invoke-A22Task -Suffix 'defaults' -Image $toolsImageName -TimeoutSeconds 30 -Command @('bash', '-c', @'
set -eu
umask 077
printf '[client]\nhost=a22-source\nport=3306\nuser=root\nprotocol=tcp\n' > /work/source.cnf
printf '[client]\nhost=a22-target\nport=3306\nuser=root\nprotocol=tcp\n' > /work/target.cnf
chmod 0600 /work/source.cnf /work/target.cnf
mkdir -p /work/backups /work/releases /work/exports
'@))

    $sourceFields = @(Get-A22MySQLFields -Container $sourceName -ExpectedFields 7 -Query @'
SELECT VERSION(), @@server_uuid, DATABASE(), @@transaction_isolation,
       @@character_set_server, @@collation_server, @@time_zone
'@)
    $targetFields = @(Get-A22MySQLFields -Container $targetName -ExpectedFields 7 -Query @'
SELECT VERSION(), @@server_uuid, DATABASE(), @@transaction_isolation,
       @@character_set_server, @@collation_server, @@time_zone
'@)
    $sourceLogBin = @(Get-A22MySQLFields -Container $sourceName -ExpectedFields 1 -Query 'SELECT @@log_bin')[0]
    $targetLogBin = @(Get-A22MySQLFields -Container $targetName -ExpectedFields 1 -Query 'SELECT @@log_bin')[0]
    if ($sourceFields[0] -notmatch '^8\.4\.' -or $targetFields[0] -notmatch '^8\.4\.' -or
        $sourceFields[2] -cne $databaseName -or $targetFields[2] -cne $databaseName -or
        $sourceFields[1] -ceq $targetFields[1] -or $sourceFields[3] -cne 'READ-COMMITTED' -or
        $targetFields[3] -cne 'READ-COMMITTED' -or $sourceFields[4] -cne 'utf8mb4' -or
        $targetFields[4] -cne 'utf8mb4' -or $sourceFields[5] -cne 'utf8mb4_unicode_ci' -or
        $targetFields[5] -cne 'utf8mb4_unicode_ci' -or $sourceFields[6] -cne '+08:00' -or
        $targetFields[6] -cne '+08:00' -or $sourceLogBin -ne '1' -or $targetLogBin -ne '1') {
        throw 'A22 source/target MySQL contract is invalid.'
    }
    $sourceUUIDFingerprint = Get-A22Fingerprint -Value $sourceFields[1]
    $targetUUIDFingerprint = Get-A22Fingerprint -Value $targetFields[1]

    $commandReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A22'
        status = 'passed'
        evidence_class = $evidenceClass
        scope = 'controlled_technical_drill'
        command = $canonicalCommand
        tool_commands = @(
            'migrate', 'seed', 'snapshot-source', 'backup', 'negative-manifest',
            'negative-target-mismatch', 'restore', 'verify-restore', 'snapshot-target', 'app-smoke', 'report'
        )
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-command.json') -Payload ($commandReport | ConvertTo-Json -Depth 6)
    $environmentReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A22'
        status = 'passed'
        evidence_class = $evidenceClass
        commit = $gitState.Commit
        worktree_dirty = $gitState.WorktreeDirty
        images = [ordered]@{ go = $goIdentity; mysql = $mysqlIdentity; tools = $toolsIdentity }
        network = [ordered]@{ internal = $true; host_ports = @() }
        source = [ordered]@{
            database = $sourceFields[2]; server_uuid_fingerprint = $sourceUUIDFingerprint; version = $sourceFields[0]
            transaction_isolation = $sourceFields[3]; character_set_server = $sourceFields[4]
            collation_server = $sourceFields[5]; time_zone = $sourceFields[6]; binary_logging_enabled = $true
        }
        target = [ordered]@{
            database = $targetFields[2]; server_uuid_fingerprint = $targetUUIDFingerprint; version = $targetFields[0]
            transaction_isolation = $targetFields[3]; character_set_server = $targetFields[4]
            collation_server = $targetFields[5]; time_zone = $targetFields[6]; binary_logging_enabled = $true
        }
        production_release_authorized = $false
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 8)
    $fixturePath = Join-Path $repositoryRoot 'testdata\design\f05-ops-capacity.yaml'
    $fixtureReport = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'; fixture_id = 'F05'
        path = 'testdata/design/f05-ops-capacity.yaml'
        sha256 = (Get-FileHash -LiteralPath $fixturePath -Algorithm SHA256).Hash.ToLowerInvariant()
        rpo_seconds = 3600; rto_seconds = 14400
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-fixture.json') -Payload ($fixtureReport | ConvertTo-Json -Depth 4)

    [void](Invoke-A22Task -Suffix 'migrate' -Image $toolsImageName -TimeoutSeconds 180 `
        -Environment (Get-A22ApplicationEnvironment -DatabaseDSN $sourceDSN) `
        -Command @('/work/new-api-pilot', 'migrate') -LogPath (Join-Path $evidenceDirectory 'a22-migration.log'))
    [void](Invoke-A22Task -Suffix 'seed' -Image $toolsImageName -TimeoutSeconds 180 `
        -Environment (Get-A22SecretEnvironment -DatabaseDSN $sourceDSN) `
        -Command @('/work/a22tool', 'seed', '--fixture', '/workspace/testdata/design/f05-ops-capacity.yaml',
            '--report', '/evidence/a22-seed.json'))
    [void](Invoke-A22Task -Suffix 'snapshot-source' -Image $toolsImageName -TimeoutSeconds 180 `
        -Environment (Get-A22SecretEnvironment -DatabaseDSN $sourceDSN) `
        -Command @('/work/a22tool', 'snapshot', '--role', 'source', '--report', '/evidence/a22-source-snapshot.json'))
    $sourceSnapshot = Get-Content -Raw -Encoding utf8 (Join-Path $evidenceDirectory 'a22-source-snapshot.json') | ConvertFrom-Json

    $backupEnvironment = @(
        'BACKUP_ROOT=/work/backups', 'MYSQL_DEFAULTS_FILE=/work/source.cnf', "MYSQL_DATABASE=$databaseName",
        "IMAGE_DIGEST=$($toolsIdentity.id)", "ENCRYPTION_KEY=$encryptionKey"
    )
    $backupResult = Invoke-A22Task -Suffix 'backup' -Image $toolsImageName -TimeoutSeconds 300 `
        -Environment $backupEnvironment -Command @('bash', '/workspace/scripts/backup.sh') `
        -LogPath (Join-Path $evidenceDirectory 'a22-backup.log')
    $backupOutput = $backupResult.Stdout | ConvertFrom-Json -ErrorAction Stop
    if ([string]$backupOutput.status -cne 'success' -or [string]$backupOutput.backup_id -notmatch '^backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8,64}$') {
        throw 'A22 backup command output is invalid.'
    }
    $backupID = [string]$backupOutput.backup_id
    $backupDirectory = "/work/backups/$backupID"
    $manifestPath = "$backupDirectory/manifest.json"
    $manifestRead = Invoke-A22Task -Suffix 'manifest-read' -Image $toolsImageName -TimeoutSeconds 30 `
        -Command @('jq', '-c', '{created_at_utc,dump_size_bytes,image_digest,source}', $manifestPath)
    $manifestSummary = $manifestRead.Stdout | ConvertFrom-Json -ErrorAction Stop
    $sourceCoordinatePresent = -not [string]::IsNullOrWhiteSpace([string]$manifestSummary.source.log_file) -and
        [uint64]$manifestSummary.source.log_position -gt 0
    $backupArtifact = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'; backup_id = $backupID
        created_at_utc = [string]$manifestSummary.created_at_utc
        manifest_sha256 = [string]$backupOutput.manifest_sha256
        dump_sha256 = [string]$backupOutput.dump_sha256
        dump_size_bytes = [int64]$manifestSummary.dump_size_bytes
        encryption_key_fingerprint = [string]$backupOutput.encryption_key_id
        image_digest = [string]$manifestSummary.image_digest
        source_coordinate_present = $sourceCoordinatePresent
        atomic_publish = $true
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-backup.json') -Payload ($backupArtifact | ConvertTo-Json -Depth 5)

    [void](Invoke-A22Task -Suffix 'tamper-prepare' -Image $toolsImageName -TimeoutSeconds 60 -Command @(
        'bash', '-c', @'
set -eu
rm -rf /work/tampered-backup /work/negative-manifest-releases
cp -a "$1" /work/tampered-backup
jq '.schema_migrations[0].checksum = "0000000000000000000000000000000000000000000000000000000000000000"' \
  /work/tampered-backup/manifest.json > /work/tampered-backup/manifest.json.tmp
mv /work/tampered-backup/manifest.json.tmp /work/tampered-backup/manifest.json
cd /work/tampered-backup
sha256sum manifest.json | awk '{print $1 "  manifest.json"}' > manifest.json.sha256
'@, '--', $backupDirectory))
    $negativeManifest = Invoke-A22Task -Suffix 'negative-manifest' -Image $toolsImageName -TimeoutSeconds 180 -AllowFailure `
        -Environment (Get-A22RestoreEnvironment -Manifest '/work/tampered-backup/manifest.json' `
            -ReleaseRoot '/work/negative-manifest-releases' -DefaultsFile '/work/target.cnf' -DatabaseDSN $targetDSN) `
        -Command @('bash', '/workspace/scripts/restore.sh')
    $negativeManifestTables = Get-A22TableCount -Container $targetName
    $negativeManifestGate = Invoke-A22Task -Suffix 'negative-manifest-gate' -Image $toolsImageName -TimeoutSeconds 30 -AllowFailure `
        -Command @('bash', '-c', "test -e /work/negative-manifest-releases/$backupID")
    $sourceAfterManifest = Get-A22SnapshotHash -Suffix 'source-after-manifest' -DatabaseDSN $sourceDSN -Role 'source'
    $negativeManifestPassed = $negativeManifest.ExitCode -ne 0 -and $negativeManifestTables -eq 0 -and
        $negativeManifestGate.ExitCode -ne 0 -and $sourceAfterManifest -ceq [string]$sourceSnapshot.snapshot_sha256 -and
        ($negativeManifest.Stderr -match 'backup manifest preflight failed')
    if (-not $negativeManifestPassed) { throw 'A22 manifest tamper branch did not fail before import.' }
    $negativeManifestReport = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'; passed = $true
        failure_stage = 'manifest_preflight'; exit_code = [int]$negativeManifest.ExitCode; import_started = $false
        target_table_count = $negativeManifestTables; release_gate_exists = $false; source_unchanged = $true
        production_release_authorized = $false
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-negative-manifest.json') -Payload ($negativeManifestReport | ConvertTo-Json -Depth 4)

    $negativeTarget = Invoke-A22Task -Suffix 'negative-target' -Image $toolsImageName -TimeoutSeconds 180 -AllowFailure `
        -Environment (Get-A22RestoreEnvironment -Manifest $manifestPath -ReleaseRoot '/work/negative-target-releases' `
            -DefaultsFile '/work/target.cnf' -DatabaseDSN $sourceDSN) `
        -Command @('bash', '/workspace/scripts/restore.sh')
    $negativeTargetTables = Get-A22TableCount -Container $targetName
    $negativeTargetGate = Invoke-A22Task -Suffix 'negative-target-gate' -Image $toolsImageName -TimeoutSeconds 30 -AllowFailure `
        -Command @('bash', '-c', "test -e /work/negative-target-releases/$backupID")
    $sourceAfterTarget = Get-A22SnapshotHash -Suffix 'source-after-target' -DatabaseDSN $sourceDSN -Role 'source'
    $negativeTargetPassed = $negativeTarget.ExitCode -ne 0 -and $negativeTargetTables -eq 0 -and
        $negativeTargetGate.ExitCode -ne 0 -and $sourceAfterTarget -ceq [string]$sourceSnapshot.snapshot_sha256 -and
        ($negativeTarget.Stderr -match 'must identify the same MySQL server')
    if (-not $negativeTargetPassed) { throw 'A22 target identity mismatch branch did not fail before import.' }
    $negativeTargetReport = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'; passed = $true
        failure_stage = 'target_identity'; exit_code = [int]$negativeTarget.ExitCode; import_started = $false
        target_table_count = $negativeTargetTables; release_gate_exists = $false; source_unchanged = $true
        production_release_authorized = $false
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-negative-target-mismatch.json') -Payload ($negativeTargetReport | ConvertTo-Json -Depth 4)

    $restoreStarted = [DateTimeOffset]::UtcNow
    $restoreResult = Invoke-A22Task -Suffix 'restore' -Image $toolsImageName -TimeoutSeconds 600 `
        -Environment (Get-A22RestoreEnvironment -Manifest $manifestPath -ReleaseRoot '/work/releases' `
            -DefaultsFile '/work/target.cnf' -DatabaseDSN $targetDSN) `
        -Command @('bash', '/workspace/scripts/restore.sh') -LogPath (Join-Path $evidenceDirectory 'a22-restore.log')
    $restoreOutput = $restoreResult.Stdout | ConvertFrom-Json -ErrorAction Stop
    if ([string]$restoreOutput.status -cne 'success' -or [string]$restoreOutput.backup_id -cne $backupID) {
        throw 'A22 restore command output is invalid.'
    }
    $releaseDirectory = [string]$restoreOutput.release_directory
    [void](Invoke-A22Task -Suffix 'copy-verify' -Image $toolsImageName -TimeoutSeconds 30 `
        -Command @('bash', '-c', 'set -eu; test -f "$1/release.json"; cp "$1/verify-report.json" /evidence/a22-verify-restore.json',
            '--', $releaseDirectory))
    $restoreArtifact = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'; backup_id = $backupID
        release_gate_exists = $true; verify_report = 'a22-verify-restore.json'; exit_code = 0
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-restore.json') -Payload ($restoreArtifact | ConvertTo-Json -Depth 4)

    [void](Invoke-A22Task -Suffix 'snapshot-target' -Image $toolsImageName -TimeoutSeconds 180 `
        -Environment (Get-A22SecretEnvironment -DatabaseDSN $targetDSN) `
        -Command @('/work/a22tool', 'snapshot', '--role', 'target', '--report', '/evidence/a22-target-snapshot.json'))
    $targetSnapshot = Get-Content -Raw -Encoding utf8 (Join-Path $evidenceDirectory 'a22-target-snapshot.json') | ConvertFrom-Json
    if ([string]$targetSnapshot.snapshot_sha256 -cne [string]$sourceSnapshot.snapshot_sha256 -or
        [string]$targetSnapshot.server_uuid -ceq [string]$sourceSnapshot.server_uuid) {
        throw 'A22 restored target snapshot is not exact and isolated.'
    }

    $createdContainers[$appName] = $false
    $appArguments = @(
        'create', '--name', $appName, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A22', '--label', $runLabel,
        '--memory', '2g', '--cpus', '2', '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount, '--mount', $workMount
    )
    $appEnvironment = @(Get-A22ApplicationEnvironment -DatabaseDSN $targetDSN) + @(
        "A22_ADMIN_PASSWORD=$adminPassword", "A22_EXPECTED_DATABASE=$databaseName",
        "A22_SOURCE_UUID_FINGERPRINT=$sourceUUIDFingerprint", "A22_TARGET_UUID_FINGERPRINT=$targetUUIDFingerprint",
        "A22_RELEASE_GATE=$releaseDirectory/release.json"
    )
    foreach ($entry in $appEnvironment) { $appArguments += @('--env', $entry) }
    $appArguments += @($toolsImageName, 'bash', '-c', 'sleep infinity')
    [void](Invoke-OpsDocker -Arguments $appArguments -TimeoutSeconds 60)
    $createdContainers[$appName] = $true
    $containerResourcesCreated = $true
    [void](Invoke-OpsDocker -Arguments @('start', $appName) -TimeoutSeconds 30)
    [void](Invoke-OpsDocker -Arguments @(
        'exec', '--detach', $appName, 'bash', '-c', '/work/new-api-pilot >/tmp/a22-app.stdout 2>/tmp/a22-app.stderr'
    ) -TimeoutSeconds 30)
    $smoke = Invoke-OpsProcess -FileName 'docker' -Arguments @(
        'exec', $appName, 'bash', '/workspace/scripts/acceptance/a22-smoke.sh'
    ) -TimeoutSeconds 180
    $smokeReport = $null
    if (-not [string]::IsNullOrWhiteSpace($smoke.Stdout)) {
        try {
            $smokeReport = $smoke.Stdout | ConvertFrom-Json -ErrorAction Stop
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-app-smoke.json') -Payload ($smokeReport | ConvertTo-Json -Depth 6)
        }
        catch {}
    }
    if ($smoke.TimedOut -or $smoke.ExitCode -ne 0) {
        $appLogs = Invoke-OpsProcess -FileName 'docker' -Arguments @(
            'exec', $appName, 'bash', '-c', 'tail -n 20 /tmp/a22-app.stderr /tmp/a22-app.stdout 2>/dev/null || true'
        ) -TimeoutSeconds 30
        $diagnostic = Protect-OpsDiagnostic -Payload ($smoke.Stderr + $appLogs.Stdout + $appLogs.Stderr)
        $diagnostic = [regex]::Replace($diagnostic, '(?i)\b[0-9a-f]{64}\b', '[fingerprint]')
        if ($diagnostic.Length -gt 1200) { $diagnostic = $diagnostic.Substring(0, 1200) }
        throw "A22 restored application smoke failed: $($diagnostic.Trim())"
    }
    if ($null -eq $smokeReport) { $smokeReport = $smoke.Stdout | ConvertFrom-Json -ErrorAction Stop }
    if ([string]$smokeReport.status -cne 'passed' -or -not [bool]$smokeReport.connected_to_target) {
        throw 'A22 restored application smoke identity is invalid.'
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-app-smoke.json') -Payload ($smokeReport | ConvertTo-Json -Depth 6)
    if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $appName))) { throw 'A22 app container cleanup failed.' }
    $createdContainers[$appName] = $false
    $restoreFinished = [DateTimeOffset]::UtcNow

    $backupCreatedUnix = [DateTimeOffset]::Parse([string]$backupArtifact.created_at_utc).ToUnixTimeSeconds()
    $lastBusinessUnix = [int64]$sourceSnapshot.last_business_time_unix
    $recoverableAge = $backupCreatedUnix - $lastBusinessUnix
    $rtoSeconds = [int64][Math]::Ceiling(($restoreFinished - $restoreStarted).TotalSeconds)
    $rpoPassed = $recoverableAge -ge 0 -and $recoverableAge -le 3600
    $rtoPassed = $rtoSeconds -ge 0 -and $rtoSeconds -le 14400
    if (-not $rpoPassed -or -not $rtoPassed) { throw 'A22 RPO/RTO limit was exceeded.' }
    $timingReport = [ordered]@{
        schema_version = 1; acceptance_id = 'A22'; status = 'passed'
        backup_created_at_unix = $backupCreatedUnix; last_business_time_unix = $lastBusinessUnix
        recoverable_age_seconds = $recoverableAge; actual_data_loss_seconds = 0
        rpo_seconds = $recoverableAge; rto_seconds = $rtoSeconds
        rpo_limit_seconds = 3600; rto_limit_seconds = 14400
        rpo_passed = $true; rto_passed = $true
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-rpo-rto.json') -Payload ($timingReport | ConvertTo-Json -Depth 4)
}
catch {
    $failure = $_.Exception.Message
}
finally {
    foreach ($container in @($createdContainers.Keys)) {
        if ($createdContainers[$container]) {
            if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $container))) { $cleanupFailed = $true }
            $createdContainers[$container] = $false
        }
    }
    if ($networkMayExist -and -not (Remove-OpsDockerResource -Arguments @('network', 'rm', $networkName))) {
        $cleanupFailed = $true
    }
    foreach ($volume in @(
        [pscustomobject]@{ MayExist = $sourceVolumeMayExist; Name = $sourceVolumeName },
        [pscustomobject]@{ MayExist = $targetVolumeMayExist; Name = $targetVolumeName },
        [pscustomobject]@{ MayExist = $workVolumeMayExist; Name = $workVolumeName }
    )) {
        if ($volume.MayExist -and -not (Remove-OpsDockerResource -Arguments @('volume', 'rm', $volume.Name))) {
            $cleanupFailed = $true
        }
    }
    if ($toolsImageMayExist) {
        $removed = Invoke-OpsProcess -FileName 'docker' -Arguments @('image', 'rm', '--force', $toolsImageName) -TimeoutSeconds 90
        if ($removed.TimedOut -or $removed.ExitCode -ne 0) { $cleanupFailed = $true }
    }
    if ($null -ne $evidenceDirectory -and $null -ne $runLabel) {
        $containerSweep = Get-OpsResidualSweep -Arguments @('ps', '-a', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Names}}')
        $networkSweep = Get-OpsResidualSweep -Arguments @('network', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Name}}')
        $volumeSweep = Get-OpsResidualSweep -Arguments @('volume', 'ls', '--filter', "label=$runLabel", '--format', '{{.Name}}')
        $imageSweep = Get-OpsResidualSweep -Arguments @('image', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Repository}}:{{.Tag}}')
        $sweepsSucceeded = $containerSweep.Succeeded -and $networkSweep.Succeeded -and $volumeSweep.Succeeded -and $imageSweep.Succeeded
        $noResiduals = $containerSweep.Items.Count -eq 0 -and $networkSweep.Items.Count -eq 0 -and
            $volumeSweep.Items.Count -eq 0 -and $imageSweep.Items.Count -eq 0
        $cleanupPassed = (-not $cleanupFailed) -and $sweepsSucceeded -and $noResiduals -and
            $containerResourcesCreated -and $networkCreated -and $volumesCreated -and $imageCreated
        $cleanupReport = [ordered]@{
            schema_version = 1; acceptance_id = 'A22'; status = if ($cleanupPassed) { 'passed' } else { 'failed' }
            passed = $cleanupPassed
            lifecycle = [ordered]@{
                containers = if ($containerResourcesCreated -and $containerSweep.Items.Count -eq 0) { 'created_and_removed' } else { 'cleanup_failed' }
                networks = if ($networkCreated -and $networkSweep.Items.Count -eq 0) { 'created_and_removed' } else { 'cleanup_failed' }
                volumes = if ($volumesCreated -and $volumeSweep.Items.Count -eq 0) { 'created_and_removed' } else { 'cleanup_failed' }
                images = if ($imageCreated -and $imageSweep.Items.Count -eq 0) { 'created_and_removed' } else { 'cleanup_failed' }
            }
            residuals = [ordered]@{
                containers = @($containerSweep.Items); networks = @($networkSweep.Items)
                volumes = @($volumeSweep.Items); images = @($imageSweep.Items)
            }
        }
        try {
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6)
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
    [Console]::Error.WriteLine('A22 exact-resource cleanup failed.')
    exit 1
}

try {
    $scanFiles = @($requiredArtifacts | Where-Object { $_ -notin @('a22-secret-scan.json', 'a22-report.json') })
    Write-A22SecretScan -Directory $evidenceDirectory -Files $scanFiles

    $reportCommand = @(
        'run', '--rm', '--network', 'none', '--workdir', '/workspace',
        '--mount', "type=bind,source=$repositoryRoot,target=/workspace,readonly",
        '--mount', "type=bind,source=$evidenceDirectory,target=/evidence",
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off', $goImage,
        '/usr/local/go/bin/go', 'run', './scripts/acceptance/a22tool', 'report',
        '--evidence-dir', '/evidence', '--evidence-class', $evidenceClass, '--output', '/evidence/a22-report.json'
    )
    $reportResult = Invoke-OpsProcess -FileName 'docker' -Arguments $reportCommand -TimeoutSeconds 180
    if ($reportResult.TimedOut -or $reportResult.ExitCode -ne 0) {
        throw "A22 report generation failed: $((Protect-OpsDiagnostic -Payload $reportResult.Stderr).Trim())"
    }
    Write-A22ArtifactInventory -Directory $evidenceDirectory -EvidenceClass $evidenceClass

    $validateCommand = @(
        'run', '--rm', '--network', 'none', '--workdir', '/workspace',
        '--mount', "type=bind,source=$repositoryRoot,target=/workspace,readonly",
        '--mount', "type=bind,source=$evidenceDirectory,target=/evidence,readonly",
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off', $goImage,
        '/usr/local/go/bin/go', 'run', './scripts/acceptance/a22tool', 'validate',
        '--evidence-dir', '/evidence', '--evidence-class', $evidenceClass
    )
    $validateResult = Invoke-OpsProcess -FileName 'docker' -Arguments $validateCommand -TimeoutSeconds 180
    if ($validateResult.TimedOut -or $validateResult.ExitCode -ne 0) {
        throw "A22 evidence validation failed: $((Protect-OpsDiagnostic -Payload $validateResult.Stderr).Trim())"
    }
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}

[Console]::Out.WriteLine("A22 controlled technical drill passed evidence=$evidenceDirectory production_release_authorized=false")
exit 0
