package dto

import "testing"

func TestSiteAuthorizeRequestConditionalValidation(t *testing.T) {
	rootID := "1"
	token := "secret"
	username := "root"
	password := "password"
	confirmed := true

	validExisting := SiteAuthorizeRequest{Mode: "existing_token", RootUserID: &rootID, AccessToken: &token}
	if errors := validExisting.Validate(); errors != nil {
		t.Fatalf("valid existing-token request errors = %#v", errors)
	}
	mixed := validExisting
	mixed.Username = &username
	if errors := mixed.Validate(); errors == nil || errors["username"] == "" {
		t.Fatalf("mixed-mode request errors = %#v", errors)
	}
	validLogin := SiteAuthorizeRequest{
		Mode: "login_generate_token", Username: &username, Password: &password, ConfirmTokenRotation: &confirmed,
	}
	if errors := validLogin.Validate(); errors != nil {
		t.Fatalf("valid login request errors = %#v", errors)
	}
	leadingZero := "01"
	invalidID := validExisting
	invalidID.RootUserID = &leadingZero
	if errors := invalidID.Validate(); errors == nil || errors["root_user_id"] == "" {
		t.Fatalf("leading-zero ID errors = %#v", errors)
	}
}

func TestSiteBatchRefreshAndBackfillValidation(t *testing.T) {
	if errors := (SiteBatchRefreshRequest{SiteIDs: []string{"1", "2"}}).Validate(); errors != nil {
		t.Fatalf("valid batch errors = %#v", errors)
	}
	if errors := (SiteBatchRefreshRequest{SiteIDs: []string{"1", "1"}}).Validate(); errors == nil {
		t.Fatal("duplicate batch IDs were accepted")
	}
	start := int64(3600)
	end := int64(7200)
	if errors := (SiteBackfillRequest{StartTimestamp: &start, EndTimestamp: &end}).Validate(); errors != nil {
		t.Fatalf("valid backfill errors = %#v", errors)
	}
	unaligned := int64(3601)
	if errors := (SiteBackfillRequest{StartTimestamp: &unaligned, EndTimestamp: &end}).Validate(); errors == nil || errors["start_timestamp"] == "" {
		t.Fatalf("unaligned backfill errors = %#v", errors)
	}
}
