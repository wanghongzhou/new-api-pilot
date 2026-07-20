package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ResourceGapKindInstanceHourly = "instance_hourly"
	ResourceGapKindSiteHourly     = "site_hourly"
)

type resourceGapCandidate struct {
	Kind        string
	SiteID      int64
	NodeName    string
	BucketStart int64
}

type resourceGapExpected struct {
	SampleCount         int    `gorm:"column:sample_count"`
	ExpectedSampleCount int    `gorm:"column:expected_sample_count"`
	DataStatus          string `gorm:"column:data_status"`
}

func resourceGapStillNeedsRepair(ctx context.Context, tx *gorm.DB, c resourceGapCandidate) (bool, error) {
	var expected resourceGapExpected
	if c.Kind == ResourceGapKindInstanceHourly {
		err := tx.WithContext(ctx).Raw(`WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (SELECT g.minute_ts,MAX(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=l.start_minute_ts AND (l.end_minute_ts IS NULL OR g.minute_ts<l.end_minute_ts) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected FROM minute_grid g JOIN site s ON s.id=? JOIN site_instance_lifecycle l ON l.site_id=s.id AND l.node_name=? AND l.evidence_status='known' AND l.start_minute_ts<? AND (l.end_minute_ts IS NULL OR l.end_minute_ts>?) GROUP BY g.minute_ts) SELECT SUM(e.expected AND m.id IS NOT NULL) sample_count,SUM(e.expected) expected_sample_count,CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END data_status FROM eligible e LEFT JOIN site_instance_status_minutely m ON m.site_id=? AND m.node_name=? AND m.minute_ts=e.minute_ts`, c.BucketStart, c.BucketStart+3600, c.SiteID, c.NodeName, c.BucketStart+3600, c.BucketStart, c.SiteID, c.NodeName).Scan(&expected).Error
		if err != nil {
			return false, err
		}
		var existing resourceGapExpected
		err = tx.WithContext(ctx).Table("site_instance_status_hourly").Clauses(clause.Locking{Strength: "UPDATE"}).Select("sample_count,expected_sample_count,data_status").Where("site_id=? AND node_name=? AND hour_ts=?", c.SiteID, c.NodeName, c.BucketStart).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return existing != expected, nil
	}
	if c.Kind != ResourceGapKindSiteHourly {
		return false, ErrDataMaintenanceContract
	}
	err := tx.WithContext(ctx).Raw(`WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (SELECT g.minute_ts,(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected FROM minute_grid g JOIN site s ON s.id=?) SELECT SUM(e.expected AND m.id IS NOT NULL) sample_count,SUM(e.expected) expected_sample_count,CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END data_status FROM eligible e LEFT JOIN site_status_minutely m ON m.site_id=? AND m.minute_ts=e.minute_ts`, c.BucketStart, c.BucketStart+3600, c.SiteID, c.SiteID).Scan(&expected).Error
	if err != nil {
		return false, err
	}
	var existing resourceGapExpected
	err = tx.WithContext(ctx).Table("site_status_hourly").Clauses(clause.Locking{Strength: "UPDATE"}).Select("sample_count,expected_sample_count,data_status").Where("site_id=? AND hour_ts=?", c.SiteID, c.BucketStart).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return existing != expected, nil
}

