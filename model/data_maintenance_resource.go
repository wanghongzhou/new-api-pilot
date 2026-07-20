package model

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResourceMaintenanceBatchResult struct {
	Attempted    bool
	Items        int
	Sites        int
	Complete     bool
	CursorSiteID int64
}

func (repository *DataMaintenanceRepository) RepairResourceRollupGaps(
	ctx context.Context, dateKey int, start, end int64, maximumSites int, now int64,
) (ResourceMaintenanceBatchResult, error) {
	return repository.repairResourceRollupGapItems(ctx, dateKey, start, end, maximumSites, now)
}

func (repository *DataMaintenanceRepository) FinalizeResourceDaily(
	ctx context.Context, dateKey int, start, end int64, maximumSites int, now int64,
) (ResourceMaintenanceBatchResult, error) {
	if repository == nil || repository.db == nil || dateKey <= 0 || start <= 0 || end <= start || maximumSites <= 0 || maximumSites > maximumDataMaintenanceBatchSize || now <= 0 {
		return ResourceMaintenanceBatchResult{}, ErrDataMaintenanceContract
	}
	if err := repository.ensureGlobalState(ctx, MaintenanceResourceDaily, now); err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	revision, err := repository.ResourceScopeRevision(ctx, 0, start, end)
	if err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	noOp, err := repository.prepareResourceDailyPhase(ctx, dateKey, start, end, revision, now)
	if err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	if noOp {
		return ResourceMaintenanceBatchResult{Complete: true}, nil
	}
	result, err := repository.runResourceSiteBatch(ctx, MaintenanceResourceDaily, dateKey, start, end, maximumSites, now, finalizeResourceSite)
	if err != nil || !result.Complete {
		return result, err
	}
	if !result.Attempted {
		return result, nil
	}
	err = repository.publishResourceDaily(ctx, dateKey, start, end, now)
	return result, err
}

func (repository *DataMaintenanceRepository) eligibleResourceSiteIDs(ctx context.Context, start, end int64) ([]int64, error) {
	var ids []int64
	err := repository.db.WithContext(ctx).Model(&Site{}).Where("monitoring_start_at IS NOT NULL AND monitoring_start_at < ? AND (statistics_end_at IS NULL OR statistics_end_at > ?)", end, start).Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}

type resourceKnownNode struct {
	SiteID   int64  `gorm:"column:site_id"`
	NodeName string `gorm:"column:node_name"`
}

