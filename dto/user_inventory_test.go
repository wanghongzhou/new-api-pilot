package dto

import "testing"

func TestUserInventoryQueryRejectsUnknownRoleAndStatus(t *testing.T) {
	for _, query := range []UserInventoryQuery{
		{Page: 1, PageSize: 20, Roles: []int{2}},
		{Page: 1, PageSize: 20, Statuses: []int{0}},
	} {
		if fields := query.Validate(); fields == nil {
			t.Fatalf("accepted invalid inventory enums: %+v", query)
		}
	}
	if fields := (UserInventoryQuery{Page: 1, PageSize: 20, Roles: []int{0, 1, 10, 100}, Statuses: []int{1, 2}}).Validate(); fields != nil {
		t.Fatalf("rejected authoritative inventory enums: %v", fields)
	}
}
