[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function ConvertTo-NativeArgument {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)
    if ($Value.Length -gt 0 -and $Value -notmatch '[\s"]') { return $Value }
    return '"' + $Value.Replace('\', '\\').Replace('"', '\"') + '"'
}

. (Join-Path $PSScriptRoot 'a49-git-state.ps1')

$temporaryBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$testRoot = [System.IO.Path]::GetFullPath((Join-Path $temporaryBase ("new-api-pilot-a49-git-test-" + [guid]::NewGuid().ToString('N'))))
if (-not $testRoot.StartsWith($temporaryBase, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'A49 Git test root escaped the system temporary directory.'
}
try {
    $unborn = Join-Path $testRoot 'unborn'
    $normal = Join-Path $testRoot 'normal'
    $nonRepository = Join-Path $testRoot 'not-a-repository'
    [void](New-Item -ItemType Directory -Path $unborn, $normal, $nonRepository -Force)
    foreach ($repository in @($unborn, $normal)) {
        $init = Invoke-A49GitProcess -Arguments @('-C', $repository, 'init', '-q')
        if ($init.TimedOut -or $init.ExitCode -ne 0) { throw 'A49 Git test repository initialization failed.' }
    }

    $unbornState = Get-A49GitState -RepositoryRoot $unborn
    if ($unbornState.Commit -cne 'unborn') { throw 'A49 Git state did not classify an unborn repository.' }

    [System.IO.File]::WriteAllText((Join-Path $normal 'tracked.txt'), "tracked`n")
    $add = Invoke-A49GitProcess -Arguments @('-C', $normal, 'add', 'tracked.txt')
    $commit = Invoke-A49GitProcess -Arguments @(
        '-C', $normal, '-c', 'user.name=A49 Test', '-c', 'user.email=a49@example.invalid',
        'commit', '-q', '-m', 'test commit'
    )
    if ($add.TimedOut -or $add.ExitCode -ne 0 -or $commit.TimedOut -or $commit.ExitCode -ne 0) {
        throw 'A49 Git test commit setup failed.'
    }
    $normalState = Get-A49GitState -RepositoryRoot $normal
    if ($normalState.Commit -notmatch '^[0-9a-f]{40,64}$' -or $normalState.WorktreeDirty) {
        throw 'A49 Git state did not return a clean normal commit.'
    }

    $rejected = $false
    try { [void](Get-A49GitState -RepositoryRoot $nonRepository) } catch { $rejected = $true }
    if (-not $rejected) { throw 'A49 Git state accepted a non-repository directory.' }
}
finally {
    $resolvedTestRoot = [System.IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($temporaryBase, [System.StringComparison]::OrdinalIgnoreCase) -and
        $resolvedTestRoot -ne $temporaryBase -and (Test-Path -LiteralPath $resolvedTestRoot)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
    }
}

Write-Output 'A49 PowerShell Git state tests passed.'
