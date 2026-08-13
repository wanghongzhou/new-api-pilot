package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogPageTotalsMarshalAsBigintStrings(t *testing.T) {
	const total = "9007199254740993"
	pages := []any{
		ModelCatalogPageResponse{Total: total},
		MissingModelPageResponse{Total: total},
		PricingCatalogPageResponse{Total: total},
		PricingGroupPageResponse{Total: total},
		UserInventoryPage{Total: total},
		ChannelInventoryPage{Total: total},
		SubscriptionPlanPageResponse{Total: total},
	}
	for _, page := range pages {
		payload, err := json.Marshal(page)
		if err != nil {
			t.Fatalf("marshal %T: %v", page, err)
		}
		if !strings.Contains(string(payload), `"total":"`+total+`"`) {
			t.Fatalf("%T encoded total without bigint string precision: %s", page, payload)
		}
	}
}
