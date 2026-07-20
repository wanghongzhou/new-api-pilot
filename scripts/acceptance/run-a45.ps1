[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = 'A45'
$targetTest = 'TestA45SecurityBoundaryAcceptance'
$testPackage = 'new-api-pilot/tests/integration'
$goImage = 'golang:1.25.1'
$testTimeoutSeconds = 300
$requiredSubtests = @(
    'TestA45SecurityBoundaryAcceptance/origin_fail_closed',
    'TestA45SecurityBoundaryAcceptance/trusted_proxy_boundary',
    'TestA45SecurityBoundaryAcceptance/upstream_DNS_TLS_and_credential_boundary',
    'TestA45SecurityBoundaryAcceptance/sensitive_response_and_logs'
)
$innerCommand = @(
    'go', 'test', '-json', './tests/integration', '-run', '^TestA45SecurityBoundaryAcceptance$',
    '-count=1', '-timeout=4m'
)
$fixtureF01Path = 'testdata/design/f01-auth.json'
$fixtureF01SHA256 = 'd232dc1a6b83ba80f49995dadbd8afe11d7b73120f7474a2abcece7e1b46e1da'
$fixtureF02Path = 'testdata/design/f02-upstream/manifest.json'
$fixtureF02SHA256 = 'f1a12b434ab24c01bf53d12bc65ccd86c90cd3e8f620c94f865e67f14b210f2f'
$fixtureManifestPath = 'testdata/design/manifest.sha256'
$secretScanTargets = @(
    'a45-test.jsonl',
    'a45-test.stderr.log',
    'a45-test-summary.json',
    'a45-command.json',
    'a45-environment.json',
    'a45-fixture.json',
    'a45-report.json',
    'a45-cleanup.json'
)
$requiredArtifacts = @($secretScanTargets) + @('a45-secret-scan.json')

function Get-A45ImageIdentity {
    param([Parameter(Mandatory = $true)][string]$Reference)

    $result = Invoke-OpsProcess -FileName 'docker' -Arguments @(
        'image', 'inspect', $Reference, '--format', '{{.Id}}|{{index .RepoDigests 0}}'
    ) -TimeoutSeconds 30
    if ($result.TimedOut -or $result.ExitCode -ne 0) {
        throw "A45 required image is unavailable: $Reference"
    }
    $fields = $result.Stdout.Trim().Split('|')
    if ($fields.Count -ne 2 -or $fields[0] -notmatch '^sha256:[0-9a-f]{64}$' -or
        $fields[1] -notmatch '^[^|]+@sha256:[0-9a-f]{64}$') {
        throw "A45 image identity is invalid: $Reference"
    }
    return [ordered]@{ reference = $Reference; id = $fields[0]; digest = $fields[1] }
}

function Assert-A45TestJSON {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $lines = 0
    $targetPasses = 0
    $packagePasses = 0
    $failures = 0
    $skips = 0
    $noTests = $false
    $subtestPasses = @{}
    foreach ($subtest in $requiredSubtests) { $subtestPasses[$subtest] = 0 }
    foreach ($rawLine in ($Payload -split "`r?`n")) {
        $line = $rawLine.Trim()
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $lines++
        try { $event = $line | ConvertFrom-Json -ErrorAction Stop }
        catch { throw "A45 go test stdout line $lines is not valid JSON." }
        if ([string]$event.Package -cne $testPackage) {
            throw "A45 go test stdout line $lines has an unexpected package."
        }
        $action = [string]$event.Action
        $test = if ($null -ne $event.PSObject.Properties['Test']) { [string]$event.Test } else { '' }
        $output = if ($null -ne $event.PSObject.Properties['Output']) { [string]$event.Output } else { '' }
        if ($action -ceq 'skip') { $skips++ }
        if ($action -ceq 'fail') { $failures++ }
        if ($action -ceq 'pass') {
            if ($test -ceq $targetTest) {
                $targetPasses++
            }
            elseif ([string]::IsNullOrEmpty($test)) {
                $packagePasses++
            }
            elseif ($test.StartsWith("$targetTest/", [System.StringComparison]::Ordinal)) {
                if (-not $subtestPasses.ContainsKey($test)) {
                    throw "A45 emitted an unexpected passing subtest: $test"
                }
                $subtestPasses[$test] = [int]$subtestPasses[$test] + 1
            }
        }
        if ($output -match '(?i)no tests to run|\[no test files\]') { $noTests = $true }
    }
    foreach ($subtest in $requiredSubtests) {
        if ([int]$subtestPasses[$subtest] -ne 1) {
            throw "A45 required subtest did not emit exactly one pass event: $subtest"
        }
    }
    if ($lines -le 0 -or $targetPasses -ne 1 -or $packagePasses -ne 1 -or
        $failures -ne 0 -or $skips -ne 0 -or $noTests) {
        throw 'A45 go test stream did not prove one unskipped passing target and all four required scenarios.'
    }
    return [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        status = 'passed'
        target_test = $targetTest
        package = $testPackage
        pass_events = $targetPasses
        subtest_pass_events = $requiredSubtests.Count
        package_pass_events = $packagePasses
        fail_events = $failures
        skip_events = $skips
        no_tests = $noTests
        json_lines = $lines
        required_subtests = @($requiredSubtests)
        json_path = 'a45-test.jsonl'
        stderr_path = 'a45-test.stderr.log'
    }
}

function Get-A45ContainerIsolation {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][string]$Network
    )

    $networkInternal = Invoke-OpsDocker -Arguments @('network', 'inspect', '--format', '{{.Internal}}', $Network) -TimeoutSeconds 30
    if ($networkInternal.Stdout.Trim() -cne 'true') { throw 'A45 Docker network is not internal.' }

    $attached = Invoke-OpsDocker -Arguments @(
        'inspect', '--format', '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}', $Container
    ) -TimeoutSeconds 30
    $attachedNetworks = @(($attached.Stdout -split "`r?`n") | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($attachedNetworks.Count -ne 1 -or $attachedNetworks[0] -cne $Network) {
        throw 'A45 test container is not attached only to its unique internal network.'
    }

    $networkContainers = Invoke-OpsDocker -Arguments @('network', 'inspect', '--format', '{{json .Containers}}', $Network) -TimeoutSeconds 30
    $networkContainerObject = $networkContainers.Stdout.Trim() | ConvertFrom-Json -ErrorAction Stop
    if (@($networkContainerObject.PSObject.Properties).Count -gt 1) {
        throw 'A45 internal network contains another container.'
    }

    $portResult = Invoke-OpsProcess -FileName 'docker' -Arguments @('port', $Container) -TimeoutSeconds 30
    if ($portResult.TimedOut -or $portResult.ExitCode -ne 0 -or -not [string]::IsNullOrWhiteSpace($portResult.Stdout)) {
        throw 'A45 test container exposes a host port.'
    }

    $rootFS = Invoke-OpsDocker -Arguments @('inspect', '--format', '{{.HostConfig.ReadonlyRootfs}}', $Container) -TimeoutSeconds 30
    if ($rootFS.Stdout.Trim() -cne 'true') { throw 'A45 test container root filesystem is writable.' }

    $securityOptions = Invoke-OpsDocker -Arguments @('inspect', '--format', '{{json .HostConfig.SecurityOpt}}', $Container) -TimeoutSeconds 30
    $parsedSecurityOptions = $securityOptions.Stdout.Trim() | ConvertFrom-Json -ErrorAction Stop
    $securityOptionValues = @($parsedSecurityOptions)
    if ($securityOptionValues -cnotcontains 'no-new-privileges:true') {
        throw 'A45 test container does not enforce no-new-privileges.'
    }

    $capabilities = Invoke-OpsDocker -Arguments @('inspect', '--format', '{{json .HostConfig.CapDrop}}', $Container) -TimeoutSeconds 30
    $parsedCapabilities = $capabilities.Stdout.Trim() | ConvertFrom-Json -ErrorAction Stop
    $capabilityValues = @($parsedCapabilities)
    if ($capabilityValues.Count -ne 1 -or $capabilityValues[0] -cne 'ALL') {
        throw 'A45 test container did not drop every Linux capability.'
    }

    $mounts = Invoke-OpsDocker -Arguments @('inspect', '--format', '{{json .Mounts}}', $Container) -TimeoutSeconds 30
    $parsedMounts = $mounts.Stdout.Trim() | ConvertFrom-Json -ErrorAction Stop
    $mountValues = @($parsedMounts)
    $workspaceMount = @($mountValues | Where-Object { [string]$_.Destination -ceq '/workspace' })
    $evidenceMount = @($mountValues | Where-Object { [string]$_.Destination -ceq '/evidence' })
    if ($workspaceMount.Count -ne 1 -or [string]$workspaceMount[0].Type -cne 'bind' -or
        [bool]$workspaceMount[0].RW -or $evidenceMount.Count -ne 0) {
        throw 'A45 test container source/evidence mount contract is invalid.'
    }

    $environmentResult = Invoke-OpsDocker -Arguments @('inspect', '--format', '{{json .Config.Env}}', $Container) -TimeoutSeconds 30
    $parsedEnvironment = $environmentResult.Stdout.Trim() | ConvertFrom-Json -ErrorAction Stop
    $environmentValues = @($parsedEnvironment)
    $environment = @{}
    foreach ($entry in $environmentValues) {
        $separator = ([string]$entry).IndexOf('=')
        if ($separator -gt 0) {
            $environment[([string]$entry).Substring(0, $separator)] = ([string]$entry).Substring($separator + 1)
        }
    }
    foreach ($name in @('HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY', 'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy')) {
        if (-not $environment.ContainsKey($name) -or -not [string]::IsNullOrEmpty([string]$environment[$name])) {
            throw "A45 test container proxy variable is not explicitly cleared: $name"
        }
    }
    if ([string]$environment['GOPROXY'] -cne 'off' -or [string]$environment['GOSUMDB'] -cne 'off') {
        throw 'A45 test container Go dependency network is not disabled.'
    }
    return [ordered]@{
        internal = $true
        host_ports = @()
        attached_networks = @($attachedNetworks)
        repository_read_only = $true
        container_rootfs_read_only = $true
        no_new_privileges = $true
        all_capabilities_dropped = $true
        evidence_mounted_in_test = $false
        offline_test_network = $true
        go_proxy_off = $true
        go_sumdb_off = $true
        environment_proxies_cleared = $true
    }
}

