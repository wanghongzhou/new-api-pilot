package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSiteInstanceLifecycleContract = errors.New("site instance lifecycle contract is invalid")

type SiteCapability struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID        int64  `gorm:"column:site_id"`
	CapabilityKey string `gorm:"column:capability_key"`
	Status        string `gorm:"column:status"`
	MessageCode   string `gorm:"column:message_code"`
	MessageParams []byte `gorm:"column:message_params;type:json"`
	CheckedAt     int64  `gorm:"column:checked_at"`
}

func (SiteCapability) TableName() string { return "site_capability" }

type SiteChannel struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	RemoteChannelID  int64  `gorm:"column:remote_channel_id"`
	Name             string `gorm:"column:name"`
	LastSyncedAt     int64  `gorm:"column:last_synced_at"`
	RemoteMissing    bool   `gorm:"column:remote_missing"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
	RemoteType       int    `gorm:"-"`
	RemoteStatus     int32  `gorm:"-"`
	TestTime         int64  `gorm:"-"`
	ResponseTimeMS   int64  `gorm:"-"`
	Balance          string `gorm:"-"`
	BalanceUpdatedAt int64  `gorm:"-"`
	Models           string `gorm:"-"`
	RemoteGroup      string `gorm:"-"`
	UsedQuota        int64  `gorm:"-"`
	Priority         int64  `gorm:"-"`
	Weight           int64  `gorm:"-"`
	AutoBan          int    `gorm:"-"`
	Tag              string `gorm:"-"`
}

func (SiteChannel) TableName() string { return "site_channel" }

type SiteInstance struct {
	ID                        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID                    int64  `gorm:"column:site_id"`
	NodeName                  string `gorm:"column:node_name"`
	Hostname                  string `gorm:"column:hostname"`
	IsMaster                  bool   `gorm:"column:is_master"`
	RuntimeVersion            string `gorm:"column:runtime_version"`
	GOOS                      string `gorm:"column:goos"`
	GOARCH                    string `gorm:"column:goarch"`
	UpstreamStatus            string `gorm:"column:upstream_status"`
	UpstreamStaleAfterSeconds *int64 `gorm:"column:upstream_stale_after_seconds"`
	CurrentStatus             string `gorm:"column:current_status"`
	FirstSeenAt               int64  `gorm:"column:first_seen_at"`
	StartedAt                 *int64 `gorm:"column:started_at"`
	LastSeenAt                *int64 `gorm:"column:last_seen_at"`
	LastSyncedAt              int64  `gorm:"column:last_synced_at"`
	CreatedAt                 int64  `gorm:"column:created_at"`
	UpdatedAt                 int64  `gorm:"column:updated_at"`
	RetiredAt                 *int64 `gorm:"column:retired_at"`
}

func (SiteInstance) TableName() string { return "site_instance" }

type SiteInstanceLifecycle struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID         int64  `gorm:"column:site_id"`
	NodeName       string `gorm:"column:node_name"`
	StartMinuteTS  int64  `gorm:"column:start_minute_ts"`
	EndMinuteTS    *int64 `gorm:"column:end_minute_ts"`
	EvidenceStatus string `gorm:"column:evidence_status"`
	CreatedAt      int64  `gorm:"column:created_at"`
	UpdatedAt      int64  `gorm:"column:updated_at"`
}

func (SiteInstanceLifecycle) TableName() string { return "site_instance_lifecycle" }

type SiteInstanceStatusMinutely struct {
	ID              int64    `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID          int64    `gorm:"column:site_id"`
	NodeName        string   `gorm:"column:node_name"`
	MinuteTS        int64    `gorm:"column:minute_ts"`
	Status          string   `gorm:"column:status"`
	CPUPercent      *float64 `gorm:"column:cpu_percent"`
	MemoryPercent   *float64 `gorm:"column:memory_percent"`
	DiskUsedPercent *float64 `gorm:"column:disk_used_percent"`
	DiskTotalBytes  *int64   `gorm:"column:disk_total_bytes"`
	DiskUsedBytes   *int64   `gorm:"column:disk_used_bytes"`
	StartedAt       *int64   `gorm:"column:started_at"`
	LastSeenAt      *int64   `gorm:"column:last_seen_at"`
	CreatedAt       int64    `gorm:"column:created_at"`
}

