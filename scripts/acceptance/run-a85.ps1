[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$acceptanceID = 'A85'
$mysqlImage = 'mysql:8.4'
$goImage = 'golang:1.25.1'
$databaseName = 'pilot_a85'
$targetTest = 'TestA85AlertFixtureDeliveryDrill'
$dockerOperationTimeoutSeconds = 180
$moduleWarmTimeoutSeconds = 300
$mysqlHealthTimeoutSeconds = 90
$testTimeoutSeconds = 600

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
        [Parameter(Mandatory = $true)][ValidateRange(1, 3600)][int]$TimeoutSeconds
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
        throw 'Docker CLI could not be started.'
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

function Remove-RunDockerResource {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)

    try {
        $result = Invoke-DockerProcess -Arguments $Arguments -TimeoutSeconds 30
        if ($result.TimedOut) {
            return $false
        }
        if ($result.ExitCode -eq 0) {
            return $true
        }
        return ($result.Stderr -match '(?i)no such (container|network)')
    }
    catch {
        return $false
    }
}

function Protect-A85DiagnosticLog {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $protected = [regex]::Replace(
        $Payload,
        '(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@',
        '$1[redacted]@'
    )
    return [regex]::Replace(
        $protected,
        '(?i)(\b(?:password|token|secret|dsn)=)[^&\s]+',
        '$1[redacted]'
    )
}

function Write-A85WarmFailureLogs {
    param([Parameter(Mandatory = $true)][string]$ContainerName)

    $logs = Invoke-DockerProcess -Arguments @('logs', $ContainerName) -TimeoutSeconds 30
    if ($logs.TimedOut -or $logs.ExitCode -ne 0) {
        [Console]::Error.WriteLine('A85 Go module cache warm-up logs could not be captured.')
        return
    }
    if (-not [string]::IsNullOrEmpty($logs.Stdout)) {
        [Console]::Out.Write((Protect-A85DiagnosticLog -Payload $logs.Stdout))
    }
    if (-not [string]::IsNullOrEmpty($logs.Stderr)) {
        [Console]::Error.Write((Protect-A85DiagnosticLog -Payload $logs.Stderr))
    }
}

