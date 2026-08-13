package common

import (
	"encoding/json"
	"testing"
)

func TestPageDataTotalUsesDecimalStringJSON(t *testing.T) {
	page := NewPageData(2, 20, 9_007_199_254_740_993, []string{"item"})
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	var wire struct {
		Total any `json:"total"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode wire page: %v", err)
	}
	if wire.Total != "9007199254740993" {
		t.Fatalf("wire total = %#v, want exact decimal string", wire.Total)
	}

	var decoded PageData[string]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal page: %v", err)
	}
	if decoded.Total != page.Total || len(decoded.Items) != 1 {
		t.Fatalf("decoded page = %#v", decoded)
	}
}