func (SiteInstanceStatusMinutely) TableName() string { return "site_instance_status_minutely" }

type SiteInstanceWrite struct {
	Instance SiteInstance
	Sample   SiteInstanceStatusMinutely
}

type SiteInstanceResourceState struct {
	Instance          SiteInstance
	PriorTwoNonOnline bool
}

type SiteInstanceSnapshot struct {
	SiteInstance
	SampleStatus    *string  `gorm:"column:sample_status"`
	SampledAt       *int64   `gorm:"column:sampled_at"`
	CPUPercent      *float64 `gorm:"column:cpu_percent"`
	MemoryPercent   *float64 `gorm:"column:memory_percent"`
	DiskUsedPercent *float64 `gorm:"column:disk_used_percent"`
	DiskTotalBytes  *int64   `gorm:"column:disk_total_bytes"`
	DiskUsedBytes   *int64   `gorm:"column:disk_used_bytes"`
}

// ListLatestResourceSummaries returns the newest site-wide resource sample for
// each requested site. Sites without a sample are intentionally absent.
func (repository *SiteRepository) ListLatestResourceSummaries(
	ctx context.Context,
	siteIDs []int64,
) (map[int64]SiteStatusMinutely, error) {
	result := make(map[int64]SiteStatusMinutely, len(siteIDs))
	if len(siteIDs) == 0 {
		return result, nil
	}
	var samples []SiteStatusMinutely
	err := repository.db.WithContext(ctx).Raw(`SELECT s.*
FROM site_status_minutely s
WHERE s.site_id IN ?
  AND s.minute_ts = (
    SELECT MAX(latest.minute_ts)
    FROM site_status_minutely latest
    WHERE latest.site_id = s.site_id
  )`, siteIDs).Scan(&samples).Error
	if err != nil {
		return nil, err
	}
	for _, sample := range samples {
		result[sample.SiteID] = sample
	}
	return result, nil
}

func (repository *SiteRepository) ListCapabilities(ctx context.Context, siteID int64) ([]SiteCapability, error) {
	var capabilities []SiteCapability
	err := repository.db.WithContext(ctx).Where("site_id = ?", siteID).Order("capability_key ASC").Find(&capabilities).Error
	return capabilities, err
}

func (repository *SiteRepository) ReplaceCapabilities(ctx context.Context, siteID int64, capabilities []SiteCapability) error {
	keys := make([]string, 0, len(capabilities))
	for index := range capabilities {
		capabilities[index].SiteID = siteID
		keys = append(keys, capabilities[index].CapabilityKey)
		if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "site_id"}, {Name: "capability_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "message_code", "message_params", "checked_at"}),
		}).Create(&capabilities[index]).Error; err != nil {
			return err
		}
	}
	query := repository.db.WithContext(ctx).Where("site_id = ?", siteID)
	if len(keys) > 0 {
		query = query.Where("capability_key NOT IN ?", keys)
	}
	return query.Delete(&SiteCapability{}).Error
}

func (repository *SiteRepository) UpsertCapabilities(ctx context.Context, siteID int64, capabilities []SiteCapability) error {
	for index := range capabilities {
		capabilities[index].SiteID = siteID
		if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "site_id"}, {Name: "capability_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "message_code", "message_params", "checked_at"}),
		}).Create(&capabilities[index]).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *SiteRepository) DeleteCapabilities(ctx context.Context, siteID int64) error {
	return repository.db.WithContext(ctx).Where("site_id = ?", siteID).Delete(&SiteCapability{}).Error
}

