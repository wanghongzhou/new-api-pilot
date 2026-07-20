[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = 'A62'
$targetTest = 'TestA62ResourceMinuteRetention'
$mysqlImage = 'mysql:8.4'
$goImage = 'golang:1.25.1'
$databaseName = 'pilot_a62'
$dockerTimeoutSeconds = 180
$moduleWarmTimeoutSeconds = 300
$mysqlHealthTimeoutSeconds = 90
$testTimeoutSeconds = 480
$innerCommand = @('go', 'test', '-json', './tests/integration', '-run', '^TestA62ResourceMinuteRetention$', '-count=1', '-timeout=6m')

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

function Invoke-NativeProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FileName,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][ValidateRange(1, 3600)][int]$TimeoutSeconds
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FileName
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-NativeArgument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            throw 'Native process could not be started.'
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
        [int]$TimeoutSeconds = $dockerTimeoutSeconds
    )

    $result = Invoke-NativeProcess -FileName 'docker' -Arguments $Arguments -TimeoutSeconds $TimeoutSeconds
    if ($result.TimedOut) {
        throw 'A bounded Docker operation timed out.'
    }
    if ($result.ExitCode -ne 0) {
        throw 'A Docker operation failed.'
    }
    return $result
}

function Remove-A62DockerResource {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)

    try {
        $result = Invoke-NativeProcess -FileName 'docker' -Arguments $Arguments -TimeoutSeconds 30
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

function Assert-A62TestJSON {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $passes = 0
	$failures = 0
    $sawSkip = $false
    $sawNoTests = $false
	$lines = 0
    foreach ($line in ($Payload -split "`r?`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }
		$lines++
        try {
            $event = $line | ConvertFrom-Json -ErrorAction Stop
        }
        catch {
            throw 'A62 go test stdout was not a complete JSON event stream.'
        }
        $action = if ($null -ne $event.PSObject.Properties['Action']) { [string]$event.Action } else { '' }
        $test = if ($null -ne $event.PSObject.Properties['Test']) { [string]$event.Test } else { '' }
        $output = if ($null -ne $event.PSObject.Properties['Output']) { [string]$event.Output } else { '' }
        if ($action -ceq 'skip') {
            $sawSkip = $true
        }
		if ($action -ceq 'fail') {
			$failures++
		}
        if ($output -match '(?i)no tests to run|\[no test files\]') {
            $sawNoTests = $true
        }
        if ($test -ceq $targetTest -and $action -ceq 'pass') {
            $passes++
        }
    }
    if ($passes -ne 1) {
        throw 'A62 target test did not emit exactly one pass event.'
    }
    if ($sawSkip) {
        throw 'A62 test execution emitted a skip event.'
    }
    if ($sawNoTests) {
        throw 'A62 test execution reported that no tests ran.'
    }
	if ($failures -ne 0) {
		throw 'A62 test execution emitted a fail event.'
	}
	return [ordered]@{
		schema_version = 1
		acceptance_id = 'A62'
		status = 'passed'
		target_test = $targetTest
		package = 'new-api-pilot/tests/integration'
		pass_events = $passes
		fail_events = $failures
		skip_events = if ($sawSkip) { 1 } else { 0 }
		no_tests = $sawNoTests
		json_lines = $lines
		json_path = 'a62-test.jsonl'
		stderr_path = 'a62-test.stderr.log'
	}
}

function Assert-A62Report {
    param([Parameter(Mandatory = $true)][string]$EvidenceDirectory)

    $path = Join-Path $EvidenceDirectory 'a62-report.json'
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw 'A62 report was not produced.'
    }
    $report = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json -ErrorAction Stop
    if ([int]$report.schema_version -ne 1 -or [string]$report.acceptance_id -cne 'A62' -or
        [string]$report.status -cne 'passed' -or [int]$report.retention_days -ne 90 -or
        [int]$report.batch_size -ne 257 -or [int64]$report.initial_rows_per_table -le 5000 -or
        [int64]$report.first_run.instance.deleted -le 5000 -or
        [int64]$report.first_run.site.deleted -le 5000 -or
        [bool]$report.first_run.complete -or -not [bool]$report.first_run.instance.pending_rows -or
        -not [bool]$report.first_run.site.pending_rows -or -not [bool]$report.second_run.complete -or
        [int64]$report.idempotent_run.instance.deleted -ne 0 -or
        [int64]$report.idempotent_run.site.deleted -ne 0 -or
        [int64]$report.rows_after_first_run -ne 4 -or [int64]$report.rows_after_final_run -ne 2 -or
        -not [bool]$report.business_facts_preserved -or -not [bool]$report.exact_boundary_preserved -or
        -not [bool]$report.missing_hourly_blocked -or -not [bool]$report.daily_not_final_blocked -or
        -not [bool]$report.invalid_retention_rejected -or -not [bool]$report.hourly_daily_values_unchanged) {
        throw 'A62 report did not satisfy the formal acceptance contract.'
    }
	$starvation = $report.starvation_proof
	if ([int]$starvation.blocked_prefix_rows -le ([int]$starvation.batch_size * [int]$starvation.maximum_batches) -or
		[bool]$starvation.first_run.complete -or [int64]$starvation.first_run.instance.deleted -ne 1 -or
		[int64]$starvation.first_run.site.deleted -ne 1 -or
		-not [bool]$starvation.first_run.instance.blocked_diagnostics_truncated -or
		-not [bool]$starvation.first_run.site.blocked_diagnostics_truncated -or
		[bool]$starvation.restart_run.complete -or
		[int64]$starvation.restart_run.instance.deleted -ne ([int]$starvation.batch_size * [int]$starvation.maximum_batches) -or
		[int64]$starvation.restart_run.site.deleted -ne ([int]$starvation.batch_size * [int]$starvation.maximum_batches) -or
		-not [bool]$starvation.final_run.complete -or [int64]$starvation.final_run.instance.deleted -ne 1 -or
		[int64]$starvation.final_run.site.deleted -ne 1 -or
		-not [bool]$starvation.eligible_deleted_behind_blocked_prefix -or
		-not [bool]$starvation.restart_continuation_proved) {
		throw 'A62 starvation and restart proof did not satisfy the formal acceptance contract.'
	}
    if ([string]$report.protected_aggregate_sha256 -notmatch '^[0-9a-f]{64}$') {
        throw 'A62 protected aggregate checksum is invalid.'
    }
	return $report
}

function Write-A62ArtifactInventory {
	param(
		[Parameter(Mandatory = $true)][string]$Directory,
		[Parameter(Mandatory = $true)][string]$EvidenceClass
	)

	$required = @(
		'a62-test.jsonl', 'a62-test.stderr.log', 'a62-test-summary.json', 'a62-command.json',
		'a62-environment.json', 'a62-fixture.json', 'a62-report.json', 'a62-cleanup.json'
	)
	$files = @()
	foreach ($relative in $required) {
		$path = Join-Path $Directory $relative
		if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
			throw "A62 required evidence artifact is missing: $relative"
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
		acceptance_id = 'A62'
		evidence_class = $EvidenceClass
		files = $files
	}
	Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a62-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

$failure = $null
$cleanupFailed = $false
$containerCleanupFailed = $false
$networkCleanupFailed = $false
$containerResourcesCreated = $false
$networkWasCreated = $false
$cleanupPassed = $false
$runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
$runLabel = "new-api-pilot.acceptance-run=$runToken"
$networkName = "new-api-pilot-a62-$runToken-network"
$warmContainerName = "new-api-pilot-a62-$runToken-modules"
$mysqlContainerName = "new-api-pilot-a62-$runToken-mysql"
$testContainerName = "new-api-pilot-a62-$runToken-test"
$networkMayExist = $false
$warmContainerMayExist = $false
$mysqlContainerMayExist = $false
$testContainerMayExist = $false
$evidenceDirectory = $null
$developmentMode = $env:A62_DEVELOPMENT -ceq 'true'
$evidenceClass = if ($developmentMode) { 'development' } else { 'formal' }

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID) {
        throw 'This script must be invoked by the acceptance runner with ACCEPTANCE_ID=A62.'
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
			-not $relativeEvidence.StartsWith('artifacts/smoke/A62-dev-', [System.StringComparison]::Ordinal)) {
			throw 'A62 development evidence must use a unique artifacts/smoke/A62-dev-* directory without a formal class.'
		}
	}
	elseif ($env:ACCEPTANCE_EVIDENCE_CLASS -cne 'formal' -or
		-not $relativeEvidence.StartsWith('artifacts/acceptance/A62/', [System.StringComparison]::Ordinal) -or
		[string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE)) {
		throw 'A62 formal evidence must be invoked by the canonical wrapper under artifacts/acceptance/A62/.'
	}
    [void](Invoke-Docker -Arguments @('version', '--format', '{{.Client.Version}}') -TimeoutSeconds 30)
    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"
    $evidenceMount = "type=bind,source=$evidenceDirectory,target=/evidence"

    $warmContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'create', '--name', $warmContainerName,
        '--label', 'new-api-pilot.acceptance=A62', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        $goImage, 'go', 'mod', 'download'
    ))
	$containerResourcesCreated = $true
    [void](Invoke-Docker -Arguments @('start', $warmContainerName) -TimeoutSeconds 30)
    $warmWait = Invoke-NativeProcess -FileName 'docker' -Arguments @('wait', $warmContainerName) -TimeoutSeconds $moduleWarmTimeoutSeconds
    if ($warmWait.TimedOut -or $warmWait.ExitCode -ne 0 -or $warmWait.Stdout.Trim() -cne '0') {
        throw 'A62 Go module cache warm-up failed.'
    }
    if (-not (Remove-A62DockerResource -Arguments @('rm', '--force', $warmContainerName))) {
        throw 'A62 Go module cache warm-up cleanup failed.'
    }
    $warmContainerMayExist = $false

    $networkMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'network', 'create', '--internal',
        '--label', 'new-api-pilot.acceptance=A62', '--label', $runLabel, $networkName
    ) -TimeoutSeconds 30)
	$networkWasCreated = $true

    $mysqlContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'run', '--detach', '--name', $mysqlContainerName,
        '--network', $networkName, '--network-alias', 'mysql-a62',
        '--label', 'new-api-pilot.acceptance=A62', '--label', $runLabel,
        '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes', '--env', "MYSQL_DATABASE=$databaseName",
        '--health-cmd', 'mysqladmin ping --host=127.0.0.1 --user=root --silent',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '45', '--health-start-period', '5s',
        $mysqlImage, '--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci',
        '--transaction-isolation=READ-COMMITTED', '--default-time-zone=+08:00'
    ))
	$containerResourcesCreated = $true
    $healthDeadline = [DateTimeOffset]::UtcNow.AddSeconds($mysqlHealthTimeoutSeconds)
    while ($true) {
        $health = Invoke-Docker -Arguments @('inspect', '--format', '{{.State.Status}} {{.State.Health.Status}}', $mysqlContainerName) -TimeoutSeconds 15
        if ($health.Stdout.Trim() -ceq 'running healthy') {
            break
        }
        if ([DateTimeOffset]::UtcNow -ge $healthDeadline) {
            throw 'The isolated A62 MySQL health wait timed out.'
        }
        Start-Sleep -Seconds 2
    }

	$mysqlContract = Invoke-Docker -Arguments @(
		'exec', $mysqlContainerName, 'mysql', '--batch', '--skip-column-names', '--user=root',
		'--execute', 'SELECT VERSION(), @@transaction_isolation, @@character_set_server, @@collation_server, @@time_zone'
	) -TimeoutSeconds 30
	$mysqlFields = $mysqlContract.Stdout.Trim().Split("`t")
	if ($mysqlFields.Count -ne 5) {
		throw 'A62 MySQL environment contract query returned an unexpected shape.'
	}
	$gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot
	$commandReport = [ordered]@{
		schema_version = 1
		acceptance_id = 'A62'
		evidence_class = $evidenceClass
		target_test = $targetTest
		working_directory = '/workspace'
		command = $innerCommand
		go_image = $goImage
		mysql_image = $mysqlImage
	}
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-command.json') -Payload ($commandReport | ConvertTo-Json -Depth 5)
	$environmentReport = [ordered]@{
		schema_version = 1
		acceptance_id = 'A62'
		evidence_class = $evidenceClass
		commit = $gitState.Commit
		worktree_dirty = $gitState.WorktreeDirty
		isolated_guard = $true
		mysql = [ordered]@{
			version = $mysqlFields[0]
			transaction_isolation = $mysqlFields[1]
			character_set_server = $mysqlFields[2]
			collation_server = $mysqlFields[3]
			time_zone = $mysqlFields[4]
		}
		network = [ordered]@{ internal = $true; host_ports = @() }
		database = $databaseName
	}
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 6)

    $testDatabaseDSN = "root:@tcp(mysql-a62:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
    $testContainerMayExist = $true
    [void](Invoke-Docker -Arguments @(
        'create', '--name', $testContainerName, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A62', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount, '--mount', $evidenceMount,
        '--mount', 'type=volume,source=new-api-pilot-go-mod-cache,target=/go/pkg/mod',
        '--mount', 'type=volume,source=new-api-pilot-go-build-cache,target=/root/.cache/go-build',
        '--env', "TEST_DATABASE_DSN=$testDatabaseDSN", '--env', 'ACCEPTANCE_ID=A62',
        '--env', 'ACCEPTANCE_EVIDENCE_DIR=/evidence', '--env', 'A62_ISOLATED_MYSQL=true',
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
        $goImage, 'go', 'test', '-json', './tests/integration',
        '-run', '^TestA62ResourceMinuteRetention$', '-count=1', '-timeout=6m'
    ))
	$containerResourcesCreated = $true
    [void](Invoke-Docker -Arguments @('start', $testContainerName) -TimeoutSeconds 30)
    $wait = Invoke-NativeProcess -FileName 'docker' -Arguments @('wait', $testContainerName) -TimeoutSeconds $testTimeoutSeconds
    if ($wait.TimedOut) {
        [void](Remove-A62DockerResource -Arguments @('kill', $testContainerName))
    }
    $logs = Invoke-NativeProcess -FileName 'docker' -Arguments @('logs', $testContainerName) -TimeoutSeconds 30
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-test.jsonl') -Payload $logs.Stdout
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-test.stderr.log') -Payload $logs.Stderr
    if (-not [string]::IsNullOrEmpty($logs.Stdout)) { [Console]::Out.Write($logs.Stdout) }
    if (-not [string]::IsNullOrEmpty($logs.Stderr)) { [Console]::Error.Write($logs.Stderr) }
    if ($wait.TimedOut -or $wait.ExitCode -ne 0 -or $logs.TimedOut -or $logs.ExitCode -ne 0) {
        throw 'A62 test execution did not complete cleanly.'
    }
    $containerExitCode = 0
    if (-not [int]::TryParse($wait.Stdout.Trim(), [ref]$containerExitCode) -or $containerExitCode -ne 0) {
        throw 'A62 go test failed.'
    }
	$testSummary = Assert-A62TestJSON -Payload $logs.Stdout
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-test-summary.json') -Payload ($testSummary | ConvertTo-Json -Depth 4)
	$report = Assert-A62Report -EvidenceDirectory $evidenceDirectory
	$fixtureReport = [ordered]@{
		schema_version = 1
		acceptance_id = 'A62'
		fixture_id = 'F05'
		path = [string]$report.fixture_path
		sha256 = [string]$report.fixture_sha256
		fixed_now_unix = [int64]$report.fixed_now_unix
		retention_days = [int]$report.retention_days
	}
	Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-fixture.json') -Payload ($fixtureReport | ConvertTo-Json -Depth 4)
}
catch {
    $failure = $_.Exception.Message
}
finally {
    if ($testContainerMayExist -and -not (Remove-A62DockerResource -Arguments @('rm', '--force', $testContainerName))) {
		$containerCleanupFailed = $true
    }
    if ($warmContainerMayExist -and -not (Remove-A62DockerResource -Arguments @('rm', '--force', $warmContainerName))) {
		$containerCleanupFailed = $true
    }
    if ($mysqlContainerMayExist -and -not (Remove-A62DockerResource -Arguments @('rm', '--force', $mysqlContainerName))) {
		$containerCleanupFailed = $true
    }
    if ($networkMayExist -and -not (Remove-A62DockerResource -Arguments @('network', 'rm', $networkName))) {
		$networkCleanupFailed = $true
    }
	$cleanupFailed = $containerCleanupFailed -or $networkCleanupFailed
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
			$containerResourcesCreated -and $networkWasCreated
		$cleanupReport = [ordered]@{
			schema_version = 1
			acceptance_id = 'A62'
			evidence_class = $evidenceClass
			passed = $cleanupPassed
			sweeps_succeeded = $sweepsSucceeded
			lifecycle = [ordered]@{
				containers = if ($containerResourcesCreated -and -not $containerCleanupFailed -and $containerSweep.Items.Count -eq 0) { 'created_and_removed' } elseif ($containerResourcesCreated) { 'cleanup_failed' } else { 'not_created' }
				networks = if ($networkWasCreated -and -not $networkCleanupFailed -and $networkSweep.Items.Count -eq 0) { 'created_and_removed' } elseif ($networkWasCreated) { 'cleanup_failed' } else { 'not_created' }
				volumes = 'not_created'
			}
			residuals = [ordered]@{
				containers = @($containerSweep.Items)
				networks = @($networkSweep.Items)
				volumes = @($volumeSweep.Items)
				images = @($imageSweep.Items)
			}
		}
		try {
			Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6)
		}
		catch {
			$cleanupPassed = $false
		}
		if (-not $cleanupPassed) {
			$cleanupFailed = $true
		}
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A62 Docker resource cleanup failed.')
    exit 1
}
try {
	Write-A62ArtifactInventory -Directory $evidenceDirectory -EvidenceClass $evidenceClass
}
catch {
	[Console]::Error.WriteLine($_.Exception.Message)
	exit 1
}
exit 0
