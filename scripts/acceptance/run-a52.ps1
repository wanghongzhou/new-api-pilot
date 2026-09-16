[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'controlled-ops-runner.ps1')
try {
    Invoke-ControlledOpsAcceptance -AcceptanceID 'A52'
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
