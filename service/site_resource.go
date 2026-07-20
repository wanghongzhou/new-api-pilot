package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

var siteResourceLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func (service *SiteService) ResourceStatus(
	ctx context.Context,
	siteID int64,
	query dto.ResourceQuery,
) (dto.SiteResourceResponse, error) {
	if query.Validate() != nil {
		return dto.SiteResourceResponse{}, ErrSiteResourceRange
	}
	now := service.clock.Now().Unix()
	if !validClosedResourceRange(query, now) {
		return dto.SiteResourceResponse{}, ErrSiteResourceRange
	}
	if query.Granularity == dto.ResourceGranularityMinute {
		retentionDays, err := service.sites.IntSetting(ctx, "collector.minute_retention_days", 1, 3650)
		if err != nil {
			return dto.SiteResourceResponse{}, fmt.Errorf("read resource minute retention: %w", err)
		}
		if !validMinuteResourceRange(query, now, retentionDays) {
			return dto.SiteResourceResponse{}, ErrSiteResourceRange
		}
	}

	scope, err := service.sites.FindResourceScope(ctx, siteID, query.NodeName)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteResourceResponse{}, ErrSiteNotFound
		}
		return dto.SiteResourceResponse{}, fmt.Errorf("read site resource scope: %w", err)
	}
	response := dto.SiteResourceResponse{
		SiteID: strconv.FormatInt(siteID, 10), NodeName: cloneResourceString(query.NodeName),
		Granularity: query.Granularity, Trend: []dto.ResourcePoint{},
	}
	if query.NodeName != nil && !scope.NodeExists() {
		return response, nil
	}
	lifecycleStart := scope.MonitoringStartAt
	if query.NodeName != nil {
		value := floorMinute(*scope.NodeFirstSeenAt)
		if lifecycleStart != nil && *lifecycleStart > value {
			value = *lifecycleStart
		}
		lifecycleStart = &value
	}
	if lifecycleStart == nil {
		return response, nil
	}

	readRequest := model.SiteResourceReadRequest{
		SiteID: siteID, NodeName: query.NodeName, Granularity: query.Granularity,
		StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp,
	}
	if query.Granularity == dto.ResourceGranularityDay {
		readRequest.StartDateKey = siteResourceDateKey(query.StartTimestamp)
		readRequest.EndDateKey = siteResourceDateKey(query.EndTimestamp)
	}
	rows, err := service.sites.ListResourceRows(ctx, readRequest)
	if err != nil {
		return dto.SiteResourceResponse{}, fmt.Errorf("list site resource rows: %w", err)
	}
	pauses, err := service.sites.ListMonitoringPauses(ctx, siteID, query.StartTimestamp, query.EndTimestamp)
	if err != nil {
		return dto.SiteResourceResponse{}, fmt.Errorf("list site monitoring pauses: %w", err)
	}
	builder := siteResourceBuilder{
		siteID: siteID, query: query, now: now, lifecycleStart: *lifecycleStart,
		lifecycleEnd: scope.StatisticsEndAt, rows: rows, pauses: pauses,
	}
	trend, err := builder.buildTrend()
	if err != nil {
		return dto.SiteResourceResponse{}, err
	}
	response.Trend = trend
	response.Summary = builder.buildSummary(trend)
	return response, nil
}

type siteResourceBuilder struct {
	siteID         int64
	query          dto.ResourceQuery
	now            int64
	lifecycleStart int64
	lifecycleEnd   *int64
	rows           []model.SiteResourceRow
	pauses         []model.SiteMonitoringPause
}

type siteResourceBucket struct {
	start int64
	end   int64
}

func (builder siteResourceBuilder) buildTrend() ([]dto.ResourcePoint, error) {
	rows, err := builder.rowsByBucket()
	if err != nil {
		return nil, err
	}
	trend := make([]dto.ResourcePoint, 0)
	for _, bucket := range resourceBuckets(builder.query) {
		activeMinutes, expectedMinutes, pausedMinutes := builder.expectedMinutes(bucket)
		if activeMinutes == 0 {
			continue
		}
		row, exists := rows[bucket.start]
		point, err := builder.pointForBucket(bucket, row, exists, expectedMinutes, pausedMinutes)
		if err != nil {
			return nil, err
		}
		trend = append(trend, point)
	}
	return trend, nil
}

