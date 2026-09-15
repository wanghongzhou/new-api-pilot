package dto

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"time"
)

// BalanceRecord preserves collector decimal strings without float conversion.
type BalanceRecord struct {
	Monthly         *MonthlyBalanceSnapshot `json:"monthly,omitempty"`
	OpeningBalance  *string                 `json:"opening_balance,omitempty"`
	ClosingBalance  *string                 `json:"closing_balance,omitempty"`
	OpeningAt       string                  `json:"opening_at,omitempty"`
	ClosingAt       string                  `json:"closing_at,omitempty"`
	SourceID        string                  `json:"source_id"`
	RecordID        string                  `json:"record_id"`
	Revision        string                  `json:"revision"`
	Kind            string                  `json:"kind"`
	AccountID       string                  `json:"account_id"`
	SiteID          string                  `json:"site_id"`
	Name            string                  `json:"name"`
	SiteName        string                  `json:"site_name"`
	Day             string                  `json:"day"`
	SampledAt       string                  `json:"sampled_at"`
	StartedAt       string                  `json:"started_at,omitempty"`
	ReceivedAt      string                  `json:"received_at,omitempty"`
	Field           string                  `json:"field,omitempty"`
	Balance         *string                 `json:"balance"`
	PreviousBalance *string                 `json:"previous_balance,omitempty"`
	BalanceDelta    *string                 `json:"balance_delta"`
	Recharge        *string                 `json:"recharge,omitempty"`
	Consumption     *string                 `json:"consumption"`
	Rate            *string                 `json:"rate,omitempty"`
	BalanceYuan     *string                 `json:"balance_yuan"`
	ConsumptionYuan *string                 `json:"consumption_yuan"`
	UnitPrice       *string                 `json:"unit_price,omitempty"`
	Flags           []string                `json:"flags"`
	Standalone      bool                    `json:"standalone,omitempty"`
	CoverageSeconds string                  `json:"coverage_seconds,omitempty"`
	SampleCount     string                  `json:"sample_count,omitempty"`
	AnomalyCount    string                  `json:"anomaly_count,omitempty"`
	LastSuccessAt   string                  `json:"last_success_at,omitempty"`
	LastBalance     *string                 `json:"last_balance,omitempty"`
}

type BalanceIngest struct {
	SchemaVersion int             `json:"schema_version"`
	SourceID      string          `json:"source_id"`
	Records       []BalanceRecord `json:"records"`
}

var balanceID = regexp.MustCompile(`^[a-f0-9-]{32,64}$`)
var balanceDecimal = regexp.MustCompile(`^-?[0-9]{1,20}(\.[0-9]{1,10})?$`)

func (v BalanceIngest) Validate() error {
	if v.SchemaVersion != 1 || !balanceID.MatchString(v.SourceID) || len(v.Records) == 0 || len(v.Records) > 200 {
		return fmt.Errorf("invalid batch")
	}
	for _, r := range v.Records {
		if r.SourceID != v.SourceID || !balanceID.MatchString(r.RecordID) || len(r.Name) > 128 || len(r.SiteName) > 255 {
			return fmt.Errorf("invalid identity")
		}
		switch r.Kind {
		case "interval", "daily", "current":
			if !balanceID.MatchString(r.AccountID) || !balanceID.MatchString(r.SiteID) {
				return fmt.Errorf("invalid account")
			}
		case "inventory":
			if r.AccountID != "" || r.SiteID != "" {
				return fmt.Errorf("invalid inventory")
			}
		case "monthly":
			if r.AccountID != "" || r.SiteID != "" || r.Monthly == nil {
				return fmt.Errorf("invalid monthly record")
			}
			if err := r.Monthly.Validate(); err != nil {
				return err
			}
			if r.Day != r.Monthly.Period+"-01" || r.Consumption != nil || r.ConsumptionYuan != nil {
				return fmt.Errorf("invalid monthly semantics")
			}
		default:
			return fmt.Errorf("invalid kind")
		}
		if r.Kind != "monthly" && r.Monthly != nil {
			return fmt.Errorf("unexpected monthly payload")
		}
		for _, n := range []string{r.Revision, r.SampledAt} {
			x, e := strconv.ParseInt(n, 10, 64)
			if e != nil || x <= 0 {
				return fmt.Errorf("invalid sequence")
			}
		}
		if _, e := time.Parse("2006-01-02", r.Day); e != nil {
			return fmt.Errorf("invalid day")
		}
		if len(r.Flags) > 10 || len(r.Field) > 64 {
			return fmt.Errorf("invalid metadata")
		}
		for _, f := range r.Flags {
			switch f {
			case "baseline", "unavailable", "gap", "recharge_unknown", "balance_increase", "allocated", "partial", "anomaly":
			default:
				return fmt.Errorf("invalid flag")
			}
		}
		for _, n := range []*string{r.Balance, r.PreviousBalance, r.BalanceDelta, r.Recharge, r.Consumption, r.Rate, r.BalanceYuan, r.ConsumptionYuan, r.UnitPrice, r.LastBalance, r.OpeningBalance, r.ClosingBalance} {
			if n != nil && !balanceDecimal.MatchString(*n) {
				return fmt.Errorf("invalid decimal")
			}
		}
		for _, n := range []*string{r.Consumption, r.ConsumptionYuan, r.Rate, r.Recharge, r.UnitPrice} {
			if n != nil {
				value, ok := new(big.Rat).SetString(*n)
				if !ok || value.Sign() < 0 {
					return fmt.Errorf("negative amount")
				}
			}
		}
		for _, n := range []string{r.StartedAt, r.CoverageSeconds, r.SampleCount, r.AnomalyCount, r.LastSuccessAt, r.OpeningAt, r.ClosingAt} {
			if n != "" {
				x, e := strconv.ParseInt(n, 10, 64)
				if e != nil || x < 0 {
					return fmt.Errorf("invalid count")
				}
			}
		}
	}
	return nil
}

