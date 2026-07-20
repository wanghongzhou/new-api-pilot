package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/dto"
)

type SiteSystemTask struct {
	ID, SiteID, RemoteID                                                      int64
	RemoteTaskID, TaskType, RemoteStatus                                      string
	ErrorPresent                                                              bool
	ErrorCode                                                                 string
	Total, Processed, Progress, Remaining, DeletedCount                       *int64
	Tested, Succeeded, Failed, Disabled, Enabled                              *int64
	CheckedChannels, ChangedChannels, DetectedAddModels, DetectedRemoveModels *int64
	FailedChannels, AutoAddedModels                                           *int64
	UnfinishedTasks, ChannelsScanned, PlatformsScanned, NullTasksFailed       *int64
	RemoteCreatedAt, RemoteUpdatedAt                                          int64
	SourceHash                                                                string
	ConfigVersion                                                             int
	FirstSeenAt, LastSeenAt, CollectedAt, CreatedAt, UpdatedAt                int64
}

func (SiteSystemTask) TableName() string { return "site_system_task" }

type SiteSystemTaskCollectionState struct {
	SiteID                             int64
	ResourceKind                       string
	DataStatus                         string
	Truncated, IDGap                   bool
	AsOf, LastSuccessAt, LastFailureAt *int64
	LastErrorCode                      string
	ObservedCount                      int64
	ConfigVersion                      int
	UpdatedAt                          int64
}

func (SiteSystemTaskCollectionState) TableName() string { return "site_system_task_collection_state" }

func systemTaskSourceHash(item dto.UpstreamSystemTask) string {
	payload, _ := json.Marshal(item)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
func SystemTaskTerminal(status string) bool { return status == "succeeded" || status == "failed" }

func (r *SiteRepository) MarkSystemTaskCollectionFailure(ctx context.Context, site Site, at int64, code string) error {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || code == "" {
		return errors.New("invalid system task failure")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, site.ID).Error; err != nil {
			return err
		}
		if current.ConfigVersion != site.ConfigVersion {
			return ErrSiteRunConfigChanged
		}
		row := SiteSystemTaskCollectionState{SiteID: site.ID, ResourceKind: "list", DataStatus: "unavailable", LastFailureAt: &at, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "resource_kind"}}, DoUpdates: clause.AssignmentColumns([]string{"data_status", "last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
	})
}

func systemTaskRow(site Site, at int64, item dto.UpstreamSystemTask) SiteSystemTask {
	return SiteSystemTask{SiteID: site.ID, RemoteID: item.ID, RemoteTaskID: item.TaskID, TaskType: item.Type, RemoteStatus: item.Status, ErrorPresent: item.ErrorPresent, ErrorCode: item.ErrorCode, Total: item.Total, Processed: item.Processed, Progress: item.Progress, Remaining: item.Remaining, DeletedCount: item.DeletedCount, Tested: item.Tested, Succeeded: item.Succeeded, Failed: item.Failed, Disabled: item.Disabled, Enabled: item.Enabled, CheckedChannels: item.CheckedChannels, ChangedChannels: item.ChangedChannels, DetectedAddModels: item.DetectedAddModels, DetectedRemoveModels: item.DetectedRemoveModels, FailedChannels: item.FailedChannels, AutoAddedModels: item.AutoAddedModels, UnfinishedTasks: item.UnfinishedTasks, ChannelsScanned: item.ChannelsScanned, PlatformsScanned: item.PlatformsScanned, NullTasksFailed: item.NullTasksFailed, RemoteCreatedAt: item.CreatedAt, RemoteUpdatedAt: item.UpdatedAt, SourceHash: systemTaskSourceHash(item), ConfigVersion: site.ConfigVersion, FirstSeenAt: at, LastSeenAt: at, CollectedAt: at, CreatedAt: at, UpdatedAt: at}
}

