[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'a49-path-guard.ps1')

$root = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), 'new-api-pilot-a49-path-root')
$inside = [System.IO.Path]::Combine($root, 'artifacts', 'smoke', 'A49', 'run')
$expected = 'artifacts/smoke/A49/run'
if ((Get-A49RepositoryRelativePath -RepositoryRoot $root -CandidatePath $inside) -cne $expected) {
    throw 'A49 path guard rejected a canonical descendant.'
}
if ((Get-A49RepositoryRelativePath -RepositoryRoot $root.ToUpperInvariant() -CandidatePath $inside) -cne $expected) {
    throw 'A49 path guard did not use case-insensitive Windows containment.'
}

$invalid = @(
    [System.IO.Path]::Combine($root, '..', 'outside', 'run'),
    ($root + '-sibling\artifacts\smoke\A49\run'),
    $root
)
foreach ($candidate in $invalid) {
    $rejected = $false
    try {
        [void](Get-A49RepositoryRelativePath -RepositoryRoot $root -CandidatePath $candidate)
    }
    catch {
        $rejected = $true
    }
    if (-not $rejected) {
        throw "A49 path guard accepted an escaping/non-descendant path: $candidate"
    }
}

Write-Output 'A49 PowerShell path guard tests passed.'