func (builder siteResourceBuilder) rowsByBucket() (map[int64]model.SiteResourceRow, error) {
	result := make(map[int64]model.SiteResourceRow, len(builder.rows))
	for _, row := range builder.rows {
		bucketStart := row.BucketStart
		if builder.query.Granularity == dto.ResourceGranularityDay {
			var ok bool
			bucketStart, ok = siteResourceTimestampFromDateKey(row.DateKey)
			if !ok {
				return nil, errors.New("site resource daily row has an invalid date_key")
			}
		}
		if _, duplicate := result[bucketStart]; duplicate {
			return nil, errors.New("site resource query returned duplicate buckets")
		}
		row.BucketStart = bucketStart
		result[bucketStart] = row
	}
	return result, nil
}

func (builder siteResourceBuilder) expectedMinutes(bucket siteResourceBucket) (int, int, int) {
	start := maxResourceTimestamp(bucket.start, builder.lifecycleStart)
	end := bucket.end
	if builder.lifecycleEnd != nil {
		end = minResourceTimestamp(end, *builder.lifecycleEnd)
	}
	horizon := floorMinute(builder.now) + 60
	end = minResourceTimestamp(end, horizon)
	if end <= start {
		return 0, 0, 0
	}
	active := int((end - start) / 60)
	paused := builder.pausedMinutes(start, end)
	if paused > active {
		paused = active
	}
	return active, active - paused, paused
}

func (builder siteResourceBuilder) pausedMinutes(start, end int64) int {
	covered := 0
	cursor := start
	for _, pause := range builder.pauses {
		pauseStart := maxResourceTimestamp(start, pause.StartMinuteTS)
		pauseEnd := end
		if pause.EndMinuteTS != nil {
			pauseEnd = minResourceTimestamp(pauseEnd, *pause.EndMinuteTS)
		}
		if pauseStart < cursor {
			pauseStart = cursor
		}
		if pauseEnd <= pauseStart {
			continue
		}
		covered += int((pauseEnd - pauseStart) / 60)
		cursor = pauseEnd
		if cursor >= end {
			break
		}
	}
	return covered
}

func (builder siteResourceBuilder) pointForBucket(
	bucket siteResourceBucket,
	row model.SiteResourceRow,
	exists bool,
	expectedMinutes, pausedMinutes int,
) (dto.ResourcePoint, error) {
	if !exists || (builder.query.Granularity == dto.ResourceGranularityMinute && expectedMinutes == 0) {
		status := "missing"
		switch {
		case expectedMinutes == 0 && pausedMinutes > 0:
			status = "paused"
		case !builder.bucketEnded(bucket):
			status = "pending"
		}
		point := dto.ResourcePoint{
			BucketStart: bucket.start, BucketEnd: bucket.end,
			SampleCount: 0, ExpectedSampleCount: expectedMinutes,
			HealthStatus: constant.SiteHealthUnavailable, DataStatus: status,
			IsFinal: builder.derivedFinal(bucket, status),
		}
		point.Reason = resourcePointReason(builder.siteID, status, bucket.start, bucket.end)
		return point, nil
	}
	if builder.query.Granularity == dto.ResourceGranularityMinute {
		row.ExpectedSampleCount = expectedMinutes
	}
	if err := validateSiteResourceRow(row, builder.query.Granularity); err != nil {
		return dto.ResourcePoint{}, err
	}
	point := dto.ResourcePoint{
		BucketStart: bucket.start, BucketEnd: bucket.end,
		CPUMaxPercent: row.CPUMaxPercent, CPUAvgPercent: row.CPUAvgPercent,
		MemoryMaxPercent: row.MemoryMaxPercent, MemoryAvgPercent: row.MemoryAvgPercent,
		DiskMaxUsedPercent: row.DiskMaxUsedPercent, DiskLastUsedPercent: row.DiskLastUsedPercent,
		InstanceCount: row.InstanceCount, OnlineInstanceCount: row.OnlineInstanceCount,
		SampleCount: row.SampleCount, ExpectedSampleCount: row.ExpectedSampleCount,
		HealthStatus: row.HealthStatus, DataStatus: row.DataStatus, AsOf: row.SourceAsOf,
		IsFinal: row.IsFinal,
	}
	if builder.query.Granularity != dto.ResourceGranularityDay {
		point.IsFinal = builder.derivedFinal(bucket, row.DataStatus)
	}
	point.Reason = resourcePointReason(builder.siteID, point.DataStatus, bucket.start, bucket.end)
	return point, nil
}

