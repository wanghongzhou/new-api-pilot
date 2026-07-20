package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/dto"
	"sort"
	"unicode/utf8"
)

var ErrUpstreamTaskSnapshotTooLarge = errors.New("upstream task snapshot exceeds supported limit")

type SiteUpstreamTask struct {
	ID, SiteID, RemoteID, RemoteCreatedAt, RemoteUpdatedAt     int64
	TaskID, Platform                                           string
	RemoteUserID                                               int64
	RemoteGroup                                                string
	RemoteChannelID, Quota                                     int64
	Action, RemoteStatus                                       string
	SubmitTime, StartTime, FinishTime                          int64
	Progress, ModelName, SourceHash                            string
	ConfigVersion                                              int
	FirstSeenAt, LastSeenAt, CollectedAt, CreatedAt, UpdatedAt int64
}

func (SiteUpstreamTask) TableName() string { return "site_upstream_task" }

type SiteUpstreamTaskCollectionState struct {
	SiteID, OverlapStart         int64
	LastSuccessAt, LastFailureAt *int64
	LastErrorCode                string
	ObservedCount                int64
	ConfigVersion                int
	UpdatedAt                    int64
}

func (SiteUpstreamTaskCollectionState) TableName() string {
	return "site_upstream_task_collection_state"
}
func (r *SiteRepository) MarkUpstreamTaskCollectionFailure(ctx context.Context, site Site, observedAt int64, code string) error {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || code == "" {
		return errors.New("invalid upstream task failure")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, site.ID).Error; err != nil {
			return err
		}
		if current.ConfigVersion != site.ConfigVersion {
			return ErrSiteRunConfigChanged
		}
		row := SiteUpstreamTaskCollectionState{SiteID: site.ID, LastFailureAt: &observedAt, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
	})
}
func UpstreamTaskTerminal(status string) bool { return status == "SUCCESS" || status == "FAILURE" }

