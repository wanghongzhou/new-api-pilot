package model

import (
	"strings"
	"testing"
)

func TestAlertEventOrderUsesDocumentedColumnsAndStableTieBreak(t *testing.T) {
	tests := map[string]string{
		"rule_key":       "e.rule_key ASC",
		"status":         "CASE e.status",
		"level":          "CASE e.level",
		"site_name":      "CASE WHEN s.name IS NULL OR s.name = '' THEN 1 ELSE 0 END ASC, s.name ASC",
		"first_fired_at": "e.first_fired_at ASC",
		"last_fired_at":  "e.last_fired_at ASC",
		"resolved_at":    "e.resolved_at ASC",
	}
	for sortBy, expected := range tests {
		order := alertEventOrder(sortBy, "asc")
		if !strings.Contains(order, expected) || !strings.HasSuffix(order, ", e.id DESC") {
			t.Errorf("sort %s order = %q", sortBy, order)
		}
	}
	if order := alertEventOrder("current_value", "asc"); strings.Contains(order, "current_value") {
		t.Fatalf("invalid numeric cross-metric sort leaked into order: %q", order)
	}
}
