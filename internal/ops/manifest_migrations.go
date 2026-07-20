package ops

import (
	"new-api-pilot/migrations"
	"new-api-pilot/model"
)

func validateManifestMigrations(manifest []ManifestMigration) ([]model.MigrationVersion, error) {
	repository, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil {
		return nil, err
	}
	manifestVersions := make([]model.MigrationVersion, 0, len(manifest))
	for _, migration := range manifest {
		manifestVersions = append(manifestVersions, model.MigrationVersion{
			Version: migration.Version, Checksum: migration.Checksum,
		})
	}
	if err := model.ValidateMigrationVersionPrefix(repository, manifestVersions, true); err != nil {
		return nil, err
	}
	return repository, nil
}