func (repository *DataMaintenanceRepository) knownResourceNodeHourTuples(ctx context.Context, siteID, start, end int64) ([]string, [][]any, error) {
	var site Site
	if err := repository.db.WithContext(ctx).Select("id,monitoring_start_at,statistics_end_at").First(&site, siteID).Error; err != nil {
		return nil, nil, err
	}
	if site.MonitoringStartAt == nil {
		return []string{}, [][]any{}, nil
	}
	var intervals []SiteInstanceLifecycle
	if err := repository.db.WithContext(ctx).Where("site_id=? AND evidence_status='known' AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)", siteID, end, start).Order("node_name COLLATE utf8mb4_bin ASC,start_minute_ts ASC,id ASC").Find(&intervals).Error; err != nil {
		return nil, nil, err
	}
	nodeSet := map[string]struct{}{}
	tupleSet := map[string][]any{}
	for _, interval := range intervals {
		from := interval.StartMinuteTS
		if from < start {
			from = start
		}
		if from < *site.MonitoringStartAt {
			from = *site.MonitoringStartAt
		}
		to := end
		if site.StatisticsEndAt != nil && *site.StatisticsEndAt < to {
			to = *site.StatisticsEndAt
		}
		if interval.EndMinuteTS != nil && *interval.EndMinuteTS < to {
			to = *interval.EndMinuteTS
		}
		if to <= from {
			continue
		}
		nodeSet[interval.NodeName] = struct{}{}
		hour := from - from%3600
		for ; hour < to; hour += 3600 {
			key := fmt.Sprintf("%s\x00%d", interval.NodeName, hour)
			tupleSet[key] = []any{interval.NodeName, hour}
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	keys := make([]string, 0, len(tupleSet))
	for key := range tupleSet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	tuples := make([][]any, 0, len(keys))
	for _, key := range keys {
		tuples = append(tuples, tupleSet[key])
	}
	return nodes, tuples, nil
}

func (repository *DataMaintenanceRepository) knownResourceNodes(ctx context.Context, ids []int64, start, end int64) ([]resourceKnownNode, error) {
	if len(ids) == 0 {
		return []resourceKnownNode{}, nil
	}
	var nodes []resourceKnownNode
	err := repository.db.WithContext(ctx).Table("site_instance_lifecycle l").Select("l.site_id,l.node_name").Joins("JOIN site s ON s.id=l.site_id").Where("l.site_id IN ? AND l.evidence_status='known' AND GREATEST(l.start_minute_ts,s.monitoring_start_at,?) < LEAST(COALESCE(l.end_minute_ts,?),COALESCE(s.statistics_end_at,?),?)", ids, start, end, end, end).Group("l.site_id,l.node_name").Order("l.site_id ASC,l.node_name COLLATE utf8mb4_bin ASC").Find(&nodes).Error
	return nodes, err
}

func (repository *DataMaintenanceRepository) prepareResourceDailyPhase(ctx context.Context, dateKey int, start, end int64, revision string, now int64) (bool, error) {
	ids, err := repository.eligibleResourceSiteIDs(ctx, start, end)
	if err != nil {
		return false, err
	}
	knownNodes, err := repository.knownResourceNodes(ctx, ids, start, end)
	if err != nil {
		return false, err
	}
	nodeTuples := make([][]any, 0, len(knownNodes))
	for _, node := range knownNodes {
		nodeTuples = append(nodeTuples, []any{node.SiteID, node.NodeName})
	}
	noOp := false
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
		currentRevision, err := NewDataMaintenanceRepository(tx).ResourceScopeRevision(ctx, 0, start, end)
		if err != nil {
			return err
		}
		if currentRevision != revision {
			return ErrSiteRunConfigChanged
		}
		var state DataMaintenanceState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=? AND scope_key='global'", MaintenanceResourceDaily).First(&state).Error; err != nil {
			return err
		}
		if state.DateKey == dateKey && state.ScopeRevision == revision && state.Status == MaintenanceStatusComplete {
			noOp = true
			return nil
		}
		if state.DateKey == dateKey && state.ScopeRevision == revision {
			return nil
		}
		if len(ids) > 0 {
			if err := tx.Table("site_status_daily").Where("date_key=? AND site_id IN ?", dateKey, ids).Update("is_final", 0).Error; err != nil {
				return err
			}
		}
		if len(nodeTuples) > 0 {
			if err := tx.Table("site_instance_status_daily").Where("date_key=? AND (site_id,node_name) IN ?", dateKey, nodeTuples).Update("is_final", 0).Error; err != nil {
				return err
			}
		}
		state.DateKey, state.ScopeRevision = dateKey, revision
		state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusPending, "", now
		state.CursorKind, state.CursorSiteID, state.CursorNodeName, state.CursorBucketStart = "site", 0, "", 0
		state.UpdatedAt = now
		return tx.Save(&state).Error
	})
	return noOp, err
}

type resourceSiteMaintenance func(context.Context, *gorm.DB, int64, int, int64, int64, int64) error