func (r *SiteRepository) SyncSystemTasks(ctx context.Context, site Site, at int64, snapshot dto.UpstreamSystemTaskSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || len(snapshot.Items) > 105 {
		return 0, errors.New("invalid system task snapshot")
	}
	items := append([]dto.UpstreamSystemTask{}, snapshot.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var written int64
	for index, item := range items {
		if item.ID <= 0 || item.TaskID == "" || item.CreatedAt < 0 || item.UpdatedAt < item.CreatedAt || index > 0 && items[index-1].ID == item.ID {
			return 0, errors.New("invalid system task observation")
		}
		incoming := systemTaskRow(site, at, item)
		var existing SiteSystemTask
		err := r.db.WithContext(ctx).Where("site_id=? AND remote_id=?", site.ID, item.ID).Take(&existing).Error
		if err == nil {
			if item.UpdatedAt < existing.RemoteUpdatedAt || item.UpdatedAt == existing.RemoteUpdatedAt && incoming.SourceHash == existing.SourceHash {
				if e := r.db.WithContext(ctx).Model(&existing).UpdateColumns(map[string]any{"last_seen_at": at, "collected_at": at}).Error; e != nil {
					return written, e
				}
				continue
			}
			incoming.ID = existing.ID
			incoming.FirstSeenAt = existing.FirstSeenAt
			incoming.CreatedAt = existing.CreatedAt
			if e := r.db.WithContext(ctx).Save(&incoming).Error; e != nil {
				return written, e
			}
			written++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return written, err
		}
		if e := r.db.WithContext(ctx).Create(&incoming).Error; e != nil {
			return written, e
		}
		written++
	}
	listStatus := "complete"
	if snapshot.Truncated || snapshot.IDGap {
		listStatus = "partial"
	}
	observed := snapshot.ListObservedCount
	if observed == 0 && len(items) > 0 {
		observed = int64(len(items))
	}
	states := []SiteSystemTaskCollectionState{{SiteID: site.ID, ResourceKind: "list", DataStatus: listStatus, Truncated: snapshot.Truncated, IDGap: snapshot.IDGap, AsOf: &at, LastSuccessAt: &at, ObservedCount: observed, ConfigVersion: site.ConfigVersion, UpdatedAt: at}}
	failures := map[string]bool{}
	for _, kind := range snapshot.CurrentFailures {
		failures[kind] = true
	}
	for _, kind := range []string{"log_cleanup", "channel_test", "model_update", "midjourney_poll", "async_task_poll"} {
		state := SiteSystemTaskCollectionState{SiteID: site.ID, ResourceKind: "current:" + kind, DataStatus: "complete", AsOf: &at, LastSuccessAt: &at, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
		if failures[kind] {
			state.DataStatus = "unavailable"
			state.LastSuccessAt = nil
			state.LastFailureAt = &at
			state.LastErrorCode = "SYSTEM_TASK_CURRENT_UNAVAILABLE"
		}
		for _, item := range items {
			if item.Type == kind && (item.Status == "pending" || item.Status == "running") {
				state.ObservedCount = 1
				break
			}
		}
		states = append(states, state)
	}
	for _, state := range states {
		updates:=[]string{"data_status","truncated","id_gap","as_of","last_success_at","last_error_code","observed_count","config_version","updated_at"}
		if state.DataStatus=="unavailable"{updates=[]string{"data_status","last_failure_at","last_error_code","config_version","updated_at"}}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "resource_kind"}}, DoUpdates: clause.AssignmentColumns(updates)}).Create(&state).Error; err != nil {
			return written, err
		}
	}
	return written, nil
}

func (r *SiteRepository) DeleteTerminalSystemTasksBefore(ctx context.Context, cutoff int64) error {
	if cutoff <= 0 {
		return errors.New("invalid system task retention cutoff")
	}
	return r.db.WithContext(ctx).Where("remote_status IN ? AND remote_updated_at<?", []string{"succeeded", "failed"}, cutoff).Delete(&SiteSystemTask{}).Error
}

type SystemTaskReadRow struct {
	SiteSystemTask
	SiteName string
}
type SystemTaskMetricRow struct {
	DimensionID, DimensionName                     string
	SiteID                                         int64
	SiteName                                       string
	Total, Active, Succeeded, Failed, ErrorPresent int64
	AsOf                                           *int64
}
type SystemTaskRepository struct{ db *gorm.DB }
type SystemTaskSiteCollectionState struct {
	DataStatus       string
	Truncated, IDGap bool
	AsOf             *int64
	ObservedCount    int64
}

func NewSystemTaskRepository(db *gorm.DB) *SystemTaskRepository { return &SystemTaskRepository{db: db} }