func (repository *DataMaintenanceRepository) repairResourceRollupGapItems(ctx context.Context, dateKey int, start, end int64, maximumRows int, now int64) (ResourceMaintenanceBatchResult, error) {
	if repository == nil || repository.db == nil || dateKey <= 0 || start <= 0 || end <= start || maximumRows <= 0 || maximumRows > maximumDataMaintenanceBatchSize || now <= 0 {
		return ResourceMaintenanceBatchResult{}, ErrDataMaintenanceContract
	}
	if err := repository.ensureGlobalState(ctx, MaintenanceResourceGapRepair, now); err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	revision, err := repository.ResourceScopeRevision(ctx, 0, start, end)
	if err != nil {
		_ = repository.markGlobalFailure(ctx, MaintenanceResourceGapRepair, now, "REVISION_FAILED")
		return ResourceMaintenanceBatchResult{}, err
	}
	if err = repository.prepareGapState(ctx, dateKey, revision, now); err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	prepared, err := repository.loadMaintenanceState(ctx, MaintenanceResourceGapRepair)
	if err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	if prepared.Status == MaintenanceStatusComplete && prepared.ScopeRevision == revision {
		return ResourceMaintenanceBatchResult{Complete: true}, nil
	}
	result := ResourceMaintenanceBatchResult{Attempted: true}
	state, err := repository.loadMaintenanceState(ctx, MaintenanceResourceGapRepair)
	if err != nil {
		return result, err
	}
	candidates, err := repository.nextResourceGapCandidates(ctx, start, end, state, maximumRows)
	if err != nil {
		_ = repository.markGlobalFailure(ctx, MaintenanceResourceGapRepair, now, "SCAN_FAILED")
		return result, err
	}
	if len(candidates) == 0 {
		ids, scanErr := repository.eligibleResourceSiteIDs(ctx, start, end)
		if scanErr != nil {
			_ = repository.markGlobalFailure(ctx, MaintenanceResourceGapRepair, now, "REVISION_FAILED")
			return result, scanErr
		}
		err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if len(ids) > 0 {
				var sites []Site
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id ASC").Find(&sites).Error; err != nil {
					return err
				}
				maintenance := NewDataMaintenanceRepository(tx)
				for _, id := range ids {
					if err := maintenance.lockResourceRevisionRows(ctx, id, start, end); err != nil {
						return err
					}
				}
			}
			var locked DataMaintenanceState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=? AND scope_key='global'", MaintenanceResourceGapRepair).First(&locked).Error; err != nil {
				return err
			}
			latest, revisionErr := NewDataMaintenanceRepository(tx).ResourceScopeRevision(ctx, 0, start, end)
			if revisionErr != nil {
				return revisionErr
			}
			if locked.ScopeRevision != latest {
				locked.ScopeRevision, locked.CursorKind, locked.CursorSiteID, locked.CursorNodeName, locked.CursorBucketStart = latest, "", 0, "", 0
				locked.Status, locked.ErrorCode, locked.NextAttemptAt, locked.UpdatedAt = MaintenanceStatusPending, "", now, now
				return tx.Save(&locked).Error
			}
			locked.Status, locked.ErrorCode, locked.NextAttemptAt = MaintenanceStatusComplete, "", 0
			locked.LastSuccessAt, locked.UpdatedAt = &now, now
			result.Complete = true
			return tx.Save(&locked).Error
		})
		return result, err
	}
	for _, candidate := range candidates {
		err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var site Site
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, candidate.SiteID).Error; err != nil {
				return err
			}
			maintenance := NewDataMaintenanceRepository(tx)
			if err := maintenance.lockResourceRevisionRows(ctx, candidate.SiteID, start, end); err != nil {
				return err
			}
			var state DataMaintenanceState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=? AND scope_key='global'", MaintenanceResourceGapRepair).First(&state).Error; err != nil {
				return err
			}
			if state.DateKey != dateKey || state.ScopeRevision != revision || !resourceGapCursorBefore(state, candidate) {
				return ErrSiteRunConfigChanged
			}
			before, err := maintenance.ResourceScopeRevision(ctx, candidate.SiteID, start, end)
			if err != nil {
				return err
			}
			needsRepair, err := resourceGapStillNeedsRepair(ctx, tx, candidate)
			if err != nil {
				return err
			}
			if needsRepair {
				if err := repairResourceGapCandidate(ctx, tx, candidate, now); err != nil {
					return err
				}
			}
			after, err := maintenance.ResourceScopeRevision(ctx, candidate.SiteID, start, end)
			if err != nil {
				return err
			}
			if before != after {
				return ErrSiteRunConfigChanged
			}
			state.CursorKind, state.CursorSiteID, state.CursorNodeName, state.CursorBucketStart = candidate.Kind, candidate.SiteID, candidate.NodeName, candidate.BucketStart
			state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusPending, "", now
			state.AttemptCount++
			state.LastAttemptAt, state.UpdatedAt = &now, now
			return tx.Save(&state).Error
		})
		if err != nil {
			_ = repository.markGlobalFailure(ctx, MaintenanceResourceGapRepair, now, "BATCH_FAILED")
			return result, err
		}
		result.Items++
		result.CursorSiteID = candidate.SiteID
	}
	return result, nil
}

func (repository *DataMaintenanceRepository) prepareGapState(ctx context.Context, dateKey int, revision string, now int64) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state DataMaintenanceState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=? AND scope_key='global'", MaintenanceResourceGapRepair).First(&state).Error; err != nil {
			return err
		}
		if state.DateKey != dateKey || state.ScopeRevision != revision {
			state.DateKey, state.ScopeRevision = dateKey, revision
			state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusPending, "", now
			state.CursorKind, state.CursorSiteID, state.CursorNodeName, state.CursorBucketStart = "", 0, "", 0
		}
		return tx.Save(&state).Error
	})
}
func (repository *DataMaintenanceRepository) loadMaintenanceState(ctx context.Context, operation string) (DataMaintenanceState, error) {
	var state DataMaintenanceState
	err := repository.db.WithContext(ctx).Where("operation_id=? AND scope_key='global'", operation).First(&state).Error
	return state, err
}