function Assert-A85TestJSON {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $targetPasses = 0
    $sawSkip = $false
    $sawNoTests = $false
    foreach ($line in ($Payload -split "`r?`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }
        try {
            $event = $line | ConvertFrom-Json -ErrorAction Stop
        }
        catch {
            throw 'A85 go test stdout was not a complete JSON event stream.'
        }

        $actionProperty = $event.PSObject.Properties['Action']
        $testProperty = $event.PSObject.Properties['Test']
        $outputProperty = $event.PSObject.Properties['Output']
        $action = if ($null -ne $actionProperty) { [string]$actionProperty.Value } else { '' }
        $test = if ($null -ne $testProperty) { [string]$testProperty.Value } else { '' }
        $output = if ($null -ne $outputProperty) { [string]$outputProperty.Value } else { '' }

        if ($action -ceq 'skip') {
            $sawSkip = $true
        }
        if ($output -match '(?i)no tests to run|\[no test files\]') {
            $sawNoTests = $true
        }
        if ($test -ceq $targetTest -and $action -ceq 'pass') {
            $targetPasses++
        }
    }

    if ($targetPasses -ne 1) {
        throw 'A85 target test did not emit exactly one pass event.'
    }
    if ($sawSkip) {
        throw 'A85 test execution emitted a skip event.'
    }
    if ($sawNoTests) {
        throw 'A85 test execution reported that no tests ran.'
    }
}

$failure = $null
$cleanupFailed = $false
$networkMayExist = $false
$warmContainerMayExist = $false
$mysqlContainerMayExist = $false
$testContainerMayExist = $false
$networkName = $null
$warmContainerName = $null
$mysqlContainerName = $null
$testContainerName = $null

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID) {
        throw 'This script must be invoked by the acceptance runner with ACCEPTANCE_ID=A85.'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory.'
    }

    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
    $networkName = "new-api-pilot-a85-$runToken-network"
    $warmContainerName = "new-api-pilot-a85-$runToken-modules"
    $mysqlContainerName = "new-api-pilot-a85-$runToken-mysql"
    $testContainerName = "new-api-pilot-a85-$runToken-test"
    $runLabel = "new-api-pilot.acceptance-run=$runToken"

    [void](Invoke-Docker -Arguments @('version', '--format', '{{.Client.Version}}') -TimeoutSeconds 30)

    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $warmContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'create', '--name', $warmContainerName,
        '--label', 'new-api-pilot.acceptance=A85', '--label', $runLabel,
        '--workdir', '/workspace',
        '--mount', $repositoryMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        $goImage,
        'go', 'mod', 'download'
    ))
    [void](Invoke-Docker -Arguments @('start', $warmContainerName) -TimeoutSeconds 30)
    $warmWait = Invoke-DockerProcess -Arguments @('wait', $warmContainerName) -TimeoutSeconds $moduleWarmTimeoutSeconds
    if ($warmWait.TimedOut) {
        [void](Remove-RunDockerResource -Arguments @('kill', $warmContainerName))
        Write-A85WarmFailureLogs -ContainerName $warmContainerName
        throw 'A85 Go module cache warm-up timed out.'
    }
    if ($warmWait.ExitCode -ne 0) {
        Write-A85WarmFailureLogs -ContainerName $warmContainerName
        throw 'Waiting for the A85 Go module cache warm-up failed.'
    }
    $warmExitCode = 0
    if (-not [int]::TryParse($warmWait.Stdout.Trim(), [ref]$warmExitCode) -or $warmExitCode -ne 0) {
        Write-A85WarmFailureLogs -ContainerName $warmContainerName
        throw 'A85 Go module cache warm-up failed.'
    }
    if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $warmContainerName))) {
        throw 'A85 Go module cache warm-up cleanup failed.'
    }
    $warmContainerMayExist = $false

    $networkMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'network', 'create', '--internal',
        '--label', 'new-api-pilot.acceptance=A85', '--label', $runLabel,
        $networkName
    ) -TimeoutSeconds 30)

    $mysqlContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'run', '--detach', '--name', $mysqlContainerName,
        '--network', $networkName, '--network-alias', 'mysql-a85',
        '--label', 'new-api-pilot.acceptance=A85', '--label', $runLabel,
        '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes',
        '--env', "MYSQL_DATABASE=$databaseName",
        '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s',
        '--health-retries', '45', '--health-start-period', '5s',
        $mysqlImage,
        '--character-set-server=utf8mb4',
        '--collation-server=utf8mb4_unicode_ci',
        '--transaction-isolation=READ-COMMITTED'
    ))

    $healthDeadline = [DateTimeOffset]::UtcNow.AddSeconds($mysqlHealthTimeoutSeconds)
    while ($true) {
        $healthResult = Invoke-Docker -Arguments @(
            'inspect', '--format', '{{.State.Status}} {{.State.Health.Status}}', $mysqlContainerName
        ) -TimeoutSeconds 15
        $healthState = $healthResult.Stdout.Trim()
        if ($healthState -ceq 'running healthy') {
            break
        }
        if ($healthState -match '^(exited|dead) ' -or $healthState -match ' unhealthy$') {
            throw 'The isolated A85 MySQL container did not become healthy.'
        }
        if ([DateTimeOffset]::UtcNow -ge $healthDeadline) {
            throw 'The isolated A85 MySQL health wait timed out.'
        }
        Start-Sleep -Seconds 2
    }

    $testDatabaseDSN = "root:@tcp(mysql-a85:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Local"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"

    $testContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'create', '--name', $testContainerName,
        '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A85', '--label', $runLabel,
        '--workdir', '/workspace',
        '--mount', $repositoryMount,
        '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', "TEST_DATABASE_DSN=$testDatabaseDSN",
        '--env', 'ACCEPTANCE_ID=A85',
        '--env', 'ACCEPTANCE_EVIDENCE_DIR=/evidence',
        '--env', 'A85_ISOLATED_MYSQL=true',
        '--env', 'GOPROXY=off',
        '--env', 'GOSUMDB=off',
        $goImage,
        'go', 'test', '-json', './tests/integration',
        '-run', '^TestA85AlertFixtureDeliveryDrill$', '-count=1', '-timeout=8m'
    ))

    [void](Invoke-Docker -Arguments @('start', $testContainerName) -TimeoutSeconds 30)

    $waitResult = Invoke-DockerProcess -Arguments @('wait', $testContainerName) -TimeoutSeconds $testTimeoutSeconds
    if ($waitResult.TimedOut) {
        [void](Remove-RunDockerResource -Arguments @('kill', $testContainerName))
    }

    $logs = Invoke-DockerProcess -Arguments @('logs', $testContainerName) -TimeoutSeconds 30
    if (-not [string]::IsNullOrEmpty($logs.Stdout)) {
        [Console]::Out.Write($logs.Stdout)
    }
    if (-not [string]::IsNullOrEmpty($logs.Stderr)) {
        [Console]::Error.Write($logs.Stderr)
    }
    if ($logs.TimedOut -or $logs.ExitCode -ne 0) {
        throw 'A85 container logs could not be captured.'
    }
    if ($waitResult.TimedOut) {
        throw 'A85 go test exceeded its bounded execution timeout.'
    }
    if ($waitResult.ExitCode -ne 0) {
        throw 'Waiting for the A85 test container failed.'
    }

    $containerExitText = $waitResult.Stdout.Trim()
    $containerExitCode = 0
    if (-not [int]::TryParse($containerExitText, [ref]$containerExitCode)) {
        throw 'A85 test container did not report an exit code.'
    }
    if ($containerExitCode -ne 0) {
        throw "A85 go test failed with exit code $containerExitCode."
    }
    Assert-A85TestJSON -Payload $logs.Stdout
}
catch {
    $failure = $_.Exception.Message
}
finally {
    if ($testContainerMayExist -and $null -ne $testContainerName) {
        if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $testContainerName))) {
            $cleanupFailed = $true
        }
    }
    if ($warmContainerMayExist -and $null -ne $warmContainerName) {
        if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $warmContainerName))) {
            $cleanupFailed = $true
        }
    }
    if ($mysqlContainerMayExist -and $null -ne $mysqlContainerName) {
        if (-not (Remove-RunDockerResource -Arguments @('rm', '--force', $mysqlContainerName))) {
            $cleanupFailed = $true
        }
    }
    if ($networkMayExist -and $null -ne $networkName) {
        if (-not (Remove-RunDockerResource -Arguments @('network', 'rm', $networkName))) {
            $cleanupFailed = $true
        }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A85 Docker resource cleanup failed.')
    exit 1
}
exit 0
