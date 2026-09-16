function Invoke-ControlledOpsAcceptance {
    param([Parameter(Mandatory = $true)][ValidateSet('A52', 'A74', 'A75')][string]$AcceptanceID)

    Set-StrictMode -Version Latest
    $ErrorActionPreference = 'Stop'
    . (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

    $evidenceDirectory = [string]$env:ACCEPTANCE_EVIDENCE_DIR
    if ($env:ACCEPTANCE_ID -cne $AcceptanceID -or $env:ACCEPTANCE_EVIDENCE_CLASS -cne 'formal' -or
        [string]::IsNullOrWhiteSpace($evidenceDirectory) -or -not [System.IO.Path]::IsPathRooted($evidenceDirectory) -or
        -not (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
        throw "$AcceptanceID formal acceptance harness environment is required"
    }

    $inputVariable = "${AcceptanceID}_CONTROLLED_INPUT"
    $inputPath = [Environment]::GetEnvironmentVariable($inputVariable)
    if ([string]::IsNullOrWhiteSpace($inputPath) -or -not [System.IO.Path]::IsPathRooted($inputPath) -or
        -not (Test-Path -LiteralPath $inputPath -PathType Leaf)) {
        $blocked = [ordered]@{
            schema_version = 1; acceptance_id = $AcceptanceID; status = 'blocked'; passed = $false
            blocker = 'controlled_formal_input_is_unavailable'; required_environment = $inputVariable
        }
        Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'blocked-report.json') -Payload (($blocked | ConvertTo-Json -Depth 6) + "`n")
        throw "$AcceptanceID BLOCKED: $inputVariable must reference an absolute controlled-input JSON file."
    }

    $input = Get-Content -Raw -LiteralPath $inputPath | ConvertFrom-Json -ErrorAction Stop
    $materialPath = [string]$input.controlled_material_path
    if ([int]$input.schema_version -ne 1 -or [string]$input.acceptance_id -cne $AcceptanceID -or
        [string]$input.status -cne 'passed' -or -not [bool]$input.passed -or
        [string]$input.evidence_class -cne 'formal' -or -not [bool]$input.acceptance_eligible -or
        [string]::IsNullOrWhiteSpace($materialPath) -or -not [System.IO.Path]::IsPathRooted($materialPath) -or
        -not (Test-Path -LiteralPath $materialPath -PathType Leaf)) {
        throw "$AcceptanceID controlled input contract is invalid."
    }
    if (([System.IO.Path]::GetExtension($materialPath)).ToLowerInvariant() -cne '.zip') {
        throw "$AcceptanceID controlled material must be a ZIP archive."
    }

    $prefix = $AcceptanceID.ToLowerInvariant()
    $materialName = "$prefix-controlled-material.zip"
    $materialDestination = Join-Path $evidenceDirectory $materialName
    Copy-Item -LiteralPath $materialPath -Destination $materialDestination
    $materialInfo = Get-Item -LiteralPath $materialDestination
    $materialSHA = Get-ControlledOpsFileSHA256 -Path $materialDestination
    $input.PSObject.Properties.Remove('controlled_material_path')
    if ($null -ne $input.PSObject.Properties['material_sha256']) { $input.material_sha256 = $materialSHA }
    else { $input | Add-Member -NotePropertyName material_sha256 -NotePropertyValue $materialSHA }
    if ($null -ne $input.PSObject.Properties['material_size_bytes']) { $input.material_size_bytes = [int64]$materialInfo.Length }
    else { $input | Add-Member -NotePropertyName material_size_bytes -NotePropertyValue ([int64]$materialInfo.Length) }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory "$prefix-report.json") -Payload (($input | ConvertTo-Json -Depth 20) + "`n")

    $command = @('powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "scripts/acceptance/run-$prefix.ps1")
    $commandReport = [ordered]@{ schema_version = 1; acceptance_id = $AcceptanceID; evidence_class = 'formal'; command = $command }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory "$prefix-command.json") -Payload (($commandReport | ConvertTo-Json -Depth 6) + "`n")

    $repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
    $manifestPath = Join-Path $repositoryRoot 'testdata\design\manifest.sha256'
    $fixtureIDs = if ($AcceptanceID -eq 'A52') { @('F02', 'F05') } else { @('F05') }
    $fixture = [ordered]@{
        schema_version = 1; acceptance_id = $AcceptanceID; manifest_path = 'testdata/design/manifest.sha256'
        manifest_sha256 = Get-ControlledOpsFileSHA256 -Path $manifestPath
        fixture_ids = @($fixtureIDs)
    }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory "$prefix-fixture.json") -Payload (($fixture | ConvertTo-Json -Depth 6) + "`n")

    $artifactNames = @("$prefix-command.json", $materialName, "$prefix-fixture.json", "$prefix-report.json")
    $files = foreach ($name in $artifactNames) {
        $path = Join-Path $evidenceDirectory $name
        $info = Get-Item -LiteralPath $path
        [ordered]@{ path = $name; size_bytes = [int64]$info.Length; sha256 = Get-ControlledOpsFileSHA256 -Path $path }
    }
    $inventory = [ordered]@{ schema_version = 1; acceptance_id = $AcceptanceID; evidence_class = 'formal'; files = @($files) }
    Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory "$prefix-artifacts.json") -Payload (($inventory | ConvertTo-Json -Depth 8) + "`n")
    Write-Output "$AcceptanceID controlled formal material staged for closed-contract validation."
}

function Get-ControlledOpsFileSHA256 {
    param([Parameter(Mandatory = $true)][string]$Path)

    $stream = [System.IO.File]::OpenRead($Path)
    $hasher = [System.Security.Cryptography.SHA256]::Create()
    try {
        return ([System.BitConverter]::ToString($hasher.ComputeHash($stream))).Replace('-', '').ToLowerInvariant()
    }
    finally {
        $hasher.Dispose()
        $stream.Dispose()
    }
}