func (r *SiteRepository) ListUnfinishedUpstreamTaskIDs(ctx context.Context, siteID int64) ([]string, error) {
	if r == nil || r.db == nil || siteID <= 0 {
		return nil, errors.New("invalid task site")
	}
	var ids []string
	err := r.db.WithContext(ctx).Model(&SiteUpstreamTask{}).Where("site_id=? AND remote_status NOT IN ?", siteID, []string{"SUCCESS", "FAILURE"}).Order("remote_id").Limit(100001).Pluck("task_id", &ids).Error
	if err != nil {
		return nil, err
	}
	if len(ids) > 100000 {
		return nil, ErrUpstreamTaskSnapshotTooLarge
	}
	return ids, nil
}
func taskSourceHash(item dto.UpstreamTask) (string, error) {
	payload, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
func validTaskText(v string, max int, required bool) bool {
	return utf8.ValidString(v) && len(v) <= max && (!required || v != "")
}
func (r *SiteRepository) SyncUpstreamTasks(ctx context.Context, site Site, observedAt, overlapStart int64, snapshot dto.UpstreamTaskSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || overlapStart <= 0 || overlapStart >= observedAt || len(snapshot.Items) > 100000 {
		return 0, errors.New("invalid upstream task snapshot")
	}
	items := append([]dto.UpstreamTask{}, snapshot.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var written int64
	for i, item := range items {
		if item.ID <= 0 || item.CreatedAt < 0 || item.UpdatedAt < item.CreatedAt || item.UserID < 0 || item.ChannelID < 0 || item.Quota < 0 || item.SubmitTime < 0 || item.StartTime < 0 || item.FinishTime < 0 || !validTaskText(item.TaskID, 191, true) || !validTaskText(item.Platform, 30, false) || !validTaskText(item.Group, 50, false) || !validTaskText(item.Action, 40, false) || !validTaskText(item.Status, 20, true) || !validTaskText(item.Progress, 20, false) || !validTaskText(item.Properties.Model, 255, false) || i > 0 && items[i-1].ID == item.ID {
			return 0, errors.New("invalid upstream task observation")
		}
		hash, err := taskSourceHash(item)
		if err != nil {
			return 0, err
		}
		var existing SiteUpstreamTask
		findErr := r.db.WithContext(ctx).Where("site_id=? AND remote_id=?", site.ID, item.ID).Take(&existing).Error
		if findErr == nil {
			if item.UpdatedAt < existing.RemoteUpdatedAt || item.UpdatedAt == existing.RemoteUpdatedAt && hash == existing.SourceHash {
				if err := r.db.WithContext(ctx).Model(&existing).UpdateColumns(map[string]any{"last_seen_at": observedAt, "collected_at": observedAt}).Error; err != nil {
					return 0, err
				}
				continue
			}
			updates := map[string]any{"remote_created_at": item.CreatedAt, "remote_updated_at": item.UpdatedAt, "task_id": item.TaskID, "platform": item.Platform, "remote_user_id": item.UserID, "remote_group": item.Group, "remote_channel_id": item.ChannelID, "quota": item.Quota, "action": item.Action, "remote_status": item.Status, "submit_time": item.SubmitTime, "start_time": item.StartTime, "finish_time": item.FinishTime, "progress": item.Progress, "model_name": item.Properties.Model, "source_hash": hash, "config_version": site.ConfigVersion, "last_seen_at": observedAt, "collected_at": observedAt, "updated_at": observedAt}
			if err := r.db.WithContext(ctx).Model(&existing).UpdateColumns(updates).Error; err != nil {
				return 0, err
			}
			written++
			continue
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return 0, findErr
		}
		row := SiteUpstreamTask{SiteID: site.ID, RemoteID: item.ID, RemoteCreatedAt: item.CreatedAt, RemoteUpdatedAt: item.UpdatedAt, TaskID: item.TaskID, Platform: item.Platform, RemoteUserID: item.UserID, RemoteGroup: item.Group, RemoteChannelID: item.ChannelID, Quota: item.Quota, Action: item.Action, RemoteStatus: item.Status, SubmitTime: item.SubmitTime, StartTime: item.StartTime, FinishTime: item.FinishTime, Progress: item.Progress, ModelName: item.Properties.Model, SourceHash: hash, ConfigVersion: site.ConfigVersion, FirstSeenAt: observedAt, LastSeenAt: observedAt, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return 0, err
		}
		written++
	}
	state := SiteUpstreamTaskCollectionState{SiteID: site.ID, OverlapStart: overlapStart, LastSuccessAt: &observedAt, ObservedCount: int64(len(items)), ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"overlap_start", "last_success_at", "last_error_code", "observed_count", "config_version", "updated_at"})}).Create(&state).Error; err != nil {
		return 0, err
	}
	return written, nil
}
func (r *SiteRepository) DeleteTerminalUpstreamTasksBefore(ctx context.Context, cutoff int64) error {
	if cutoff <= 0 {
		return errors.New("invalid task retention cutoff")
	}
	return r.db.WithContext(ctx).Where("remote_status IN ? AND finish_time>0 AND finish_time<?", []string{"SUCCESS", "FAILURE"}, cutoff).Delete(&SiteUpstreamTask{}).Error
}

type UpstreamTaskReadRow struct {
	SiteUpstreamTask
	SiteName string
}
type UpstreamTaskMetricRow struct {
	DimensionID, DimensionName                                   string
	SiteID                                                       int64
	SiteName                                                     string
	Total, Queued, Running, Success, Failure                     int64
	QueueSum, QueueCount, RunSum, RunCount, TotalSum, TotalCount int64
	AsOf                                                         *int64
}
type UpstreamTaskRepository struct{ db *gorm.DB }