func (repository *DataMaintenanceRepository) runResourceSiteBatch(
	ctx context.Context, operation string, dateKey int, start, end int64, maximumSites int, now int64,
	maintain resourceSiteMaintenance,
) (ResourceMaintenanceBatchResult, error) {
	if repository == nil || repository.db == nil || dateKey <= 0 || start <= 0 || end <= start ||
		maximumSites <= 0 || maximumSites > maximumDataMaintenanceBatchSize || now <= 0 || maintain == nil {
		return ResourceMaintenanceBatchResult{}, ErrDataMaintenanceContract
	}
	if err := repository.ensureGlobalState(ctx, operation, now); err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	scopeRevision, err := repository.ResourceScopeRevision(ctx, 0, start, end)
	if err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	result := ResourceMaintenanceBatchResult{Attempted: true}
	state, err := repository.loadMaintenanceState(ctx, operation)
	if err != nil {
		return ResourceMaintenanceBatchResult{}, err
	}
	if state.DateKey != dateKey || state.ScopeRevision != scopeRevision {
		return ResourceMaintenanceBatchResult{}, ErrSiteRunConfigChanged
	}
	result.CursorSiteID = state.CursorSiteID
	if state.Status == MaintenanceStatusComplete {
		result.Attempted, result.Complete = false, true
	}
	if result.Complete {
		return result, nil
	}
	var siteIDs []int64
	if err := repository.db.WithContext(ctx).Model(&Site{}).
		Where("id > ? AND monitoring_start_at IS NOT NULL AND monitoring_start_at < ? AND (statistics_end_at IS NULL OR statistics_end_at > ?)", result.CursorSiteID, end, start).
		Order("id ASC").Limit(maximumSites).Pluck("id", &siteIDs).Error; err != nil {
		_ = repository.markGlobalFailure(ctx, operation, now, "SCAN_FAILED")
		return ResourceMaintenanceBatchResult{}, err
	}
	for _, siteID := range siteIDs {
		err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var site Site
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, siteID).Error; err != nil {
				return err
			}
			maintenance := NewDataMaintenanceRepository(tx)
			if err := maintenance.lockResourceRevisionRows(ctx, siteID, start, end); err != nil {
				return err
			}
			var state DataMaintenanceState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ? AND scope_key = 'global'", operation).First(&state).Error; err != nil {
				return err
			}
			if state.DateKey != dateKey || state.Status == MaintenanceStatusComplete || state.CursorSiteID >= siteID {
				return nil
			}
			if site.MonitoringStartAt == nil || *site.MonitoringStartAt >= end || (site.StatisticsEndAt != nil && *site.StatisticsEndAt <= start) {
				return ErrSiteRunConfigChanged
			}
			siteRevision, err := maintenance.ResourceScopeRevision(ctx, siteID, start, end)
			if err != nil {
				return err
			}
			if err := maintain(ctx, tx, siteID, dateKey, start, end, now); err != nil {
				return err
			}
			afterRevision, err := maintenance.ResourceScopeRevision(ctx, siteID, start, end)
			if err != nil {
				return err
			}
			if afterRevision != siteRevision {
				return ErrSiteRunConfigChanged
			}
			state.CursorSiteID, state.CursorKind = siteID, "site"
			state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusPending, "", now
			state.AttemptCount++
			state.LastAttemptAt, state.UpdatedAt = &now, now
			return tx.Save(&state).Error
		})
		if err != nil {
			_ = repository.markGlobalFailure(ctx, operation, now, "BATCH_FAILED")
			return result, fmt.Errorf("run %s site %d: %w", operation, siteID, err)
		}
		result.Sites++
		result.CursorSiteID = siteID
	}
	result.Complete = len(siteIDs) < maximumSites
	if result.Complete && operation == MaintenanceResourceDaily {
		return result, nil
	}
	if result.Complete {
		latestRevision, revisionErr := repository.ResourceScopeRevision(ctx, 0, start, end)
		if revisionErr != nil {
			return result, revisionErr
		}
		err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var state DataMaintenanceState
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ? AND scope_key = 'global'", operation).First(&state).Error; err != nil {
				return err
			}
			if state.DateKey != dateKey || state.CursorSiteID != result.CursorSiteID {
				return ErrDataMaintenanceContract
			}
			if state.ScopeRevision != latestRevision {
				if operation == MaintenanceResourceDaily {
					return ErrSiteRunConfigChanged
				}
				state.ScopeRevision, state.CursorKind, state.CursorSiteID, state.CursorNodeName, state.CursorBucketStart = latestRevision, "site", 0, "", 0
				state.Status, state.NextAttemptAt, state.UpdatedAt = MaintenanceStatusPending, now, now
				result.Complete, result.CursorSiteID = false, 0
				return tx.Save(&state).Error
			}
			if operation == MaintenanceResourceDaily {
				state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusPending, "", now
			} else {
				state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusComplete, "", 0
				state.LastSuccessAt = &now
			}
			state.UpdatedAt = now
			return tx.Save(&state).Error
		})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func repairResourceSite(ctx context.Context, tx *gorm.DB, siteID int64, dateKey int, start, end, now int64) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO site_instance_status_hourly
