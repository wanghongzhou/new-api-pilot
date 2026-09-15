package dto

import "testing"

func validBalanceBatch() BalanceIngest {
	amount := "212.5000000000"
	return BalanceIngest{SchemaVersion: 1, SourceID: "12345678-1234-1234-1234-123456789abc", Records: []BalanceRecord{{SourceID: "12345678-1234-1234-1234-123456789abc", RecordID: "12345678-1234-1234-1234-123456789abc", AccountID: "12345678-1234-1234-1234-123456789abc", SiteID: "12345678-1234-1234-1234-123456789abc", Revision: "1", Kind: "interval", Day: "2026-09-15", SampledAt: "1789459200", ConsumptionYuan: &amount}}}
}
func TestBalanceBatchValidation(t *testing.T) {
	if e := validBalanceBatch().Validate(); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"NaN", "1e20", "1.00000000001", "-0.1", "100000000000000000000"} {
		b := validBalanceBatch()
		b.Records[0].ConsumptionYuan = &value
		if b.Validate() == nil {
			t.Errorf("accepted %s", value)
		}
	}
	b := validBalanceBatch()
	b.Records[0].SourceID = "different"
	if b.Validate() == nil {
		t.Fatal("cross-source accepted")
	}
	b = validBalanceBatch()
	b.Records[0].Revision = "9223372036854775808"
	if b.Validate() == nil {
		t.Fatal("overflow accepted")
	}
}
func TestBalanceQueryBounds(t *testing.T) {
	q := BalanceQuery{Kind: "daily", Start: "2026-09-01", End: "2026-09-15", Page: 1, PageSize: 50}
	if e := q.Validate(); e != nil {
		t.Fatal(e)
	}
	q.End = "2025-09-01"
	if q.Validate() == nil {
		t.Fatal("reverse date accepted")
	}
	q.End = "2028-09-01"
	if q.Validate() == nil {
		t.Fatal("unbounded dates accepted")
	}
	q.Kind = "current"
	q.PageSize = 201
	if q.Validate() == nil {
		t.Fatal("unbounded page accepted")
	}
}

func TestMonthlyBalanceValidation(t *testing.T) {
	m := &MonthlyBalanceSnapshot{Period: "2026-08", Mode: "legacy", GrandTotal: "1290.00", ExcludingNowcoding: "1240.00", Foxcode: "410.00", Other: "20.00", RedeemValue: "810.00", RedeemCount: "2", Nowcoding: "50.00", Accounts: []MonthlyBalanceAccount{{Name: "fox", Raw: "2000000000", ResidualYuan: "410"}}, Failed: []string{"offline"}}
	b := validBalanceBatch()
	r := &b.Records[0]
	r.Kind = "monthly"
	r.AccountID = ""
	r.SiteID = ""
	r.Day = "2026-08-01"
	r.ConsumptionYuan = nil
	r.Monthly = m
	if e := b.Validate(); e != nil {
		t.Fatal(e)
	}
	m.RedeemCount = "-1"
	if b.Validate() == nil {
		t.Fatal("negative inventory accepted")
	}
	m.RedeemCount = "2"
	r.Day = "2026-09-01"
	if b.Validate() == nil {
		t.Fatal("month mismatch accepted")
	}
	r.Day = "2026-08-01"
	m.GrandTotal = "NaN"
	if b.Validate() == nil {
		t.Fatal("nonfinite total accepted")
	}
	m.GrandTotal = "1290"
	r.Kind = "inventory"
	if b.Validate() == nil {
		t.Fatal("monthly payload on other kind accepted")
	}
}

func TestMonthlyBalanceQueryBounds(t *testing.T) {
	q := BalanceQuery{Kind: "monthly", Start: "2025-10-01", End: "2026-09-01", Page: 1, PageSize: 12}
	if e := q.Validate(); e != nil {
		t.Fatal(e)
	}
	q.Start = "2025-10-02"
	if q.Validate() == nil {
		t.Fatal("nonmonth boundary accepted")
	}
	q.Start = "2025-10-01"
	q.End = "2035-10-01"
	if q.Validate() == nil {
		t.Fatal("unbounded months accepted")
	}
	q.End = "2026-09-01"
	q.AccountID = "12345678-1234-1234-1234-123456789abc"
	if q.Validate() == nil {
		t.Fatal("partial report filter accepted")
	}
}
