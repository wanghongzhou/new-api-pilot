$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = [string]$env:ACCEPTANCE_ID
$evidenceDirectory = [string]$env:ACCEPTANCE_EVIDENCE_DIR
$evidenceClass = [string]$env:ACCEPTANCE_EVIDENCE_CLASS
if ($acceptanceID -notmatch '^A(?:0[1-9]|[1-9]\d|10[0-2])$' -or $evidenceClass -cne 'formal' -or
    [string]::IsNullOrWhiteSpace($evidenceDirectory) -or -not [System.IO.Path]::IsPathRooted($evidenceDirectory) -or
    -not (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
    throw 'formal acceptance harness environment is required'
}

$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$bunScript = (Get-Command bun -ErrorAction Stop).Source
$fixtureMap = @{
    A35 = @('F02', 'F04'); A36 = @('F02', 'F04'); A37 = @('F03', 'F04')
    A38 = @('F02', 'F03'); A39 = @('F03'); A40 = @('F03'); A41 = @('F03', 'F05')
    A42 = @('F04'); A43 = @('F04'); A44 = @('F04', 'F05'); A46 = @('F01', 'F05')
    A47 = @('F01'); A48 = @('F04', 'F05'); A53 = @('F04'); A54 = @('F02', 'F04')
    A55 = @('F02'); A56 = @('F02'); A57 = @('F02', 'F04'); A58 = @('F04')
    A59 = @('F04'); A60 = @('F04'); A61 = @('F03', 'F04'); A63 = @('F02', 'F04', 'F05')
    A64 = @('F03'); A65 = @('F03'); A66 = @('F01', 'F04'); A67 = @('F03', 'F05'); A68 = @('F03', 'F04')
}

$goCases = @{
    A35 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A36 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A37 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A38 = @('./tests/integration', '^TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity$')
    A39 = @('./tests/integration', '^TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity$')
    A40 = @('./tests/integration', '^TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity$')
    A41 = @('./tests/integration', '^TestA41ExportFormulaInjectionAcceptance$')
    A42 = @('./tests/integration', '^TestA42A43A53A78AlertStateMachineAcceptance$')
    A43 = @('./tests/integration', '^TestA42A43A53A78AlertStateMachineAcceptance$')
    A44 = @('./tests/integration', '^TestA44DingTalkWebhookBoundaryAcceptance$')
    A46 = @('./tests/contract', '^TestA46SettingsAPIContract$')
    A48 = @('./tests/contract', '^TestA48OpsEndpointAcceptance$')
    A53 = @('./tests/integration', '^TestA42A43A53A78AlertStateMachineAcceptance$')
    A54 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A56 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A57 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A58 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A59 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A60 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A61 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A63 = @('./tests/integration', '^TestA63PerformanceHistoryAverageBoundaryAndWeightedCounters$')
    A64 = @('./tests/contract', '^TestA13A40A64A82StatisticsAPIContract$')
    A65 = @('./tests/integration', '^TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity$')
    A68 = @('./tests/integration', '^TestA29A68StatisticsMissingDerivationAndPausedRecovery$')
}
$e2eCases = @{
    A55 = 'e2e/site-authorization.spec.ts'
    A66 = 'e2e/alerts.spec.ts'
    A67 = 'e2e/exports.spec.ts'
}

function Write-CaseJson {
    param([Parameter(Mandatory = $true)][string]$Name, [Parameter(Mandatory = $true)]$Value)
    $payload = $Value | ConvertTo-Json -Depth 20
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory $Name) -Payload ($payload + "`n")
}

function Get-FixtureEvidence {
    $manifestPath = Join-Path $repositoryRoot 'testdata\design\manifest.sha256'
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    return [ordered]@{
        manifest_path = 'testdata/design/manifest.sha256'
        manifest_sha256 = $manifestHash
        fixture_ids = @($fixtureMap[$acceptanceID])
    }
}

