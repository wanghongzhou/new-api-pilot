package service

import (
	"sort"
	"strings"

	"new-api-pilot/dto"
)

func pricingMapValue(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func pricingMapPointer(values map[string]string, key string) *string {
	value, ok := values[key]
	if !ok {
		return nil
	}
	return &value
}

func pricingSource(item dto.UpstreamPricingItem, configuration dto.UpstreamPricingConfiguration) string {
	if configuration.BillingMode[item.ModelName] == "tiered_expr" && strings.TrimSpace(configuration.BillingExpr[item.ModelName]) != "" {
		return "tiered_expr"
	}
	if _, ok := configuration.ModelPrice[item.ModelName]; ok || item.QuotaType == 1 {
		return "fixed"
	}
	if _, ok := configuration.ModelRatio[item.ModelName]; ok {
		return "token_explicit"
	}
	return "token_default"
}

func mergePricingConfiguration(groups dto.UpstreamPricingGroupSnapshot, pricing dto.UpstreamPricingOnlySnapshot, configuration dto.UpstreamPricingConfiguration) dto.UpstreamPricingSnapshot {
	itemsByModel := make(map[string]dto.UpstreamPricingItem, len(pricing.Items))
	for _, item := range pricing.Items {
		if mode := configuration.BillingMode[item.ModelName]; mode != "" {
			item.BillingMode = mode
		} else if _, fixed := configuration.ModelPrice[item.ModelName]; fixed || item.QuotaType == 1 {
			item.BillingMode = "fixed"
		} else {
			item.BillingMode = "token"
		}
		if expression, configured := configuration.BillingExpr[item.ModelName]; configured {
			item.BillingExpr = expression
		}
		item.PricingSource = pricingSource(item, configuration)
		itemsByModel[item.ModelName] = item
	}
	configuredModels := map[string]struct{}{}
	modelMaps := []map[string]string{
		configuration.ModelPrice, configuration.ModelRatio, configuration.CompletionRatio, configuration.CacheRatio,
		configuration.CreateCacheRatio, configuration.ImageRatio, configuration.AudioRatio, configuration.AudioCompletionRatio,
		configuration.BillingMode, configuration.BillingExpr,
	}
	for _, values := range modelMaps {
		for modelName := range values {
			configuredModels[modelName] = struct{}{}
		}
	}
	for modelName := range configuredModels {
		if _, exists := itemsByModel[modelName]; exists {
			continue
		}
		item := dto.UpstreamPricingItem{
			ModelName: modelName, VendorName: "unknown", ModelRatio: pricingMapValue(configuration.ModelRatio, modelName, "0"),
			ModelPrice: pricingMapValue(configuration.ModelPrice, modelName, "0"), CompletionRatio: pricingMapValue(configuration.CompletionRatio, modelName, "1"),
			CacheRatio: pricingMapPointer(configuration.CacheRatio, modelName), CreateCacheRatio: pricingMapPointer(configuration.CreateCacheRatio, modelName),
			ImageRatio: pricingMapPointer(configuration.ImageRatio, modelName), AudioRatio: pricingMapPointer(configuration.AudioRatio, modelName),
			AudioCompletionRatio: pricingMapPointer(configuration.AudioCompletionRatio, modelName), BillingMode: configuration.BillingMode[modelName],
			BillingExpr: configuration.BillingExpr[modelName], EnableGroups: []string{}, SupportedEndpointTypes: []string{}, AbilityAvailable: false,
		}
		if _, fixed := configuration.ModelPrice[modelName]; fixed {
			item.QuotaType = 1
		}
		if item.BillingMode == "" {
			if item.QuotaType == 1 {
				item.BillingMode = "fixed"
			} else {
				item.BillingMode = "token"
			}
		}
		item.PricingSource = pricingSource(item, configuration)
		itemsByModel[modelName] = item
	}
	items := make([]dto.UpstreamPricingItem, 0, len(itemsByModel))
	for _, item := range itemsByModel {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModelName < items[j].ModelName })

	groupByName := map[string]dto.UpstreamPricingGroup{}
	for _, group := range groups.Groups {
		groupByName[group.Name] = group
	}
	for _, group := range pricing.Groups {
		groupByName[group.Name] = group
	}
	ensureGroup := func(name string) {
		if name != "" {
			if _, exists := groupByName[name]; !exists {
				groupByName[name] = dto.UpstreamPricingGroup{Name: name}
			}
		}
	}
	for name := range configuration.GroupRatio {
		ensureGroup(name)
	}
	for name := range configuration.TopupGroupRatio {
		ensureGroup(name)
	}
	for name := range configuration.UserUsableGroups {
		ensureGroup(name)
	}
	for source, targets := range configuration.GroupGroupRatio {
		ensureGroup(source)
		for target := range targets {
			ensureGroup(target)
		}
	}
	for _, name := range configuration.AutoGroups {
		ensureGroup(name)
	}
	for source, rules := range configuration.GroupSpecialUsableGroup {
		ensureGroup(source)
		for rawTarget := range rules {
			ensureGroup(strings.TrimPrefix(strings.TrimPrefix(rawTarget, "+:"), "-:"))
		}
	}
	for name, group := range groupByName {
		group.Ratio = pricingMapPointer(configuration.GroupRatio, name)
		group.TopupRatio = pricingMapPointer(configuration.TopupGroupRatio, name)
		group.Description, group.UserSelectable = configuration.UserUsableGroups[name]
		group.DefaultUseAutoGroup = configuration.DefaultUseAutoGroup
		group.OutgoingOverrides = map[string]string{}
		group.IncomingOverrides = map[string]string{}
		group.VisibleToGroups = map[string]string{}
		group.HiddenFromGroups = []string{}
		if overrides, exists := configuration.GroupGroupRatio[name]; exists {
			for target, ratio := range overrides {
				group.OutgoingOverrides[target] = ratio
			}
		}
		for source, overrides := range configuration.GroupGroupRatio {
			if ratio, exists := overrides[name]; exists {
				group.IncomingOverrides[source] = ratio
			}
		}
		for index, autoGroup := range configuration.AutoGroups {
			if autoGroup == name {
				priority := index + 1
				group.AutoPriority = &priority
				break
			}
		}
		for userGroup, rules := range configuration.GroupSpecialUsableGroup {
			for rawTarget, description := range rules {
				hidden := strings.HasPrefix(rawTarget, "-:")
				target := strings.TrimPrefix(strings.TrimPrefix(rawTarget, "+:"), "-:")
				if target != name {
					continue
				}
				if hidden {
					group.HiddenFromGroups = append(group.HiddenFromGroups, userGroup)
				} else {
					group.VisibleToGroups[userGroup] = description
				}
			}
		}
		sort.Strings(group.HiddenFromGroups)
		groupByName[name] = group
	}
	mergedGroups := make([]dto.UpstreamPricingGroup, 0, len(groupByName))
	for _, group := range groupByName {
		mergedGroups = append(mergedGroups, group)
	}
	sort.Slice(mergedGroups, func(i, j int) bool { return mergedGroups[i].Name < mergedGroups[j].Name })
	return dto.UpstreamPricingSnapshot{PricingVersion: pricing.PricingVersion, Items: items, Groups: mergedGroups}
}
