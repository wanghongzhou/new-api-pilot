package dto

import (
	"strings"
	"testing"
	"time"
)

func TestResourceQueryValidateBoundaries(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	dayStart := time.Date(2024, time.February, 29, 0, 0, 0, 0, location)
	nodeName := "Node-A"
	tests := []struct {
		name  string
		query ResourceQuery
		valid bool
	}{
		{
			name: "minute",
			query: ResourceQuery{StartTimestamp: dayStart.Unix(), EndTimestamp: dayStart.Add(time.Minute).Unix(),
				Granularity: ResourceGranularityMinute, NodeName: &nodeName},
			valid: true,
		},
		{
			name: "one calendar year of hours",
			query: ResourceQuery{StartTimestamp: dayStart.Unix(), EndTimestamp: dayStart.AddDate(1, 0, 0).Unix(),
				Granularity: ResourceGranularityHour},
			valid: true,
		},
		{
			name: "more than one calendar year of hours",
			query: ResourceQuery{StartTimestamp: dayStart.Unix(), EndTimestamp: dayStart.AddDate(1, 0, 0).Add(time.Hour).Unix(),
				Granularity: ResourceGranularityHour},
		},
		{
			name: "five calendar years of days",
			query: ResourceQuery{StartTimestamp: dayStart.Unix(), EndTimestamp: dayStart.AddDate(5, 0, 0).Unix(),
				Granularity: ResourceGranularityDay},
			valid: true,
		},
		{
			name: "day not aligned in Beijing",
			query: ResourceQuery{StartTimestamp: dayStart.Add(time.Hour).Unix(), EndTimestamp: dayStart.AddDate(0, 0, 1).Unix(),
				Granularity: ResourceGranularityDay},
		},
		{
			name: "node too long",
			query: ResourceQuery{StartTimestamp: dayStart.Unix(), EndTimestamp: dayStart.Add(time.Minute).Unix(),
				Granularity: ResourceGranularityMinute, NodeName: resourceStringPointer(strings.Repeat("界", 129))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errors := test.query.Validate()
			if test.valid && errors != nil {
				t.Fatalf("Validate() errors = %#v", errors)
			}
			if !test.valid && errors == nil {
				t.Fatal("Validate() accepted invalid query")
			}
		})
	}
}

func TestResourceQueryRejectsExcessiveBucketsAndUnsupportedCalendar(t *testing.T) {
	start := int64(1_752_400_800)
	tooMany := ResourceQuery{
		StartTimestamp: start, EndTimestamp: start + int64(ResourceMaximumBuckets+1)*60,
		Granularity: ResourceGranularityMinute,
	}
	if fields := tooMany.Validate(); fields == nil || fields["range"] == "" {
		t.Fatalf("excessive minute query fields = %#v", fields)
	}
	farFuture := ResourceQuery{
		StartTimestamp: 253402300800, EndTimestamp: 253402304400,
		Granularity: ResourceGranularityHour,
	}
	if fields := farFuture.Validate(); fields == nil || fields["range"] == "" {
		t.Fatalf("far-future query fields = %#v", fields)
	}
}

func resourceStringPointer(value string) *string { return &value }
