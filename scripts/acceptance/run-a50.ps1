[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

Add-Type -AssemblyName System.Net.Http -ErrorAction Stop

$acceptanceID = 'A50'
$approvedSpecSHA = '38b71931dbce622dc82dbf9323836aa8ffcaf5aa475b8c87319864c3f750a40c'
$approvedPackageSHA = '1a7799fb3dd87e6897536e4d33539c866a8d6a471add2799fd27cde4b8873683'
$approvedPlaywrightConfigSHA = '16c060617c14eefd3e70d3dc2bf15139ae7c8d389a3c43a1e022f6c06151f155'
$fixturePath = 'testdata/design/f03-statistics.sql'
$fixtureSHA256 = 'bcceaaf7d6b171014258b9d935fbb1e7cab4585b49403d760a6db373e5aabe94'
$fixtureManifestPath = 'testdata/design/manifest.sha256'
$requiredProjects = @('chromium-desktop', 'chromium-mobile')
$requiredRoutes = @(
    [ordered]@{ key = 'global'; path = '/statistics/global'; title = 'global covers five states, refresh retention and reload' },
    [ordered]@{ key = 'sites'; path = '/statistics/sites'; title = 'sites covers five states, refresh retention and reload' },
    [ordered]@{ key = 'customers'; path = '/statistics/customers'; title = 'customers covers five states, refresh retention and reload' },
    [ordered]@{ key = 'accounts'; path = '/statistics/accounts'; title = 'accounts covers five states, refresh retention and reload' },
    [ordered]@{ key = 'models'; path = '/statistics/models'; title = 'models covers five states, refresh retention and reload' },
    [ordered]@{ key = 'channels'; path = '/statistics/channels'; title = 'channels covers five states, refresh retention and reload' },
    [ordered]@{ key = 'site-deep-link'; path = '/sites/1/stats'; title = 'site-deep-link covers five states, refresh retention and reload' },
    [ordered]@{ key = 'customer-deep-link'; path = '/customers/7/stats'; title = 'customer-deep-link covers five states, refresh retention and reload' },
    [ordered]@{ key = 'account-deep-link'; path = '/accounts/9/stats'; title = 'account-deep-link covers five states, refresh retention and reload' }
)
$checkCommand = @('bun', 'run', 'check')
$checkSteps = @('routes:generate', 'typecheck', 'lint', 'format:check', 'i18n:check', 'build:app')
$secretScanTargets = @(
    'a50-playwright.json',
    'a50-playwright-report.html',
    'a50-playwright.stdout.log',
    'a50-playwright.stderr.log',
    'a50-check.stdout.log',
    'a50-check.stderr.log',
    'a50-server.stdout.log',
    'a50-server.stderr.log',
    'a50-check-summary.json',
    'a50-command.json',
    'a50-environment.json',
    'a50-fixture.json',
    'a50-report.json',
    'a50-cleanup.json'
)
$requiredArtifacts = @($secretScanTargets) + @('a50-secret-scan.json')

function Get-A50FreePort {
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
        try {
            $listener.Start()
            $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
        }
        finally {
            $listener.Stop()
        }
        if ($port -ne 5173) { return $port }
    }
    throw 'A50 could not reserve an independent non-5173 loopback port.'
}

function Test-A50PortOpen {
    param(
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][int]$Port,
        [int]$TimeoutMilliseconds = 500
    )

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $task = $client.ConnectAsync($HostName, $Port)
        if (-not $task.Wait($TimeoutMilliseconds)) { return $false }
        return $client.Connected
    }
    catch {
        return $false
    }
    finally {
        $client.Dispose()
    }
}

function Wait-A50ServerReady {
    param(
        [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process,
        [Parameter(Mandatory = $true)][string]$BaseURL,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds
    )

    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    $client = [System.Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromSeconds(2)
    try {
        while ([DateTimeOffset]::UtcNow -lt $deadline) {
            if ($Process.HasExited) { throw 'A50 independent web server exited before becoming ready.' }
            try {
                $response = $client.GetAsync($BaseURL).GetAwaiter().GetResult()
                if ([int]$response.StatusCode -ge 200 -and [int]$response.StatusCode -lt 500) {
                    $response.Dispose()
                    return
                }
                $response.Dispose()
            }
            catch {}
            Start-Sleep -Milliseconds 500
        }
    }
    finally {
        $client.Dispose()
    }
    throw 'A50 independent web server readiness timed out.'
}

function Start-A50ServerProcess {
    param(
        [Parameter(Mandatory = $true)][string]$BunExecutable,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $BunExecutable
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-OpsNativeArgument $_ }) -join ' ')
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.EnvironmentVariables['CI'] = ''
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) { throw 'A50 independent web server process could not be started.' }
    }
    catch {
        $process.Dispose()
        throw
    }
    return [pscustomobject]@{
        Process = $process
        StdoutTask = $process.StandardOutput.ReadToEndAsync()
        StderrTask = $process.StandardError.ReadToEndAsync()
    }
}

