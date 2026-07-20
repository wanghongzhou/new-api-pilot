package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

var (
	ErrExportInvalid      = errors.New("export request is invalid")
	ErrExportNotFound     = errors.New("export job was not found")
	ErrExportLimitReached = errors.New("export active task limit was reached")
	ErrExportDiskLow      = errors.New("export disk free space is below the configured threshold")
	ErrExportNotReady     = errors.New("export file is not ready")
	ErrExportExpired      = errors.New("export file has expired")
	ErrExportFileMissing  = errors.New("export file is missing")
	ErrExportContract     = errors.New("export persistence contract is invalid")
)

type ExportDiskLowError struct {
	FreeBytes      uint64
	ThresholdBytes int64
}

func (err *ExportDiskLowError) Error() string { return ErrExportDiskLow.Error() }
func (err *ExportDiskLowError) Unwrap() error { return ErrExportDiskLow }

type ExportDiskFreeFunc func(string) (uint64, error)

type ExportServiceOptions struct {
	Database  *gorm.DB
	Clock     common.Clock
	ExportDir string
	DiskFree  ExportDiskFreeFunc
}

type ExportService struct {
	repository *model.ExportRepository
	clock      common.Clock
	exportDir  string
	diskFree   ExportDiskFreeFunc
}

type ExportDownload struct {
	File               *os.File
	Name               string
	ContentType        string
	ContentDisposition string
	Size               int64
}

func NewExportService(options ExportServiceOptions) (*ExportService, error) {
	if options.Database == nil || options.Clock == nil || strings.TrimSpace(options.ExportDir) == "" {
		return nil, errors.New("export service dependencies are required")
	}
	directory, err := secureExportDirectory(options.ExportDir)
	if err != nil {
		return nil, err
	}
	diskFree := options.DiskFree
	if diskFree == nil {
		diskFree = availableExportDiskBytes
	}
	return &ExportService{
		repository: model.NewExportRepository(options.Database),
		clock:      options.Clock,
		exportDir:  directory,
		diskFree:   diskFree,
	}, nil
}