func (r *SystemTaskRepository) CollectionStatuses(ctx context.Context, siteIDs []int64) (map[int64]SystemTaskSiteCollectionState, string, *int64, error) {
	sites := r.db.WithContext(ctx).Model(&Site{}).Where("management_status='active'")
	if len(siteIDs) > 0 {
		sites = sites.Where("id IN ?", siteIDs)
	}
	var expected int64
	if err := sites.Count(&expected).Error; err != nil {
		return nil, "", nil, err
	}
	var states []SiteSystemTaskCollectionState
	db := r.db.WithContext(ctx)
	if len(siteIDs) > 0 {
		db = db.Where("site_id IN ?", siteIDs)
	}
	if err := db.Find(&states).Error; err != nil {
		return nil, "", nil, err
	}
	grouped := map[int64][]SiteSystemTaskCollectionState{}
	for _, state := range states {
		grouped[state.SiteID] = append(grouped[state.SiteID], state)
	}
	out := map[int64]SystemTaskSiteCollectionState{}
	complete, unavailable := 0, 0
	var asOf *int64
	for siteID, resources := range grouped {
		summary := SystemTaskSiteCollectionState{DataStatus: "partial"}
		completeResources, unavailableResources := 0, 0
		for _, state := range resources {
			if state.DataStatus == "complete" {
				completeResources++
			}
			if state.DataStatus == "unavailable" {
				unavailableResources++
			}
			if state.ResourceKind == "list" {
				summary.Truncated = state.Truncated
				summary.IDGap = state.IDGap
				summary.ObservedCount = state.ObservedCount
			}
			if state.AsOf != nil && (summary.AsOf == nil || *state.AsOf > *summary.AsOf) {
				v := *state.AsOf
				summary.AsOf = &v
			}
			if state.AsOf != nil && (asOf == nil || *state.AsOf > *asOf) {
				v := *state.AsOf
				asOf = &v
			}
		}
		if len(resources) == 6 && completeResources == 6 {
			summary.DataStatus = "complete"
			complete++
		} else if unavailableResources == len(resources) && len(resources) == 6 {
			summary.DataStatus = "unavailable"
			unavailable++
		}
		out[siteID] = summary
	}
	overall := "partial"
	if expected == 0 || len(states) == 0 {
		overall = "pending"
	} else if int64(complete) == expected {
		overall = "complete"
	} else if int64(unavailable) == expected {
		overall = "unavailable"
	}
	return out, overall, asOf, nil
}

func applySystemTaskFilters(db *gorm.DB, q dto.SystemTaskQuery, a string) *gorm.DB {
	if len(q.SiteIDs) > 0 {
		db = db.Where(a+".site_id IN ?", q.SiteIDs)
	}
	if len(q.Types) > 0 {
		db = db.Where(a+".task_type IN ?", q.Types)
	}
	if len(q.Statuses) > 0 {
		db = db.Where(a+".remote_status IN ?", q.Statuses)
	}
	if q.ErrorPresent != nil {
		db = db.Where(a+".error_present=?", *q.ErrorPresent)
	}
	if q.CreatedStart > 0 {
		db = db.Where(a+".remote_created_at>=?", q.CreatedStart)
	}
	if q.CreatedEnd > 0 {
		db = db.Where(a+".remote_created_at<?", q.CreatedEnd)
	}
	return db
}
func (r *SystemTaskRepository) List(ctx context.Context, q dto.SystemTaskQuery) ([]SystemTaskReadRow, int64, error) {
	db := applySystemTaskFilters(r.db.WithContext(ctx).Table("site_system_task t").Joins("JOIN site s ON s.id=t.site_id"), q, "t")
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SystemTaskReadRow
	err := db.Select("t.*,s.name site_name").Order("t.site_id,t.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *SystemTaskRepository) Metrics(ctx context.Context, q dto.SystemTaskQuery, dim string) ([]SystemTaskMetricRow, error) {
	expr := map[string]string{"summary": "'summary'", "type": "t.task_type", "status": "t.remote_status", "site": "CAST(t.site_id AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid system task dimension")
	}
	db := applySystemTaskFilters(r.db.WithContext(ctx).Table("site_system_task t").Joins("JOIN site s ON s.id=t.site_id"), q, "t")
	siteID, siteName, group := "0", "''", expr
	if dim == "site" {
		siteID, siteName, group = "t.site_id", "MAX(s.name)", "t.site_id"
	}
	db = db.Select(expr + " dimension_id," + expr + " dimension_name," + siteID + " site_id," + siteName + " site_name,COUNT(*) total,SUM(t.remote_status IN ('pending','running')) active,SUM(t.remote_status='succeeded') succeeded,SUM(t.remote_status='failed') failed,SUM(t.error_present=1) error_present,MAX(t.updated_at) as_of")
	if dim != "summary" {
		db = db.Group(group)
	}
	var rows []SystemTaskMetricRow
	err := db.Order("site_id,dimension_id").Scan(&rows).Error
	return rows, err
}