func (builder siteResourceBuilder) bucketEnded(bucket siteResourceBucket) bool {
	switch builder.query.Granularity {
	case dto.ResourceGranularityMinute:
		return bucket.end <= floorMinute(builder.now)
	case dto.ResourceGranularityHour:
		return bucket.end <= builder.now-builder.now%3600
	case dto.ResourceGranularityDay:
		currentDay := time.Unix(builder.now, 0).In(siteResourceLocation)
		start := time.Date(currentDay.Year(), currentDay.Month(), currentDay.Day(), 0, 0, 0, 0, siteResourceLocation).Unix()
		return bucket.end <= start
	default:
		return false
	}
}

func (builder siteResourceBuilder) derivedFinal(bucket siteResourceBucket, status string) bool {
	if builder.query.Granularity == dto.ResourceGranularityDay {
		return false
	}
	return builder.bucketEnded(bucket) && (status == "complete" || status == "paused")
}

func (builder siteResourceBuilder) buildSummary(trend []dto.ResourcePoint) *dto.ResourcePoint {
	if len(trend) == 0 {
		return nil
	}
	summary := &dto.ResourcePoint{
		BucketStart: builder.query.StartTimestamp, BucketEnd: builder.query.EndTimestamp,
		HealthStatus: constant.SiteHealthUnavailable, DataStatus: "complete", IsFinal: true,
	}
	var cpuAvgTotal, memoryAvgTotal float64
	var cpuAvgWeight, memoryAvgWeight int
	hasObservedHealth := false
	hasIncomplete := false
	zeroSampleStatus := "paused"
	for _, point := range trend {
		summary.SampleCount += point.SampleCount
		summary.ExpectedSampleCount += point.ExpectedSampleCount
		summary.CPUMaxPercent = resourceMaxFloat(summary.CPUMaxPercent, point.CPUMaxPercent)
		summary.MemoryMaxPercent = resourceMaxFloat(summary.MemoryMaxPercent, point.MemoryMaxPercent)
		summary.DiskMaxUsedPercent = resourceMaxFloat(summary.DiskMaxUsedPercent, point.DiskMaxUsedPercent)
		if point.DiskLastUsedPercent != nil {
			value := *point.DiskLastUsedPercent
			summary.DiskLastUsedPercent = &value
		}
		summary.InstanceCount = resourceMaxInt(summary.InstanceCount, point.InstanceCount)
		summary.OnlineInstanceCount = resourceMinInt(summary.OnlineInstanceCount, point.OnlineInstanceCount)
		if point.CPUAvgPercent != nil && point.SampleCount > 0 {
			cpuAvgTotal += *point.CPUAvgPercent * float64(point.SampleCount)
			cpuAvgWeight += point.SampleCount
		}
		if point.MemoryAvgPercent != nil && point.SampleCount > 0 {
			memoryAvgTotal += *point.MemoryAvgPercent * float64(point.SampleCount)
			memoryAvgWeight += point.SampleCount
		}
		if point.SampleCount > 0 {
			summary.HealthStatus = resourceWorseHealth(summary.HealthStatus, point.HealthStatus, hasObservedHealth)
			hasObservedHealth = true
		}
		if point.AsOf != nil && (summary.AsOf == nil || *point.AsOf > *summary.AsOf) {
			value := *point.AsOf
			summary.AsOf = &value
		}
		if !point.IsFinal {
			summary.IsFinal = false
		}
		if point.DataStatus != "complete" && point.DataStatus != "paused" {
			hasIncomplete = true
			zeroSampleStatus = resourceHigherStatus(zeroSampleStatus, point.DataStatus)
		}
	}
	if cpuAvgWeight > 0 {
		value := cpuAvgTotal / float64(cpuAvgWeight)
		summary.CPUAvgPercent = &value
	}
	if memoryAvgWeight > 0 {
		value := memoryAvgTotal / float64(memoryAvgWeight)
		summary.MemoryAvgPercent = &value
	}
	switch {
	case summary.ExpectedSampleCount == 0:
		summary.DataStatus = zeroSampleStatus
	case summary.SampleCount == summary.ExpectedSampleCount && !hasIncomplete:
		summary.DataStatus = "complete"
	case summary.SampleCount > 0:
		summary.DataStatus = "partial"
	default:
		summary.DataStatus = zeroSampleStatus
	}
	if summary.DataStatus != "complete" && summary.DataStatus != "paused" {
		summary.IsFinal = false
	}
	summary.Reason = resourcePointReason(builder.siteID, summary.DataStatus, summary.BucketStart, summary.BucketEnd)
	return summary
}

