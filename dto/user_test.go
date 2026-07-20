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
