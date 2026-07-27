$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'ops-runner-common.ps1')

$acceptanceID = [string]$env:ACCEPTANCE_ID
$evidenceDirectory = [string]$env:ACCEPTANCE_EVIDENCE_DIR
$evidenceClass = [string]$env:ACCEPTANCE_EVIDENCE_CLASS
if ($acceptanceID -notin @('A52', 'A74', 'A75') -or $evidenceClass -cne 'formal' -or
    [string]::IsNullOrWhiteSpace($evidenceDirectory) -or -not [System.IO.Path]::IsPathRooted($evidenceDirectory) -or
    -not (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
    throw 'supported blocked-case formal acceptance harness environment is required'
}

$blockers = @{
    A52 = [ordered]@{
        blocker = 'production_site_onboarding_inventory_and_accountable_owner_confirmations_are_not_available_in_the_local_isolated_environment'
        required_external_inputs = @(
            'production site inventory',
            'per-site status identity API-contract and export verification',
            'named owner confirmation before first business record'
        )
    }
    A74 = [ordered]@{
        blocker = 'controlled_deployment_and_rollback_environment_with_immutable_image_digests_monitoring_and_named_approvals_is_not_available_locally'
        required_external_inputs = @(
            'controlled deployment target and immutable image digests',
            'monitoring and rollback observation evidence',
            'named operator and independent reviewer approvals'
        )
    }
    A75 = [ordered]@{
        blocker = 'controlled_backup_pitr_and_key_recovery_inputs_with_named_approvals_are_not_available_locally'
        required_external_inputs = @(
            'real backup and point-in-time recovery material',
            'controlled key-recovery execution environment',
            'named operator and independent reviewer approvals'
        )
    }
}
$case = $blockers[$acceptanceID]
$report = [ordered]@{
    schema_version = 1
    acceptance_id = $acceptanceID
    status = 'blocked'
    passed = $false
    blocker = $case.blocker
    required_external_inputs = @($case.required_external_inputs)
}
Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'blocked-report.json') -Payload (($report | ConvertTo-Json -Depth 10) + "`n")
Write-Error "$acceptanceID BLOCKED: required controlled-environment inputs and independent approvals are unavailable."
exit 3