func resourceBuckets(query dto.ResourceQuery) []siteResourceBucket {
	count := 0
	switch query.Granularity {
	case dto.ResourceGranularityMinute:
		count = int((query.EndTimestamp - query.StartTimestamp) / 60)
	case dto.ResourceGranularityHour:
		count = int((query.EndTimestamp - query.StartTimestamp) / 3600)
	case dto.ResourceGranularityDay:
		count = int((query.EndTimestamp - query.StartTimestamp) / (24 * 60 * 60))
	}
	if count < 1 || count > dto.ResourceMaximumBuckets {
		return []siteResourceBucket{}
	}
	result := make([]siteResourceBucket, 0, count)
	step := int64(60)
	if query.Granularity == dto.ResourceGranularityHour {
		step = 3600
	} else if query.Granularity == dto.ResourceGranularityDay {
		step = 24 * 60 * 60
	}
	for start := query.StartTimestamp; start < query.EndTimestamp; start += step {
		result = append(result, siteResourceBucket{start: start, end: start + step})
	}
	return result
}

func validMinuteResourceRange(query dto.ResourceQuery, now int64, retentionDays int) bool {
	if query.Granularity != dto.ResourceGranularityMinute || retentionDays < 1 || retentionDays > 3650 {
		return false
	}
	closedEnd := floorMinute(now)
	retentionSeconds := int64(retentionDays) * 24 * 60 * 60
	if query.EndTimestamp > closedEnd || query.StartTimestamp < closedEnd-retentionSeconds {
		return false
	}
	span := query.EndTimestamp - query.StartTimestamp
	return span > 0 && span <= retentionSeconds && span/60 <= dto.ResourceMaximumBuckets
}

func validClosedResourceRange(query dto.ResourceQuery, now int64) bool {
	if now <= 0 || query.EndTimestamp <= query.StartTimestamp {
		return false
	}
	var bucketSeconds, closedEnd int64
	switch query.Granularity {
	case dto.ResourceGranularityMinute:
		bucketSeconds, closedEnd = 60, floorMinute(now)
	case dto.ResourceGranularityHour:
		bucketSeconds, closedEnd = 3600, now-now%3600
	case dto.ResourceGranularityDay:
		bucketSeconds = 24 * 60 * 60
		local := time.Unix(now, 0).In(siteResourceLocation)
		closedEnd = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, siteResourceLocation).Unix()
	default:
		return false
	}
	span := query.EndTimestamp - query.StartTimestamp
	return query.EndTimestamp <= closedEnd && span%bucketSeconds == 0 &&
		span/bucketSeconds > 0 && span/bucketSeconds <= dto.ResourceMaximumBuckets
}

func validateSiteResourceRow(row model.SiteResourceRow, granularity string) error {
	if row.SampleCount < 0 || row.ExpectedSampleCount < 0 || row.SampleCount > row.ExpectedSampleCount {
		return errors.New("site resource row has invalid sample counts")
	}
	if row.SourceAsOf == nil || *row.SourceAsOf <= 0 {
		return errors.New("site resource row has invalid freshness evidence")
	}
	if !validResourceDataStatus(row.DataStatus) || !dto.ValidSiteHealthStatus(row.HealthStatus) {
		return errors.New("site resource row has invalid status")
	}
	if row.DataStatus == "complete" && (row.ExpectedSampleCount == 0 || row.SampleCount != row.ExpectedSampleCount) {
		return errors.New("complete site resource row has inconsistent sample counts")
	}
	if row.DataStatus == "partial" && (row.SampleCount == 0 || row.SampleCount >= row.ExpectedSampleCount) {
		return errors.New("partial site resource row has inconsistent sample counts")
	}
	if row.DataStatus == "missing" && (row.SampleCount != 0 || row.ExpectedSampleCount == 0) {
		return errors.New("missing site resource row has inconsistent sample counts")
	}
	if row.DataStatus == "paused" && (row.SampleCount != 0 || row.ExpectedSampleCount != 0) {
		return errors.New("paused site resource row has inconsistent sample counts")
	}
	if granularity == dto.ResourceGranularityDay && row.IsFinal && row.DataStatus != "complete" && row.DataStatus != "paused" {
		return errors.New("final site resource row is not complete or paused")
	}
	for _, value := range []*float64{
		row.CPUMaxPercent, row.CPUAvgPercent, row.MemoryMaxPercent, row.MemoryAvgPercent,
		row.DiskMaxUsedPercent, row.DiskLastUsedPercent,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 100) {
			return errors.New("site resource row has invalid percentage")
		}
	}
	if row.InstanceCount != nil && *row.InstanceCount < 0 || row.OnlineInstanceCount != nil && *row.OnlineInstanceCount < 0 {
		return errors.New("site resource row has invalid instance counts")
	}
	if row.InstanceCount != nil && row.OnlineInstanceCount != nil && *row.OnlineInstanceCount > *row.InstanceCount {
		return errors.New("site resource row has inconsistent instance counts")
	}
	return nil
}