function Invoke-A50Process {
    param(
        [Parameter(Mandatory = $true)][string]$FileName,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][ValidateRange(1, 3600)][int]$TimeoutSeconds,
        [hashtable]$Environment
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FileName
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-OpsNativeArgument $_ }) -join ' ')
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    if ($null -ne $Environment) {
        foreach ($name in $Environment.Keys) {
            $startInfo.EnvironmentVariables[[string]$name] = [string]$Environment[$name]
        }
    }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) { throw 'A50 native process could not be started.' }
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

function Add-A50PlaywrightSpecs {
    param(
        [Parameter(Mandatory = $true)]$Suites,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][System.Collections.ArrayList]$Collector
    )

    foreach ($suite in @($Suites)) {
        if ($null -eq $suite) { continue }
        if ($null -ne $suite.PSObject.Properties['specs']) {
            foreach ($spec in @($suite.specs)) { [void]$Collector.Add($spec) }
        }
        if ($null -ne $suite.PSObject.Properties['suites']) {
            Add-A50PlaywrightSpecs -Suites $suite.suites -Collector $Collector
        }
    }
}

function Assert-A50PlaywrightReport {
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'A50 Playwright JSON report is missing.' }
    $report = [System.IO.File]::ReadAllText($Path) | ConvertFrom-Json -ErrorAction Stop
    if (-not [bool]$report.config.forbidOnly -or [int]$report.config.workers -ne 2 -or
        @($report.config.projects).Count -ne 2 -or @($report.errors).Count -ne 0 -or
        [int]$report.stats.expected -ne 18 -or [int]$report.stats.unexpected -ne 0 -or
        [int]$report.stats.flaky -ne 0 -or [int]$report.stats.skipped -ne 0 -or
        [int64]$report.stats.duration -le 0) {
        throw 'A50 Playwright config/stats contract is invalid.'
    }
    $projectNames = @()
    foreach ($project in @($report.config.projects)) {
        $testDir = ([string]$project.testDir).Replace('\', '/')
        if ([int]$project.retries -ne 0 -or [int]$project.repeatEach -ne 1 -or
            [string]::IsNullOrWhiteSpace([string]$project.id) -or
            -not $testDir.EndsWith('/web/e2e', [System.StringComparison]::Ordinal)) {
            throw 'A50 Playwright project contract is invalid.'
        }
        $projectNames += [string]$project.name
    }
    if (($projectNames -join "`n") -cne ($requiredProjects -join "`n")) {
        throw 'A50 Playwright projects are not the exact desktop/mobile pair.'
    }

    $specs = [System.Collections.ArrayList]::new()
    Add-A50PlaywrightSpecs -Suites $report.suites -Collector $specs
    if ($specs.Count -ne 18) { throw "A50 Playwright report has $($specs.Count) specs, want 18." }
    $expectedTitles = @{}
    foreach ($route in $requiredRoutes) { $expectedTitles[[string]$route.title] = $true }
    $seenCombinations = @{}
    $titleCounts = @{}
    $desktop = 0
    $mobile = 0
    $retriesObserved = 0
    foreach ($spec in $specs) {
        $title = [string]$spec.title
        $file = ([string]$spec.file).Replace('\', '/')
        $tagCount = if ($null -ne $spec.PSObject.Properties['tags']) { @($spec.tags).Count } else { 0 }
        if (-not $expectedTitles.ContainsKey($title) -or
            -not [bool]$spec.ok -or [string]::IsNullOrWhiteSpace([string]$spec.id) -or
            [int]$spec.line -le 0 -or [int]$spec.column -le 0 -or
            [System.IO.Path]::GetFileName($file) -cne 'statistics-states.spec.ts' -or
            @($spec.tests).Count -ne 1 -or $tagCount -ne 0) {
            throw "A50 Playwright spec is invalid: $title"
        }
        if (-not $titleCounts.ContainsKey($title)) { $titleCounts[$title] = 0 }
        $titleCounts[$title] = [int]$titleCounts[$title] + 1
        foreach ($test in @($spec.tests)) {
            $project = [string]$test.projectName
            if ([string]$test.expectedStatus -cne 'passed' -or [string]$test.status -cne 'expected' -or
                @($test.annotations).Count -ne 0 -or @($test.results).Count -ne 1 -or
                $project -notin $requiredProjects -or
                [string]::IsNullOrWhiteSpace([string]$test.projectId)) {
                throw "A50 Playwright test is invalid: $title/$project"
            }
            $combination = "$title`0$project"
            if ($seenCombinations.ContainsKey($combination)) {
                throw "A50 Playwright route/project combination is duplicated: $title/$project"
            }
            $seenCombinations[$combination] = $true
            $result = @($test.results)[0]
            $resultHasError = $null -ne $result.PSObject.Properties['error'] -and $null -ne $result.error
            if ([string]$result.status -cne 'passed' -or [int64]$result.duration -le 0 -or
                [int]$result.retry -ne 0 -or $resultHasError -or @($result.errors).Count -ne 0 -or
                @($result.stdout).Count -ne 0 -or @($result.stderr).Count -ne 0 -or
                @($result.attachments).Count -ne 0 -or @($result.annotations).Count -ne 0) {
                throw "A50 Playwright result is invalid: $title/$project"
            }
            if ($project -ceq 'chromium-desktop') { $desktop++ } else { $mobile++ }
            $retriesObserved += [int]$result.retry
        }
    }
    foreach ($route in $requiredRoutes) {
        if (-not $titleCounts.ContainsKey([string]$route.title) -or [int]$titleCounts[[string]$route.title] -ne 2) {
            throw "A50 Playwright route does not have both projects: $($route.title)"
        }
    }
    if ($seenCombinations.Count -ne 18 -or $desktop -ne 9 -or $mobile -ne 9 -or $retriesObserved -ne 0) {
        throw 'A50 Playwright route/project matrix is incomplete.'
    }
    return [ordered]@{
        expected = 18
        desktop = $desktop
        mobile = $mobile
        unexpected = 0
        flaky = 0
        skipped = 0
        retries_observed = $retriesObserved
    }
}

function Remove-A50TemporaryDirectory {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$TemporaryRoot
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $fullRoot = [System.IO.Path]::GetFullPath($TemporaryRoot).TrimEnd(
        [char[]]@([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    )
    $prefix = $fullRoot + [System.IO.Path]::DirectorySeparatorChar
    $leaf = [System.IO.Path]::GetFileName($fullPath)
    if (-not $fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase) -or
        $leaf -notmatch '^new-api-pilot-a50-[0-9a-f]+-[0-9a-f]{12}-(results|html)$') {
        throw 'A50 temporary directory escaped its bounded root.'
    }
    if ([System.IO.Directory]::Exists($fullPath)) { [System.IO.Directory]::Delete($fullPath, $true) }
    return -not [System.IO.Directory]::Exists($fullPath)
}

function Write-A50SecretScan {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $pattern = '(?i)(?:(?:authorization|access[_-]?token|webhook(?:[_-]?url)?|password|cookie|database[_-]?dsn|session[_-]?secret|encryption[_-]?key)\s*["'']?\s*[:=]\s*["'']?[^\s"'']{6,}|[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)'
    $rawDSNPattern = '(?i)\b[A-Za-z0-9_.-]{1,64}:[^@\s]{0,256}@tcp\([A-Za-z0-9_.:-]{1,255}\)/[A-Za-z0-9_.-]{1,128}'
    $urlCredentialPattern = '(?i)https?://[^\s"''<>]{0,1000}(?:access[_-]?token|token|secret|key|signature)=[^&\s"''<>]{1,512}'
    $htmlSecretAllowlist = @(
        'password:u,rawPassword:c,signed:f,encryptionStrength:r,checkPasswordOnly:o}){super({start(){Object.assign(this,{ready:new',
        'password:I2(u,c),signed:f,strength:r-1,pending:new',
        'password:A,strength:x,resolveReady:T,ready:D}=y;A?(await',
        'password:u,rawPassword:c,encryptionStrength:f}){let',
        'password:I2(u,c),strength:f-1,pending:new',
        'password:y,strength:A,resolveReady:x,ready:T}=v;let',
        'password=null;const',
        'password:u,passwordVerification:c,checkPasswordOnly:f}){super({start(){Object.assign(this,{password:u,passwordVerification:c}),q2(this,u)},transform(r,o){const',
        'password=null,v.at(-1)!=h.passwordVerification)throw',
        'password:u,passwordVerification:c}){super({start(){Object.assign(this,{password:u,passwordVerification:c}),q2(this,u)},transform(f,r){const',
        'password=null;const',
        'password:H,rawPassword:j,zipCrypto:st,encryptionStrength:A&&A.strength,signed:ie(o,r,z8)&&!Y,passwordVerification:st&&(R?p>>>8&255:q>>>24&255),outputSize:E,signature:q,compressed:T!=0&&!Y,encrypted:o.encrypted&&!Y,useWebWorkers:ie(o,r,Y8),useCompressionStream:ie(o,r,L8),transferStreams:ie(o,r,G8),checkPasswordOnly:ht},config:D,streamOptions:{signal:$,size:M,onstart:L,onprogress:W,onend:et}};tt&&await',
        'PASSWORD:lr,ERR_INVALID_SIGNATURE:ar,ERR_INVALID_UNCOMPRESSED_SIZE:sr,ERR_ITERATOR_COMPLETED_TOO_SOON:$2,ERR_LOCAL_FILE_HEADER_NOT_FOUND:Eh,ERR_OVERLAPPING_ENTRY:Sh,ERR_SPLIT_ZIP_FILE:Pf,ERR_UNSUPPORTED_COMPRESSION:_f,ERR_UNSUPPORTED_ENCRYPTION:bh,GenericReader:ah,GenericWriter:ih,Reader:sc,SplitDataReader:lh,SplitDataWriter:kf,TextWriter:a8,ZipReader:I8,configure:H2,initStream:Oi,readUint8Array:_t},Symbol.toStringTag,{value:',
        'password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function'
    )
    if ($htmlSecretAllowlist.Count -ne 14) { throw 'A50 HTML reporter secret allowlist must contain exactly fourteen contexts.' }
    $matches = 0
    foreach ($relative in $secretScanTargets) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "A50 secret scan target is missing: $relative" }
        $payload = [System.IO.File]::ReadAllText($path)
        $forbidden = $false
        if ($relative -eq 'a50-playwright-report.html') {
            $allowedCounts = [System.Collections.Generic.Dictionary[string,int]]::new([System.StringComparer]::Ordinal)
            foreach ($allowed in $htmlSecretAllowlist) {
                if ($allowedCounts.ContainsKey($allowed)) { $allowedCounts[$allowed] = $allowedCounts[$allowed] + 1 }
                else { $allowedCounts.Add($allowed, 1) }
            }
            $seenCounts = [System.Collections.Generic.Dictionary[string,int]]::new([System.StringComparer]::Ordinal)
            $htmlMatches = [regex]::Matches($payload, $pattern)
            if ($htmlMatches.Count -ne $htmlSecretAllowlist.Count) { $forbidden = $true }
            if (-not $forbidden) {
                foreach ($match in $htmlMatches) {
                    $value = $match.Value
                    if (-not $allowedCounts.ContainsKey($value)) { $forbidden = $true; break }
                    $seen = if ($seenCounts.ContainsKey($value)) { $seenCounts[$value] } else { 0 }
                    if ($seen -ge $allowedCounts[$value]) { $forbidden = $true; break }
                    $seenCounts[$value] = $seen + 1
                }
            }
            if (-not $forbidden) {
                foreach ($entry in $allowedCounts.GetEnumerator()) {
                    $seen = if ($seenCounts.ContainsKey($entry.Key)) { $seenCounts[$entry.Key] } else { 0 }
                    if ($seen -ne $entry.Value) { $forbidden = $true; break }
                }
            }
            if ([regex]::IsMatch($payload, $rawDSNPattern) -or [regex]::IsMatch($payload, $urlCredentialPattern)) {
                $forbidden = $true
            }
        }
        elseif ([regex]::IsMatch($payload, $pattern) -or [regex]::IsMatch($payload, $rawDSNPattern) -or
            [regex]::IsMatch($payload, $urlCredentialPattern)) {
            $forbidden = $true
        }
        if ($forbidden) { $matches++ }
    }
    if ($matches -ne 0) { throw 'A50 evidence contains a forbidden credential or secret pattern.' }
    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        evidence_class = $EvidenceClass
        status = 'passed'
        files = @($secretScanTargets)
        matches = $matches
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a50-secret-scan.json') -Payload ($report | ConvertTo-Json -Depth 4)
}