func resourceGapCursorBefore(state DataMaintenanceState, c resourceGapCandidate) bool {
	if state.CursorKind == "" {
		return true
	}
	order := func(kind string) int {
		if kind == ResourceGapKindInstanceHourly {
			return 0
		}
		return 1
	}
	if order(c.Kind) != order(state.CursorKind) {
		return order(c.Kind) > order(state.CursorKind)
	}
	if c.SiteID != state.CursorSiteID {
		return c.SiteID > state.CursorSiteID
	}
	if c.NodeName != state.CursorNodeName {
		return c.NodeName > state.CursorNodeName
	}
	return c.BucketStart > state.CursorBucketStart
}

func (repository *DataMaintenanceRepository) nextResourceGapCandidates(ctx context.Context, start, end int64, state DataMaintenanceState, limit int) ([]resourceGapCandidate, error) {
	result := make([]resourceGapCandidate, 0, limit)
	if state.CursorKind == "" || state.CursorKind == ResourceGapKindInstanceHourly {
		var rows []resourceGapCandidate
		err := repository.db.WithContext(ctx).Raw(`WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (
SELECT l.site_id,l.node_name,g.minute_ts,g.minute_ts-MOD(g.minute_ts,3600) bucket_start,
(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=l.start_minute_ts AND (l.end_minute_ts IS NULL OR g.minute_ts<l.end_minute_ts)) covered,
(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=l.start_minute_ts AND (l.end_minute_ts IS NULL OR g.minute_ts<l.end_minute_ts) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected
FROM site_instance_lifecycle l JOIN site s ON s.id=l.site_id CROSS JOIN minute_grid g WHERE l.evidence_status='known' AND l.start_minute_ts<? AND (l.end_minute_ts IS NULL OR l.end_minute_ts>?)), computed AS (
SELECT e.site_id,e.node_name,e.bucket_start,SUM(e.expected AND m.id IS NOT NULL) sample_count,SUM(e.expected) expected_sample_count,CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END data_status FROM eligible e LEFT JOIN site_instance_status_minutely m ON m.site_id=e.site_id AND m.node_name=e.node_name AND m.minute_ts=e.minute_ts GROUP BY e.site_id,e.node_name,e.bucket_start HAVING SUM(e.covered)>0)
SELECT /*+ SET_VAR(cte_max_recursion_depth=2000) */ ? kind,c.site_id,c.node_name,c.bucket_start FROM computed c LEFT JOIN site_instance_status_hourly h ON h.site_id=c.site_id AND h.node_name=c.node_name AND h.hour_ts=c.bucket_start WHERE (c.site_id>? OR (c.site_id=? AND (c.node_name COLLATE utf8mb4_bin>CAST(? AS CHAR) COLLATE utf8mb4_bin OR (c.node_name COLLATE utf8mb4_bin=CAST(? AS CHAR) COLLATE utf8mb4_bin AND c.bucket_start>?)))) AND (h.id IS NULL OR h.sample_count<>c.sample_count OR h.expected_sample_count<>c.expected_sample_count OR h.data_status<>c.data_status) ORDER BY c.site_id,c.node_name COLLATE utf8mb4_bin,c.bucket_start LIMIT ?`, start, end, end, start, ResourceGapKindInstanceHourly, state.CursorSiteID, state.CursorSiteID, state.CursorNodeName, state.CursorNodeName, state.CursorBucketStart, limit).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
		if len(result) >= limit {
			return result, nil
		}
	}
	var rows []resourceGapCandidate
	afterSite, afterBucket := int64(0), int64(0)
	if state.CursorKind == ResourceGapKindSiteHourly {
		afterSite, afterBucket = state.CursorSiteID, state.CursorBucketStart
	}
	err := repository.db.WithContext(ctx).Raw(`WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (SELECT s.id site_id,g.minute_ts,g.minute_ts-MOD(g.minute_ts,3600) bucket_start,(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at)) covered,(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected FROM site s CROSS JOIN minute_grid g WHERE s.monitoring_start_at<? AND (s.statistics_end_at IS NULL OR s.statistics_end_at>?)), computed AS (SELECT e.site_id,e.bucket_start,SUM(e.expected AND m.id IS NOT NULL) sample_count,SUM(e.expected) expected_sample_count,CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END data_status FROM eligible e LEFT JOIN site_status_minutely m ON m.site_id=e.site_id AND m.minute_ts=e.minute_ts GROUP BY e.site_id,e.bucket_start HAVING SUM(e.covered)>0) SELECT /*+ SET_VAR(cte_max_recursion_depth=2000) */ ? kind,c.site_id,'' node_name,c.bucket_start FROM computed c LEFT JOIN site_status_hourly h ON h.site_id=c.site_id AND h.hour_ts=c.bucket_start WHERE (c.site_id,c.bucket_start)>(?,?) AND (h.id IS NULL OR h.sample_count<>c.sample_count OR h.expected_sample_count<>c.expected_sample_count OR h.data_status<>c.data_status) ORDER BY c.site_id,c.bucket_start LIMIT ?`, start, end, end, start, ResourceGapKindSiteHourly, afterSite, afterBucket, limit-len(result)).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result = append(result, rows...)
	return result, nil
}

