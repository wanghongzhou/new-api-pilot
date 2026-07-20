[CmdletBinding()]
param(
    [switch]$Smoke
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'a49-path-guard.ps1')
. (Join-Path $PSScriptRoot 'a49-git-state.ps1')

$acceptanceID = 'A49'
$mysqlImage = 'mysql:8.4'
$goImage = 'golang:1.25.1'
$databaseName = 'pilot_a49'
$fixedNowUnix = '1768665599'
$dockerOperationTimeoutSeconds = 180
$moduleWarmTimeoutSeconds = 600
$buildTimeoutSeconds = 2400
$mysqlHealthTimeoutSeconds = 180
$migrationTimeoutSeconds = 600
$appReadyTimeoutSeconds = 180
$mode = if ($Smoke.IsPresent) { 'smoke' } else { 'full' }
$seedTimeoutSeconds = if ($Smoke.IsPresent) { 900 } else { 14400 }
$loadTimeoutSeconds = if ($Smoke.IsPresent) { 300 } else { 3000 }
$reportTimeoutSeconds = if ($Smoke.IsPresent) { 300 } else { 900 }
$mysqlMemory = if ($Smoke.IsPresent) { '1536m' } else { '6g' }
$mysqlCPUs = if ($Smoke.IsPresent) { '2' } else { '4' }
$mysqlBufferPool = if ($Smoke.IsPresent) { '256M' } else { '3G' }
$appMemory = if ($Smoke.IsPresent) { '768m' } else { '3g' }
$appCPUs = if ($Smoke.IsPresent) { '1' } else { '2' }
$toolMemory = if ($Smoke.IsPresent) { '1g' } else { '4g' }
$toolCPUs = if ($Smoke.IsPresent) { '2' } else { '4' }

function ConvertTo-NativeArgument {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)

    if ($Value.Length -gt 0 -and $Value -notmatch '[\s"]') {
        return $Value
    }
    $builder = [System.Text.StringBuilder]::new()
    [void]$builder.Append('"')
    $backslashes = 0
    foreach ($character in $Value.ToCharArray()) {
        if ($character -eq '\') {
            $backslashes++
            continue
        }
        if ($character -eq '"') {
            [void]$builder.Append(('\' * (($backslashes * 2) + 1)))
            [void]$builder.Append('"')
            $backslashes = 0
            continue
        }
        if ($backslashes -gt 0) {
            [void]$builder.Append(('\' * $backslashes))
            $backslashes = 0
        }
        [void]$builder.Append($character)
    }
    if ($backslashes -gt 0) {
        [void]$builder.Append(('\' * ($backslashes * 2)))
    }
    [void]$builder.Append('"')
    return $builder.ToString()
}

function Invoke-DockerProcess {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][ValidateRange(1, 20000)][int]$TimeoutSeconds
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = 'docker'
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-NativeArgument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            throw 'Docker CLI could not be started.'
        }
    }
    catch {
        $process.Dispose()
        throw
    }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $finished = $process.WaitForExit($TimeoutSeconds * 1000)
    if (-not $finished) {
        try { $process.Kill() } catch {}
        $process.WaitForExit()
    }
    $stdout = ''
    $stderr = ''
    try { $stdout = $stdoutTask.GetAwaiter().GetResult() } catch {}
    try { $stderr = $stderrTask.GetAwaiter().GetResult() } catch {}
    $exitCode = if ($finished) { $process.ExitCode } else { -1 }
    $process.Dispose()
    return [pscustomobject]@{
        TimedOut = -not $finished
        ExitCode = $exitCode
        Stdout = $stdout
        Stderr = $stderr
    }
}

function Invoke-Docker {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [int]$TimeoutSeconds = $dockerOperationTimeoutSeconds
    )
    $result = Invoke-DockerProcess -Arguments $Arguments -TimeoutSeconds $TimeoutSeconds
    if ($result.TimedOut) {
        throw 'A bounded Docker operation timed out.'
    }
    if ($result.ExitCode -ne 0) {
        throw 'A Docker operation failed.'
    }
    return $result
}

function Protect-A49DiagnosticLog {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $protected = [regex]::Replace($Payload, '(?i)(DATABASE_DSN=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)(A49_VIEWER_PASSWORD=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)(PLATFORM_BOOTSTRAP_ADMIN_PASSWORD=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)(SESSION_SECRET=|ENCRYPTION_KEY=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@', '$1[redacted]@')
    return [regex]::Replace($protected, '(?i)(\b(?:password|token|secret|dsn)=)[^&\s]+', '$1[redacted]')
}

function Write-Utf8NoBom {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload
    )
    [System.IO.File]::WriteAllText($Path, $Payload, [System.Text.UTF8Encoding]::new($false))
}

