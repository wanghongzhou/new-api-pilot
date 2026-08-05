package dto

import "testing"

func TestLoginRequestValidatesOnlyRequiredCredentials(t *testing.T) {
	if fields := (LoginRequest{Username: "viewer-one", Password: "wrong"}).Validate(); fields != nil {
		t.Fatalf("short login password rejected: %#v", fields)
	}
	if fields := (LoginRequest{Username: "viewer-one", Password: ""}).Validate(); fields == nil || fields["password"] == "" {
		t.Fatalf("empty login password accepted: %#v", fields)
	}
}

func TestUpdatePlatformUserRequestRequiresOptimisticTimestamp(t *testing.T) {
	request := UpdatePlatformUserRequest{
		Username: "viewer-one", DisplayName: "Viewer One", Role: "viewer", ExpectedUpdatedAt: 100,
	}
	if fields := request.Validate(); fields != nil {
		t.Fatalf("valid platform user update rejected: %#v", fields)
	}
	request.ExpectedUpdatedAt = 0
	if fields := request.Validate(); fields == nil || fields["expected_updated_at"] == "" {
		t.Fatalf("missing optimistic timestamp accepted: %#v", fields)
	}
}