(site_id,node_name,hour_ts,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,disk_last_used_percent,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,last_calculated_at)
WITH RECURSIVE minute_grid AS (SELECT ? AS minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?),
eligible AS (SELECT i.site_id,i.node_name,g.minute_ts,g.minute_ts-MOD(g.minute_ts,3600) hour_ts,
 (g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=i.start_minute_ts AND (i.end_minute_ts IS NULL OR g.minute_ts<i.end_minute_ts)) covered,
 (g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND g.minute_ts>=i.start_minute_ts AND (i.end_minute_ts IS NULL OR g.minute_ts<i.end_minute_ts) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected
 FROM site_instance_lifecycle i JOIN site s ON s.id=i.site_id CROSS JOIN minute_grid g WHERE i.site_id=? AND i.evidence_status='known' AND i.start_minute_ts<? AND (i.end_minute_ts IS NULL OR i.end_minute_ts>?))
SELECT /*+ SET_VAR(cte_max_recursion_depth=2000) */ e.site_id,e.node_name,e.hour_ts,
 MAX(CASE WHEN e.expected THEN m.cpu_percent END),AVG(CASE WHEN e.expected THEN m.cpu_percent END),MAX(CASE WHEN e.expected THEN m.memory_percent END),AVG(CASE WHEN e.expected THEN m.memory_percent END),MAX(CASE WHEN e.expected THEN m.disk_used_percent END),SUBSTRING_INDEX(GROUP_CONCAT(CASE WHEN e.expected THEN m.disk_used_percent END ORDER BY e.minute_ts DESC),',',1),
 SUM(e.expected AND m.id IS NOT NULL AND m.status='online'),SUM(e.expected AND m.id IS NOT NULL AND m.status<>'online'),SUM(e.expected AND m.id IS NOT NULL),SUM(e.expected),
 CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END,?
FROM eligible e LEFT JOIN site_instance_status_minutely m ON m.site_id=e.site_id AND m.node_name=e.node_name AND m.minute_ts=e.minute_ts
GROUP BY e.site_id,e.node_name,e.hour_ts HAVING SUM(e.covered)>0
ON DUPLICATE KEY UPDATE cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),disk_last_used_percent=VALUES(disk_last_used_percent),online_samples=VALUES(online_samples),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),last_calculated_at=VALUES(last_calculated_at)`, []any{start, end, siteID, end, start, now}},
		{`INSERT INTO site_status_hourly
(site_id,hour_ts,instance_count_max,online_instance_count_min,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,abnormal_samples,sample_count,expected_sample_count,data_status,health_status,last_calculated_at)
WITH RECURSIVE minute_grid AS (SELECT ? AS minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?),
eligible AS (SELECT s.id site_id,g.minute_ts,g.minute_ts-MOD(g.minute_ts,3600) hour_ts,
 (g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at)) covered,
 (g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at) AND NOT EXISTS(SELECT 1 FROM site_monitoring_pause p WHERE p.site_id=s.id AND p.start_minute_ts<=g.minute_ts AND (p.end_minute_ts IS NULL OR p.end_minute_ts>g.minute_ts))) expected
 FROM site s CROSS JOIN minute_grid g WHERE s.id=?)
