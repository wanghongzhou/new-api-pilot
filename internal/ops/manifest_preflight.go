package ops

import "fmt"

func RunManifestPreflight(manifestPath, expectedKeyID string) (VerifyReport, error) {
	report := newVerifyReport(expectedKeyID)
	report.Command = "verify-backup"
	report.Mode = "manifest-only"
	validated, err := ValidateBackupManifest(manifestPath, expectedKeyID)
	if err != nil {
		report.addCheck(failedCheck("backup_manifest", "VERIFY_MANIFEST_INVALID"))
		return report, fmt.Errorf("VERIFY_MANIFEST_INVALID: %w", err)
	}
	report.BackupID = validated.Manifest.BackupID
	report.addCheck(passedCheck("backup_manifest", map[string]any{
		"dump_size_bytes":   validated.Manifest.DumpSizeBytes,
		"manifest_sha256":   validated.ManifestHash,
		"schema_migrations": len(validated.Manifest.Migrations),
	}))
	if _, err := validateManifestMigrations(validated.Manifest.Migrations); err != nil {
		report.addCheck(failedCheck("migrations", "VERIFY_MIGRATION_INVALID"))
		return report, fmt.Errorf("VERIFY_MIGRATION_INVALID: %w", err)
	}
	report.addCheck(passedCheck("migrations", map[string]any{
		"schema_migrations": len(validated.Manifest.Migrations),
	}))
	report.Status = "success"
	report.Error = nil
	return report, nil
}
