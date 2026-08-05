package dto

import "testing"

func TestPerformanceHistoryQueryRequiresAlignedBoundedFilters(t *testing.T) {
	valid := PerformanceHistoryQuery{Page: 1, PageSize: 20, StartTimestamp: 3600, EndTimestamp: 7200}
	if fields := valid.Validate(); fields != nil {
		t.Fatalf("valid query rejected: %#v", fields)
	}
	unaligned := valid
	unaligned.StartTimestamp++
	if fields := unaligned.Validate(); fields == nil || fields["range"] == "" {
		t.Fatalf("unaligned range accepted: %#v", fields)
	}
	tooMany := valid
	tooMany.ModelNames = make([]string, 101)
	for i := range tooMany.ModelNames {
		tooMany.ModelNames[i] = "model"
	}
	if fields := tooMany.Validate(); fields == nil || fields["filters"] == "" {
		t.Fatalf("oversized filters accepted: %#v", fields)
	}
}

func TestFinanceRedemptionStatusesAreCanonical(t *testing.T) {
	for _, statuses := range [][]string{{"0", "1", "expired"}, {}} {
		if fields := (FinanceInventoryQuery{Statuses: statuses}).ValidateRedemptionStatuses(); fields != nil {
			t.Fatalf("valid statuses rejected: %#v", fields)
		}
	}
	for _, status := range []string{"01", "1abc", "-1", "unknown"} {
		if fields := (FinanceInventoryQuery{Statuses: []string{status}}).ValidateRedemptionStatuses(); fields == nil {
			t.Fatalf("invalid status %q accepted", status)
		}
	}
}