func repairResourceGapCandidate(ctx context.Context, tx *gorm.DB, c resourceGapCandidate, now int64) error {
	hourEnd := c.BucketStart + 3600
	if c.Kind == ResourceGapKindInstanceHourly {
		return tx.WithContext(ctx).Exec(`INSERT INTO site_instance_status_hourly
(site_id,node_name,hour_ts,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,disk_last_used_percent,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,last_calculated_at)
WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (
SELECT g.minute_ts,MAX(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=l.start_minute_ts AND (l.end_minute_ts IS NULL OR g.minute_ts<l.end_minute_ts)) covered,
MAX(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=l.start_minute_ts AND (l.end_minute_ts IS NULL OR g.minute_ts<l.end_minute_ts) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected
FROM minute_grid g JOIN site s ON s.id=? JOIN site_instance_lifecycle l ON l.site_id=s.id AND l.node_name=? AND l.evidence_status='known' AND l.start_minute_ts<? AND (l.end_minute_ts IS NULL OR l.end_minute_ts>?) GROUP BY g.minute_ts)
SELECT ?,?,?,MAX(CASE WHEN e.expected THEN m.cpu_percent END),AVG(CASE WHEN e.expected THEN m.cpu_percent END),MAX(CASE WHEN e.expected THEN m.memory_percent END),AVG(CASE WHEN e.expected THEN m.memory_percent END),MAX(CASE WHEN e.expected THEN m.disk_used_percent END),SUBSTRING_INDEX(GROUP_CONCAT(CASE WHEN e.expected THEN m.disk_used_percent END ORDER BY e.minute_ts DESC),',',1),SUM(e.expected AND m.id IS NOT NULL AND m.status='online'),SUM(e.expected AND m.id IS NOT NULL AND m.status<>'online'),SUM(e.expected AND m.id IS NOT NULL),SUM(e.expected),CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END,?
FROM eligible e LEFT JOIN site_instance_status_minutely m ON m.site_id=? AND m.node_name=? AND m.minute_ts=e.minute_ts HAVING SUM(e.covered)>0
ON DUPLICATE KEY UPDATE cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),disk_last_used_percent=VALUES(disk_last_used_percent),online_samples=VALUES(online_samples),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),last_calculated_at=VALUES(last_calculated_at)`,
			c.BucketStart, hourEnd, c.SiteID, c.NodeName, hourEnd, c.BucketStart, c.SiteID, c.NodeName, c.BucketStart, now, c.SiteID, c.NodeName).Error
	}
	if c.Kind != ResourceGapKindSiteHourly {
		return fmt.Errorf("%w: unknown resource gap kind", ErrDataMaintenanceContract)
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO site_status_hourly
(site_id,hour_ts,instance_count_max,online_instance_count_min,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,abnormal_samples,sample_count,expected_sample_count,data_status,health_status,last_calculated_at)
WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?), eligible AS (
SELECT g.minute_ts,(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at)) covered,(g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected FROM minute_grid g JOIN site s ON s.id=?)
SELECT ?,?,COALESCE(MAX(CASE WHEN e.expected THEN m.instance_count END),0),COALESCE(MIN(CASE WHEN e.expected THEN m.online_instance_count END),0),MAX(CASE WHEN e.expected THEN m.cpu_max_percent END),AVG(CASE WHEN e.expected THEN m.cpu_avg_percent END),MAX(CASE WHEN e.expected THEN m.memory_max_percent END),AVG(CASE WHEN e.expected THEN m.memory_avg_percent END),MAX(CASE WHEN e.expected THEN m.disk_max_used_percent END),SUM(e.expected AND m.id IS NOT NULL AND m.health_status<>'ok'),SUM(e.expected AND m.id IS NOT NULL),SUM(e.expected),CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END,CASE WHEN SUM(e.expected)=0 OR SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'unavailable' WHEN SUM(e.expected AND m.id IS NOT NULL AND m.health_status='critical')>0 THEN 'critical' WHEN SUM(e.expected AND m.id IS NOT NULL AND m.health_status='warning')>0 THEN 'warning' ELSE 'ok' END,?
FROM eligible e LEFT JOIN site_status_minutely m ON m.site_id=? AND m.minute_ts=e.minute_ts HAVING SUM(e.covered)>0
ON DUPLICATE KEY UPDATE instance_count_max=VALUES(instance_count_max),online_instance_count_min=VALUES(online_instance_count_min),cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),health_status=VALUES(health_status),last_calculated_at=VALUES(last_calculated_at)`, c.BucketStart, hourEnd, c.SiteID, c.SiteID, c.BucketStart, now, c.SiteID).Error
}