func resourcePointReason(siteID int64, status string, start, end int64) *dto.MessageRef {
	siteIDString := strconv.FormatInt(siteID, 10)
	var reason dto.MessageRef
	switch status {
	case "complete":
		return nil
	case "partial", "missing":
		reason = dto.MustMessageRef(constant.MessageDataWindowMissing, map[string]any{
			"site_id": siteIDString, "start_timestamp": start, "end_timestamp": end,
		}, "")
	case "unavailable":
		reason = dto.MustMessageRef(constant.MessageDataUpstreamUnavailable, map[string]any{
			"site_id": siteIDString, "start_timestamp": start, "end_timestamp": end,
		}, "")
	case "paused":
		reason = dto.MustMessageRef(constant.MessageDataScopePaused, map[string]any{
			"scope_type": "site", "scope_id": siteIDString,
			"start_timestamp": start, "end_timestamp": end,
		}, "")
	case "backfilling":
		reason = dto.MustMessageRef(constant.MessageDataBackfilling, map[string]any{
			"scope_type": "site", "scope_id": siteIDString, "progress": float64(0),
		}, "")
	default:
		reason = dto.MustMessageRef(constant.MessageDataPending, map[string]any{
			"scope_type": "site", "scope_id": siteIDString, "progress": float64(0),
		}, "")
	}
	return &reason
}

func validResourceDataStatus(value string) bool {
	switch value {
	case "complete", "partial", "pending", "missing", "unavailable", "paused", "backfilling":
		return true
	default:
		return false
	}
}

func siteResourceDateKey(timestamp int64) int {
	local := time.Unix(timestamp, 0).In(siteResourceLocation)
	return local.Year()*10000 + int(local.Month())*100 + local.Day()
}

func siteResourceTimestampFromDateKey(dateKey int) (int64, bool) {
	year := dateKey / 10000
	month := time.Month((dateKey / 100) % 100)
	day := dateKey % 100
	if year < 1970 || month < time.January || month > time.December || day < 1 || day > 31 {
		return 0, false
	}
	value := time.Date(year, month, day, 0, 0, 0, 0, siteResourceLocation)
	if value.Year() != year || value.Month() != month || value.Day() != day {
		return 0, false
	}
	return value.Unix(), true
}

func cloneResourceString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func resourceMaxFloat(current, candidate *float64) *float64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		value := *candidate
		return &value
	}
	return current
}

func resourceMaxInt(current, candidate *int) *int {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		value := *candidate
		return &value
	}
	return current
}

func resourceMinInt(current, candidate *int) *int {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		value := *candidate
		return &value
	}
	return current
}

func resourceWorseHealth(current, candidate string, hasCurrent bool) string {
	if !hasCurrent {
		return candidate
	}
	priority := map[string]int{
		constant.SiteHealthOK: 1, constant.SiteHealthUnavailable: 2,
		constant.SiteHealthWarning: 3, constant.SiteHealthCritical: 4,
	}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}

func resourceHigherStatus(current, candidate string) string {
	priority := map[string]int{
		"complete": 0, "paused": 1, "pending": 2, "backfilling": 3,
		"unavailable": 4, "missing": 5, "partial": 6,
	}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}

func minResourceTimestamp(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxResourceTimestamp(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