func (repository *SiteRepository) SyncChannels(ctx context.Context, siteID, syncedAt int64, channels []SiteChannel) error {
	if err := applySiteChannelInventorySnapshot(ctx, repository.db, siteID, syncedAt, channels); err != nil {
		return err
	}
	if err := repository.db.WithContext(ctx).Model(&SiteChannel{}).Where("site_id = ?", siteID).
		Updates(map[string]any{"remote_missing": true, "updated_at": syncedAt}).Error; err != nil {
		return err
	}
	for index := range channels {
		channels[index].SiteID = siteID
		channels[index].LastSyncedAt = syncedAt
		channels[index].RemoteMissing = false
		channels[index].UpdatedAt = syncedAt
		if channels[index].CreatedAt == 0 {
			channels[index].CreatedAt = syncedAt
		}
		if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "site_id"}, {Name: "remote_channel_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "last_synced_at", "remote_missing", "updated_at"}),
		}).Create(&channels[index]).Error; err != nil {
			return err
		}
	}
	var site Site
	if err := repository.db.WithContext(ctx).First(&site, siteID).Error; err != nil {
		return err
	}
	return repository.ReplaceChannelModelMappings(ctx, site, syncedAt, channels)
}

func (repository *SiteRepository) SyncInstances(ctx context.Context, writes []SiteInstanceWrite) error {
	for index := range writes {
		instance := &writes[index].Instance
		var existing SiteInstance
		existingErr := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id=? AND node_name=?", instance.SiteID, instance.NodeName).Take(&existing).Error
		if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		startKnown := errors.Is(existingErr, gorm.ErrRecordNotFound) || existing.RetiredAt != nil
		if !startKnown {
			var open SiteInstanceLifecycle
			if err := repository.db.WithContext(ctx).Where("site_id=? AND node_name=? AND end_minute_ts IS NULL", instance.SiteID, instance.NodeName).Take(&open).Error; err == nil && open.EvidenceStatus == "legacy_unknown" {
				if writes[index].Sample.MinuteTS <= 0 {
					return ErrSiteInstanceLifecycleContract
				}
				boundary := writes[index].Sample.MinuteTS - writes[index].Sample.MinuteTS%60
				if updateErr := repository.db.WithContext(ctx).Model(&open).Updates(map[string]any{"end_minute_ts": boundary, "updated_at": instance.UpdatedAt}).Error; updateErr != nil {
					return updateErr
				}
				startKnown = true
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if startKnown {
			startMinute := instance.FirstSeenAt - instance.FirstSeenAt%60
			if existing.RetiredAt != nil && writes[index].Sample.MinuteTS > 0 {
				startMinute = writes[index].Sample.MinuteTS - writes[index].Sample.MinuteTS%60
			}
			lifecycle := SiteInstanceLifecycle{SiteID: instance.SiteID, NodeName: instance.NodeName, StartMinuteTS: startMinute, EvidenceStatus: "known", CreatedAt: instance.UpdatedAt, UpdatedAt: instance.UpdatedAt}
			if lifecycle.CreatedAt <= 0 {
				lifecycle.CreatedAt, lifecycle.UpdatedAt = startMinute, startMinute
			}
			if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "node_name"}, {Name: "start_minute_ts"}}, DoUpdates: clause.Assignments(map[string]any{"end_minute_ts": nil, "evidence_status": "known", "updated_at": instance.UpdatedAt})}).Create(&lifecycle).Error; err != nil {
				if !IsDuplicateKey(err) {
					return err
				}
				var openCount int64
				if countErr := repository.db.WithContext(ctx).Model(&SiteInstanceLifecycle{}).Where("site_id=? AND node_name=? AND end_minute_ts IS NULL AND evidence_status='known'", instance.SiteID, instance.NodeName).Count(&openCount).Error; countErr != nil {
					return countErr
				}
				if openCount != 1 {
					return err
				}
			}
		}
		if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "site_id"}, {Name: "node_name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"hostname", "is_master", "runtime_version", "goos", "goarch", "upstream_status",
				"upstream_stale_after_seconds", "current_status", "started_at", "last_seen_at",
				"last_synced_at", "updated_at", "retired_at",
			}),
		}).Create(instance).Error; err != nil {
			return err
		}
		sample := &writes[index].Sample
		if err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "site_id"}, {Name: "node_name"}, {Name: "minute_ts"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "cpu_percent", "memory_percent", "disk_used_percent", "disk_total_bytes",
				"disk_used_bytes", "started_at", "last_seen_at", "created_at",
			}),
		}).Create(sample).Error; err != nil {
			return err
		}
	}
	return nil
}