function Write-A45SecretScan {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $pattern = '(?i)(?:a45-sensitive-value-never-log|a45-old-token-never-send|(?:authorization|access[_-]?token|webhook(?:[_-]?url)?|password|session[_-]?secret|encryption[_-]?key)\s*["'']?\s*[:=]\s*["'']?[^\s"'']{4,}|[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)'
    $matches = 0
    foreach ($relative in $secretScanTargets) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "A45 secret scan target is missing: $relative"
        }
        $payload = [System.IO.File]::ReadAllText($path)
        if ([regex]::IsMatch($payload, $pattern)) { $matches++ }
    }
    if ($matches -ne 0) { throw 'A45 evidence contains a forbidden credential or secret pattern.' }
    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        evidence_class = $EvidenceClass
        status = 'passed'
        files = @($secretScanTargets)
        matches = $matches
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a45-secret-scan.json') -Payload ($report | ConvertTo-Json -Depth 4)
}

function Write-A45ArtifactInventory {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $files = @()
    foreach ($relative in $requiredArtifacts) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "A45 required evidence artifact is missing: $relative"
        }
        $info = Get-Item -LiteralPath $path
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    if ($files.Count -ne 9) { throw 'A45 inventory must hash exactly nine payload artifacts.' }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        evidence_class = $EvidenceClass
        files = $files
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a45-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
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
$networkName = "new-api-pilot-a45-$runToken-network"
$moduleVolumeName = "new-api-pilot-a45-$runToken-gomod"
$buildVolumeName = "new-api-pilot-a45-$runToken-gobuild"
$warmContainerName = "new-api-pilot-a45-$runToken-warm"
$testContainerName = "new-api-pilot-a45-$runToken-test"
$warmContainerMayExist = $false
$testContainerMayExist = $false
$networkMayExist = $false
$moduleVolumeMayExist = $false
$buildVolumeMayExist = $false
$evidenceDirectory = $null
$evidenceClass = $null

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID) {
        throw 'This script must be invoked by the acceptance runner with ACCEPTANCE_ID=A45.'
    }
    if ($env:ACCEPTANCE_EVIDENCE_CLASS -notin @('formal', 'development') -or
        [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE)) {
        throw 'A45 evidence must be invoked by the canonical acceptance wrapper.'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory.'
    }
    $evidenceClass = [string]$env:ACCEPTANCE_EVIDENCE_CLASS
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $relativeEvidence = Get-OpsRepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    $expectedPrefix = if ($evidenceClass -ceq 'formal') { 'artifacts/acceptance/A45/' } else { 'artifacts/smoke/A45/' }
    if (-not $relativeEvidence.StartsWith($expectedPrefix, [System.StringComparison]::Ordinal)) {
        throw "A45 $evidenceClass evidence directory is not under $expectedPrefix"
    }

    [void](Invoke-OpsDocker -Arguments @('version', '--format', '{{.Client.Version}}') -TimeoutSeconds 30)
    $imageIdentity = Get-A45ImageIdentity -Reference $goImage
    $repositoryMount = "type=bind,source=$repositoryRoot,target=/workspace,readonly"

    $moduleVolumeMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'volume', 'create', '--label', 'new-api-pilot.acceptance=A45', '--label', $runLabel, $moduleVolumeName
    ) -TimeoutSeconds 30)
    $buildVolumeMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'volume', 'create', '--label', 'new-api-pilot.acceptance=A45', '--label', $runLabel, $buildVolumeName
    ) -TimeoutSeconds 30)
    $volumesCreated = $true

    $warmContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'create', '--name', $warmContainerName,
        '--label', 'new-api-pilot.acceptance=A45', '--label', $runLabel,
        '--workdir', '/workspace', '--mount', $repositoryMount,
        '--mount', "type=volume,source=$moduleVolumeName,target=/go/pkg/mod",
        '--mount', "type=volume,source=$buildVolumeName,target=/go-build",
        '--env', 'GOCACHE=/go-build', '--env', 'GOTELEMETRY=off',
        $goImage, 'go', 'mod', 'download'
    ) -TimeoutSeconds 60)
    $containersCreated = $true
    [void](Invoke-OpsDocker -Arguments @('start', $warmContainerName) -TimeoutSeconds 30)
    $warmExit = Wait-OpsContainer -Container $warmContainerName -TimeoutSeconds 600
    if ($warmExit -ne 0) { throw 'A45 Go dependency warm-up failed.' }
    if (-not (Remove-OpsDockerResource -Arguments @('rm', '--force', $warmContainerName))) {
        throw 'A45 Go dependency warm-up cleanup failed.'
    }
    $warmContainerMayExist = $false

    $networkMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'network', 'create', '--internal',
        '--label', 'new-api-pilot.acceptance=A45', '--label', $runLabel, $networkName
    ) -TimeoutSeconds 30)
    $networkCreated = $true

    $testContainerMayExist = $true
    [void](Invoke-OpsDocker -Arguments @(
        'create', '--name', $testContainerName, '--network', $networkName,
        '--label', 'new-api-pilot.acceptance=A45', '--label', $runLabel,
        '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges:true',
        '--pids-limit', '512', '--memory', '2g', '--cpus', '2',
        '--workdir', '/workspace', '--mount', $repositoryMount,
        '--mount', "type=volume,source=$moduleVolumeName,target=/go/pkg/mod,readonly",
        '--mount', "type=volume,source=$buildVolumeName,target=/go-build",
        '--tmpfs', '/tmp:rw,exec,nosuid,nodev,size=536870912',
        '--env', 'ACCEPTANCE_ID=A45', '--env', 'A45_ISOLATED_NETWORK=true',
        '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off', '--env', 'GOTELEMETRY=off',
        '--env', 'GOCACHE=/go-build', '--env', 'GOMODCACHE=/go/pkg/mod', '--env', 'GOTMPDIR=/tmp',
        '--env', 'HOME=/tmp/home',
        '--env', 'HTTP_PROXY=', '--env', 'HTTPS_PROXY=', '--env', 'ALL_PROXY=', '--env', 'NO_PROXY=',
        '--env', 'http_proxy=', '--env', 'https_proxy=', '--env', 'all_proxy=', '--env', 'no_proxy=',
        $goImage, 'go', 'test', '-json', './tests/integration',
        '-run', '^TestA45SecurityBoundaryAcceptance$', '-count=1', '-timeout=4m'
    ) -TimeoutSeconds 60)
    $isolation = Get-A45ContainerIsolation -Container $testContainerName -Network $networkName

    $gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot
    $commandReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        evidence_class = $evidenceClass
        target_test = $targetTest
        working_directory = '/workspace'
        command = $innerCommand
        go_image = $goImage
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-command.json') -Payload ($commandReport | ConvertTo-Json -Depth 5)
    $environmentReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        evidence_class = $evidenceClass
        commit = $gitState.Commit
        worktree_dirty = $gitState.WorktreeDirty
        go_image = $imageIdentity
        resources = [ordered]@{
            network = $networkName
            module_cache = $moduleVolumeName
            build_cache = $buildVolumeName
        }
        network = [ordered]@{
            internal = [bool]$isolation.internal
            host_ports = @($isolation.host_ports)
            attached_networks = @($isolation.attached_networks)
        }
        repository_read_only = [bool]$isolation.repository_read_only
        container_rootfs_read_only = [bool]$isolation.container_rootfs_read_only
        no_new_privileges = [bool]$isolation.no_new_privileges
        all_capabilities_dropped = [bool]$isolation.all_capabilities_dropped
        evidence_mounted_in_test = [bool]$isolation.evidence_mounted_in_test
        offline_test_network = [bool]$isolation.offline_test_network
        go_proxy_off = [bool]$isolation.go_proxy_off
        go_sumdb_off = [bool]$isolation.go_sumdb_off
        environment_proxies_cleared = [bool]$isolation.environment_proxies_cleared
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 7)

    $manifestPath = Join-Path $repositoryRoot $fixtureManifestPath
    $f01Path = Join-Path $repositoryRoot $fixtureF01Path
    $f02Path = Join-Path $repositoryRoot $fixtureF02Path
    $f01SHA = (Get-FileHash -LiteralPath $f01Path -Algorithm SHA256).Hash.ToLowerInvariant()
    $f02SHA = (Get-FileHash -LiteralPath $f02Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($f01SHA -cne $fixtureF01SHA256 -or $f02SHA -cne $fixtureF02SHA256) {
        throw 'A45 fixed F01/F02 fixture checksum differs from the approved contract.'
    }
    $manifestText = Get-Content -Raw -LiteralPath $manifestPath
    if ($manifestText -notmatch "(?m)^$fixtureF01SHA256  $([regex]::Escape($fixtureF01Path))$" -or
        $manifestText -notmatch "(?m)^$fixtureF02SHA256  $([regex]::Escape($fixtureF02Path))$") {
        throw 'A45 fixed fixture checksums are not bound by the fixture manifest.'
    }
    $manifestSHA = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $fixtureReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        fixtures = @(
            [ordered]@{ fixture_id = 'F01'; path = $fixtureF01Path; sha256 = $f01SHA },
            [ordered]@{ fixture_id = 'F02'; path = $fixtureF02Path; sha256 = $f02SHA }
        )
        manifest_path = $fixtureManifestPath
        manifest_sha256 = $manifestSHA
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-fixture.json') -Payload ($fixtureReport | ConvertTo-Json -Depth 5)

    [void](Invoke-OpsDocker -Arguments @('start', $testContainerName) -TimeoutSeconds 30)
    $testExit = Wait-OpsContainer -Container $testContainerName -TimeoutSeconds $testTimeoutSeconds
    if ($testExit -eq -1) { [void](Remove-OpsDockerResource -Arguments @('kill', $testContainerName)) }
    $logs = Invoke-OpsProcess -FileName 'docker' -Arguments @('logs', $testContainerName) -TimeoutSeconds 30
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-test.jsonl') -Payload $logs.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-test.stderr.log') -Payload $logs.Stderr
    if (-not [string]::IsNullOrEmpty($logs.Stdout)) { [Console]::Out.Write($logs.Stdout) }
    if (-not [string]::IsNullOrEmpty($logs.Stderr)) { [Console]::Error.Write($logs.Stderr) }
    if ($testExit -ne 0 -or $logs.TimedOut -or $logs.ExitCode -ne 0) {
        throw 'A45 isolated go test did not complete cleanly.'
    }
    $testSummary = Assert-A45TestJSON -Payload $logs.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-test-summary.json') -Payload ($testSummary | ConvertTo-Json -Depth 5)

    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A45'
        status = 'passed'
        target_test = $targetTest
        required_subtests = @($requiredSubtests)
        scenarios = [ordered]@{
            origin_fail_closed = $true
            trusted_proxy_boundary = $true
            upstream_dns_tls_credential_boundary = $true
            sensitive_response_and_logs = $true
        }
        security_checks = [ordered]@{
            forged_forwarded_for_rejected = $true
            invalid_origin_rejected = $true
            dns_rebinding_rejected = $true
            non_allowlisted_address_rejected = $true
            tls_downgrade_redirect_rejected = $true
            old_token_origin_guarded = $true
            environment_proxy_unused = $true
            sensitive_response_and_logs_redacted = $true
        }
        isolation_checks = [ordered]@{
            unique_internal_network = $true
            no_host_ports = $true
            repository_read_only = $true
            container_rootfs_read_only = $true
            environment_proxies_cleared = $true
            go_proxy_off = $true
        }
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-report.json') -Payload ($report | ConvertTo-Json -Depth 6)
}
catch {
    $failure = $_.Exception.Message
}
finally {
    foreach ($container in @(
        [pscustomobject]@{ MayExist = $testContainerMayExist; Name = $testContainerName },
        [pscustomobject]@{ MayExist = $warmContainerMayExist; Name = $warmContainerName }
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
    if ($null -ne $evidenceDirectory -and $null -ne $evidenceClass) {
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
            acceptance_id = 'A45'
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
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6)
        }
        catch {
            $cleanupFailed = $true
        }
        if (-not $cleanupPassed) { $cleanupFailed = $true }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A45 Docker resource cleanup failed.')
    exit 1
}
try {
    Write-A45SecretScan -Directory $evidenceDirectory -EvidenceClass $evidenceClass
    Write-A45ArtifactInventory -Directory $evidenceDirectory -EvidenceClass $evidenceClass
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
exit 0
