package service

import (
	"encoding/json"
	"testing"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestExportJobItemAcceptsEveryCreateableExportScope(t *testing.T) {
	filters, err := json.Marshal(dto.ExportFilters{})
	if err != nil {
		t.Fatalf("marshal filters: %v", err)
	}
	scopes := []string{
		dto.StatisticsScopeGlobal, dto.StatisticsScopeSite, dto.StatisticsScopeCustomer,
		dto.StatisticsScopeAccount, dto.StatisticsScopeModel, dto.StatisticsScopeChannel,
		dto.StatisticsScopeGroup, dto.StatisticsScopeToken, dto.StatisticsScopeNode,
		"logs", "user_inventory", "channel_inventory", "performance_history",
		"topup_inventory", "redemption_inventory", "upstream_tasks", "model_catalog",
		"model_rankings", "vendor_rankings", "subscription_plans", "pricing_catalog",
		"group_catalog", "system_tasks",
	}
	for _, scope := range scopes {
		t.Run(scope, func(t *testing.T) {
			item, itemErr := exportJobItem(model.ExportJob{
				ID: 1, Format: dto.ExportFormatCSV, StatisticsType: scope,
				Filters: filters, Status: dto.ExportStatusExpired, Progress: 100,
			})
			if itemErr != nil {
				t.Fatalf("exportJobItem(%q): %v", scope, itemErr)
			}
			if item.StatisticsType != scope {
				t.Fatalf("statistics_type = %q, want %q", item.StatisticsType, scope)
			}
		})
	}
}