SELECT /*+ SET_VAR(cte_max_recursion_depth=2000) */ e.site_id,e.hour_ts,COALESCE(MAX(CASE WHEN e.expected THEN m.instance_count END),0),COALESCE(MIN(CASE WHEN e.expected THEN m.online_instance_count END),0),MAX(CASE WHEN e.expected THEN m.cpu_max_percent END),AVG(CASE WHEN e.expected THEN m.cpu_avg_percent END),MAX(CASE WHEN e.expected THEN m.memory_max_percent END),AVG(CASE WHEN e.expected THEN m.memory_avg_percent END),MAX(CASE WHEN e.expected THEN m.disk_max_used_percent END),
 SUM(e.expected AND m.id IS NOT NULL AND m.health_status<>'ok'),SUM(e.expected AND m.id IS NOT NULL),SUM(e.expected),
 CASE WHEN SUM(e.expected)=0 THEN 'paused' WHEN SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'missing' WHEN SUM(e.expected AND m.id IS NOT NULL)<SUM(e.expected) THEN 'partial' ELSE 'complete' END,
 CASE WHEN SUM(e.expected)=0 OR SUM(e.expected AND m.id IS NOT NULL)=0 THEN 'unavailable' WHEN SUM(e.expected AND m.id IS NOT NULL AND m.health_status='critical')>0 THEN 'critical' WHEN SUM(e.expected AND m.id IS NOT NULL AND m.health_status='warning')>0 THEN 'warning' ELSE 'ok' END,?
FROM eligible e LEFT JOIN site_status_minutely m ON m.site_id=e.site_id AND m.minute_ts=e.minute_ts
GROUP BY e.site_id,e.hour_ts HAVING SUM(e.covered)>0
ON DUPLICATE KEY UPDATE instance_count_max=VALUES(instance_count_max),online_instance_count_min=VALUES(online_instance_count_min),cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),health_status=VALUES(health_status),last_calculated_at=VALUES(last_calculated_at)`, []any{start, end, siteID, now}},
		{`INSERT INTO site_instance_status_daily
(site_id,node_name,date_key,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,disk_last_used_percent,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,is_final,last_calculated_at)
SELECT site_id,node_name,?,MAX(cpu_max_percent),SUM(cpu_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(memory_max_percent),SUM(memory_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(disk_max_used_percent),SUBSTRING_INDEX(GROUP_CONCAT(disk_last_used_percent ORDER BY hour_ts DESC),',',1),SUM(online_samples),SUM(abnormal_samples),SUM(sample_count),SUM(expected_sample_count),CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END,0,?
FROM site_instance_status_hourly WHERE site_id=? AND hour_ts>=? AND hour_ts<? GROUP BY site_id,node_name
ON DUPLICATE KEY UPDATE cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),disk_last_used_percent=VALUES(disk_last_used_percent),online_samples=VALUES(online_samples),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),is_final=0,last_calculated_at=VALUES(last_calculated_at)`, []any{dateKey, now, siteID, start, end}},
		{`INSERT INTO site_status_daily
