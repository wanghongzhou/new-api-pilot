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
    A01 = @('F01'); A02 = @('F01'); A03 = @('F01'); A04 = @('F02'); A05 = @('F02')
    A06 = @('F02', 'F04'); A07 = @('F02'); A08 = @('F02', 'F03'); A09 = @('F04')
    A10 = @('F02', 'F04'); A11 = @('F02', 'F04'); A12 = @('F03', 'F04'); A13 = @('F03', 'F04')
    A14 = @('F03', 'F04'); A15 = @('F03', 'F05'); A16 = @('F05'); A17 = @('F05')
    A18 = @('F03', 'F05'); A19 = @('F02', 'F04'); A20 = @('F01', 'F02', 'F03', 'F05')
    A21 = @('F02', 'F04'); A23 = @('F02', 'F03', 'F04'); A24 = @('F02', 'F04')
    A26 = @('F01', 'F03'); A27 = @('F03'); A28 = @('F03', 'F04'); A29 = @('F03', 'F04')
    A30 = @('F04'); A31 = @('F04', 'F05'); A32 = @('F03', 'F04'); A33 = @('F03', 'F04'); A34 = @('F02', 'F04')
    A35 = @('F02', 'F04'); A36 = @('F02', 'F04'); A37 = @('F03', 'F04')
    A38 = @('F02', 'F03'); A39 = @('F03'); A40 = @('F03'); A41 = @('F03', 'F05')
    A42 = @('F04'); A43 = @('F04'); A44 = @('F04', 'F05'); A46 = @('F01', 'F05')
    A47 = @('F01'); A48 = @('F04', 'F05'); A53 = @('F04'); A54 = @('F02', 'F04')
    A55 = @('F02'); A56 = @('F02'); A57 = @('F02', 'F04'); A58 = @('F04')
    A59 = @('F04'); A60 = @('F04'); A61 = @('F03', 'F04'); A63 = @('F02', 'F04', 'F05')
    A64 = @('F03'); A65 = @('F03'); A66 = @('F01', 'F04'); A67 = @('F03', 'F05'); A68 = @('F03', 'F04')
    A69 = @('F05'); A70 = @('F01', 'F04', 'F05'); A71 = @('F01', 'F04')
    A72 = @('F01', 'F03', 'F04', 'F05'); A73 = @('F01', 'F04', 'F05')
    A76 = @('F01', 'F02', 'F04'); A77 = @('F01', 'F02', 'F03', 'F04', 'F05'); A78 = @('F04')
    A79 = @('F04'); A80 = @('F02', 'F04'); A81 = @('F02', 'F03', 'F04'); A82 = @('F03')
    A83 = @('F01', 'F02', 'F03', 'F04', 'F05'); A84 = @('F04', 'F05'); A86 = @('F02', 'F04')
    A87 = @('F01', 'F02', 'F03', 'F04', 'F05'); A88 = @('F03', 'F04')
    A89 = @('F02', 'F04', 'F05'); A90 = @('F02', 'F03', 'F04', 'F05'); A91 = @('F02', 'F03', 'F04', 'F05')
    A92 = @('F02', 'F03', 'F04', 'F05'); A93 = @('F02', 'F04', 'F05', 'F06'); A94 = @('F02', 'F04', 'F05', 'F06')
    A95 = @('F02', 'F04', 'F05', 'F07'); A96 = @('F02', 'F04', 'F05', 'F08'); A97 = @('F02', 'F03', 'F04')
    A98 = @('F02', 'F04', 'F05', 'F09'); A99 = @('F02', 'F04', 'F05', 'F10'); A100 = @('F02', 'F04', 'F05', 'F11')
    A101 = @('F12'); A102 = @('F04', 'F12', 'F13')
}

