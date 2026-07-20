package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/internal/ops"
	"new-api-pilot/model"
)

const (
	exitSuccess       = 0
	exitCheckFailed   = 1
	exitArgumentError = 2
)

type reencryptCLIOptions struct {
	DryRun    bool
	BatchSize int
}

type verifyRestoreCLIOptions struct {
	ManifestPath string
}

type manifestPreflightCLIOptions struct {
	ManifestPath string
}

func isMaintenanceCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "secrets" || args[0] == "verify-restore" || args[0] == "verify-backup" ||
		args[0] == "database-identity"
}

func runMaintenanceCommand(args []string, output io.Writer) int {
	if len(args) == 0 || output == nil {
		return exitArgumentError
	}
	switch args[0] {
	case "secrets":
		return runReencryptCommand(args[1:], output)
	case "verify-restore":
		return runVerifyRestoreCommand(args[1:], output)
	case "verify-backup":
		return runManifestPreflightCommand(args[1:], output)
	case "database-identity":
		return runDatabaseIdentityCommand(args[1:], output)
	default:
		return exitArgumentError
	}
}

func runDatabaseIdentityCommand(args []string, output io.Writer) int {
	if len(args) != 0 {
		writeDatabaseIdentityFailure(output, "DATABASE_IDENTITY_ARGUMENT_INVALID")
		return exitArgumentError
	}
	maintenanceConfig, err := config.LoadDatabaseIdentity()
	if err != nil {
		writeDatabaseIdentityFailure(output, "DATABASE_IDENTITY_CONFIG_INVALID")
		return exitArgumentError
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := openMaintenanceDatabase(ctx, maintenanceConfig.Database)
	if err != nil {
		writeDatabaseIdentityFailure(output, "DATABASE_IDENTITY_DATABASE_ERROR")
		return exitCheckFailed
	}
	defer func() { _ = database.Close() }()
	report, operationErr := ops.RunDatabaseIdentity(ctx, database.SQL)
	if err := writeJSONReport(output, report); err != nil {
		return exitCheckFailed
	}
	if operationErr != nil {
		return exitCheckFailed
	}
	return exitSuccess
}

func runReencryptCommand(args []string, output io.Writer) int {
	options, err := parseReencryptCLI(args)
	if err != nil {
		writeReencryptFailure(output, "REENCRYPT_ARGUMENT_INVALID", false, "", "")
		return exitArgumentError
	}
	maintenanceConfig, err := config.LoadReencrypt()
	if err != nil {
		writeReencryptFailure(output, "REENCRYPT_CONFIG_INVALID", options.DryRun, "", "")
		return exitArgumentError
	}
	oldCipher, err := common.NewCipher(maintenanceConfig.OldKey)
	if err != nil {
		writeReencryptFailure(
			output, "REENCRYPT_CONFIG_INVALID", options.DryRun,
			maintenanceConfig.OldKeyID, maintenanceConfig.NewKeyID,
		)
		return exitArgumentError
	}
	newCipher, err := common.NewCipher(maintenanceConfig.NewKey)
	if err != nil {
		writeReencryptFailure(
			output, "REENCRYPT_CONFIG_INVALID", options.DryRun,
			maintenanceConfig.OldKeyID, maintenanceConfig.NewKeyID,
		)
		return exitArgumentError
	}

	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	database, err := openMaintenanceDatabase(ctx, maintenanceConfig.Database)
	if err != nil {
		writeReencryptFailure(
			output, "REENCRYPT_DATABASE_ERROR", options.DryRun,
			maintenanceConfig.OldKeyID, maintenanceConfig.NewKeyID,
		)
		return exitCheckFailed
	}
	defer func() { _ = database.Close() }()
	report, operationErr := ops.RunReencrypt(ctx, ops.ReencryptOptions{
		Database: database.SQL, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: options.BatchSize, DryRun: options.DryRun, Now: time.Now,
	})
	if err := writeJSONReport(output, report); err != nil {
		return exitCheckFailed
	}
	if operationErr != nil {
		return exitCheckFailed
	}
	return exitSuccess
}

func runVerifyRestoreCommand(args []string, output io.Writer) int {
	options, err := parseVerifyRestoreCLI(args)
	if err != nil {
		writeVerifyFailure(output, "VERIFY_ARGUMENT_INVALID", "")
		return exitArgumentError
	}
	maintenanceConfig, err := config.LoadVerifyRestore()
	if err != nil {
		writeVerifyFailure(output, "VERIFY_CONFIG_INVALID", "")
		return exitArgumentError
	}
	cipher, err := common.NewCipher(maintenanceConfig.EncryptionKey)
	if err != nil {
		writeVerifyFailure(output, "VERIFY_CONFIG_INVALID", maintenanceConfig.KeyID)
		return exitArgumentError
	}

	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	database, err := openMaintenanceDatabase(ctx, maintenanceConfig.Database)
	if err != nil {
		writeVerifyFailure(output, "VERIFY_DATABASE_ERROR", maintenanceConfig.KeyID)
		return exitCheckFailed
	}
	defer func() { _ = database.Close() }()
	report, operationErr := ops.RunVerifyRestore(ctx, ops.VerifyRestoreOptions{
		Database: database.SQL, Cipher: cipher, ManifestPath: options.ManifestPath,
	})
	if err := writeJSONReport(output, report); err != nil {
		return exitCheckFailed
	}
	if operationErr != nil {
		return exitCheckFailed
	}
	return exitSuccess
}

func runManifestPreflightCommand(args []string, output io.Writer) int {
	options, err := parseManifestPreflightCLI(args)
	if err != nil {
		writeManifestPreflightFailure(output, "MANIFEST_ARGUMENT_INVALID", "")
		return exitArgumentError
	}
	maintenanceConfig, err := config.LoadManifestPreflight()
	if err != nil {
		writeManifestPreflightFailure(output, "MANIFEST_CONFIG_INVALID", "")
		return exitArgumentError
	}
	report, operationErr := ops.RunManifestPreflight(options.ManifestPath, maintenanceConfig.KeyID)
	if err := writeJSONReport(output, report); err != nil {
		return exitCheckFailed
	}
	if operationErr != nil {
		return exitCheckFailed
	}
	return exitSuccess
}

func parseReencryptCLI(args []string) (reencryptCLIOptions, error) {
	if len(args) == 0 || args[0] != "reencrypt" {
		return reencryptCLIOptions{}, errors.New("expected secrets reencrypt")
	}
	options := reencryptCLIOptions{BatchSize: 100}
	flags := flag.NewFlagSet("secrets reencrypt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.DryRun, "dry-run", false, "")
	flags.IntVar(&options.BatchSize, "batch-size", 100, "")
	if err := flags.Parse(args[1:]); err != nil {
		return reencryptCLIOptions{}, err
	}
	if flags.NArg() != 0 || options.BatchSize < 1 || options.BatchSize > 1000 {
		return reencryptCLIOptions{}, errors.New("batch-size must be between 1 and 1000")
	}
	return options, nil
}

func parseVerifyRestoreCLI(args []string) (verifyRestoreCLIOptions, error) {
	options := verifyRestoreCLIOptions{}
	mode := ""
	flags := flag.NewFlagSet("verify-restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&mode, "mode", "", "")
	flags.StringVar(&options.ManifestPath, "manifest", "", "")
	if err := flags.Parse(args); err != nil {
		return verifyRestoreCLIOptions{}, err
	}
	if flags.NArg() != 0 || mode != "full" || options.ManifestPath == "" ||
		!filepath.IsAbs(options.ManifestPath) || filepath.Base(options.ManifestPath) != "manifest.json" {
		return verifyRestoreCLIOptions{}, errors.New("mode=full and manifest are required")
	}
	return options, nil
}

func parseManifestPreflightCLI(args []string) (manifestPreflightCLIOptions, error) {
	options := manifestPreflightCLIOptions{}
	mode := ""
	flags := flag.NewFlagSet("verify-backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&mode, "mode", "", "")
	flags.StringVar(&options.ManifestPath, "manifest", "", "")
	if err := flags.Parse(args); err != nil {
		return manifestPreflightCLIOptions{}, err
	}
	if flags.NArg() != 0 || mode != "manifest-only" || options.ManifestPath == "" ||
		!filepath.IsAbs(options.ManifestPath) || filepath.Base(options.ManifestPath) != "manifest.json" {
		return manifestPreflightCLIOptions{}, errors.New("mode=manifest-only and manifest are required")
	}
	return options, nil
}

func openMaintenanceDatabase(
	ctx context.Context,
	databaseConfig config.MaintenanceDatabaseConfig,
) (*model.Database, error) {
	database, err := model.Open(ctx, model.Options{
		DSN: databaseConfig.DatabaseDSN, MaxIdle: databaseConfig.SQLMaxIdleConns,
		MaxOpen: databaseConfig.SQLMaxOpenConns, MaxLifetime: databaseConfig.SQLMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize maintenance database: %w", err)
	}
	return database, nil
}

func writeReencryptFailure(output io.Writer, code string, dryRun bool, oldKeyID, newKeyID string) {
	_ = writeJSONReport(output, ops.ReencryptReport{
		SchemaVersion: ops.ReportSchemaVersion,
		Command:       "secrets reencrypt",
		Status:        "failed",
		DryRun:        dryRun,
		OldKeyID:      ops.ShortFingerprint(oldKeyID),
		NewKeyID:      ops.ShortFingerprint(newKeyID),
		Error:         &ops.OperationError{Code: code},
	})
}

func writeVerifyFailure(output io.Writer, code, keyID string) {
	_ = writeJSONReport(output, ops.VerifyReport{
		SchemaVersion:   ops.ReportSchemaVersion,
		Command:         "verify-restore",
		Mode:            "full",
		Status:          "failed",
		EncryptionKeyID: ops.ShortFingerprint(keyID),
		Checks: []ops.VerifyCheck{
			{Name: "configuration", Status: "failed", Code: code},
		},
		Summary: ops.VerifySummary{Failed: 1},
		Error:   &ops.OperationError{Code: code},
	})
}

func writeManifestPreflightFailure(output io.Writer, code, keyID string) {
	_ = writeJSONReport(output, ops.VerifyReport{
		SchemaVersion:   ops.ReportSchemaVersion,
		Command:         "verify-backup",
		Mode:            "manifest-only",
		Status:          "failed",
		EncryptionKeyID: ops.ShortFingerprint(keyID),
		Checks: []ops.VerifyCheck{
			{Name: "configuration", Status: "failed", Code: code},
		},
		Summary: ops.VerifySummary{Failed: 1},
		Error:   &ops.OperationError{Code: code},
	})
}

func writeDatabaseIdentityFailure(output io.Writer, code string) {
	_ = writeJSONReport(output, ops.DatabaseIdentityReport{
		SchemaVersion: ops.ReportSchemaVersion,
		Command:       "database-identity",
		Status:        "failed",
		Error:         &ops.OperationError{Code: code},
	})
}

func writeJSONReport(output io.Writer, report any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}