function Remove-RunDockerResource {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    try {
        $result = Invoke-DockerProcess -Arguments $Arguments -TimeoutSeconds 60
        if ($result.TimedOut) { return $false }
        if ($result.ExitCode -eq 0) { return $true }
        return ($result.Stderr -match '(?i)no such (container|network|volume|image)')
    }
    catch {
        return $false
    }
}

function Wait-A49Container {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds
    )
    $wait = Invoke-DockerProcess -Arguments @('wait', $Container) -TimeoutSeconds $TimeoutSeconds
    if ($wait.TimedOut) {
        [void](Remove-RunDockerResource -Arguments @('kill', $Container))
        throw "A49 container $Container timed out."
    }
    if ($wait.ExitCode -ne 0) {
        throw "Waiting for A49 container $Container failed."
    }
    $exitCode = 0
    if (-not [int]::TryParse($wait.Stdout.Trim(), [ref]$exitCode)) {
        throw "A49 container $Container returned an invalid exit code."
    }
    return $exitCode
}

function Get-A49ContainerLogs {
    param([Parameter(Mandatory = $true)][string]$Container)
    return Invoke-DockerProcess -Arguments @('logs', '--timestamps', $Container) -TimeoutSeconds 180
}

function Save-A49ContainerLogs {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][string]$Path
    )
    $logs = Get-A49ContainerLogs -Container $Container
    if ($logs.TimedOut -or $logs.ExitCode -ne 0) {
        return $false
    }
    Write-Utf8NoBom -Path $Path -Payload (Protect-A49DiagnosticLog -Payload ($logs.Stdout + $logs.Stderr))
    return $true
}

function New-A49Password {
    $bytes = New-Object byte[] 32
    $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    return ([Convert]::ToBase64String($bytes) + 'Aa1!')
}

function Get-AppEnvironmentArguments {
    param(
        [Parameter(Mandatory = $true)][string]$DatabaseDSN,
        [Parameter(Mandatory = $true)][string]$ViewerPassword,
        [switch]$IncludeCapacityGuards
    )
    $arguments = @(
        '--env', 'APP_ENV=test',
        '--env', 'PORT=3000',
        '--env', 'TZ=Asia/Shanghai',
        '--env', "DATABASE_DSN=$DatabaseDSN",
        '--env', 'SQL_MAX_IDLE_CONNS=20',
        '--env', 'SQL_MAX_OPEN_CONNS=100',
        '--env', 'SQL_MAX_LIFETIME_SECONDS=60',
        '--env', 'SESSION_SECRET=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=',
        '--env', 'ENCRYPTION_KEY=YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=',
        '--env', "PLATFORM_BOOTSTRAP_ADMIN_PASSWORD=$ViewerPassword",
        '--env', 'SESSION_COOKIE_SECURE=false',
        '--env', 'EXPORT_DIR=/data/exports',
        '--env', 'PUBLIC_ORIGIN=http://app-a49:3000',
        '--env', 'TRUSTED_PROXIES=',
        '--env', 'UPSTREAM_ALLOWED_HOST_SUFFIXES=',
        '--env', 'UPSTREAM_ALLOWED_CIDRS=10.0.0.0/8',
        '--env', 'UPSTREAM_CA_FILE=',
        '--env', 'UPSTREAM_CONNECT_TIMEOUT_SECONDS=5',
        '--env', 'UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS=15',
        '--env', 'UPSTREAM_REQUEST_TIMEOUT_SECONDS=30',
        '--env', 'UPSTREAM_EXPORT_TIMEOUT_SECONDS=120',
        '--env', 'DINGTALK_ALLOWED_HOSTS=',
        '--env', 'METRICS_ALLOWED_CIDRS=127.0.0.0/8,172.16.0.0/12'
    )
    if ($IncludeCapacityGuards.IsPresent) {
        $arguments += @('--env', 'ACCEPTANCE_ID=A49', '--env', "A49_FIXED_NOW_UNIX=$fixedNowUnix")
    }
    return $arguments
}