function Write-A50ArtifactInventory {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$EvidenceClass
    )

    $files = @()
    foreach ($relative in $requiredArtifacts) {
        $path = Join-Path $Directory $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "A50 required artifact is missing: $relative" }
        $info = Get-Item -LiteralPath $path
        $files += [ordered]@{
            path = $relative
            size_bytes = [int64]$info.Length
            sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    if ($files.Count -ne 15) { throw 'A50 inventory must hash exactly fifteen payload artifacts.' }
    $inventory = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        evidence_class = $EvidenceClass
        files = $files
    }
    Write-OpsUtf8NoBom -Path (Join-Path $Directory 'a50-artifacts.json') -Payload ($inventory | ConvertTo-Json -Depth 6)
}

$failure = $null
$cleanupFailed = $false
$evidenceDirectory = $null
$evidenceClass = $null
$serverHandle = $null
$serverPID = 0
$serverStarted = $false
$serverStopped = $false
$pidTreeStopped = $false
$portReleased = $false
$serverLogsWritten = $false
$testOutputCreated = $false
$testOutputRemoved = $false
$htmlOutputCreated = $false
$htmlOutputRemoved = $false
$port = 0
$temporaryRoot = [System.IO.Path]::GetTempPath()
$runToken = ('{0:x}-{1}' -f $PID, ([guid]::NewGuid().ToString('N').Substring(0, 12)))
$testOutputDirectory = Join-Path $temporaryRoot "new-api-pilot-a50-$runToken-results"
$htmlOutputDirectory = Join-Path $temporaryRoot "new-api-pilot-a50-$runToken-html"