$goCases = @{
    A01 = @('./tests/integration', '^TestA01A02A03AuthenticationLifecycleAcceptance$')
    A02 = @('./tests/integration', '^TestA01A02A03AuthenticationLifecycleAcceptance$')
    A03 = @('./tests/integration', '^TestA01A02A03AuthenticationLifecycleAcceptance$')
    A04 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A06 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A07 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A08 = @('./tests/integration', '^TestA08A12A13A28CollectionHourReplacementAndVisibility$')
    A09 = @('./tests/integration', '^TestA09A79CollectionWindowDeduplicationAndOverlap$')
    A10 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A11 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A12 = @('./tests/integration', '^TestA08A12A13A28CollectionHourReplacementAndVisibility$')
    A13 = @('./tests/contract', '^TestA13A40A64A82StatisticsAPIContract$')
    A14 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A15 = @('./tests/integration', '^TestA15A16A17ExportCreationAcceptance$')
    A16 = @('./tests/integration', '^TestA15A16A17ExportCreationAcceptance$')
    A17 = @('./tests/integration', '^TestA15A16A17ExportCreationAcceptance$')
    A18 = @('./tests/contract', '^TestA18DashboardPartialRealtimeContract$')
    A19 = @('./tests/integration', '^TestA19A63ResourceSnapshotAcceptance$')
    A20 = @('./tests/contract', '^TestA20A26A87APIEnvelopePaginationAndAuthorizationAcceptance$')
    A21 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A23 = @('./tests/integration', '^TestA23AuthorizationCreatesFrozenInitialBackfillRange$')
    A24 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A26 = @('./tests/contract', '^TestA20A26A87APIEnvelopePaginationAndAuthorizationAcceptance$')
    A27 = @('./tests/integration', '^TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity$')
    A28 = @('./tests/integration', '^TestA08A12A13A28CollectionHourReplacementAndVisibility$')
    A29 = @('./tests/integration', '^TestA29A68StatisticsMissingDerivationAndPausedRecovery$')
    A30 = @('./tests/integration', '^TestA30A84SchedulerCadenceAndRealtimePriority$')
    A31 = @('./tests/integration', '^TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership$')
    A32 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A33 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A34 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
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
    A69 = @('./tests/integration', '^TestA69SettingsWithoutH15GateAndWithNinetyDayLogRetention$')
    A70 = @('./tests/contract', '^TestA70MessageRefAndZhCNContractAcceptance$')
    A76 = @('./router', '^(TestUserAPIForcePasswordChangeAndSessionRotation|TestViewerCanReadButCannotWritePlatformUsers|TestLoginRateLimitReturnsRetryAfter|TestDisabledUserLoginReportsDisabledOnlyAfterPasswordVerification|TestPlatformUserAdminLifecycleOverHTTP)$')
    A78 = @('./tests/integration', '^TestA42A43A53A78AlertStateMachineAcceptance$')
    A79 = @('./tests/integration', '^TestA09A79CollectionWindowDeduplicationAndOverlap$')
    A80 = @('./tests/integration', '^TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance$')
    A81 = @('./tests/integration', '^TestA81CompleteUserInventorySnapshotStatisticsAndPrivacyAcceptance$')
    A82 = @('./tests/contract', '^TestA13A40A64A82StatisticsAPIContract$')
    A84 = @('./tests/integration', '^TestA30A84SchedulerCadenceAndRealtimePriority$')
    A86 = @('./tests/integration', '^TestA04A06A07A10A24A36A37A56A57A86SiteAcceptance$')
    A87 = @('./tests/contract', '^TestA20A26A87APIEnvelopePaginationAndAuthorizationAcceptance$')
    A89 = @('./tests/integration', '^TestA45SecurityBoundaryAcceptance$')
    A90 = @('./tests/integration', '^TestA81CompleteUserInventorySnapshotStatisticsAndPrivacyAcceptance$')
    A91 = @('./tests/integration', '^TestChannelInventorySnapshotStatisticsAndPrivacyAcceptance$')
    A92 = @('./tests/integration', '^TestA63PerformanceHistoryAverageBoundaryAndWeightedCounters$')
    A93 = @('./tests/integration', '^TestA93A94FinanceOperationsExactPrivacyAndAggregationBoundaries$')
    A94 = @('./tests/integration', '^TestA93A94FinanceOperationsExactPrivacyAndAggregationBoundaries$')
    A95 = @('./tests/integration', '^TestA95UpstreamTaskTransitionStatisticsRetentionAndPrivacy$')
    A96 = @('./tests/integration', '^TestA96ModelCatalogCoverageMissingAndPrivacy$')
    A97 = @('./tests/integration', '^TestA97LocalModelAndVendorRankings$')
    A98 = @('./tests/integration', '^TestA98SubscriptionPlanCatalogMissingPrivacyAndExport$')
    A99 = @('./tests/integration', '^TestA99PricingAndGroupCatalog$')
    A100 = @('./tests/integration', '^TestA100SystemTaskReadOnlyMonitoring$')
    A101 = @('./tests/integration', '^TestA101SiteTaskCatalogMatchesBackendTaskContract$')
    A102 = @('./tests/integration ./internal/docscheck', '^(TestA102.*|TestExpectedDataMaintenanceContractsContainFiveOperations|TestDataMaintenanceCatalogDetectsMissingExtraAndTriggerDrift)$')
}
$e2eCases = @{
    A05 = 'e2e/site-authorization.spec.ts'
    A55 = 'e2e/site-authorization.spec.ts'
    A66 = 'e2e/alerts.spec.ts'
    A67 = 'e2e/exports.spec.ts'
    A71 = 'e2e/alert-rules.spec.ts'
    A72 = 'e2e/deep-links.spec.ts'
    A73 = 'e2e/settings.spec.ts'
    A77 = 'e2e/accessibility-responsive.spec.ts'
    A88 = 'e2e/f5-dashboard-statistics.spec.ts'
    A89 = 'e2e/logs.spec.ts'
    A90 = 'e2e/user-inventory.spec.ts'
    A91 = 'e2e/channel-inventory.spec.ts'
    A92 = 'e2e/performance-history.spec.ts'
    A93 = 'e2e/financial-operations.spec.ts'
    A94 = 'e2e/financial-operations.spec.ts'
    A95 = 'e2e/upstream-tasks.spec.ts'
    A96 = 'e2e/model-catalog.spec.ts'
    A97 = 'e2e/rankings.spec.ts'
    A98 = 'e2e/subscription-plans.spec.ts'
    A99 = 'e2e/pricing-groups.spec.ts'
    A100 = 'e2e/system-tasks.spec.ts'
    A101 = 'e2e/site-task-catalog.spec.ts'
}
$bunCases = @{
    A89 = @('src/features/logs/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A90 = @('src/features/user-inventory/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A91 = @('src/features/channel-inventory/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A92 = @('src/features/performance-history/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A93 = @('src/features/financial-operations/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A94 = @('src/features/financial-operations/export-request.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A95 = @('src/features/upstream-tasks/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A96 = @('src/features/model-catalog/icon-boundary.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A97 = @('src/features/rankings/api.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A98 = @('src/features/subscription-plans/privacy-boundary.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A99 = @('src/features/pricing-groups/privacy-boundary.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A100 = @('src/features/system-tasks/privacy-boundary.test.ts', 'src/lib/acceptance-fixture-consumption.test.ts')
    A101 = @('src/features/sites/site-task-catalog.test.ts', 'src/features/sites/site-task-catalog-fixture-consumption.test.ts')
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
    $packages = @(([string]$mapping[0]) -split ' ' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
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
            $image, 'go', 'test', '-json', '-count=1', '-p', '1', '-run', $testPattern
        ) + $packages
        $prefix = if ($bunCases.ContainsKey($acceptanceID) -or $e2eCases.ContainsKey($acceptanceID)) { 'go-' } else { 'case-' }
        Write-CaseJson -Name ($prefix + 'command.json') -Value ([ordered]@{
            schema_version = 1; acceptance_id = $acceptanceID; command = @('docker') + $dockerArguments
            image = $image; image_digest = $imageDigest; packages = $packages; test_pattern = $testPattern
            database_class = 'isolated_new_api_pilot_test'; fixture = Get-FixtureEvidence
        })
        $result = Invoke-OpsProcess -FileName 'docker' -Arguments $dockerArguments -TimeoutSeconds 1800
        Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ($prefix + 'results.jsonl')) -Payload $result.Stdout
        Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ($prefix + 'stderr.log')) -Payload $result.Stderr
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
        Write-CaseJson -Name ($prefix + 'report.json') -Value ([ordered]@{
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
    $prefix = if ($goCases.ContainsKey($acceptanceID) -or $bunCases.ContainsKey($acceptanceID)) { 'e2e-' } else { 'case-' }
    Write-CaseJson -Name ($prefix + 'command.json') -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; command = @('bun') + $bunArguments
        working_directory = 'web'; projects = @('chromium-desktop', 'chromium-mobile'); fixture = Get-FixtureEvidence
    })
    $previousInternalPort = [Environment]::GetEnvironmentVariable('PLAYWRIGHT_INTERNAL_PORT', 'Process')
    [Environment]::SetEnvironmentVariable('PLAYWRIGHT_INTERNAL_PORT', '4173', 'Process')
    try {
        $result = Invoke-OpsProcess -FileName 'powershell.exe' -Arguments $arguments -TimeoutSeconds 1800 -WorkingDirectory (Join-Path $repositoryRoot 'web')
    }
    finally {
        [Environment]::SetEnvironmentVariable('PLAYWRIGHT_INTERNAL_PORT', $previousInternalPort, 'Process')
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ($prefix + 'playwright-report.json')) -Payload $result.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory ($prefix + 'stderr.log')) -Payload $result.Stderr
    $parsed = $result.Stdout | ConvertFrom-Json
    $counts = Get-PlaywrightResults -Node $parsed
    $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0 -and $counts.total -gt 0 -and $counts.skipped -eq 0 -and $counts.failed -eq 0 -and $counts.passed -eq $counts.total)
    Write-CaseJson -Name ($prefix + 'report.json') -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
        passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut
        total = $counts.total; passed_tests = $counts.passed; skipped = $counts.skipped; failed = $counts.failed
    })
    if (-not $passed) { throw "$acceptanceID E2E did not produce the complete desktop/mobile passing matrix" }
}