function Invoke-GoAcceptance {
    $mapping = $goCases[$acceptanceID]
    $package = [string]$mapping[0]
    $testPattern = [string]$mapping[1]
    $databaseName = ('new_api_pilot_test_acceptance_{0}_{1}_{2}' -f $acceptanceID.ToLowerInvariant(), [DateTimeOffset]::UtcNow.ToUnixTimeSeconds(), $PID)
    $composeFile = Join-Path $repositoryRoot 'docker-compose.dev.yml'
    $image = 'new-api-pilot-go-test:latest'
    $mysqlContainer = 'new-api-pilot-dev-mysql'
    $networkResult = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', '--format', '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}', $mysqlContainer) -TimeoutSeconds 30
    if ($networkResult.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($networkResult.Stdout)) { throw 'cannot resolve isolated test network' }
    $network = $networkResult.Stdout.Trim()
    $imageResult = Invoke-OpsProcess -FileName 'docker' -Arguments @('image', 'inspect', '--format', '{{.Id}}', $image) -TimeoutSeconds 30
    if ($imageResult.ExitCode -ne 0 -or $imageResult.Stdout.Trim() -notmatch '^sha256:[0-9a-f]{64}$') { throw 'acceptance test image is unavailable' }
    $imageDigest = $imageResult.Stdout.Trim()
    $mysqlArgs = @('compose', '-f', $composeFile, 'exec', '-T', '-e', 'MYSQL_PWD=root', 'mysql', 'mysql', '-uroot', '-e')
    $createSQL = "DROP DATABASE IF EXISTS ``$databaseName``; CREATE DATABASE ``$databaseName`` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; GRANT ALL PRIVILEGES ON ``$databaseName``.* TO 'pilot'@'%';"
    $dropSQL = "DROP DATABASE IF EXISTS ``$databaseName``;"
    $created = $false
    try {
        $create = Invoke-OpsProcess -FileName 'docker' -Arguments ($mysqlArgs + @($createSQL)) -TimeoutSeconds 90
        if ($create.ExitCode -ne 0) { throw 'cannot create isolated acceptance database' }
        $created = $true
        $dsn = "pilot:pilot@tcp(mysql:3306)/${databaseName}?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai"
        $dockerArguments = @(
            'run', '--rm', '--network', $network,
            '--mount', 'type=volume,source=new-api-pilot-go-test-cache,target=/root/.cache/go-build',
            '-e', 'GOPROXY=off', '-e', 'GOSUMDB=off', '-e', "ACCEPTANCE_ID=$acceptanceID",
            '-e', "TEST_DATABASE_DSN=$dsn", '-e', 'TEST_DATABASE_ADMIN_DSN=root:root@tcp(mysql:3306)/?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai',
            $image, 'go', 'test', '-json', '-count=1', '-p', '1', '-run', $testPattern, $package
        )
        Write-CaseJson -Name 'case-command.json' -Value ([ordered]@{
            schema_version = 1; acceptance_id = $acceptanceID; command = @('docker') + $dockerArguments
            image = $image; image_digest = $imageDigest; package = $package; test_pattern = $testPattern
            database_class = 'isolated_new_api_pilot_test'; fixture = Get-FixtureEvidence
        })
        $result = Invoke-OpsProcess -FileName 'docker' -Arguments $dockerArguments -TimeoutSeconds 1800
        Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'test-results.jsonl') -Payload $result.Stdout
        Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'test-stderr.log') -Payload $result.Stderr
        $events = @()
        foreach ($line in ($result.Stdout -split "`r?`n")) {
            if (-not [string]::IsNullOrWhiteSpace($line)) { $events += ($line | ConvertFrom-Json) }
        }
        $skips = @($events | Where-Object { $_.Action -eq 'skip' })
        $testPasses = @($events | Where-Object {
            $_.Action -eq 'pass' -and $null -ne $_.PSObject.Properties['Test'] -and
            -not [string]::IsNullOrWhiteSpace([string]$_.PSObject.Properties['Test'].Value)
        })
        $packagePasses = @($events | Where-Object {
            $_.Action -eq 'pass' -and ($null -eq $_.PSObject.Properties['Test'] -or
            [string]::IsNullOrWhiteSpace([string]$_.PSObject.Properties['Test'].Value))
        })
        $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0 -and $skips.Count -eq 0 -and $testPasses.Count -gt 0 -and $packagePasses.Count -gt 0)
        Write-CaseJson -Name 'case-report.json' -Value ([ordered]@{
            schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
            passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut; skipped_events = $skips.Count
            passing_test_events = $testPasses.Count; passing_package_events = $packagePasses.Count
        })
        if (-not $passed) { throw "$acceptanceID Go acceptance did not produce an unskipped passing result" }
    }
    finally {
        if ($created) { [void](Invoke-OpsProcess -FileName 'docker' -Arguments ($mysqlArgs + @($dropSQL)) -TimeoutSeconds 90) }
    }
}

