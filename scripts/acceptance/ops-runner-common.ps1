function ConvertTo-OpsNativeArgument {
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

function Invoke-OpsProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FileName,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][ValidateRange(1, 20000)][int]$TimeoutSeconds,
        [hashtable]$Environment
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FileName
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-OpsNativeArgument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
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
        if (-not $process.Start()) { throw 'native process could not be started' }
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

function Invoke-OpsDocker {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [int]$TimeoutSeconds = 180
    )
    $result = Invoke-OpsProcess -FileName 'docker' -Arguments $Arguments -TimeoutSeconds $TimeoutSeconds
    if ($result.TimedOut) { throw 'a bounded Docker operation timed out' }
    if ($result.ExitCode -ne 0) { throw 'a Docker operation failed' }
    return $result
}

function Write-OpsUtf8NoBom {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload
    )
    $encoding = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($Path, $Payload, $encoding)
}

function Protect-OpsDiagnostic {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Payload)

    $protected = [regex]::Replace($Payload, '(?i)(DATABASE_DSN=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)((?:OLD|NEW)_ENCRYPTION_KEY=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)(ENCRYPTION_KEY=|SESSION_SECRET=)[^"\s]+', '$1[redacted]')
    $protected = [regex]::Replace($protected, '(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@', '$1[redacted]@')
    return [regex]::Replace($protected, '(?i)(\b(?:password|token|secret|dsn)=)[^&\s]+', '$1[redacted]')
}

function Get-OpsRepositoryRelativePath {
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryRoot,
        [Parameter(Mandatory = $true)][string]$CandidatePath
    )
    $normalizedRoot = [System.IO.Path]::GetFullPath($RepositoryRoot).TrimEnd(
        [char[]]@([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    )
    $normalizedCandidate = [System.IO.Path]::GetFullPath($CandidatePath).TrimEnd(
        [char[]]@([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    )
    if ($normalizedCandidate.Equals($normalizedRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'candidate path must be below the repository root'
    }
    $prefix = $normalizedRoot + [System.IO.Path]::DirectorySeparatorChar
    if (-not $normalizedCandidate.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'candidate path must stay inside the repository root'
    }
    return $normalizedCandidate.Substring($prefix.Length).Replace('\', '/')
}

function Get-OpsGitState {
    param([Parameter(Mandatory = $true)][string]$RepositoryRoot)

    $inside = Invoke-OpsProcess -FileName 'git' -Arguments @('-C', $RepositoryRoot, 'rev-parse', '--is-inside-work-tree') -TimeoutSeconds 30
    if ($inside.TimedOut -or $inside.ExitCode -ne 0 -or $inside.Stdout.Trim() -cne 'true') {
        throw 'repository Git metadata is unavailable'
    }
    $head = Invoke-OpsProcess -FileName 'git' -Arguments @('-C', $RepositoryRoot, 'rev-parse', '--verify', 'HEAD') -TimeoutSeconds 30
    if (-not $head.TimedOut -and $head.ExitCode -eq 0 -and $head.Stdout.Trim() -match '^[0-9a-f]{40,64}$') {
        $commit = $head.Stdout.Trim()
    }
    else {
        $symbolic = Invoke-OpsProcess -FileName 'git' -Arguments @('-C', $RepositoryRoot, 'symbolic-ref', '-q', 'HEAD') -TimeoutSeconds 30
        $count = Invoke-OpsProcess -FileName 'git' -Arguments @('-C', $RepositoryRoot, 'rev-list', '--all', '--count') -TimeoutSeconds 30
        if ($symbolic.TimedOut -or $symbolic.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($symbolic.Stdout) -or
            $count.TimedOut -or $count.ExitCode -ne 0 -or $count.Stdout.Trim() -cne '0') {
            throw 'repository HEAD could not be classified safely'
        }
        $commit = 'unborn'
    }
    $status = Invoke-OpsProcess -FileName 'git' -Arguments @('-C', $RepositoryRoot, 'status', '--porcelain') -TimeoutSeconds 30
    if ($status.TimedOut -or $status.ExitCode -ne 0) { throw 'repository worktree state is unavailable' }
    return [pscustomobject]@{
        Commit = $commit
        WorktreeDirty = -not [string]::IsNullOrWhiteSpace($status.Stdout)
    }
}

function Wait-OpsContainer {
    param(
        [Parameter(Mandatory = $true)][string]$Container,
        [Parameter(Mandatory = $true)][ValidateRange(1, 20000)][int]$TimeoutSeconds
    )
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    while ($true) {
        $inspect = Invoke-OpsProcess -FileName 'docker' -Arguments @('inspect', '--format', '{{.State.Status}}|{{.State.ExitCode}}', $Container) -TimeoutSeconds 15
        if (-not $inspect.TimedOut -and $inspect.ExitCode -eq 0) {
            $fields = $inspect.Stdout.Trim().Split('|')
            if ($fields.Count -eq 2 -and $fields[0] -in @('exited', 'dead')) { return [int]$fields[1] }
        }
        if ([DateTimeOffset]::UtcNow -ge $deadline) { return -1 }
        Start-Sleep -Milliseconds 500
    }
}

function Remove-OpsDockerResource {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $result = Invoke-OpsProcess -FileName 'docker' -Arguments $Arguments -TimeoutSeconds 90
    return (-not $result.TimedOut -and $result.ExitCode -eq 0)
}

function Get-OpsResidualSweep {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $result = Invoke-OpsProcess -FileName 'docker' -Arguments $Arguments -TimeoutSeconds 60
    $items = @()
    if (-not $result.TimedOut -and $result.ExitCode -eq 0) {
        $items = @(($result.Stdout -split "`r?`n") | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }
    return [pscustomobject]@{
        Succeeded = (-not $result.TimedOut -and $result.ExitCode -eq 0)
        Items = $items
    }
}

function New-OpsBase64Key {
    $bytes = [byte[]]::new(32)
    $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    return [Convert]::ToBase64String($bytes)
}

function Get-OpsKeyFingerprint {
    param([Parameter(Mandatory = $true)][string]$Base64Key)
    $bytes = [Convert]::FromBase64String($Base64Key)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { return ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}