function Invoke-BunAcceptance {
    $testPaths = @($bunCases[$acceptanceID])
    $bunArguments = @('test') + $testPaths
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $bunScript) + $bunArguments
    Write-CaseJson -Name 'bun-command.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; command = @('bun') + $bunArguments
        working_directory = 'web'; test_paths = $testPaths; fixture = Get-FixtureEvidence
    })
    $result = Invoke-OpsProcess -FileName 'powershell.exe' -Arguments $arguments -TimeoutSeconds 1800 -WorkingDirectory (Join-Path $repositoryRoot 'web')
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'bun-stdout.log') -Payload $result.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'bun-stderr.log') -Payload $result.Stderr
    $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0 -and ($result.Stdout + "`n" + $result.Stderr) -match '(?im)^\s*[1-9]\d*\s+pass')
    Write-CaseJson -Name 'bun-report.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
        passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut; test_paths = $testPaths
    })
    if (-not $passed) { throw "$acceptanceID Bun acceptance failed or ran no passing tests" }
}

function Invoke-DocsNegativeAcceptance {
    $runner = [string]$env:ACCEPTANCE_RUNNER_EXE
    if ([string]::IsNullOrWhiteSpace($runner) -or -not (Test-Path -LiteralPath $runner -PathType Leaf)) {
        throw 'canonical acceptance runner executable is unavailable'
    }
    $arguments = @('docs-negative', '-root', $repositoryRoot)
    Write-CaseJson -Name 'case-command.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; command = @($runner) + $arguments
        fixture = Get-FixtureEvidence
    })
    $result = Invoke-OpsProcess -FileName $runner -Arguments $arguments -TimeoutSeconds 1800 -WorkingDirectory $repositoryRoot
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'docs-negative-stdout.log') -Payload $result.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'docs-negative-stderr.log') -Payload $result.Stderr
    $passed = (-not $result.TimedOut -and $result.ExitCode -eq 0)
    Write-CaseJson -Name 'case-report.json' -Value ([ordered]@{
        schema_version = 1; acceptance_id = $acceptanceID; status = $(if ($passed) { 'passed' } else { 'failed' })
        passed = $passed; exit_code = $result.ExitCode; timed_out = $result.TimedOut
    })
    if (-not $passed) { throw 'A83 document integrity negative acceptance failed' }
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
elseif ($acceptanceID -eq 'A83') { Invoke-DocsNegativeAcceptance }
elseif ($goCases.ContainsKey($acceptanceID) -or $bunCases.ContainsKey($acceptanceID) -or $e2eCases.ContainsKey($acceptanceID)) {
    if ($goCases.ContainsKey($acceptanceID)) { Invoke-GoAcceptance }
    if ($bunCases.ContainsKey($acceptanceID)) { Invoke-BunAcceptance }
    if ($e2eCases.ContainsKey($acceptanceID)) {
    Push-Location (Join-Path $repositoryRoot 'web')
    try { Invoke-E2EAcceptance }
    finally { Pop-Location }
    }
}
else { throw "unsupported generic acceptance case $acceptanceID" }

Write-Output "$acceptanceID formal acceptance passed"
