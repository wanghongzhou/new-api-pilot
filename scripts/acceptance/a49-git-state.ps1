function Invoke-A49GitProcess {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [ValidateRange(1, 120)][int]$TimeoutSeconds = 30
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = 'git'
    $startInfo.Arguments = (($Arguments | ForEach-Object { ConvertTo-NativeArgument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            throw 'Git CLI could not be started.'
        }
    }
    catch {
        $process.Dispose()
        throw 'A49 Git metadata process could not start.'
    }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $finished = $process.WaitForExit($TimeoutSeconds * 1000)
    if (-not $finished) {
        try { $process.Kill() } catch {}
        $process.WaitForExit()
    }
    $stdout = ''
    try { $stdout = $stdoutTask.GetAwaiter().GetResult() } catch {}
    try { [void]$stderrTask.GetAwaiter().GetResult() } catch {}
    $exitCode = if ($finished) { $process.ExitCode } else { -1 }
    $process.Dispose()
    return [pscustomobject]@{
        TimedOut = -not $finished
        ExitCode = $exitCode
        Stdout = $stdout
    }
}

function Get-A49GitState {
    param([Parameter(Mandatory = $true)][string]$RepositoryRoot)

    $inside = Invoke-A49GitProcess -Arguments @('-C', $RepositoryRoot, 'rev-parse', '--is-inside-work-tree')
    if ($inside.TimedOut -or $inside.ExitCode -ne 0 -or $inside.Stdout.Trim() -cne 'true') {
        throw 'A49 repository Git metadata is unavailable.'
    }

    $head = Invoke-A49GitProcess -Arguments @('-C', $RepositoryRoot, 'rev-parse', '--verify', 'HEAD')
    if (-not $head.TimedOut -and $head.ExitCode -eq 0 -and $head.Stdout.Trim() -match '^[0-9a-f]{40,64}$') {
        $commit = $head.Stdout.Trim()
    }
    else {
        $symbolic = Invoke-A49GitProcess -Arguments @('-C', $RepositoryRoot, 'symbolic-ref', '-q', 'HEAD')
        $count = Invoke-A49GitProcess -Arguments @('-C', $RepositoryRoot, 'rev-list', '--all', '--count')
        if ($symbolic.TimedOut -or $symbolic.ExitCode -ne 0 -or [string]::IsNullOrWhiteSpace($symbolic.Stdout) -or
            $count.TimedOut -or $count.ExitCode -ne 0 -or $count.Stdout.Trim() -cne '0') {
            throw 'A49 repository HEAD could not be classified safely.'
        }
        $commit = 'unborn'
    }

    $status = Invoke-A49GitProcess -Arguments @('-C', $RepositoryRoot, 'status', '--porcelain')
    if ($status.TimedOut -or $status.ExitCode -ne 0) {
        throw 'A49 repository worktree state is unavailable.'
    }
    return [pscustomobject]@{
        Commit = $commit
        WorktreeDirty = -not [string]::IsNullOrWhiteSpace($status.Stdout)
    }
}
