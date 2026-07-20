package dto

import "testing"

func TestUserInventoryExportRemoteUserIDFilterIsCanonical(t *testing.T) {
	value := "9007199254740993"
	query, fields := (ExportFilters{RemoteUserID: &value}).UserInventoryQuery()
	if fields != nil || query.RemoteUserID == nil || *query.RemoteUserID != 9007199254740993 {
		t.Fatalf("remote user export filter query=%#v fields=%#v", query, fields)
	}
	invalid := "01"
	if _, fields = (ExportFilters{RemoteUserID: &invalid}).UserInventoryQuery(); fields == nil || fields["remote_user_id"] == "" {
		t.Fatalf("noncanonical remote user export filter fields=%#v", fields)
	}
}
