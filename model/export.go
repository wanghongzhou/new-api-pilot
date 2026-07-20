package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const exportMaximumAttempts = 2

var (
	ErrExportClaimLost       = errors.New("export claim was lost")
	ErrExportSettingContract = errors.New("export setting contract is invalid")
)

type ExportJob struct {
	ID             int64           `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64           `gorm:"column:user_id"`
	Format         string          `gorm:"column:format"`
	StatisticsType string          `gorm:"column:statistics_type"`
	Filters        json.RawMessage `gorm:"column:filters;type:json"`
	FilterHash     string          `gorm:"column:filter_hash"`
	ActiveKey      *string         `gorm:"column:active_key"`
	RateSnapshot   json.RawMessage `gorm:"column:rate_snapshot;type:json"`
	DataSnapshotAt *int64          `gorm:"column:data_snapshot_at"`
	Status         string          `gorm:"column:status"`
	Progress       int             `gorm:"column:progress"`
	AttemptCount   int             `gorm:"column:attempt_count"`
	NextAttemptAt  int64           `gorm:"column:next_attempt_at"`
	HeartbeatAt    *int64          `gorm:"column:heartbeat_at"`
	ClaimToken     *string         `gorm:"column:claim_token"`
	LeaseExpiresAt *int64          `gorm:"column:lease_expires_at"`
	FilePath       *string         `gorm:"column:file_path"`
	FileName       *string         `gorm:"column:file_name"`
	FileSize       int64           `gorm:"column:file_size"`
	RowCount       int64           `gorm:"column:row_count"`
	ErrorCode      string          `gorm:"column:error_code"`
	ErrorParams    json.RawMessage `gorm:"column:error_params;type:json"`
	ErrorMessage   *string         `gorm:"column:error_message"`
	ExpiresAt      *int64          `gorm:"column:expires_at"`
	StartedAt      *int64          `gorm:"column:started_at"`
	CreatedAt      int64           `gorm:"column:created_at"`
	FinishedAt     *int64          `gorm:"column:finished_at"`
	UpdatedAt      int64           `gorm:"column:updated_at"`
}

func (ExportJob) TableName() string { return "export_job" }

type ExportSettings struct {
	FileTTLHours     int
	MaxActivePerUser int
	MaxActiveGlobal  int
	MaxFileBytes     int64
	MinFreeDiskBytes int64
}

type ExportClaim struct {
	Job      ExportJob
	Settings ExportSettings
}

type ExportRecovery struct {
	JobID    int64
	FilePath string
	Failed   bool
}

type ExportRepository struct {
	db *gorm.DB
}

func NewExportRepository(db *gorm.DB) *ExportRepository {
	return &ExportRepository{db: db}
}

func (repository *ExportRepository) Transaction(
	ctx context.Context,
	operation func(*ExportRepository) error,
) error {
	if repository == nil || repository.db == nil || operation == nil {
		return errors.New("export repository transaction dependencies are required")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(NewExportRepository(tx))
	})
}

func (repository *ExportRepository) LoadSettingsForUpdate(ctx context.Context) (ExportSettings, error) {
	return repository.loadSettings(ctx, true)
}

func (repository *ExportRepository) LoadSettings(ctx context.Context) (ExportSettings, error) {
	return repository.loadSettings(ctx, false)
}

func (repository *ExportRepository) loadSettings(ctx context.Context, lock bool) (ExportSettings, error) {
	if repository == nil || repository.db == nil {
		return ExportSettings{}, ErrExportSettingContract
	}
	keys := []string{
		"export.file_ttl_hours",
		"export.max_active_global",
		"export.max_active_per_user",
		"export.max_file_bytes",
		"export.min_free_disk_bytes",
	}
	query := repository.db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []PlatformSetting
	if err := query.Where("setting_key IN ?", keys).Order("setting_key ASC").Find(&rows).Error; err != nil {
		return ExportSettings{}, fmt.Errorf("load export settings: %w", err)
	}
	if len(rows) != len(keys) {
		return ExportSettings{}, ErrExportSettingContract
	}
	values := make(map[string]int64, len(rows))
	for _, row := range rows {
		value, err := strconv.ParseInt(row.Value, 10, 64)
		if err != nil || value <= 0 || row.ValueType != "int" || row.Secret {
			return ExportSettings{}, ErrExportSettingContract
		}
		values[row.Key] = value
	}
	settings := ExportSettings{
		FileTTLHours:     int(values["export.file_ttl_hours"]),
		MaxActivePerUser: int(values["export.max_active_per_user"]),
		MaxActiveGlobal:  int(values["export.max_active_global"]),
		MaxFileBytes:     values["export.max_file_bytes"],
		MinFreeDiskBytes: values["export.min_free_disk_bytes"],
	}
	if settings.FileTTLHours < 1 || settings.FileTTLHours > 168 ||
		settings.MaxActivePerUser < 1 || settings.MaxActivePerUser > settings.MaxActiveGlobal ||
		settings.MaxActiveGlobal > 100 || settings.MaxFileBytes < 1 || settings.MinFreeDiskBytes < 1 {
		return ExportSettings{}, ErrExportSettingContract
	}
	return settings, nil
}

func (repository *ExportRepository) FindActiveByKey(ctx context.Context, key string) (ExportJob, error) {
	var job ExportJob
	err := repository.db.WithContext(ctx).Where("active_key = ?", key).First(&job).Error
	return job, err
}

func (repository *ExportRepository) CountActive(ctx context.Context, userID int64) (int64, int64, error) {
	active := []string{"pending", "running"}
	var perUser, global int64
	if err := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("status IN ? AND user_id = ?", active, userID).Count(&perUser).Error; err != nil {
		return 0, 0, err
	}
	if err := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("status IN ?", active).Count(&global).Error; err != nil {
		return 0, 0, err
	}
	return perUser, global, nil
}

func (repository *ExportRepository) Create(ctx context.Context, job *ExportJob) error {
	if repository == nil || repository.db == nil || job == nil {
		return errors.New("export job is required")
	}
	return repository.db.WithContext(ctx).Create(job).Error
}

func (repository *ExportRepository) GetForUser(ctx context.Context, id, userID int64) (ExportJob, error) {
	var job ExportJob
	err := repository.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&job).Error
	return job, err
}

func (repository *ExportRepository) ListForUser(
	ctx context.Context,
	userID int64,
	statuses []string,
	format, statisticsType, sortBy, sortOrder string,
	limit, offset int,
) ([]ExportJob, int64, error) {
	query := repository.db.WithContext(ctx).Model(&ExportJob{}).Where("user_id = ?", userID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	if format != "" {
		query = query.Where("format = ?", format)
	}
	if statisticsType != "" {
		query = query.Where("statistics_type = ?", statisticsType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	columns := map[string]string{
		"created_at": "created_at", "finished_at": "finished_at", "status": "status", "file_size": "file_size",
	}
	column, exists := columns[sortBy]
	if !exists || (sortOrder != "asc" && sortOrder != "desc") {
		return nil, 0, errors.New("invalid export list ordering")
	}
	var jobs []ExportJob
	err := query.Order(column + " " + sortOrder).Order("id " + sortOrder).Limit(limit).Offset(offset).Find(&jobs).Error
	return jobs, total, err
}

func (repository *ExportRepository) Claim(
	ctx context.Context,
	now int64,
	claimToken string,
	leaseExpiresAt int64,
) (*ExportClaim, error) {
	var claim *ExportClaim
	err := repository.Transaction(ctx, func(tx *ExportRepository) error {
		settings, err := tx.LoadSettingsForUpdate(ctx)
		if err != nil {
			return err
		}
		var job ExportJob
		err = tx.db.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_attempt_at <= ?", "pending", now).
			Order("next_attempt_at ASC").Order("created_at ASC").Order("id ASC").First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{
			"status": "running", "attempt_count": job.AttemptCount + 1,
			"heartbeat_at": now, "claim_token": claimToken, "lease_expires_at": leaseExpiresAt,
			"started_at": gorm.Expr("COALESCE(started_at, ?)", now), "updated_at": now,
		}
		result := tx.db.WithContext(ctx).Model(&ExportJob{}).
			Where("id = ? AND status = ?", job.ID, "pending").Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrExportClaimLost
		}
		job.Status = "running"
		job.AttemptCount++
		job.HeartbeatAt = pointerInt64(now)
		job.LeaseExpiresAt = pointerInt64(leaseExpiresAt)
		job.ClaimToken = pointerString(claimToken)
		if job.StartedAt == nil {
			job.StartedAt = pointerInt64(now)
		}
		job.UpdatedAt = now
		claim = &ExportClaim{Job: job, Settings: settings}
		return nil
	})
	return claim, err
}

func (repository *ExportRepository) Heartbeat(
	ctx context.Context,
	id int64,
	claimToken string,
	now, leaseExpiresAt int64,
	progress int,
) error {
	if progress < 0 || progress > 99 {
		return errors.New("export heartbeat progress must be between 0 and 99")
	}
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "running", claimToken).
		Updates(map[string]any{"heartbeat_at": now, "lease_expires_at": leaseExpiresAt, "progress": progress, "updated_at": now})
	return exportCASResult(result)
}

func (repository *ExportRepository) SetRateSnapshot(
	ctx context.Context,
	id int64,
	claimToken string,
	snapshot json.RawMessage,
	now int64,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ? AND rate_snapshot IS NULL", id, "running", claimToken).
		Updates(map[string]any{"rate_snapshot": snapshot, "updated_at": now})
	return exportCASResult(result)
}

func (repository *ExportRepository) SetTemporaryPath(
	ctx context.Context,
	id int64,
	claimToken, filePath string,
	now int64,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "running", claimToken).
		Updates(map[string]any{"file_path": filePath, "updated_at": now})
	return exportCASResult(result)
}

func (repository *ExportRepository) SetDataSnapshot(
	ctx context.Context,
	id int64,
	claimToken string,
	snapshotAt, now int64,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "running", claimToken).
		Updates(map[string]any{"data_snapshot_at": snapshotAt, "updated_at": now})
	return exportCASResult(result)
}

func (repository *ExportRepository) Complete(
	ctx context.Context,
	id int64,
	claimToken, filePath, fileName string,
	fileSize, rowCount, finishedAt, expiresAt int64,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "running", claimToken).
		Updates(map[string]any{
			"status": "success", "progress": 100, "active_key": nil,
			"claim_token": nil, "lease_expires_at": nil, "heartbeat_at": finishedAt,
			"file_path": filePath, "file_name": fileName, "file_size": fileSize, "row_count": rowCount,
			"error_code": "", "error_params": nil, "error_message": nil,
			"finished_at": finishedAt, "expires_at": expiresAt, "updated_at": finishedAt,
		})
	return exportCASResult(result)
}

func (repository *ExportRepository) FinishAttempt(
	ctx context.Context,
	id int64,
	claimToken, code string,
	params json.RawMessage,
	technicalDetail string,
	now int64,
	retryAt *int64,
) error {
	updates := map[string]any{
		"claim_token": nil, "lease_expires_at": nil, "heartbeat_at": now,
		"file_path": nil, "file_size": 0, "row_count": 0,
		"error_code": code, "error_params": params, "error_message": nullableString(technicalDetail), "updated_at": now,
	}
	if retryAt != nil {
		updates["status"] = "pending"
		updates["next_attempt_at"] = *retryAt
	} else {
		updates["status"] = "failed"
		updates["active_key"] = nil
		updates["finished_at"] = now
	}
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND status = ? AND claim_token = ?", id, "running", claimToken).Updates(updates)
	return exportCASResult(result)
}

func (repository *ExportRepository) RecoverRunning(ctx context.Context, now int64, takeover bool) ([]ExportRecovery, error) {
	result := make([]ExportRecovery, 0)
	err := repository.Transaction(ctx, func(tx *ExportRepository) error {
		query := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? OR (status IN ? AND file_path IS NOT NULL)", "running", []string{"pending", "failed"})
		if !takeover {
			query = query.Where("status <> ? OR lease_expires_at IS NULL OR lease_expires_at <= ?", "running", now)
		}
		var jobs []ExportJob
		if err := query.Order("id ASC").Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if job.Status != "running" {
				result = append(result, ExportRecovery{JobID: job.ID, FilePath: *job.FilePath, Failed: job.Status == "failed"})
				continue
			}
			failed := job.AttemptCount >= exportMaximumAttempts
			params, _ := json.Marshal(map[string]any{"export_id": strconv.FormatInt(job.ID, 10)})
			updates := map[string]any{
				"claim_token": nil, "lease_expires_at": nil, "heartbeat_at": now,
				"file_size": 0, "row_count": 0,
				"error_code": "EXPORT_WRITE_FAILED", "error_params": params,
				"error_message": "worker lease was recovered", "updated_at": now,
			}
			if failed {
				updates["status"] = "failed"
				updates["active_key"] = nil
				updates["finished_at"] = now
			} else {
				updates["status"] = "pending"
				updates["next_attempt_at"] = now
			}
			if err := tx.db.WithContext(ctx).Model(&ExportJob{}).Where("id = ? AND status = ?", job.ID, "running").Updates(updates).Error; err != nil {
				return err
			}
			path := ""
			if job.FilePath != nil {
				path = *job.FilePath
			}
			result = append(result, ExportRecovery{JobID: job.ID, FilePath: path, Failed: failed})
		}
		return nil
	})
	return result, err
}

func (repository *ExportRepository) Expire(ctx context.Context, now int64) ([]ExportRecovery, error) {
	result := make([]ExportRecovery, 0)
	err := repository.Transaction(ctx, func(tx *ExportRepository) error {
		var jobs []ExportJob
		if err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("(status = ? AND expires_at IS NOT NULL AND expires_at <= ?) OR (status = ? AND file_path IS NOT NULL)",
				"success", now, "expired").
			Order("id ASC").Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if err := tx.db.WithContext(ctx).Model(&ExportJob{}).Where("id = ? AND status IN ?", job.ID, []string{"success", "expired"}).
				Updates(map[string]any{"status": "expired", "updated_at": now}).Error; err != nil {
				return err
			}
			path := ""
			if job.FilePath != nil {
				path = *job.FilePath
			}
			result = append(result, ExportRecovery{JobID: job.ID, FilePath: path})
		}
		return nil
	})
	return result, err
}

func (repository *ExportRepository) ClearArtifactPath(
	ctx context.Context,
	id int64,
	expectedPath string,
	now int64,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND file_path = ?", id, expectedPath).
		Updates(map[string]any{"file_path": nil, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (repository *ExportRepository) MarkExpired(ctx context.Context, id, userID, now int64) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, "success").
		Updates(map[string]any{"status": "expired", "updated_at": now})
	return exportCASResult(result)
}

func (repository *ExportRepository) MarkFileMissing(
	ctx context.Context,
	id, userID, now int64,
	params json.RawMessage,
) error {
	result := repository.db.WithContext(ctx).Model(&ExportJob{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, "success").
		Updates(map[string]any{
			"status": "failed", "file_path": nil, "file_size": 0,
			"error_code": "EXPORT_FILE_MISSING", "error_params": params,
			"error_message": "completed export file is missing", "finished_at": now, "updated_at": now,
		})
	return exportCASResult(result)
}

func exportCASResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrExportClaimLost
	}
	return nil
}

func pointerInt64(value int64) *int64    { return &value }
func pointerString(value string) *string { return &value }

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
