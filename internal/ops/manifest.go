package ops

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BackupManifest struct {
	SchemaVersion   int                 `json:"schema_version"`
	BackupID        string              `json:"backup_id"`
	CreatedAtUTC    string              `json:"created_at_utc"`
	Database        string              `json:"database"`
	DumpFile        string              `json:"dump_file"`
	DumpSHA256      string              `json:"dump_sha256"`
	DumpSizeBytes   int64               `json:"dump_size_bytes"`
	ImageDigest     string              `json:"image_digest"`
	EncryptionKeyID string              `json:"encryption_key_id"`
	MySQLVersion    string              `json:"mysql_version"`
	ServerUUID      string              `json:"server_uuid"`
	Source          BackupSource        `json:"source"`
	Migrations      []ManifestMigration `json:"schema_migrations"`
	ExportFiles     string              `json:"export_files"`
}

type BackupSource struct {
	LogFile         string `json:"log_file,omitempty"`
	LogPosition     uint64 `json:"log_position,omitempty"`
	ExecutedGTIDSet string `json:"executed_gtid_set,omitempty"`
}

type ManifestMigration struct {
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

type ValidatedManifest struct {
	Manifest     BackupManifest
	ManifestPath string
	DumpPath     string
	ManifestHash string
}

var (
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	imageDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	backupIDPattern    = regexp.MustCompile(`^backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8,64}$`)
	sourceDataPattern  = regexp.MustCompile(`SOURCE_LOG_FILE='([^']+)', SOURCE_LOG_POS=([0-9]+)`)
)

func ValidateBackupManifest(path string, expectedKeyID string) (ValidatedManifest, error) {
	if !filepath.IsAbs(path) {
		return ValidatedManifest{}, errors.New("manifest path must be absolute")
	}
	cleanPath := filepath.Clean(path)
	if filepath.Base(cleanPath) != "manifest.json" {
		return ValidatedManifest{}, errors.New("manifest filename must be manifest.json")
	}
	if err := requireRegularPath(cleanPath); err != nil {
		return ValidatedManifest{}, fmt.Errorf("validate manifest path: %w", err)
	}
	payload, err := readBoundedFile(cleanPath, 1<<20)
	if err != nil {
		return ValidatedManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	manifestHashBytes := sha256.Sum256(payload)
	manifestHash := hex.EncodeToString(manifestHashBytes[:])
	if err := verifyChecksumSidecar(cleanPath+".sha256", filepath.Base(cleanPath), manifestHash); err != nil {
		return ValidatedManifest{}, fmt.Errorf("verify manifest checksum: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var manifest BackupManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ValidatedManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ValidatedManifest{}, errors.New("manifest must contain exactly one JSON object")
	}
	if err := validateManifestFields(manifest, expectedKeyID); err != nil {
		return ValidatedManifest{}, err
	}
	dumpPath := filepath.Join(filepath.Dir(cleanPath), manifest.DumpFile)
	if filepath.Dir(dumpPath) != filepath.Dir(cleanPath) || filepath.Base(dumpPath) != manifest.DumpFile {
		return ValidatedManifest{}, errors.New("dump_file must be a basename inside the backup directory")
	}
	if err := requireRegularPath(dumpPath); err != nil {
		return ValidatedManifest{}, fmt.Errorf("validate dump path: %w", err)
	}
	dumpHash, dumpSize, err := hashFile(dumpPath)
	if err != nil {
		return ValidatedManifest{}, fmt.Errorf("hash dump file: %w", err)
	}
	if dumpHash != manifest.DumpSHA256 || dumpSize != manifest.DumpSizeBytes {
		return ValidatedManifest{}, errors.New("dump file checksum or size mismatch")
	}
	if err := verifyChecksumSidecar(dumpPath+".sha256", manifest.DumpFile, dumpHash); err != nil {
		return ValidatedManifest{}, fmt.Errorf("verify dump checksum sidecar: %w", err)
	}
	if manifest.Source.LogFile != "" {
		file, position, err := dumpSourceCoordinate(dumpPath)
		if err != nil {
			return ValidatedManifest{}, err
		}
		if file != manifest.Source.LogFile || position != manifest.Source.LogPosition {
			return ValidatedManifest{}, errors.New("dump source coordinate does not match manifest")
		}
	}
	return ValidatedManifest{
		Manifest: manifest, ManifestPath: cleanPath, DumpPath: dumpPath, ManifestHash: manifestHash,
	}, nil
}

func validateManifestFields(manifest BackupManifest, expectedKeyID string) error {
	if manifest.SchemaVersion != 1 || !backupIDPattern.MatchString(manifest.BackupID) {
		return errors.New("manifest schema_version or backup_id is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAtUTC)
	if err != nil || createdAt.Location() != time.UTC {
		return errors.New("created_at_utc must be an RFC3339 UTC timestamp")
	}
	if manifest.Database == "" || strings.ContainsAny(manifest.Database, "`/\\\x00") {
		return errors.New("manifest database name is invalid")
	}
	if manifest.DumpFile == "" || filepath.Base(manifest.DumpFile) != manifest.DumpFile || strings.HasPrefix(manifest.DumpFile, ".") {
		return errors.New("manifest dump_file is invalid")
	}
	if !sha256Pattern.MatchString(manifest.DumpSHA256) || manifest.DumpSizeBytes <= 0 {
		return errors.New("manifest dump checksum or size is invalid")
	}
	if !imageDigestPattern.MatchString(manifest.ImageDigest) || !sha256Pattern.MatchString(manifest.EncryptionKeyID) {
		return errors.New("manifest image digest or encryption key id is invalid")
	}
	if manifest.EncryptionKeyID != expectedKeyID {
		return errors.New("manifest encryption key id does not match ENCRYPTION_KEY")
	}
	if manifest.MySQLVersion == "" || manifest.ServerUUID == "" || len(manifest.Migrations) == 0 {
		return errors.New("manifest MySQL identity or migrations are missing")
	}
	filePosition := manifest.Source.LogFile != "" || manifest.Source.LogPosition != 0
	gtid := manifest.Source.ExecutedGTIDSet != ""
	if filePosition == gtid || (filePosition && (manifest.Source.LogFile == "" || manifest.Source.LogPosition == 0)) {
		return errors.New("manifest must contain exactly one complete source coordinate mode")
	}
	seen := make(map[string]struct{}, len(manifest.Migrations))
	previous := ""
	for _, migration := range manifest.Migrations {
		if migration.Version == "" || !sha256Pattern.MatchString(migration.Checksum) || migration.Version <= previous {
			return errors.New("manifest schema_migrations must be unique and sorted")
		}
		if _, exists := seen[migration.Version]; exists {
			return errors.New("manifest schema_migrations contains a duplicate")
		}
		seen[migration.Version] = struct{}{}
		previous = migration.Version
	}
	if manifest.ExportFiles != "excluded_regenerable" {
		return errors.New("manifest export_files policy is invalid")
	}
	return nil
}

func requireRegularPath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current := filepath.Clean(absolute)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symlink", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("path is not a regular file")
	}
	return nil
}

func readBoundedFile(path string, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, errors.New("maximum file size must not be negative")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maximum {
		return nil, errors.New("file exceeds maximum allowed size")
	}
	return payload, nil
}

func verifyChecksumSidecar(path, expectedName, expectedHash string) error {
	if err := requireRegularPath(path); err != nil {
		return err
	}
	payload, err := readBoundedFile(path, 256)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(payload))
	if len(fields) != 2 || fields[0] != expectedHash || strings.TrimPrefix(fields[1], "*") != expectedName {
		return errors.New("checksum sidecar does not match its file")
	}
	return nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func dumpSourceCoordinate(path string) (string, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return "", 0, fmt.Errorf("open compressed dump: %w", err)
	}
	defer func() { _ = reader.Close() }()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		matches := sourceDataPattern.FindStringSubmatch(scanner.Text())
		if len(matches) != 3 {
			continue
		}
		position, err := strconv.ParseUint(matches[2], 10, 64)
		if err != nil || position == 0 {
			return "", 0, errors.New("dump source position is invalid")
		}
		return matches[1], position, nil
	}
	if err := scanner.Err(); err != nil {
		return "", 0, fmt.Errorf("scan dump coordinate: %w", err)
	}
	return "", 0, errors.New("dump does not contain a source coordinate")
}