func NewUpstreamTaskRepository(db *gorm.DB) *UpstreamTaskRepository {
	return &UpstreamTaskRepository{db: db}
}
func (r *UpstreamTaskRepository) CollectionStatuses(ctx context.Context, siteIDs []int64) (map[int64]string, string, error) {
	siteDB := r.db.WithContext(ctx).Model(&Site{}).Where("management_status=?", "active")
	if len(siteIDs) > 0 {
		siteDB = siteDB.Where("id IN ?", siteIDs)
	}
	var expected int64
	if err := siteDB.Count(&expected).Error; err != nil {
		return nil, "", err
	}
	var states []SiteUpstreamTaskCollectionState
	db := r.db.WithContext(ctx)
	if len(siteIDs) > 0 {
		db = db.Where("site_id IN ?", siteIDs)
	}
	if err := db.Find(&states).Error; err != nil {
		return nil, "", err
	}
	out := map[int64]string{}
	complete, failed := 0, 0
	for _, state := range states {
		status := "complete"
		if state.LastSuccessAt == nil {
			status = "pending"
		}
		if state.LastFailureAt != nil && (state.LastSuccessAt == nil || *state.LastFailureAt > *state.LastSuccessAt) {
			status = "unavailable"
			failed++
		} else if status == "complete" {
			complete++
		}
		out[state.SiteID] = status
	}
	overall := "complete"
	if expected == 0 || len(states) == 0 {
		overall = "pending"
	} else if int64(complete) == expected {
		overall = "complete"
	} else if int64(failed) == expected {
		overall = "unavailable"
	} else {
		overall = "partial"
	}
	return out, overall, nil
}
func applyTaskFilters(db *gorm.DB, q dto.UpstreamTaskQuery, a string) *gorm.DB {
	if len(q.SiteIDs) > 0 {
		db = db.Where(a+".site_id IN ?", q.SiteIDs)
	}
	if q.RemoteID != nil {
		db = db.Where(a+".remote_id=?", *q.RemoteID)
	}
	if q.RemoteUserID != nil {
		db = db.Where(a+".remote_user_id=?", *q.RemoteUserID)
	}
	if q.RemoteChannelID != nil {
		db = db.Where(a+".remote_channel_id=?", *q.RemoteChannelID)
	}
	if q.TaskID != "" {
		db = db.Where(a+".task_id=?", q.TaskID)
	}
	if len(q.Platforms) > 0 {
		db = db.Where(a+".platform IN ?", q.Platforms)
	}
	if len(q.Groups) > 0 {
		db = db.Where(a+".remote_group IN ?", q.Groups)
	}
	if len(q.Actions) > 0 {
		db = db.Where(a+".action IN ?", q.Actions)
	}
	if len(q.Statuses) > 0 {
		db = db.Where(a+".remote_status IN ?", q.Statuses)
	}
	if len(q.Models) > 0 {
		db = db.Where(a+".model_name IN ?", q.Models)
	}
	if q.StartTimestamp > 0 {
		db = db.Where(a+".submit_time>=?", q.StartTimestamp)
	}
	if q.EndTimestamp > 0 {
		db = db.Where(a+".submit_time<?", q.EndTimestamp)
	}
	return db
}
func (r *UpstreamTaskRepository) List(ctx context.Context, q dto.UpstreamTaskQuery) ([]UpstreamTaskReadRow, int64, error) {
	db := applyTaskFilters(r.db.WithContext(ctx).Table("site_upstream_task t").Joins("JOIN site s ON s.id=t.site_id"), q, "t")
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []UpstreamTaskReadRow
	err := db.Select("t.*,s.name site_name").Order("t.site_id,t.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *UpstreamTaskRepository) Metrics(ctx context.Context, q dto.UpstreamTaskQuery, dim string) ([]UpstreamTaskMetricRow, error) {
	expr := map[string]string{"summary": "'summary'", "status": "t.remote_status", "platform": "t.platform", "action": "t.action", "model": "t.model_name", "site": "CAST(t.site_id AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid task dimension")
	}
	db := applyTaskFilters(r.db.WithContext(ctx).Table("site_upstream_task t").Joins("JOIN site s ON s.id=t.site_id"), q, "t")
	siteID, siteName := "0", "''"
	group := expr
	if dim == "site" {
		siteID = "t.site_id"
		siteName = "MAX(s.name)"
		group = "t.site_id"
	}
	sql := expr + " dimension_id," + expr + " dimension_name," + siteID + " site_id," + siteName + " site_name,COUNT(*) total,SUM(t.remote_status IN ('NOT_START','SUBMITTED','QUEUED')) queued,SUM(t.remote_status='IN_PROGRESS') running,SUM(t.remote_status='SUCCESS') success,SUM(t.remote_status='FAILURE') failure,COALESCE(SUM(CASE WHEN t.start_time>=t.submit_time AND t.start_time>0 THEN t.start_time-t.submit_time ELSE 0 END),0) queue_sum,SUM(t.start_time>=t.submit_time AND t.start_time>0) queue_count,COALESCE(SUM(CASE WHEN t.finish_time>=t.start_time AND t.finish_time>0 AND t.start_time>0 THEN t.finish_time-t.start_time ELSE 0 END),0) run_sum,SUM(t.finish_time>=t.start_time AND t.finish_time>0 AND t.start_time>0) run_count,COALESCE(SUM(CASE WHEN t.finish_time>=t.submit_time AND t.finish_time>0 THEN t.finish_time-t.submit_time ELSE 0 END),0) total_sum,SUM(t.finish_time>=t.submit_time AND t.finish_time>0) total_count,MAX(t.updated_at) as_of"
	db = db.Select(sql)
	if dim != "summary" {
		db = db.Group(group)
	}
	var rows []UpstreamTaskMetricRow
	err := db.Order("site_id,dimension_id").Scan(&rows).Error
	return rows, err
}