function Assert-A49Resources {
    param(
        [Parameter(Mandatory = $true)][string]$EvidenceDirectory,
        [Parameter(Mandatory = $true)][string]$MySQLImage
    )
    $info = Invoke-Docker -Arguments @('info', '--format', '{{.NCPU}}|{{.MemTotal}}|{{.Driver}}|{{.DockerRootDir}}') -TimeoutSeconds 60
    $parts = $info.Stdout.Trim().Split('|')
    if ($parts.Count -ne 4) { throw 'Docker info resource output was invalid.' }
    $cpus = [int]$parts[0]
    $memory = [int64]$parts[1]
    $minimumCPUs = if ($Smoke.IsPresent) { 2 } else { 8 }
    $minimumMemory = if ($Smoke.IsPresent) { 2GB } else { 16GB }
    if ($cpus -lt $minimumCPUs -or $memory -lt $minimumMemory) {
        throw "A49 resource preflight failed: cpus=$cpus memory=$memory."
    }
    $disk = Invoke-Docker -Arguments @('run', '--rm', '--network', 'none', $MySQLImage, 'df', '-Pk', '/var/lib/mysql') -TimeoutSeconds 60
    $diskLine = (($disk.Stdout -split "`r?`n") | Where-Object { $_ -match '^\S+\s+\d+\s+\d+\s+\d+' } | Select-Object -Last 1)
    if ([string]::IsNullOrWhiteSpace($diskLine)) { throw 'Docker free-space preflight output was invalid.' }
    $diskFields = $diskLine.Trim() -split '\s+'
    $dockerFreeBytes = [int64]$diskFields[3] * 1024
    $minimumDockerFree = if ($Smoke.IsPresent) { 3GB } else { 35GB }
    if ($dockerFreeBytes -lt $minimumDockerFree) {
        throw "A49 Docker free-space preflight failed: free=$dockerFreeBytes."
    }
    $drive = [System.IO.DriveInfo]::new([System.IO.Path]::GetPathRoot($EvidenceDirectory))
    $minimumEvidenceFree = if ($Smoke.IsPresent) { 512MB } else { 5GB }
    if ($drive.AvailableFreeSpace -lt $minimumEvidenceFree) {
        throw "A49 evidence free-space preflight failed: free=$($drive.AvailableFreeSpace)."
    }
    return [pscustomobject]@{
        CPUs = $cpus
        MemoryBytes = $memory
        StorageDriver = $parts[2]
        DockerRootDir = $parts[3]
        DockerFreeBytes = $dockerFreeBytes
        EvidenceFreeBytes = $drive.AvailableFreeSpace
    }
}

function Write-A49FailureDiagnostics {
    param(
        [Parameter(Mandatory = $true)][string]$EvidenceDirectory,
        [Parameter(Mandatory = $true)][hashtable]$Containers
    )
    foreach ($entry in $Containers.GetEnumerator()) {
        if ([string]::IsNullOrWhiteSpace([string]$entry.Value)) { continue }
        $exists = Invoke-DockerProcess -Arguments @('inspect', [string]$entry.Value) -TimeoutSeconds 30
        if ($exists.ExitCode -ne 0) { continue }
        $safeName = [regex]::Replace([string]$entry.Key, '[^a-zA-Z0-9_-]', '_')
        [void](Save-A49ContainerLogs -Container ([string]$entry.Value) -Path (Join-Path $EvidenceDirectory "$safeName.log"))
        Write-Utf8NoBom -Path (Join-Path $EvidenceDirectory "$safeName-inspect.json") -Payload (Protect-A49DiagnosticLog -Payload $exists.Stdout)
    }
}

