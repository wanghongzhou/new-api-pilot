package dto

import (
	"time"
	"unicode/utf8"
)

const (
	ResourceGranularityMinute = "minute"
	ResourceGranularityHour   = "hour"
	ResourceGranularityDay    = "day"
	ResourceMaximumBuckets    = 200000
)

var resourceLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type ResourceQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    string
	NodeName       *string
}

func (query ResourceQuery) Validate() map[string]string {
	fieldErrors := map[string]string{}
	if query.StartTimestamp <= 0 {
		fieldErrors["start_timestamp"] = "must be a positive Unix timestamp"
	}
	if query.EndTimestamp <= 0 {
		fieldErrors["end_timestamp"] = "must be a positive Unix timestamp"
	}
	if query.StartTimestamp > 0 && query.EndTimestamp > 0 && query.EndTimestamp <= query.StartTimestamp {
		fieldErrors["range"] = "end_timestamp must be greater than start_timestamp"
	}
	if query.NodeName != nil && (!utf8.ValidString(*query.NodeName) || utf8.RuneCountInString(*query.NodeName) < 1 || utf8.RuneCountInString(*query.NodeName) > 128) {
		fieldErrors["node_name"] = "must contain 1 to 128 Unicode characters"
	}
	if query.StartTimestamp <= 0 || query.EndTimestamp <= query.StartTimestamp {
		if !validResourceGranularity(query.Granularity) {
			fieldErrors["granularity"] = "must be minute, hour, or day"
		}
		return nilIfEmptyResourceErrors(fieldErrors)
	}

	start := time.Unix(query.StartTimestamp, 0).In(resourceLocation)
	end := time.Unix(query.EndTimestamp, 0).In(resourceLocation)
	if start.Year() < 1970 || start.Year() > 9999 || end.Year() < 1970 || end.Year() > 9999 {
		fieldErrors["range"] = "timestamps must be within the supported calendar range"
		return nilIfEmptyResourceErrors(fieldErrors)
	}
	switch query.Granularity {
	case ResourceGranularityMinute:
		if query.StartTimestamp%60 != 0 || query.EndTimestamp%60 != 0 {
			fieldErrors["range"] = "minute ranges must align to minute boundaries"
		} else if (query.EndTimestamp-query.StartTimestamp)/60 > ResourceMaximumBuckets {
			fieldErrors["range"] = "minute ranges contain too many buckets"
		}
	case ResourceGranularityHour:
		if query.StartTimestamp%3600 != 0 || query.EndTimestamp%3600 != 0 {
			fieldErrors["range"] = "hour ranges must align to Beijing hour boundaries"
		} else if end.After(start.AddDate(1, 0, 0)) {
			fieldErrors["range"] = "hour ranges must not exceed 1 year"
		} else if (query.EndTimestamp-query.StartTimestamp)/3600 > ResourceMaximumBuckets {
			fieldErrors["range"] = "hour ranges contain too many buckets"
		}
	case ResourceGranularityDay:
		if !resourceDayBoundary(start) || !resourceDayBoundary(end) {
			fieldErrors["range"] = "day ranges must align to Beijing day boundaries"
		} else if end.After(start.AddDate(5, 0, 0)) {
			fieldErrors["range"] = "day ranges must not exceed 5 years"
		} else if (query.EndTimestamp-query.StartTimestamp)/(24*60*60) > ResourceMaximumBuckets {
			fieldErrors["range"] = "day ranges contain too many buckets"
		}
	default:
		fieldErrors["granularity"] = "must be minute, hour, or day"
	}
	return nilIfEmptyResourceErrors(fieldErrors)
}

func validResourceGranularity(value string) bool {
	return value == ResourceGranularityMinute || value == ResourceGranularityHour || value == ResourceGranularityDay
}

func resourceDayBoundary(value time.Time) bool {
	return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func nilIfEmptyResourceErrors(fieldErrors map[string]string) map[string]string {
	if len(fieldErrors) == 0 {
		return nil
	}
	return fieldErrors
}

type ResourcePoint struct {
	BucketStart         int64       `json:"bucket_start"`
	BucketEnd           int64       `json:"bucket_end"`
	CPUMaxPercent       *float64    `json:"cpu_max_percent"`
	CPUAvgPercent       *float64    `json:"cpu_avg_percent"`
	MemoryMaxPercent    *float64    `json:"memory_max_percent"`
	MemoryAvgPercent    *float64    `json:"memory_avg_percent"`
	DiskMaxUsedPercent  *float64    `json:"disk_max_used_percent"`
	DiskLastUsedPercent *float64    `json:"disk_last_used_percent"`
	InstanceCount       *int        `json:"instance_count"`
	OnlineInstanceCount *int        `json:"online_instance_count"`
	SampleCount         int         `json:"sample_count"`
	ExpectedSampleCount int         `json:"expected_sample_count"`
	HealthStatus        string      `json:"health_status"`
	DataStatus          string      `json:"data_status"`
	AsOf                *int64      `json:"as_of"`
	IsFinal             bool        `json:"is_final"`
	Reason              *MessageRef `json:"reason"`
}

type SiteResourceResponse struct {
	SiteID      string          `json:"site_id"`
	NodeName    *string         `json:"node_name"`
	Granularity string          `json:"granularity"`
	Summary     *ResourcePoint  `json:"summary"`
	Trend       []ResourcePoint `json:"trend"`
}
