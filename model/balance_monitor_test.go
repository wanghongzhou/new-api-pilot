package model

import (
	"context"
	"encoding/json"
	"fmt"
	"new-api-pilot/dto"
	"testing"
	"time"
)

func TestBalanceMonitorIdempotentAndOutOfOrder(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	repository := NewBalanceMonitorRepository(db.GORM)
	ctx := context.Background()
	source := fmt.Sprintf("balance-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.GORM.Where("source_id = ?", source).Delete(&BalanceMonitorRecord{}) })
	amount := "425.0000000000"
	row := dto.BalanceRecord{SourceID: source, RecordID: "current-one", Revision: "2", Kind: "current", AccountID: "one", SiteID: "site", Name: "new", Day: "2026-09-15", SampledAt: "1789459200", ConsumptionYuan: &amount}
	if e := repository.Ingest(ctx, []dto.BalanceRecord{row, row}, 1); e != nil {
		t.Fatal(e)
	}
	row.Revision = "1"
	row.Name = "old"
	if e := repository.Ingest(ctx, []dto.BalanceRecord{row}, 2); e != nil {
		t.Fatal(e)
	}
	page, e := repository.List(ctx, dto.BalanceQuery{Kind: "current", AccountID: "one", Page: 1, PageSize: 20})
	if e != nil {
		t.Fatal(e)
	}
	if page.Total != "1" || len(page.Items) != 1 {
		t.Fatalf("duplicate rows: %#v", page)
	}
	var got dto.BalanceRecord
	if e = json.Unmarshal(page.Items[0], &got); e != nil {
		t.Fatal(e)
	}
	if got.Name != "new" || got.Revision != "2" || got.ReceivedAt != "1" {
		t.Fatalf("older payload replaced current: %#v", got)
	}
	row.Revision = "3"
	row.Name = "latest"
	if e := repository.Ingest(ctx, []dto.BalanceRecord{row}, 3); e != nil {
		t.Fatal(e)
	}
	page, e = repository.List(ctx, dto.BalanceQuery{Kind: "current", AccountID: "one", Page: 1, PageSize: 20})
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(page.Items[0], &got); e != nil {
		t.Fatal(e)
	}
	if got.Name != "latest" || got.Revision != "3" {
		t.Fatal("newer update lost")
	}
	row.Kind, row.RecordID, row.Day = "daily", "daily-one", "2026-09-14"
	other := row
	other.RecordID, other.Day = "daily-two", "2026-09-15"
	if e := repository.Ingest(ctx, []dto.BalanceRecord{row, other}, 4); e != nil {
		t.Fatal(e)
	}
	page, e = repository.List(ctx, dto.BalanceQuery{Kind: "daily", Start: "2026-09-14", End: "2026-09-14", AccountID: "one", Page: 1, PageSize: 1})
	if e != nil || page.Total != "1" || len(page.Items) != 1 {
		t.Fatalf("inclusive day filter: %#v, %v", page, e)
	}
	page, e = repository.List(ctx, dto.BalanceQuery{Kind: "daily", Start: "2026-09-14", End: "2026-09-15", AccountID: "one", Page: 2, PageSize: 1})
	if e != nil || page.Total != "2" || len(page.Items) != 1 {
		t.Fatalf("page two: %#v, %v", page, e)
	}
	if e = json.Unmarshal(page.Items[0], &got); e != nil {
		t.Fatal(e)
	}
	if got.Day != "2026-09-14" || got.ConsumptionYuan == nil || *got.ConsumptionYuan != amount {
		t.Fatalf("date ordering / decimal preservation: %#v", got)
	}
	row.Kind, row.RecordID, row.Day = "monthly", "monthly-one", "2026-08-01"
	row.AccountID, row.SiteID = "", ""
	row.ConsumptionYuan = nil
	row.Monthly = &dto.MonthlyBalanceSnapshot{Period: "2026-08", Mode: "legacy", GrandTotal: "410.0000000000", ExcludingNowcoding: "410.0000000000", Foxcode: "410.0000000000", Other: "0", RedeemValue: "0", RedeemCount: "0", Nowcoding: "0", Accounts: []dto.MonthlyBalanceAccount{{Name: "old-price", Raw: "2000000000", ResidualYuan: "410.0000000000"}}, Failed: []string{}}
	if e := repository.Ingest(ctx, []dto.BalanceRecord{row, row}, 5); e != nil {
		t.Fatal(e)
	}
	page, e = repository.List(ctx, dto.BalanceQuery{Kind: "monthly", Start: "2026-08-01", End: "2026-08-01", Page: 1, PageSize: 12})
	if e != nil || page.Total != "1" {
		t.Fatalf("monthly query: %#v %v", page, e)
	}
	if e = json.Unmarshal(page.Items[0], &got); e != nil {
		t.Fatal(e)
	}
	if got.Monthly == nil || got.Monthly.Foxcode != "410.0000000000" || len(got.Monthly.Accounts) != 1 {
		t.Fatalf("monthly payload changed: %#v", got.Monthly)
	}
}