function Write-A49ArtifactInventory {
    param(
        [Parameter(Mandatory = $true)][string]$EvidenceDirectory,
        [Parameter(Mandatory = $true)][string]$Mode,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )
    $required = @(
        'a49-seed-report.json', 'a49-load-results.jsonl', 'a49-load-metadata.json',
        'a49-app.log', 'a49-environment.json', 'a49-docker-stats.tsv',
        'a49-mysql-status.tsv', 'a49-query-plans.txt', 'a49-report.json',
        'a49-negative-guard.log', 'a49-migration.log', 'a49-loader.log',
        'a49-load.log', 'a49-report.log', 'a49-image-build.log'
    )
    $files = @()
    foreach ($relative in $required) {
        $path = Join-Path $EvidenceDirectory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "A49 required artifact is missing: $relative"
        }
        $info = Get-Item -LiteralPath $path
        if ($info.Length -le 0) {
            throw "A49 required artifact is empty: $relative"
        }
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A49'
        evidence_class = $EvidenceClass
        mode = $Mode
        generated_at = [DateTimeOffset]::UtcNow.ToString('o')
        files = $files
    }
    Write-Utf8NoBom -Path (Join-Path $EvidenceDirectory 'a49-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

function Get-A49ResidualSweep {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )
    $result = Invoke-DockerProcess -Arguments $Arguments -TimeoutSeconds 60
    $items = @()
    if ($result.ExitCode -eq 0 -and -not $result.TimedOut) {
        $items = @(($result.Stdout -split "`r?`n") | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }
    return [pscustomobject]@{
        Succeeded = (-not $result.TimedOut -and $result.ExitCode -eq 0)
        Items = $items
    }
}

$failure = $null
$cleanupFailed = $false
$statsJob = $null
$networkName = $null
$mysqlVolumeName = $null
$exportVolumeName = $null
$appImage = $null
$resources = $null
$runLabel = $null
$evidenceDirectory = $null
$expectedEvidenceClass = $null
$containers = @{
    warm = $null
    negative = $null
    migration = $null
    mysql = $null
    loader = $null
    app = $null
    load = $null
    report = $null
}

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID) {
        throw 'This script must be invoked by the acceptance runner with ACCEPTANCE_ID=A49.'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory.'
    }
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $expectedEvidenceClass = if ($Smoke.IsPresent) { 'smoke' } else { 'formal' }
    if ($env:ACCEPTANCE_EVIDENCE_CLASS -cne $expectedEvidenceClass) {
        throw "A49 $expectedEvidenceClass must be launched by the matching acceptance evidence guard."
    }
    $relativeEvidence = Get-A49RepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    $expectedPrefix = if ($Smoke.IsPresent) { 'artifacts/smoke/A49/' } else { 'artifacts/acceptance/A49/' }
    if (-not $relativeEvidence.StartsWith($expectedPrefix, [System.StringComparison]::Ordinal)) {
        throw "A49 $expectedEvidenceClass evidence directory must stay under $expectedPrefix."
    }
    $runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
    $networkName = "new-api-pilot-a49-$runToken-network"
    $mysqlVolumeName = "new-api-pilot-a49-$runToken-mysql"
    $exportVolumeName = "new-api-pilot-a49-$runToken-exports"
    $appImage = "new-api-pilot-a49-$runToken`:local"
    $runLabel = "new-api-pilot.acceptance-run=$runToken"
    $viewerPassword = New-A49Password
    $databaseDSN = "root:@tcp(mysql-a49:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    foreach ($key in @($containers.Keys)) {
        $containers[$key] = "new-api-pilot-a49-$runToken-$key"
    }

    $dockerVersion = Invoke-Docker -Arguments @('version', '--format', '{{.Client.Version}}|{{.Server.Version}}') -TimeoutSeconds 60
    foreach ($image in @($mysqlImage, $goImage)) {
        $inspect = Invoke-DockerProcess -Arguments @('image', 'inspect', $image) -TimeoutSeconds 30
        if ($inspect.ExitCode -ne 0) {
            [void](Invoke-Docker -Arguments @('pull', $image) -TimeoutSeconds 1200)
        }
    }
    $resources = Assert-A49Resources -EvidenceDirectory $evidenceDirectory -MySQLImage $mysqlImage

    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"
    [void](Invoke-Docker -Arguments @(
        'create', '--name', $containers.warm,
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        $goImage, 'go', 'mod', 'download'
    ))
    [void](Invoke-Docker -Arguments @('start', $containers.warm) -TimeoutSeconds 30)
    if ((Wait-A49Container -Container $containers.warm -TimeoutSeconds $moduleWarmTimeoutSeconds) -ne 0) {
        throw 'A49 Go module cache warm-up failed.'
    }
    if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $containers.warm))) {
        throw 'A49 module warm-up cleanup failed.'
    }
    $containers.warm = $null

    $build = Invoke-DockerProcess -Arguments @(
        'build', '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--tag', $appImage, '--file', (Join-Path $repositoryRoot 'Dockerfile'), $repositoryRoot
    ) -TimeoutSeconds $buildTimeoutSeconds
    Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-image-build.log') -Payload (Protect-A49DiagnosticLog -Payload ($build.Stdout + $build.Stderr))
    if ($build.TimedOut -or $build.ExitCode -ne 0) { throw 'A49 application image build failed.' }

    [void](Invoke-Docker -Arguments @('network', 'create', '--internal', '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel, $networkName) -TimeoutSeconds 60)
    [void](Invoke-Docker -Arguments @('volume', 'create', '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel, $mysqlVolumeName) -TimeoutSeconds 60)
    [void](Invoke-Docker -Arguments @('volume', 'create', '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel, $exportVolumeName) -TimeoutSeconds 60)

    [void](Invoke-Docker -Arguments @(
        'run', '--detach', '--name', $containers.mysql, '--network', $networkName, '--network-alias', 'mysql-a49',
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--memory', $mysqlMemory, '--cpus', $mysqlCPUs,
        '--mount', "type=volume,source=$mysqlVolumeName,target=/var/lib/mysql",
        '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$databaseName", '--env', 'TZ=Asia/Shanghai',
        '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '75', '--health-start-period', '10s',
        $mysqlImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci',
        '--transaction-isolation=READ-COMMITTED', "--innodb-buffer-pool-size=$mysqlBufferPool",
        '--innodb-redo-log-capacity=2G', '--default-time-zone=+08:00', '--skip-log-bin'
    ))
    $healthDeadline = [DateTimeOffset]::UtcNow.AddSeconds($mysqlHealthTimeoutSeconds)
    while ($true) {
        $state = (Invoke-Docker -Arguments @('inspect', '--format', '{{.State.Status}} {{.State.Health.Status}}', $containers.mysql) -TimeoutSeconds 15).Stdout.Trim()
        if ($state -ceq 'running healthy') { break }
        if ($state -match '^(exited|dead) ' -or $state -match ' unhealthy$') { throw 'A49 MySQL did not become healthy.' }
        if ([DateTimeOffset]::UtcNow -ge $healthDeadline) { throw 'A49 MySQL health wait timed out.' }
        Start-Sleep -Seconds 2
    }
    $mysqlContractResult = Invoke-Docker -Arguments @(
        'exec', $containers.mysql, 'mysql', '-uroot', '-N', '-B', '-e',
        "SELECT VERSION(), @@transaction_isolation, @@character_set_server, @@collation_server, @@global.time_zone;"
    ) -TimeoutSeconds 60
    $mysqlContractFields = $mysqlContractResult.Stdout.Trim() -split "`t"
    if ($mysqlContractFields.Count -ne 5 -or -not $mysqlContractFields[0].StartsWith('8.4.', [System.StringComparison]::Ordinal) -or
        $mysqlContractFields[1] -cne 'READ-COMMITTED' -or $mysqlContractFields[2] -cne 'utf8mb4' -or
        $mysqlContractFields[3] -cne 'utf8mb4_unicode_ci' -or $mysqlContractFields[4] -cne '+08:00') {
        throw 'A49 MySQL runtime contract does not match F05.'
    }
    $mysqlContract = [ordered]@{
        version = $mysqlContractFields[0]
        transaction_isolation = $mysqlContractFields[1]
        character_set_server = $mysqlContractFields[2]
        collation_server = $mysqlContractFields[3]
        time_zone = $mysqlContractFields[4]
        database = $databaseName
    }

    $commonAppEnvironment = Get-AppEnvironmentArguments -DatabaseDSN $databaseDSN -ViewerPassword $viewerPassword
    [void](Invoke-Docker -Arguments (@(
        'create', '--name', $containers.negative, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--mount', "type=volume,source=$exportVolumeName,target=/data/exports",
        '--env', 'ACCEPTANCE_ID=A49'
    ) + $commonAppEnvironment + @($appImage, 'capacity-serve')))
    [void](Invoke-Docker -Arguments @('start', $containers.negative) -TimeoutSeconds 30)
    $negativeExit = Wait-A49Container -Container $containers.negative -TimeoutSeconds 30
    $negativeLogs = Get-A49ContainerLogs -Container $containers.negative
    Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-negative-guard.log') -Payload (Protect-A49DiagnosticLog -Payload ($negativeLogs.Stdout + $negativeLogs.Stderr))
    if ($negativeExit -eq 0 -or ($negativeLogs.Stdout + $negativeLogs.Stderr) -notmatch 'A49_FIXED_NOW_UNIX' -or
        ($negativeLogs.Stdout + $negativeLogs.Stderr) -match '(?i)initialize database|connect.*mysql') {
        throw 'A49 capacity-serve negative guard did not fail closed before database connection.'
    }
    [void](Remove-RunDockerResource -Arguments @('rm', '--force', $containers.negative))
    $containers.negative = $null

    [void](Invoke-Docker -Arguments (@(
        'create', '--name', $containers.migration, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--mount', "type=volume,source=$exportVolumeName,target=/data/exports"
    ) + $commonAppEnvironment + @($appImage, 'migrate')))
    [void](Invoke-Docker -Arguments @('start', $containers.migration) -TimeoutSeconds 30)
    if ((Wait-A49Container -Container $containers.migration -TimeoutSeconds $migrationTimeoutSeconds) -ne 0) {
        throw 'A49 migration failed.'
    }
    [void](Save-A49ContainerLogs -Container $containers.migration -Path (Join-Path $evidenceDirectory 'a49-migration.log'))
    [void](Remove-RunDockerResource -Arguments @('rm', '--force', $containers.migration))
    $containers.migration = $null

    [void](Invoke-Docker -Arguments @(
        'create', '--name', $containers.loader, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--memory', $toolMemory, '--cpus', $toolCPUs, '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', "A49_DATABASE_DSN=$databaseDSN", '--env', "A49_DATABASE_NAME=$databaseName",
        '--env', "A49_VIEWER_PASSWORD=$viewerPassword", '--env', "A49_FIXED_NOW_UNIX=$fixedNowUnix",
        '--env', 'A49_ISOLATED_MYSQL=true', '--env', 'ACCEPTANCE_ID=A49',
        '--env', "ACCEPTANCE_EVIDENCE_CLASS=$expectedEvidenceClass",
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
        $goImage, 'go', 'run', './scripts/acceptance', 'a49-seed',
        '-fixture', '/workspace/testdata/design/f05-ops-capacity.yaml', '-mode', $mode,
        '-report', '/evidence/a49-seed-report.json', '-timeout', "${seedTimeoutSeconds}s"
    ))
    [void](Invoke-Docker -Arguments @('start', $containers.loader) -TimeoutSeconds 30)
    if ((Wait-A49Container -Container $containers.loader -TimeoutSeconds $seedTimeoutSeconds) -ne 0) {
        throw 'A49 deterministic profile loader failed.'
    }
    [void](Save-A49ContainerLogs -Container $containers.loader -Path (Join-Path $evidenceDirectory 'a49-loader.log'))
    [void](Remove-RunDockerResource -Arguments @('rm', '--force', $containers.loader))
    $containers.loader = $null

    $capacityAppEnvironment = Get-AppEnvironmentArguments -DatabaseDSN $databaseDSN -ViewerPassword $viewerPassword -IncludeCapacityGuards
    [void](Invoke-Docker -Arguments (@(
        'run', '--detach', '--name', $containers.app, '--network', $networkName, '--network-alias', 'app-a49',
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--memory', $appMemory, '--cpus', $appCPUs,
        '--mount', "type=volume,source=$exportVolumeName,target=/data/exports"
    ) + $capacityAppEnvironment + @($appImage, 'capacity-serve')))
    $readyDeadline = [DateTimeOffset]::UtcNow.AddSeconds($appReadyTimeoutSeconds)
    while ($true) {
        $ready = Invoke-DockerProcess -Arguments @('exec', $containers.app, 'wget', '-q', '-Y', 'off', '-T', '2', '-O', '-', 'http://127.0.0.1:3000/readyz') -TimeoutSeconds 10
        if ($ready.ExitCode -eq 0 -and $ready.Stdout -match '"status"\s*:\s*"ready"') { break }
        $appState = (Invoke-Docker -Arguments @('inspect', '--format', '{{.State.Status}}', $containers.app) -TimeoutSeconds 15).Stdout.Trim()
        if ($appState -ne 'running') { throw 'A49 capacity application stopped before readiness.' }
        if ([DateTimeOffset]::UtcNow -ge $readyDeadline) { throw 'A49 capacity application readiness timed out.' }
        Start-Sleep -Seconds 2
    }

    [void](Invoke-Docker -Arguments @(
        'create', '--name', $containers.load, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--memory', $toolMemory, '--cpus', $toolCPUs, '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', "A49_VIEWER_PASSWORD=$viewerPassword", '--env', "A49_FIXED_NOW_UNIX=$fixedNowUnix",
        '--env', 'A49_ISOLATED_LOAD=true', '--env', 'ACCEPTANCE_ID=A49',
        '--env', "ACCEPTANCE_EVIDENCE_CLASS=$expectedEvidenceClass", '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
        $goImage, 'go', 'run', './scripts/acceptance', 'a49-load',
        '-fixture', '/workspace/testdata/design/f05-ops-capacity.yaml', '-mode', $mode,
        '-base-url', 'http://app-a49:3000', '-raw', '/evidence/a49-load-results.jsonl',
        '-metadata', '/evidence/a49-load-metadata.json'
    ))
    $statsPath = Join-Path $evidenceDirectory 'a49-docker-stats.tsv'
    [void](Invoke-Docker -Arguments @('start', $containers.load) -TimeoutSeconds 30)
    $statsJob = Start-Job -ArgumentList $containers.mysql, $containers.app, $containers.load -ScriptBlock {
        param($MySQLContainer, $AppContainer, $LoadContainer)
        while ($true) {
            $timestamp = [DateTimeOffset]::UtcNow.ToString('o')
            $lines = & docker stats --no-stream --format '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.NetIO}}|{{.BlockIO}}|{{.PIDs}}' @(
                $MySQLContainer, $AppContainer, $LoadContainer
            ) 2>$null
            foreach ($line in $lines) {
                if (-not [string]::IsNullOrWhiteSpace([string]$line)) {
                    "$timestamp|$line"
                }
            }
            Start-Sleep -Seconds 2
        }
    }
    $loadExit = Wait-A49Container -Container $containers.load -TimeoutSeconds $loadTimeoutSeconds
    if ($null -ne $statsJob) {
        Stop-Job -Job $statsJob
        $statsPayload = (Receive-Job -Job $statsJob) -join "`n"
        Write-Utf8NoBom -Path $statsPath -Payload ($statsPayload + "`n")
        Remove-Job -Job $statsJob -Force
    }
    $statsJob = $null
    if ($loadExit -ne 0) { throw 'A49 read-only load failed before report generation.' }
    [void](Save-A49ContainerLogs -Container $containers.load -Path (Join-Path $evidenceDirectory 'a49-load.log'))

    if (-not (Save-A49ContainerLogs -Container $containers.app -Path (Join-Path $evidenceDirectory 'a49-app.log'))) {
        throw 'A49 application access log could not be captured.'
    }
    $mysqlStatus = Invoke-Docker -Arguments @('exec', $containers.mysql, 'mysql', '-uroot', '-N', '-e',
        "SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Threads_running','Questions','Slow_queries','Created_tmp_disk_tables','Innodb_buffer_pool_reads','Innodb_row_lock_waits');") -TimeoutSeconds 120
    Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-mysql-status.tsv') -Payload $mysqlStatus.Stdout
    $explain = Invoke-Docker -Arguments @('exec', $containers.mysql, 'mysql', '-uroot', $databaseName, '-e',
        "EXPLAIN SELECT * FROM account WHERE managed_status='active' ORDER BY updated_at DESC,id DESC LIMIT 20; EXPLAIN SELECT site_id,hour_ts,SUM(request_count) FROM site_stat_hourly WHERE hour_ts>=1765983600 AND hour_ts<1768662000 GROUP BY site_id,hour_ts; EXPLAIN SELECT customer_id,site_id,SUM(request_count) FROM customer_stat_daily WHERE date_key>=20251219 AND date_key<20260118 GROUP BY customer_id,site_id;") -TimeoutSeconds 120
    Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-query-plans.txt') -Payload $explain.Stdout

    $versionParts = $dockerVersion.Stdout.Trim().Split('|')
    $appImageID = (Invoke-Docker -Arguments @('image', 'inspect', '--format', '{{.Id}}', $appImage) -TimeoutSeconds 30).Stdout.Trim()
    $mysqlImageID = (Invoke-Docker -Arguments @('image', 'inspect', '--format', '{{.Id}}', $mysqlImage) -TimeoutSeconds 30).Stdout.Trim()
    $goImageID = (Invoke-Docker -Arguments @('image', 'inspect', '--format', '{{.Id}}', $goImage) -TimeoutSeconds 30).Stdout.Trim()
    $gitState = Get-A49GitState -RepositoryRoot $repositoryRoot
    $commit = $gitState.Commit
    $worktreeDirty = $gitState.WorktreeDirty
    $environmentReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A49'
        mode = $mode
        evidence_class = $expectedEvidenceClass
        acceptance_eligible = (-not $Smoke.IsPresent)
        fixed_now_unix = [int64]$fixedNowUnix
        commit = $commit
        worktree_dirty = $worktreeDirty
        docker = [ordered]@{
            client_version = $versionParts[0]
            server_version = $versionParts[1]
            cpus = $resources.CPUs
            memory_bytes = $resources.MemoryBytes
            storage_driver = $resources.StorageDriver
            docker_root_dir = $resources.DockerRootDir
            docker_free_bytes_before = $resources.DockerFreeBytes
            evidence_free_bytes_before = $resources.EvidenceFreeBytes
        }
        images = [ordered]@{ application = $appImageID; mysql = $mysqlImageID; go = $goImageID }
        mysql = $mysqlContract
        limits = [ordered]@{
            mysql_memory = $mysqlMemory; mysql_cpus = $mysqlCPUs; mysql_buffer_pool = $mysqlBufferPool
            app_memory = $appMemory; app_cpus = $appCPUs; load_memory = $toolMemory; load_cpus = $toolCPUs
        }
        network = [ordered]@{ internal = $true; host_ports = @() }
        stats_timeline = 'a49-docker-stats.tsv'
    }
    Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 8)

    [void](Invoke-Docker -Arguments @(
        'create', '--name', $containers.report, '--network', 'none',
        '--label', 'new-api-pilot.acceptance=A49', '--label', $runLabel,
        '--memory', $toolMemory, '--cpus', $toolCPUs, '--workdir', '/workspace',
        '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', 'A49_ISOLATED_REPORT=true', '--env', 'ACCEPTANCE_ID=A49',
        '--env', "ACCEPTANCE_EVIDENCE_CLASS=$expectedEvidenceClass", '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
        $goImage, 'go', 'run', './scripts/acceptance', 'a49-report',
        '-fixture', '/workspace/testdata/design/f05-ops-capacity.yaml', '-mode', $mode,
        '-raw', '/evidence/a49-load-results.jsonl', '-app-log', '/evidence/a49-app.log',
        '-seed-report', '/evidence/a49-seed-report.json', '-load-metadata', '/evidence/a49-load-metadata.json',
        '-environment', '/evidence/a49-environment.json', '-output', '/evidence/a49-report.json'
    ))
    [void](Invoke-Docker -Arguments @('start', $containers.report) -TimeoutSeconds 30)
    if ((Wait-A49Container -Container $containers.report -TimeoutSeconds $reportTimeoutSeconds) -ne 0) {
        throw 'A49 report gates failed.'
    }
    [void](Save-A49ContainerLogs -Container $containers.report -Path (Join-Path $evidenceDirectory 'a49-report.log'))
    Write-A49ArtifactInventory -EvidenceDirectory $evidenceDirectory -Mode $mode -EvidenceClass $expectedEvidenceClass
    Write-Output "A49 $mode run passed. Full mode is 36 minutes of measured load; smoke mode is never acceptance evidence."
}
catch {
    $failure = $_
    if ($null -ne $statsJob) {
        try {
            Stop-Job -Job $statsJob
            $statsPayload = (Receive-Job -Job $statsJob) -join "`n"
            if ($null -ne $env:ACCEPTANCE_EVIDENCE_DIR -and (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
                Write-Utf8NoBom -Path (Join-Path $env:ACCEPTANCE_EVIDENCE_DIR 'a49-docker-stats.tsv') -Payload ($statsPayload + "`n")
            }
            Remove-Job -Job $statsJob -Force
        }
        catch {}
        $statsJob = $null
    }
    if ($null -ne $env:ACCEPTANCE_EVIDENCE_DIR -and (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        Write-A49FailureDiagnostics -EvidenceDirectory $env:ACCEPTANCE_EVIDENCE_DIR -Containers $containers
    }
}
finally {
    foreach ($name in @('report', 'load', 'app', 'loader', 'migration', 'negative', 'warm', 'mysql')) {
        $container = [string]$containers[$name]
        if (-not [string]::IsNullOrWhiteSpace($container)) {
            if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $container))) { $cleanupFailed = $true }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($networkName)) {
        if (-not (Remove-RunDockerResource -Arguments @('network', 'rm', $networkName))) { $cleanupFailed = $true }
    }
    foreach ($volume in @($mysqlVolumeName, $exportVolumeName)) {
        if (-not [string]::IsNullOrWhiteSpace($volume)) {
            if (-not (Remove-RunDockerResource -Arguments @('volume', 'rm', $volume))) { $cleanupFailed = $true }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($appImage)) {
        if (-not (Remove-RunDockerResource -Arguments @('image', 'rm', $appImage))) { $cleanupFailed = $true }
    }
    if (-not [string]::IsNullOrWhiteSpace($runLabel) -and -not [string]::IsNullOrWhiteSpace($evidenceDirectory) -and
        (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
        $containerSweep = Get-A49ResidualSweep -Arguments @('ps', '-a', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Names}}')
        $networkSweep = Get-A49ResidualSweep -Arguments @('network', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Name}}')
        $volumeSweep = Get-A49ResidualSweep -Arguments @('volume', 'ls', '--filter', "label=$runLabel", '--format', '{{.Name}}')
        $imageSweep = Get-A49ResidualSweep -Arguments @('image', 'ls', '--filter', "label=$runLabel", '--format', '{{.ID}} {{.Repository}}:{{.Tag}}')
        $sweepsSucceeded = $containerSweep.Succeeded -and $networkSweep.Succeeded -and $volumeSweep.Succeeded -and $imageSweep.Succeeded
        $noResiduals = $containerSweep.Items.Count -eq 0 -and $networkSweep.Items.Count -eq 0 -and
            $volumeSweep.Items.Count -eq 0 -and $imageSweep.Items.Count -eq 0
        $cleanupPassed = (-not $cleanupFailed) -and $sweepsSucceeded -and $noResiduals
        $cleanupReport = [ordered]@{
            schema_version = 1
            acceptance_id = 'A49'
            evidence_class = $expectedEvidenceClass
            mode = $mode
            passed = $cleanupPassed
            generated_at = [DateTimeOffset]::UtcNow.ToString('o')
            sweeps_succeeded = $sweepsSucceeded
            residuals = [ordered]@{
                containers = @($containerSweep.Items)
                networks = @($networkSweep.Items)
                volumes = @($volumeSweep.Items)
                images = @($imageSweep.Items)
            }
        }
        try {
            Write-Utf8NoBom -Path (Join-Path $evidenceDirectory 'a49-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6)
        }
        catch {
            $cleanupPassed = $false
        }
        if (-not $cleanupPassed) { $cleanupFailed = $true }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure.Exception.Message)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A49 exact-resource cleanup failed.')
    exit 1
}
exit 0