func (service *ExportService) Create(
	ctx context.Context,
	userID string,
	request dto.ExportCreateRequest,
) (dto.ExportJobItem, error) {
	ownerID, valid := canonicalExportOwnerID(userID)
	request.Normalize()
	if service == nil || service.repository == nil || service.clock == nil || !valid || request.Validate() != nil {
		return dto.ExportJobItem{}, ErrExportInvalid
	}
	filtersJSON, err := json.Marshal(request.Filters)
	if err != nil {
		return dto.ExportJobItem{}, ErrExportInvalid
	}
	filterDigest := sha256.Sum256(filtersJSON)
	filterHash := hex.EncodeToString(filterDigest[:])
	activeKey := fmt.Sprintf("%d:%s:%s:%s", ownerID, request.Format, request.StatisticsType, filterHash)
	now := service.clock.Now().Unix()
	var job model.ExportJob
	deduplicated := false
	err = service.repository.Transaction(ctx, func(repository *model.ExportRepository) error {
		settings, lockErr := repository.LoadSettingsForUpdate(ctx)
		if lockErr != nil {
			return lockErr
		}
		existing, findErr := repository.FindActiveByKey(ctx, activeKey)
		if findErr == nil {
			job = existing
			deduplicated = true
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		perUser, global, countErr := repository.CountActive(ctx, ownerID)
		if countErr != nil {
			return countErr
		}
		if perUser >= int64(settings.MaxActivePerUser) || global >= int64(settings.MaxActiveGlobal) {
			return ErrExportLimitReached
		}
		freeBytes, diskErr := service.diskFree(service.exportDir)
		if diskErr != nil {
			return diskErr
		}
		if freeBytes < uint64(settings.MinFreeDiskBytes) {
			return &ExportDiskLowError{FreeBytes: freeBytes, ThresholdBytes: settings.MinFreeDiskBytes}
		}
		job = model.ExportJob{
			UserID: ownerID, Format: request.Format, StatisticsType: request.StatisticsType,
			Filters: filtersJSON, FilterHash: filterHash, ActiveKey: &activeKey,
			Status: dto.ExportStatusPending, Progress: 0, AttemptCount: 0,
			NextAttemptAt: now, FileSize: 0, RowCount: 0, CreatedAt: now, UpdatedAt: now,
		}
		if createErr := repository.Create(ctx, &job); createErr != nil {
			if duplicateExportKey(createErr) {
				existing, readErr := repository.FindActiveByKey(ctx, activeKey)
				if readErr == nil {
					job = existing
					deduplicated = true
					return nil
				}
			}
			return createErr
		}
		return nil
	})
	if err != nil {
		return dto.ExportJobItem{}, err
	}
	item, err := exportJobItem(job)
	if err != nil {
		return dto.ExportJobItem{}, err
	}
	item.Deduplicated = deduplicated
	return item, nil
}

func (service *ExportService) List(
	ctx context.Context,
	userID string,
	query dto.ExportListQuery,
) (common.PageData[dto.ExportJobItem], error) {
	ownerID, valid := canonicalExportOwnerID(userID)
	query.Normalize()
	if service == nil || service.repository == nil || !valid || query.Validate() != nil {
		return common.PageData[dto.ExportJobItem]{}, ErrExportInvalid
	}
	jobs, total, err := service.repository.ListForUser(
		ctx, ownerID, query.Statuses, query.Format, query.StatisticsType,
		query.SortBy, query.SortOrder, query.PageSize, query.Offset(),
	)
	if err != nil {
		return common.PageData[dto.ExportJobItem]{}, err
	}
	items := make([]dto.ExportJobItem, 0, len(jobs))
	for _, job := range jobs {
		item, itemErr := exportJobItem(job)
		if itemErr != nil {
			return common.PageData[dto.ExportJobItem]{}, itemErr
		}
		items = append(items, item)
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *ExportService) Get(ctx context.Context, userID string, id int64) (dto.ExportJobItem, error) {
	ownerID, valid := canonicalExportOwnerID(userID)
	if service == nil || service.repository == nil || !valid || id <= 0 {
		return dto.ExportJobItem{}, ErrExportInvalid
	}
	job, err := service.repository.GetForUser(ctx, id, ownerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.ExportJobItem{}, ErrExportNotFound
	}
	if err != nil {
		return dto.ExportJobItem{}, err
	}
	return exportJobItem(job)
}

func (service *ExportService) OpenDownload(ctx context.Context, userID string, id int64) (ExportDownload, error) {
	ownerID, valid := canonicalExportOwnerID(userID)
	if service == nil || service.repository == nil || !valid || id <= 0 {
		return ExportDownload{}, ErrExportInvalid
	}
	job, err := service.repository.GetForUser(ctx, id, ownerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ExportDownload{}, ErrExportNotFound
	}
	if err != nil {
		return ExportDownload{}, err
	}
	now := service.clock.Now().Unix()
	if job.Status == dto.ExportStatusExpired || (job.ExpiresAt != nil && *job.ExpiresAt <= now) {
		removed := false
		if job.FilePath != nil {
			removed = service.removeManagedFile(*job.FilePath) == nil
		}
		if job.Status == dto.ExportStatusSuccess {
			_ = service.repository.MarkExpired(ctx, id, ownerID, now)
		}
		if removed {
			_ = service.repository.ClearArtifactPath(ctx, id, *job.FilePath, now)
		}
		return ExportDownload{}, ErrExportExpired
	}
	if job.Status != dto.ExportStatusSuccess || job.FilePath == nil || job.FileName == nil {
		return ExportDownload{}, ErrExportNotReady
	}
	path, pathErr := service.managedPath(*job.FilePath)
	if pathErr != nil {
		return ExportDownload{}, service.markMissing(ctx, id, ownerID, now)
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ExportDownload{}, service.markMissing(ctx, id, ownerID, now)
	}
	file, openErr := os.Open(path)
	if openErr != nil {
		return ExportDownload{}, service.markMissing(ctx, id, ownerID, now)
	}
	name := filepath.Base(*job.FileName)
	if name != *job.FileName || !validExportDownloadName(name, job.Format) {
		_ = file.Close()
		return ExportDownload{}, ErrExportContract
	}
	contentType := "text/csv; charset=utf-8"
	if job.Format == dto.ExportFormatXLSX {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return ExportDownload{
		File: file, Name: name, ContentType: contentType,
		ContentDisposition: disposition, Size: info.Size(),
	}, nil
}

func (service *ExportService) markMissing(ctx context.Context, id, ownerID, now int64) error {
	params, _ := json.Marshal(map[string]any{"export_id": strconv.FormatInt(id, 10)})
	if err := service.repository.MarkFileMissing(ctx, id, ownerID, now, params); err != nil &&
		!errors.Is(err, model.ErrExportClaimLost) {
		return err
	}
	return ErrExportFileMissing
}

func (service *ExportService) managedPath(stored string) (string, error) {
	if stored == "" || filepath.Base(stored) != stored || strings.ContainsAny(stored, `/\`) {
		return "", ErrExportContract
	}
	path := filepath.Join(service.exportDir, stored)
	relative, err := filepath.Rel(service.exportDir, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", ErrExportContract
	}
	return path, nil
}

func (service *ExportService) removeManagedFile(stored string) error {
	path, err := service.managedPath(stored)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrExportContract
	}
	return os.Remove(path)
}

func exportJobItem(job model.ExportJob) (dto.ExportJobItem, error) {
	var filters dto.ExportFilters
	if len(job.Filters) == 0 || json.Unmarshal(job.Filters, &filters) != nil {
		return dto.ExportJobItem{}, ErrExportContract
	}
	filters.Normalize()
	item := dto.ExportJobItem{
		ID: strconv.FormatInt(job.ID, 10), Format: job.Format, StatisticsType: job.StatisticsType,
		Filters: filters, Status: job.Status, Progress: job.Progress,
		FileSize: strconv.FormatInt(job.FileSize, 10), RowCount: strconv.FormatInt(job.RowCount, 10),
		DataSnapshotAt: job.DataSnapshotAt, ExpiresAt: job.ExpiresAt, CreatedAt: job.CreatedAt,
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
	}
	if job.FileName != nil {
		item.FileName = *job.FileName
	}
	if !dtoExportJobContract(item) {
		return dto.ExportJobItem{}, ErrExportContract
	}
	if job.ErrorCode != "" {
		params := map[string]any{}
		if len(job.ErrorParams) == 0 || json.Unmarshal(job.ErrorParams, &params) != nil {
			return dto.ExportJobItem{}, ErrExportContract
		}
		detail := ""
		if job.ErrorMessage != nil {
			detail = *job.ErrorMessage
		}
		message, err := dto.NewMessageRef(constant.MessageCode(job.ErrorCode), params, detail)
		if err != nil {
			return dto.ExportJobItem{}, ErrExportContract
		}
		item.Error = &message
	}
	return item, nil
}

func dtoExportJobContract(item dto.ExportJobItem) bool {
	return item.ID != "" &&
		(item.Format == dto.ExportFormatCSV || item.Format == dto.ExportFormatXLSX) &&
		(item.StatisticsType == dto.StatisticsScopeGlobal || item.StatisticsType == dto.StatisticsScopeSite ||
			item.StatisticsType == dto.StatisticsScopeCustomer || item.StatisticsType == dto.StatisticsScopeAccount ||
			item.StatisticsType == dto.StatisticsScopeModel || item.StatisticsType == dto.StatisticsScopeChannel ||
			item.StatisticsType == dto.StatisticsScopeGroup || item.StatisticsType == dto.StatisticsScopeToken ||
			item.StatisticsType == dto.StatisticsScopeNode) &&
		(item.Status == dto.ExportStatusPending || item.Status == dto.ExportStatusRunning || item.Status == dto.ExportStatusSuccess ||
			item.Status == dto.ExportStatusFailed || item.Status == dto.ExportStatusExpired) &&
		item.Progress >= 0 && item.Progress <= 100
}

func secureExportDirectory(raw string) (string, error) {
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve export directory: %w", err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("export directory must be an existing non-symlink directory")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || !sameExportPath(absolute, resolved) {
		return "", errors.New("export directory must not contain symlink components")
	}
	return absolute, nil
}

func SecureExportDirectory(raw string) (string, error) {
	return secureExportDirectory(raw)
}

func ExportArtifactPath(directory, stored string) (string, error) {
	resolved, err := secureExportDirectory(directory)
	if err != nil {
		return "", err
	}
	if stored == "" || filepath.Base(stored) != stored || strings.ContainsAny(stored, `/\`) {
		return "", ErrExportContract
	}
	path := filepath.Join(resolved, stored)
	relative, err := filepath.Rel(resolved, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", ErrExportContract
	}
	return path, nil
}

func RemoveExportArtifact(directory, stored string) error {
	if stored == "" {
		return nil
	}
	path, err := ExportArtifactPath(directory, stored)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrExportContract
	}
	return os.Remove(path)
}

func PublishExportArtifact(directory, temporary, final string) error {
	temporaryPath, err := ExportArtifactPath(directory, temporary)
	if err != nil {
		return err
	}
	finalPath, err := ExportArtifactPath(directory, final)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(finalPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return ErrExportWrite
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	return nil
}

func sameExportPath(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(first), filepath.Clean(second))
	}
	return filepath.Clean(first) == filepath.Clean(second)
}

func canonicalExportOwnerID(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func duplicateExportKey(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}

func validExportDownloadName(name, format string) bool {
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "\r\n/\\") {
		return false
	}
	extension := "." + format
	return strings.HasPrefix(name, "statistics-") && strings.HasSuffix(name, extension)
}

func CopyExportDownload(destination io.Writer, download ExportDownload) error {
	if destination == nil || download.File == nil {
		return errors.New("export download stream is not initialized")
	}
	_, err := io.Copy(destination, download.File)
	return err
}
