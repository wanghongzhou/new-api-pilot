package dto

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAccountCreateRequestStrictBindingIDs(t *testing.T) {
	request := AccountCreateRequest{SiteID: " 7 ", CustomerID: " 8 ", RemoteUserID: " 9 ", Remark: " note "}
	request.Normalize()
	if errors := request.Validate(); errors != nil {
		t.Fatalf("valid request errors = %#v", errors)
	}
	siteID, customerID, remoteUserID, err := request.BindingIDs()
	if err != nil || siteID != 7 || customerID != 8 || remoteUserID != 9 {
		t.Fatalf("binding IDs = %d/%d/%d, %v", siteID, customerID, remoteUserID, err)
	}

	for _, invalid := range []string{"", "0", "01", "+1", "-1", "1.0", "9223372036854775808"} {
		candidate := AccountCreateRequest{SiteID: invalid, CustomerID: "1", RemoteUserID: "1"}
		if errors := candidate.Validate(); errors == nil || errors["site_id"] == "" {
			t.Fatalf("invalid site_id %q accepted: %#v", invalid, errors)
		}
	}
	request.Remark = strings.Repeat("注", 501)
	if errors := request.Validate(); errors == nil || errors["remark"] == "" {
		t.Fatalf("long remark accepted: %#v", errors)
	}
}

func TestAccountUpdateContractContainsOnlyRemark(t *testing.T) {
	typeInfo := reflect.TypeOf(AccountUpdateRequest{})
	if typeInfo.NumField() != 1 || typeInfo.Field(0).Name != "Remark" {
		t.Fatalf("account update exposes immutable fields: %v", typeInfo)
	}
	request := AccountUpdateRequest{Remark: "  updated  "}
	request.Normalize()
	if request.Remark != "updated" || request.Validate() != nil {
		t.Fatalf("normalized update = %#v, errors=%#v", request, request.Validate())
	}
}

func TestAccountListQueryDefaultsFiltersAndWhitelist(t *testing.T) {
	query := AccountListQuery{}
	query.Normalize()
	if query.Page != 1 || query.PageSize != 20 || query.SortBy != "updated_at" || query.SortOrder != "desc" {
		t.Fatalf("defaults = %#v", query)
	}
	if errors := query.Validate(); errors != nil {
		t.Fatalf("default query errors = %#v", errors)
	}

	remoteStatus := 1
	query = AccountListQuery{
		Page: 2, PageSize: 10, SiteID: "1", CustomerID: "2", RemoteStatus: &remoteStatus,
		RemoteState: AccountRemoteStateIdentityMismatch, ManagedStatus: AccountManagedStatusArchived,
		SortBy: "quota", SortOrder: "asc",
	}
	if errors := query.Validate(); errors != nil || query.Offset() != 10 {
		t.Fatalf("valid filtered query errors=%#v offset=%d", errors, query.Offset())
	}

	query.SiteID = "01"
	query.CustomerID = "-2"
	query.RemoteState = "unknown"
	query.ManagedStatus = "deleted"
	query.SortBy = "quota DESC; DROP TABLE account"
	query.SortOrder = "random"
	errors := query.Validate()
	for _, field := range []string{"site_id", "customer_id", "remote_state", "managed_status", "sort_by", "sort_order"} {
		if errors[field] == "" {
			t.Fatalf("missing %s error in %#v", field, errors)
		}
	}
}

func TestAccountResponseBigintsMarshalAsStrings(t *testing.T) {
	maximum := "9223372036854775807"
	item := AccountListItem{
		ID: maximum, SiteID: maximum, CustomerID: maximum, RemoteUserID: maximum,
		Quota: maximum, UsedQuota: maximum, RequestCount: maximum,
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal account: %v", err)
	}
	text := string(payload)
	for _, field := range []string{"id", "site_id", "customer_id", "remote_user_id", "quota", "used_quota", "request_count"} {
		fragment := `"` + field + `":"` + maximum + `"`
		if !strings.Contains(text, fragment) {
			t.Fatalf("%s is not a JSON string: %s", field, text)
		}
	}
}