(site_id,date_key,instance_count_max,online_instance_count_min,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,abnormal_samples,sample_count,expected_sample_count,data_status,health_status,is_final,last_calculated_at)
SELECT site_id,?,MAX(instance_count_max),MIN(online_instance_count_min),MAX(cpu_max_percent),SUM(cpu_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(memory_max_percent),SUM(memory_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(disk_max_used_percent),SUM(abnormal_samples),SUM(sample_count),SUM(expected_sample_count),CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END,CASE WHEN SUM(expected_sample_count)=0 OR SUM(sample_count)=0 THEN 'unavailable' WHEN SUM(health_status='critical')>0 THEN 'critical' WHEN SUM(health_status='warning')>0 THEN 'warning' ELSE 'ok' END,0,?
FROM site_status_hourly WHERE site_id=? AND hour_ts>=? AND hour_ts<? GROUP BY site_id
ON DUPLICATE KEY UPDATE instance_count_max=VALUES(instance_count_max),online_instance_count_min=VALUES(online_instance_count_min),cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),health_status=VALUES(health_status),is_final=0,last_calculated_at=VALUES(last_calculated_at)`, []any{dateKey, now, siteID, start, end}},
	}
	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement.query, statement.args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func finalizeResourceSite(ctx context.Context, tx *gorm.DB, siteID int64, dateKey int, start, end int64, now int64) error {
	if err := rebuildResourceDailySite(ctx, tx, siteID, dateKey, start, end, now); err != nil {
		return err
	}
	nodes, hourTuples, err := NewDataMaintenanceRepository(tx).knownResourceNodeHourTuples(ctx, siteID, start, end)
	if err != nil {
		return err
	}
	var siteExpectedHours int64
	if err := tx.WithContext(ctx).Raw(`WITH RECURSIVE minute_grid AS (SELECT ? minute_ts UNION ALL SELECT minute_ts+60 FROM minute_grid WHERE minute_ts+60<?)
SELECT /*+ SET_VAR(cte_max_recursion_depth=2000) */ COUNT(DISTINCT g.minute_ts-MOD(g.minute_ts,3600)) FROM minute_grid g JOIN site s ON s.id=?
WHERE g.minute_ts>=s.monitoring_start_at AND (s.statistics_end_at IS NULL OR g.minute_ts<s.statistics_end_at)`, start, end, siteID).Scan(&siteExpectedHours).Error; err != nil {
		return err
	}
	instanceExpectedHours := int64(len(hourTuples))
	var siteMismatch int64
	if err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM site_status_daily d
LEFT JOIN (SELECT site_id,SUM(sample_count) sample_count,SUM(expected_sample_count) expected_sample_count,
CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END data_status
FROM site_status_hourly WHERE site_id=? AND hour_ts>=? AND hour_ts<? GROUP BY site_id) h ON h.site_id=d.site_id
WHERE d.site_id=? AND d.date_key=? AND (h.site_id IS NULL OR d.sample_count<>h.sample_count OR d.expected_sample_count<>h.expected_sample_count OR d.data_status<>h.data_status)`,
		siteID, resourceDateStart(dateKey), resourceDateEnd(dateKey), siteID, dateKey).Scan(&siteMismatch).Error; err != nil {
		return err
	}
	var siteDailyCount, siteHourCount int64
	if err := tx.WithContext(ctx).Table("site_status_daily").Where("site_id=? AND date_key=?", siteID, dateKey).Count(&siteDailyCount).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Table("site_status_hourly").Where("site_id=? AND hour_ts>=? AND hour_ts<?", siteID, resourceDateStart(dateKey), resourceDateEnd(dateKey)).Count(&siteHourCount).Error; err != nil {
		return err
	}
	var instanceMismatch, instanceDailyCount int64
	if len(hourTuples) > 0 {
		if err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM site_instance_status_daily d
LEFT JOIN (SELECT site_id,node_name,SUM(sample_count) sample_count,SUM(expected_sample_count) expected_sample_count,
CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END data_status
FROM site_instance_status_hourly WHERE site_id=? AND (node_name,hour_ts) IN ? GROUP BY site_id,node_name) h ON h.site_id=d.site_id AND h.node_name=d.node_name
WHERE d.site_id=? AND d.date_key=? AND d.node_name IN ? AND (h.site_id IS NULL OR d.sample_count<>h.sample_count OR d.expected_sample_count<>h.expected_sample_count OR d.data_status<>h.data_status)`, siteID, hourTuples, siteID, dateKey, nodes).Scan(&instanceMismatch).Error; err != nil {
			return err
		}
	}
	dailyQuery := tx.WithContext(ctx).Table("site_instance_status_daily").Where("site_id=? AND date_key=?", siteID, dateKey)
	if len(nodes) > 0 {
		dailyQuery = dailyQuery.Where("node_name IN ?", nodes)
	} else {
		dailyQuery = dailyQuery.Where("1=0")
	}
	if err := dailyQuery.Count(&instanceDailyCount).Error; err != nil {
		return err
	}
	var instanceHourCount int64
	hourQuery := tx.WithContext(ctx).Table("site_instance_status_hourly").Where("site_id=?", siteID)
	if len(hourTuples) > 0 {
		hourQuery = hourQuery.Where("(node_name,hour_ts) IN ?", hourTuples)
	} else {
		hourQuery = hourQuery.Where("1=0")
	}
	if err := hourQuery.Count(&instanceHourCount).Error; err != nil {
		return err
	}
	if siteDailyCount != 1 || siteHourCount != siteExpectedHours || siteMismatch != 0 || instanceHourCount != instanceExpectedHours || instanceDailyCount != int64(len(nodes)) || instanceMismatch != 0 {
		return fmt.Errorf("%w: resource daily inputs are incomplete", ErrDataMaintenanceContract)
	}
	return nil
}

func rebuildResourceDailySite(ctx context.Context, tx *gorm.DB, siteID int64, dateKey int, start, end, now int64) error {
	_, hourTuples, err := NewDataMaintenanceRepository(tx).knownResourceNodeHourTuples(ctx, siteID, start, end)
	if err != nil {
		return err
	}
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO site_instance_status_daily (site_id,node_name,date_key,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,disk_last_used_percent,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,is_final,last_calculated_at) SELECT site_id,node_name,?,MAX(cpu_max_percent),SUM(cpu_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(memory_max_percent),SUM(memory_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(disk_max_used_percent),SUBSTRING_INDEX(GROUP_CONCAT(disk_last_used_percent ORDER BY hour_ts DESC),',',1),SUM(online_samples),SUM(abnormal_samples),SUM(sample_count),SUM(expected_sample_count),CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END,0,? FROM site_instance_status_hourly WHERE site_id=? AND (node_name,hour_ts) IN ? GROUP BY site_id,node_name ON DUPLICATE KEY UPDATE cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),disk_last_used_percent=VALUES(disk_last_used_percent),online_samples=VALUES(online_samples),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),is_final=0,last_calculated_at=VALUES(last_calculated_at)`, []any{dateKey, now, siteID, hourTuples}},
		{`INSERT INTO site_status_daily (site_id,date_key,instance_count_max,online_instance_count_min,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,abnormal_samples,sample_count,expected_sample_count,data_status,health_status,is_final,last_calculated_at) SELECT site_id,?,MAX(instance_count_max),MIN(online_instance_count_min),MAX(cpu_max_percent),SUM(cpu_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(memory_max_percent),SUM(memory_avg_percent*sample_count)/NULLIF(SUM(sample_count),0),MAX(disk_max_used_percent),SUM(abnormal_samples),SUM(sample_count),SUM(expected_sample_count),CASE WHEN SUM(expected_sample_count)=0 THEN 'paused' WHEN SUM(sample_count)=0 THEN 'missing' WHEN SUM(sample_count)<SUM(expected_sample_count) THEN 'partial' ELSE 'complete' END,CASE WHEN SUM(expected_sample_count)=0 OR SUM(sample_count)=0 THEN 'unavailable' WHEN SUM(health_status='critical')>0 THEN 'critical' WHEN SUM(health_status='warning')>0 THEN 'warning' ELSE 'ok' END,0,? FROM site_status_hourly WHERE site_id=? AND hour_ts>=? AND hour_ts<? GROUP BY site_id ON DUPLICATE KEY UPDATE instance_count_max=VALUES(instance_count_max),online_instance_count_min=VALUES(online_instance_count_min),cpu_max_percent=VALUES(cpu_max_percent),cpu_avg_percent=VALUES(cpu_avg_percent),memory_max_percent=VALUES(memory_max_percent),memory_avg_percent=VALUES(memory_avg_percent),disk_max_used_percent=VALUES(disk_max_used_percent),abnormal_samples=VALUES(abnormal_samples),sample_count=VALUES(sample_count),expected_sample_count=VALUES(expected_sample_count),data_status=VALUES(data_status),health_status=VALUES(health_status),is_final=0,last_calculated_at=VALUES(last_calculated_at)`, []any{dateKey, now, siteID, start, end}},
	}
	startIndex := 0
	if len(hourTuples) == 0 {
		startIndex = 1
	}
	for _, query := range queries[startIndex:] {
		if err := tx.WithContext(ctx).Exec(query.sql, query.args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *DataMaintenanceRepository) publishResourceDaily(ctx context.Context, dateKey int, start, end, now int64) error {
	ids, err := repository.eligibleResourceSiteIDs(ctx, start, end)
	if err != nil {
		return err
	}
	knownNodes, err := repository.knownResourceNodes(ctx, ids, start, end)
	if err != nil {
		return err
	}
	nodeTuples := make([][]any, 0, len(knownNodes))
	for _, node := range knownNodes {
		nodeTuples = append(nodeTuples, []any{node.SiteID, node.NodeName})
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		var state DataMaintenanceState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id=? AND scope_key='global'", MaintenanceResourceDaily).First(&state).Error; err != nil {
			return err
		}
		latest, err := NewDataMaintenanceRepository(tx).ResourceScopeRevision(ctx, 0, start, end)
		if err != nil {
			return err
		}
		if state.DateKey != dateKey || state.ScopeRevision != latest || state.Status == MaintenanceStatusComplete {
			return ErrSiteRunConfigChanged
		}
		var siteTotal, sitePending int64
		siteQuery := tx.Table("site_status_daily").Where("date_key=?", dateKey)
		if len(ids) > 0 {
			siteQuery = siteQuery.Where("site_id IN ?", ids)
		} else {
			siteQuery = siteQuery.Where("1=0")
		}
		if err := siteQuery.Count(&siteTotal).Error; err != nil {
			return err
		}
		if siteTotal != int64(len(ids)) {
			return ErrDataMaintenanceContract
		}
		if err := siteQuery.Where("is_final=0").Count(&sitePending).Error; err != nil {
			return err
		}
		if sitePending != siteTotal {
			return ErrDataMaintenanceContract
		}
		expectedNodes := int64(len(knownNodes))
		instanceQuery := tx.Table("site_instance_status_daily").Where("date_key=?", dateKey)
		if len(nodeTuples) > 0 {
			instanceQuery = instanceQuery.Where("(site_id,node_name) IN ?", nodeTuples)
		} else {
			instanceQuery = instanceQuery.Where("1=0")
		}
		var instanceTotal, instancePending int64
		if err := instanceQuery.Count(&instanceTotal).Error; err != nil {
			return err
		}
		if instanceTotal != expectedNodes {
			return ErrDataMaintenanceContract
		}
		if err := instanceQuery.Where("is_final=0").Count(&instancePending).Error; err != nil {
			return err
		}
		if instancePending != instanceTotal {
			return ErrDataMaintenanceContract
		}
		for _, target := range []struct {
			table   string
			query   *gorm.DB
			pending int64
		}{{"site_status_daily", siteQuery, sitePending}, {"site_instance_status_daily", instanceQuery, instancePending}} {
			updated := target.query.Update("is_final", 1)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != target.pending {
				return ErrDataMaintenanceContract
			}
		}
		state.Status, state.ErrorCode, state.NextAttemptAt = MaintenanceStatusComplete, "", 0
		state.LastSuccessAt, state.UpdatedAt = &now, now
		return tx.Save(&state).Error
	})
}

func resourceDateStart(dateKey int) int64 {
	year, month, day := dateKey/10000, time.Month((dateKey/100)%100), dateKey%100
	return time.Date(year, month, day, 0, 0, 0, 0, resourceRetentionLocation).Unix()
}

func resourceDateEnd(dateKey int) int64 {
	return time.Unix(resourceDateStart(dateKey), 0).In(resourceRetentionLocation).AddDate(0, 0, 1).Unix()
}