function Get-PlaywrightResults {
    param([Parameter(Mandatory = $true)]$Node)
    $result = [ordered]@{ total = 0; passed = 0; skipped = 0; failed = 0 }
    function Visit-Node($current) {
        if ($null -ne $current.PSObject.Properties['specs']) {
            foreach ($spec in @($current.PSObject.Properties['specs'].Value)) {
                foreach ($test in @($spec.PSObject.Properties['tests'].Value)) {
                    $result.total++
                    if ($test.status -eq 'expected') { $result.passed++ }
                    elseif ($test.status -eq 'skipped') { $result.skipped++ }
                    else { $result.failed++ }
                }
            }
        }
        if ($null -ne $current.PSObject.Properties['suites']) {
            foreach ($suite in @($current.PSObject.Properties['suites'].Value)) { Visit-Node $suite }
        }
    }
    Visit-Node $Node
    return $result
}

function Invoke-E2EAcceptance {
    $spec = [string]$e2eCases[$acceptanceID]
    $bunArguments = @('x', 'playwright', 'test', $spec, '--project=chromium-desktop', '--project=chromium-mobile', '--workers=2', '--retries=0', '--forbid-only', '--reporter=json')
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $bunScript) + $bunArguments
    Write-CaseJson -Name 'case-command.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; command = @('bun') + $bunArguments
        working_directory = 'web'; projects = @('chromium-desktop', 'chromium-mobile'); fixture = Get-FixtureEvidence
    })
    $result = Invoke-OpsProcess -FileName 'powershell.exe' -Arguments $arguments -TimeoutSeconds 1800 -WorkingDirectory (Join-Path $repositoryRoot 'web') -Environment @{ PLAYWRIGHT_BASE_URL = 'http://127.0.0.1:5173'; CI = '1' }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'playwright-report.json') -Payload $result.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'test-stderr.log') -Payload $result.Stderr
    $parsed = $result.Stdout | ConvertFrom-Json
    $counts = Get-PlaywrightResults -Node $parsed
    $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0 -and $counts.total -gt 0 -and $counts.skipped -eq 0 -and $counts.failed -eq 0 -and $counts.passed -eq $counts.total)
    Write-CaseJson -Name 'case-report.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
        passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut
        total = $counts.total; passed_tests = $counts.passed; skipped = $counts.skipped; failed = $counts.failed
    })
    if (-not $passed) { throw "$acceptanceID E2E did not produce the complete desktop/mobile passing matrix" }
}

if ($acceptanceID -eq 'A47') {
    $bunArguments = @('run', 'check')
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $bunScript) + $bunArguments
    Write-CaseJson -Name 'case-command.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; command = @('bun') + $bunArguments
        working_directory = 'web'; fixture = Get-FixtureEvidence
    })
    $result = Invoke-OpsProcess -FileName 'powershell.exe' -Arguments $arguments -TimeoutSeconds 1800 -WorkingDirectory (Join-Path $repositoryRoot 'web')
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'check-stdout.log') -Payload $result.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'check-stderr.log') -Payload $result.Stderr
    $i18nObserved = (($result.Stdout + "`n" + $result.Stderr) -match 'i18n:check')
    $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0)
    Write-CaseJson -Name 'case-report.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
        passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut; i18n_gate_observed = $i18nObserved
    })
    if (-not $passed) { throw 'A47 complete frontend check failed' }
}
elseif ($goCases.ContainsKey($acceptanceID)) { Invoke-GoAcceptance }
elseif ($e2eCases.ContainsKey($acceptanceID)) {
    Push-Location (Join-Path $repositoryRoot 'web')
    try { Invoke-E2EAcceptance }
    finally { Pop-Location }
}
else { throw "unsupported generic acceptance case $acceptanceID" }

Write-Output "$acceptanceID formal acceptance passed"