try {
    if ($env:ACCEPTANCE_ID -cne $acceptanceID -or $env:ACCEPTANCE_EVIDENCE_CLASS -notin @('formal', 'development') -or
        [string]::IsNullOrWhiteSpace($env:ACCEPTANCE_RUNNER_EXE)) {
        throw 'A50 evidence must be invoked by the canonical acceptance wrapper.'
    }
    if ([string]::IsNullOrWhiteSpace($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not [System.IO.Path]::IsPathRooted($env:ACCEPTANCE_EVIDENCE_DIR) -or
        -not (Test-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR -PathType Container)) {
        throw 'ACCEPTANCE_EVIDENCE_DIR must be an existing absolute directory.'
    }
    $evidenceClass = [string]$env:ACCEPTANCE_EVIDENCE_CLASS
    $repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..')).Path
    $webRoot = (Resolve-Path -LiteralPath (Join-Path $repositoryRoot 'web')).Path
    $evidenceDirectory = (Resolve-Path -LiteralPath $env:ACCEPTANCE_EVIDENCE_DIR).Path
    $relativeEvidence = Get-OpsRepositoryRelativePath -RepositoryRoot $repositoryRoot -CandidatePath $evidenceDirectory
    $expectedPrefix = if ($evidenceClass -ceq 'formal') { 'artifacts/acceptance/A50/' } else { 'artifacts/smoke/A50/' }
    if (-not $relativeEvidence.StartsWith($expectedPrefix, [System.StringComparison]::Ordinal)) {
        throw "A50 $evidenceClass evidence directory is not under $expectedPrefix"
    }
    if ([System.IO.Directory]::Exists($testOutputDirectory) -or [System.IO.Directory]::Exists($htmlOutputDirectory)) {
        throw 'A50 unique temporary output directory already exists.'
    }

    $bunShim = Get-Command bun.cmd -CommandType Application -ErrorAction Stop
    $bunExecutable = Join-Path (Split-Path -Parent $bunShim.Source) 'node_modules\bun\bin\bun.exe'
    if (-not (Test-Path -LiteralPath $bunExecutable -PathType Leaf)) {
        throw 'A50 could not resolve the native bun.exe behind the command shim.'
    }
    $bunVersionResult = Invoke-OpsProcess -FileName $bunExecutable -Arguments @('--version') -TimeoutSeconds 30
    $playwrightCLI = Join-Path $webRoot 'node_modules\@playwright\test\cli.js'
    if (-not (Test-Path -LiteralPath $playwrightCLI -PathType Leaf)) {
        throw 'A50 local Playwright CLI is missing.'
    }
    $playwrightVersionResult = Invoke-A50Process -FileName $bunExecutable -Arguments @($playwrightCLI, '--version') -WorkingDirectory $webRoot -TimeoutSeconds 30
    if ($bunVersionResult.TimedOut -or $bunVersionResult.ExitCode -ne 0 -or $bunVersionResult.Stdout.Trim() -cne '1.3.13' -or
        $playwrightVersionResult.TimedOut -or $playwrightVersionResult.ExitCode -ne 0 -or
        $playwrightVersionResult.Stdout.Trim() -cne 'Version 1.61.1') {
        throw 'A50 Bun/Playwright toolchain version differs from the approved contract.'
    }
    $bunVersion = $bunVersionResult.Stdout.Trim()
    $playwrightVersion = $playwrightVersionResult.Stdout.Trim().Substring('Version '.Length)

    $specPath = Join-Path $webRoot 'e2e\statistics-states.spec.ts'
    $packagePath = Join-Path $webRoot 'package.json'
    $playwrightConfigPath = Join-Path $webRoot 'playwright.config.ts'
    $localePath = Join-Path $webRoot 'src\i18n\locales\zh-CN.json'
    $bunLockPath = Join-Path $webRoot 'bun.lock'
    $specSHA = (Get-FileHash -LiteralPath $specPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $packageSHA = (Get-FileHash -LiteralPath $packagePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $playwrightConfigSHA = (Get-FileHash -LiteralPath $playwrightConfigPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($specSHA -cne $approvedSpecSHA -or $packageSHA -cne $approvedPackageSHA -or
        $playwrightConfigSHA -cne $approvedPlaywrightConfigSHA) {
        throw 'A50 approved spec/package/Playwright configuration SHA changed.'
    }
    $specSource = [System.IO.File]::ReadAllText($specPath)
    $hasSkip = [regex]::IsMatch($specSource, '\btest(?:\.describe)?\.skip\s*\(')
    $hasOnly = [regex]::IsMatch($specSource, '\btest(?:\.describe)?\.only\s*\(')
    $hasFixme = [regex]::IsMatch($specSource, '\btest\.fixme\s*\(')
    if ($hasSkip -or $hasOnly -or $hasFixme) { throw 'A50 spec contains skip/fixme/only.' }
    $languagePattern = '(?i)changeLanguage\s*\(|LanguageDetector|i18next-browser-languagedetector|语言切换|切换语言'
    $hasLanguageSwitcher = $false
    foreach ($sourceFile in Get-ChildItem -LiteralPath (Join-Path $webRoot 'src') -Recurse -File | Where-Object { $_.Extension -in @('.ts', '.tsx') }) {
        if ([regex]::IsMatch([System.IO.File]::ReadAllText($sourceFile.FullName), $languagePattern)) {
            $hasLanguageSwitcher = $true
            break
        }
    }
    if ($hasLanguageSwitcher) { throw 'A50 frontend contains a language detector or switching entry.' }
    $localeFiles = @(Get-ChildItem -LiteralPath (Join-Path $webRoot 'src\i18n\locales') -File -Filter '*.json' | Sort-Object Name | ForEach-Object { $_.BaseName })
    if (($localeFiles -join "`n") -cne 'zh-CN') { throw 'A50 product locale files are not exactly zh-CN.' }

    $checkResult = Invoke-A50Process -FileName $bunExecutable -Arguments @('run', 'check') -WorkingDirectory $webRoot -TimeoutSeconds 900
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-check.stdout.log') -Payload $checkResult.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-check.stderr.log') -Payload $checkResult.Stderr
    if (-not [string]::IsNullOrEmpty($checkResult.Stdout)) { [Console]::Out.Write($checkResult.Stdout) }
    if (-not [string]::IsNullOrEmpty($checkResult.Stderr)) { [Console]::Error.Write($checkResult.Stderr) }
    if ($checkResult.TimedOut -or $checkResult.ExitCode -ne 0) { throw 'A50 bun run check failed.' }
    $i18nMatch = [regex]::Match($checkResult.Stdout, '(?m)i18n check passed: 1 locale, ([0-9]+) keys')
    if (-not $i18nMatch.Success) { throw 'A50 bun check did not prove the single zh-CN locale.' }
    $translationKeys = [int]$i18nMatch.Groups[1].Value
    if ($translationKeys -lt 1000) { throw 'A50 zh-CN translation key count is implausibly small.' }
    $checkSummary = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        status = 'passed'
        command = $checkCommand
        exit_code = 0
        steps = $checkSteps
        routes_generate = $true
        typecheck = $true
        lint = $true
        format_check = $true
        i18n_check = $true
        build_app = $true
        locales = @('zh-CN')
        locale_count = 1
        translation_keys = $translationKeys
        stdout_path = 'a50-check.stdout.log'
        stderr_path = 'a50-check.stderr.log'
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-check-summary.json') -Payload ($checkSummary | ConvertTo-Json -Depth 5)

    $port = Get-A50FreePort
    if ($port -eq 5173 -or (Test-A50PortOpen -HostName '127.0.0.1' -Port $port)) {
        throw 'A50 selected port is not isolated.'
    }
    $baseURL = "http://127.0.0.1:$port"
    $serverArguments = @('run', 'dev', '--', '--host', '127.0.0.1', '--port', [string]$port)
    $serverHandle = Start-A50ServerProcess -BunExecutable $bunExecutable -Arguments $serverArguments -WorkingDirectory $webRoot
    $serverPID = $serverHandle.Process.Id
    $serverStarted = $true
    Wait-A50ServerReady -Process $serverHandle.Process -BaseURL $baseURL -TimeoutSeconds 120

    $playwrightArguments = @(
        'x', '--no-install', 'playwright', 'test', 'e2e/statistics-states.spec.ts',
        '--workers=2', '--retries=0', '--forbid-only',
        '--project=chromium-desktop', '--project=chromium-mobile',
        '--reporter=json,html', '--output', $testOutputDirectory
    )
    $playwrightEnvironment = @{
        PLAYWRIGHT_BASE_URL = $baseURL
        PLAYWRIGHT_JSON_OUTPUT_FILE = (Join-Path $evidenceDirectory 'a50-playwright.json')
        PLAYWRIGHT_HTML_OUTPUT_DIR = $htmlOutputDirectory
        PLAYWRIGHT_HTML_OPEN = 'never'
        CI = ''
    }
    $playwrightResult = Invoke-A50Process -FileName $bunExecutable -Arguments $playwrightArguments -WorkingDirectory $webRoot -TimeoutSeconds 900 -Environment $playwrightEnvironment
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-playwright.stdout.log') -Payload $playwrightResult.Stdout
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-playwright.stderr.log') -Payload $playwrightResult.Stderr
    if (-not [string]::IsNullOrEmpty($playwrightResult.Stdout)) { [Console]::Out.Write($playwrightResult.Stdout) }
    if (-not [string]::IsNullOrEmpty($playwrightResult.Stderr)) { [Console]::Error.Write($playwrightResult.Stderr) }
    if ($playwrightResult.TimedOut -or $playwrightResult.ExitCode -ne 0) { throw 'A50 Playwright execution failed.' }
    $facts = Assert-A50PlaywrightReport -Path (Join-Path $evidenceDirectory 'a50-playwright.json')
    $htmlIndex = Join-Path $htmlOutputDirectory 'index.html'
    if (-not (Test-Path -LiteralPath $htmlIndex -PathType Leaf) -or (Get-Item -LiteralPath $htmlIndex).Length -lt 100000) {
        throw 'A50 standalone Playwright HTML report was not produced.'
    }
    Copy-Item -LiteralPath $htmlIndex -Destination (Join-Path $evidenceDirectory 'a50-playwright-report.html') -Force

    $gitState = Get-OpsGitState -RepositoryRoot $repositoryRoot
    $commandReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        evidence_class = $evidenceClass
        working_directory = 'web'
        check = $checkCommand
        server = @('bun', 'run', 'dev', '--', '--host', '127.0.0.1', '--port', [string]$port)
        playwright = @('bun') + $playwrightArguments
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-command.json') -Payload ($commandReport | ConvertTo-Json -Depth 6)
    $environmentReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        evidence_class = $evidenceClass
        commit = $gitState.Commit
        worktree_dirty = $gitState.WorktreeDirty
        os = [System.Environment]::OSVersion.Platform.ToString()
        architecture = [string]$env:PROCESSOR_ARCHITECTURE
        bun_version = $bunVersion
        playwright_version = $playwrightVersion
        base_url = $baseURL
        port = $port
        server_pid = $serverPID
        workers = 2
        retries = 0
        projects = @($requiredProjects)
        desktop_viewport = [ordered]@{ width = 1440; height = 900 }
        mobile_viewport = [ordered]@{ width = 390; height = 844 }
        shared_port_5173_used = $false
        test_output_directory = [System.IO.Path]::GetFullPath($testOutputDirectory)
        html_output_directory = [System.IO.Path]::GetFullPath($htmlOutputDirectory)
        spec_path = 'web/e2e/statistics-states.spec.ts'
        spec_sha256 = $specSHA
        locale_path = 'web/src/i18n/locales/zh-CN.json'
        locale_sha256 = (Get-FileHash -LiteralPath $localePath -Algorithm SHA256).Hash.ToLowerInvariant()
        bun_lock_path = 'web/bun.lock'
        bun_lock_sha256 = (Get-FileHash -LiteralPath $bunLockPath -Algorithm SHA256).Hash.ToLowerInvariant()
        package_path = 'web/package.json'
        package_sha256 = $packageSHA
        playwright_config_path = 'web/playwright.config.ts'
        playwright_config_sha256 = $playwrightConfigSHA
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-environment.json') -Payload ($environmentReport | ConvertTo-Json -Depth 7)

    $fixtureFullPath = Join-Path $repositoryRoot $fixturePath
    $manifestFullPath = Join-Path $repositoryRoot $fixtureManifestPath
    $actualFixtureSHA = (Get-FileHash -LiteralPath $fixtureFullPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualFixtureSHA -cne $fixtureSHA256) { throw 'A50 F03 fixture checksum differs from the approved contract.' }
    $manifestText = [System.IO.File]::ReadAllText($manifestFullPath)
    if ($manifestText -notmatch "(?m)^$fixtureSHA256  $([regex]::Escape($fixturePath))$") {
        throw 'A50 F03 checksum is not bound by the fixture manifest.'
    }
    $fixtureReport = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        fixture_id = 'F03'
        path = $fixturePath
        sha256 = $actualFixtureSHA
        manifest_path = $fixtureManifestPath
        manifest_sha256 = (Get-FileHash -LiteralPath $manifestFullPath -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-fixture.json') -Payload ($fixtureReport | ConvertTo-Json -Depth 4)

    $report = [ordered]@{
        schema_version = 1
        acceptance_id = 'A50'
        status = 'passed'
        spec_path = 'web/e2e/statistics-states.spec.ts'
        spec_sha256 = $specSHA
        routes = @($requiredRoutes)
        projects = @($requiredProjects)
        expected_tests = [int]$facts.expected
        desktop_tests = [int]$facts.desktop
        mobile_tests = [int]$facts.mobile
        unexpected = [int]$facts.unexpected
        flaky = [int]$facts.flaky
        skipped = [int]$facts.skipped
        retries_observed = [int]$facts.retries_observed
        source_guards = [ordered]@{
            no_skip = -not $hasSkip
            no_only = -not $hasOnly
            no_fixme = -not $hasFixme
            no_language_switcher = -not $hasLanguageSwitcher
        }
        state_checks = [ordered]@{
            complete_zero_visible = $true
            partial_known_data_preserved = $true
            missing_reason_visible = $true
            unavailable_reason_visible = $true
            paused_data_and_reason_visible = $true
            refresh_retains_previous_data = $true
            url_reload_restores_search = $true
            responsive_no_horizontal_overflow = $true
        }
        i18n = [ordered]@{
            check_passed = $true
            locales = @('zh-CN')
            locale_count = 1
            translation_keys = $translationKeys
            extra_locale_absent = $true
        }
        artifacts = [ordered]@{ playwright_json = $true; standalone_html = $true }
        independent_port = ($port -ne 5173)
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-report.json') -Payload ($report | ConvertTo-Json -Depth 7)
}
catch {
    $failure = if ($_.InvocationInfo.ScriptLineNumber -gt 0) {
        '{0} (run-a50.ps1 line {1})' -f $_.Exception.Message, $_.InvocationInfo.ScriptLineNumber
    }
    else {
        $_.Exception.Message
    }
}
finally {
    $serverStdout = ''
    $serverStderr = ''
    if ($null -ne $serverHandle) {
        try {
            if (-not $serverHandle.Process.HasExited) {
                $kill = Invoke-OpsProcess -FileName 'taskkill.exe' -Arguments @('/PID', [string]$serverPID, '/T', '/F') -TimeoutSeconds 30
                if (-not $kill.TimedOut -and $kill.ExitCode -eq 0) { $pidTreeStopped = $true }
            }
            else {
                $pidTreeStopped = $true
            }
            try { $serverHandle.Process.WaitForExit(10000) | Out-Null } catch {}
            $serverStopped = $serverHandle.Process.HasExited
            if ($serverStopped) { $pidTreeStopped = $true }
            try { $serverStdout = $serverHandle.StdoutTask.GetAwaiter().GetResult() } catch {}
            try { $serverStderr = $serverHandle.StderrTask.GetAwaiter().GetResult() } catch {}
        }
        finally {
            $serverHandle.Process.Dispose()
        }
    }
    if ($null -ne $evidenceDirectory) {
        try {
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-server.stdout.log') -Payload $serverStdout
            Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-server.stderr.log') -Payload $serverStderr
            $serverLogsWritten = $true
        }
        catch {
            $serverLogsWritten = $false
        }
    }
    if ($port -gt 0) {
        $releaseDeadline = [DateTimeOffset]::UtcNow.AddSeconds(20)
        while ([DateTimeOffset]::UtcNow -lt $releaseDeadline -and (Test-A50PortOpen -HostName '127.0.0.1' -Port $port)) {
            Start-Sleep -Milliseconds 250
        }
        $portReleased = -not (Test-A50PortOpen -HostName '127.0.0.1' -Port $port)
    }
    $testOutputCreated = [System.IO.Directory]::Exists($testOutputDirectory)
    $htmlOutputCreated = [System.IO.Directory]::Exists($htmlOutputDirectory)
    try { $testOutputRemoved = Remove-A50TemporaryDirectory -Path $testOutputDirectory -TemporaryRoot $temporaryRoot } catch { $testOutputRemoved = $false }
    try { $htmlOutputRemoved = Remove-A50TemporaryDirectory -Path $htmlOutputDirectory -TemporaryRoot $temporaryRoot } catch { $htmlOutputRemoved = $false }
    $residualPIDs = @()
    if ($null -ne $serverHandle -and -not $serverStopped) { $residualPIDs += $serverPID }
    $residualPorts = @()
    if ($port -gt 0 -and -not $portReleased) { $residualPorts += $port }
    $residualDirectories = @()
    foreach ($directory in @($testOutputDirectory, $htmlOutputDirectory)) {
        if ([System.IO.Directory]::Exists($directory)) { $residualDirectories += $directory }
    }
    $cleanupPassed = $serverStarted -and $serverStopped -and $pidTreeStopped -and $portReleased -and
        $testOutputCreated -and $testOutputRemoved -and $htmlOutputCreated -and $htmlOutputRemoved -and
        $serverLogsWritten -and $residualPIDs.Count -eq 0 -and $residualPorts.Count -eq 0 -and $residualDirectories.Count -eq 0
    $cleanupFailed = -not $cleanupPassed
    if ($null -ne $evidenceDirectory -and $null -ne $evidenceClass) {
        $cleanupReport = [ordered]@{
            schema_version = 1
            acceptance_id = 'A50'
            evidence_class = $evidenceClass
            passed = $cleanupPassed
            server_pid = $serverPID
            port = $port
            server_started = $serverStarted
            server_stopped = $serverStopped
            pid_tree_stopped = $pidTreeStopped
            port_released = $portReleased
            test_output_created = $testOutputCreated
            test_output_removed = $testOutputRemoved
            html_output_created = $htmlOutputCreated
            html_output_removed = $htmlOutputRemoved
            server_logs_written = $serverLogsWritten
            shared_port_5173_touched = $false
            residuals = [ordered]@{
                pids = @($residualPIDs)
                ports = @($residualPorts)
                directories = @($residualDirectories)
            }
        }
        try { Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-cleanup.json') -Payload ($cleanupReport | ConvertTo-Json -Depth 6) }
        catch { $cleanupFailed = $true }
    }
}

if ($null -ne $failure) {
    [Console]::Error.WriteLine($failure)
    exit 1
}
if ($cleanupFailed) {
    [Console]::Error.WriteLine('A50 independent server or temporary output cleanup failed.')
    exit 1
}
try {
    Write-A50SecretScan -Directory $evidenceDirectory -EvidenceClass $evidenceClass
    Write-A50ArtifactInventory -Directory $evidenceDirectory -EvidenceClass $evidenceClass
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
exit 0