type BalanceQuery struct {
	Kind, Start, End, AccountID, SiteID string
	Page, PageSize                      int
}

func (q BalanceQuery) Validate() error {
	if q.Kind != "current" && q.Kind != "interval" && q.Kind != "daily" && q.Kind != "inventory" && q.Kind != "monthly" {
		return fmt.Errorf("invalid kind")
	}
	if q.Page < 1 || q.Page > 100000 || q.PageSize < 1 || q.PageSize > 200 {
		return fmt.Errorf("invalid pagination")
	}
	if q.AccountID != "" && !balanceID.MatchString(q.AccountID) || q.SiteID != "" && !balanceID.MatchString(q.SiteID) {
		return fmt.Errorf("invalid filter")
	}
	if q.Kind == "interval" || q.Kind == "daily" {
		a, e := time.Parse("2006-01-02", q.Start)
		b, f := time.Parse("2006-01-02", q.End)
		if e != nil || f != nil || b.Before(a) || b.Sub(a) > 365*24*time.Hour {
			return fmt.Errorf("invalid dates")
		}
	}
	if q.Kind == "monthly" {
		a, e := time.Parse("2006-01-02", q.Start)
		b, f := time.Parse("2006-01-02", q.End)
		if e != nil || f != nil || a.Day() != 1 || b.Day() != 1 || b.Before(a) || (b.Year()-a.Year())*12+int(b.Month())-int(a.Month()) > 119 || q.AccountID != "" || q.SiteID != "" {
			return fmt.Errorf("invalid monthly query")
		}
	}
	return nil
}

type MonthlyBalanceAccount struct {
	Name         string `json:"name"`
	Raw          string `json:"raw"`
	ResidualYuan string `json:"residual_yuan"`
}

type MonthlyBalanceSnapshot struct {
	Period             string                  `json:"period"`
	Mode               string                  `json:"mode"`
	GrandTotal         string                  `json:"grand_total_residual_yuan"`
	ExcludingNowcoding string                  `json:"total_excluding_nowcoding_residual_yuan"`
	Foxcode            string                  `json:"foxcode_residual_yuan"`
	Other              string                  `json:"other_residual_yuan"`
	RedeemValue        string                  `json:"redeem_code_residual_yuan"`
	RedeemCount        string                  `json:"redeem_code_count"`
	Nowcoding          string                  `json:"nowcoding_residual_yuan"`
	Accounts           []MonthlyBalanceAccount `json:"accounts"`
	Failed             []string                `json:"failed"`
}

func (m MonthlyBalanceSnapshot) Validate() error {
	if _, e := time.Parse("2006-01", m.Period); e != nil {
		return fmt.Errorf("invalid month")
	}
	if m.Mode != "scheduled" && m.Mode != "manual" && m.Mode != "legacy" {
		return fmt.Errorf("invalid generation mode")
	}
	if len(m.Accounts) > 2000 || len(m.Failed) > 2000 {
		return fmt.Errorf("monthly report too large")
	}
	for _, v := range []string{m.GrandTotal, m.ExcludingNowcoding, m.Foxcode, m.Other, m.RedeemValue, m.Nowcoding} {
		if !balanceDecimal.MatchString(v) {
			return fmt.Errorf("invalid monthly amount")
		}
	}
	count, e := strconv.ParseInt(m.RedeemCount, 10, 64)
	if e != nil || count < 0 {
		return fmt.Errorf("invalid monthly stock")
	}
	for _, a := range m.Accounts {
		if a.Name == "" || len(a.Name) > 128 || !balanceDecimal.MatchString(a.Raw) || !balanceDecimal.MatchString(a.ResidualYuan) {
			return fmt.Errorf("invalid monthly account")
		}
	}
	for _, n := range m.Failed {
		if n == "" || len(n) > 128 {
			return fmt.Errorf("invalid failed account")
		}
	}
	return nil
}

type BalancePage struct {
	Items []json.RawMessage `json:"items"`
	Total string            `json:"total"`
}