// RetireMissingInstances marks nodes absent from a successful authoritative
// upstream snapshot. It is deliberately separate from SyncInstances so failed
// upstream requests never retire monitoring targets.
func (repository *SiteRepository) RetireMissingInstances(ctx context.Context, siteID, retiredAt int64, nodeNames []string) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id=? AND retired_at IS NULL", siteID)
		if len(nodeNames) > 0 {
			query = query.Where("node_name NOT IN ?", nodeNames)
		}
		var retiring []SiteInstance
		if err := query.Find(&retiring).Error; err != nil {
			return err
		}
		if len(retiring) == 0 {
			return nil
		}
		names := make([]string, 0, len(retiring))
		for _, instance := range retiring {
			names = append(names, instance.NodeName)
		}
		endMinute := retiredAt - retiredAt%60
		if err := tx.Model(&SiteInstanceLifecycle{}).Where("site_id=? AND node_name IN ? AND end_minute_ts IS NULL", siteID, names).
			Updates(map[string]any{"end_minute_ts": gorm.Expr("GREATEST(start_minute_ts, ?)", endMinute), "updated_at": retiredAt}).Error; err != nil {
			return err
		}
		return tx.Model(&SiteInstance{}).Where("site_id=? AND node_name IN ? AND retired_at IS NULL", siteID, names).
			Updates(map[string]any{"retired_at": retiredAt, "updated_at": retiredAt}).Error
	})
}

func (repository *SiteRepository) ListInstanceResourceStatesForUpdate(
	ctx context.Context,
	siteID int64,
	currentMinute int64,
) ([]SiteInstanceResourceState, error) {
	var instances []SiteInstance
	if err := repository.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("site_id = ? AND retired_at IS NULL", siteID).
		Order("node_name ASC").
		Find(&instances).Error; err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return []SiteInstanceResourceState{}, nil
	}

	type recentSample struct {
		NodeName string `gorm:"column:node_name"`
		Status   string `gorm:"column:status"`
	}
	var recent []recentSample
	if err := repository.db.WithContext(ctx).Raw(`SELECT ranked.node_name, ranked.status
FROM (
  SELECT node_name, status,
         ROW_NUMBER() OVER (PARTITION BY node_name ORDER BY minute_ts DESC) AS sample_rank
  FROM site_instance_status_minutely
  WHERE site_id = ? AND minute_ts < ?
) ranked
WHERE ranked.sample_rank <= 2`, siteID, currentMinute).Scan(&recent).Error; err != nil {
		return nil, err
	}

	sampleCounts := make(map[string]int, len(instances))
	nonOnlineCounts := make(map[string]int, len(instances))
	for _, sample := range recent {
		sampleCounts[sample.NodeName]++
		if sample.Status != "online" {
			nonOnlineCounts[sample.NodeName]++
		}
	}
	states := make([]SiteInstanceResourceState, 0, len(instances))
	for _, instance := range instances {
		states = append(states, SiteInstanceResourceState{
			Instance: instance,
			PriorTwoNonOnline: sampleCounts[instance.NodeName] == 2 &&
				nonOnlineCounts[instance.NodeName] == 2,
		})
	}
	return states, nil
}

func (repository *SiteRepository) ListInstanceSnapshots(ctx context.Context, siteID int64) ([]SiteInstanceSnapshot, error) {
	var snapshots []SiteInstanceSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT i.*,
	       m.status AS sample_status, m.minute_ts AS sampled_at, m.cpu_percent, m.memory_percent, m.disk_used_percent,
       m.disk_total_bytes, m.disk_used_bytes
FROM site_instance i
LEFT JOIN site_instance_status_minutely m
  ON m.site_id = i.site_id AND m.node_name = i.node_name
 AND m.minute_ts = (
   SELECT MAX(latest.minute_ts)
   FROM site_instance_status_minutely latest
   WHERE latest.site_id = i.site_id AND latest.node_name = i.node_name
 )
WHERE i.site_id = ?
ORDER BY i.node_name ASC`, siteID).Scan(&snapshots).Error
	return snapshots, err
}
