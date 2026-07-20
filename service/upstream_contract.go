package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"new-api-pilot/dto"
)

type upstreamStatusWire struct {
	Version           *string      `json:"version"`
	SystemName        *string      `json:"system_name"`
	QuotaPerUnit      *json.Number `json:"quota_per_unit"`
	USDExchangeRate   *json.Number `json:"usd_exchange_rate"`
	DataExportEnabled *bool        `json:"enable_data_export"`
}

type upstreamIdentityWire struct {
	ID          *int64  `json:"id"`
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	Role        *int32  `json:"role"`
	Status      *int32  `json:"status"`
	Group       *string `json:"group"`
}

type upstreamUserWire struct {
	ID           *int64          `json:"id"`
	Username     *string         `json:"username"`
	DisplayName  *string         `json:"display_name"`
	Role         *int32          `json:"role"`
	Status       *int32          `json:"status"`
	Group        *string         `json:"group"`
	Quota        *int64          `json:"quota"`
	UsedQuota    *int64          `json:"used_quota"`
	RequestCount *int64          `json:"request_count"`
	CreatedAt    *int64          `json:"created_at"`
	LastLoginAt  *int64          `json:"last_login_at"`
	DeletedAt    json.RawMessage `json:"DeletedAt"`
}

type upstreamUserPageWire struct {
	Page     *int                `json:"page"`
	PageSize *int                `json:"page_size"`
	Total    *int64              `json:"total"`
	Items    *[]upstreamUserWire `json:"items"`
}

type upstreamChannelWire struct {
	ID                 *int64       `json:"id"`
	Name               *string      `json:"name"`
	Type               *int         `json:"type"`
	Status             *int32       `json:"status"`
	TestTime           *int64       `json:"test_time"`
	ResponseTime       *int64       `json:"response_time"`
	Balance            *json.Number `json:"balance"`
	BalanceUpdatedTime *int64       `json:"balance_updated_time"`
	Models             *string      `json:"models"`
	Group              *string      `json:"group"`
	UsedQuota          *int64       `json:"used_quota"`
	Priority           *int64       `json:"priority"`
	Weight             *int64       `json:"weight"`
	AutoBan            *int         `json:"auto_ban"`
	Tag                *string      `json:"tag"`
}

type upstreamChannelPageWire struct {
	Page     *int                   `json:"page"`
	PageSize *int                   `json:"page_size"`
	Total    *int64                 `json:"total"`
	Items    *[]upstreamChannelWire `json:"items"`
}

type upstreamTopupWire struct {
	ID              *int64       `json:"id"`
	UserID          *int64       `json:"user_id"`
	Amount          *int64       `json:"amount"`
	Money           *json.Number `json:"money"`
	PaymentMethod   *string      `json:"payment_method"`
	PaymentProvider *string      `json:"payment_provider"`
	CreateTime      *int64       `json:"create_time"`
	CompleteTime    *int64       `json:"complete_time"`
	Status          *string      `json:"status"`
}

func (wire *upstreamTopupWire) UnmarshalJSON(data []byte) error {
	type alias upstreamTopupWire
	return decodeDiscardingSensitive(data, (*alias)(wire), []string{"trade", "no"})
}

type upstreamTopupPageWire struct {
	Page     *int                 `json:"page"`
	PageSize *int                 `json:"page_size"`
	Total    *int64               `json:"total"`
	Items    *[]upstreamTopupWire `json:"items"`
}

type upstreamRedemptionWire struct {
	ID           *int64  `json:"id"`
	UserID       *int64  `json:"user_id"`
	Status       *int    `json:"status"`
	Name         *string `json:"name"`
	Quota        *int64  `json:"quota"`
	CreatedTime  *int64  `json:"created_time"`
	RedeemedTime *int64  `json:"redeemed_time"`
	UsedUserID   *int64  `json:"used_user_id"`
	ExpiredTime  *int64  `json:"expired_time"`
}

func (wire *upstreamRedemptionWire) UnmarshalJSON(data []byte) error {
	type alias upstreamRedemptionWire
	return decodeDiscardingSensitive(data, (*alias)(wire), []string{"key"})
}

type upstreamRedemptionPageWire struct {
	Page     *int                      `json:"page"`
	PageSize *int                      `json:"page_size"`
	Total    *int64                    `json:"total"`
	Items    *[]upstreamRedemptionWire `json:"items"`
}

type upstreamTaskPropertiesWire struct {
	UpstreamModelName *string `json:"upstream_model_name"`
	OriginModelName   *string `json:"origin_model_name"`
}

func (w *upstreamTaskPropertiesWire) UnmarshalJSON(data []byte) error {
	type alias upstreamTaskPropertiesWire
	return decodeDiscardingSensitiveFields(data, (*alias)(w), [][]string{{"in", "put"}})
}

type upstreamTaskWire struct {
	ID         *int64                      `json:"id"`
	CreatedAt  *int64                      `json:"created_at"`
	UpdatedAt  *int64                      `json:"updated_at"`
	TaskID     *string                     `json:"task_id"`
	Platform   *string                     `json:"platform"`
	UserID     *int64                      `json:"user_id"`
	Group      *string                     `json:"group"`
	ChannelID  *int64                      `json:"channel_id"`
	Quota      *int64                      `json:"quota"`
	Action     *string                     `json:"action"`
	Status     *string                     `json:"status"`
	SubmitTime *int64                      `json:"submit_time"`
	StartTime  *int64                      `json:"start_time"`
	FinishTime *int64                      `json:"finish_time"`
	Progress   *string                     `json:"progress"`
	Properties *upstreamTaskPropertiesWire `json:"properties"`
}

func (w *upstreamTaskWire) UnmarshalJSON(data []byte) error {
	type alias upstreamTaskWire
	return decodeDiscardingSensitiveFields(data, (*alias)(w), [][]string{{"da", "ta"}, {"in", "put"}, {"fail", "_", "reason"}, {"result", "_", "url"}, {"private", "_", "data"}, {"user", "name"}})
}

type upstreamTaskPageWire struct {
	Page     *int                `json:"page"`
	PageSize *int                `json:"page_size"`
	Total    *int64              `json:"total"`
	Items    *[]upstreamTaskWire `json:"items"`
}

type upstreamModelMetaWire struct {
	ID           *int64  `json:"id"`
	ModelName    *string `json:"model_name"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
	Tags         *string `json:"tags"`
	VendorID     *int64  `json:"vendor_id"`
	Status       *int    `json:"status"`
	SyncOfficial *int    `json:"sync_official"`
	NameRule     *int    `json:"name_rule"`
	CreatedTime  *int64  `json:"created_time"`
	UpdatedTime  *int64  `json:"updated_time"`
}

func (w *upstreamModelMetaWire) UnmarshalJSON(data []byte) error {
	type alias upstreamModelMetaWire
	return decodeDiscardingSensitiveFields(data, (*alias)(w), [][]string{{"pricing"}, {"billing", "_", "expr"}, {"endpoints"}, {"bound", "_", "channels"}, {"enable", "_", "groups"}, {"quota", "_", "types"}, {"matched", "_", "models"}, {"matched", "_", "count"}})
}

type upstreamModelMetaPageWire struct {
	Page     *int                     `json:"page"`
	PageSize *int                     `json:"page_size"`
	Total    *int64                   `json:"total"`
	Items    *[]upstreamModelMetaWire `json:"items"`
}
type upstreamSubscriptionPlanWire struct {
	ID                      *int64       `json:"id"`
	Title                   *string      `json:"title"`
	Subtitle                *string      `json:"subtitle"`
	PriceAmount             *json.Number `json:"price_amount"`
	Currency                *string      `json:"currency"`
	DurationUnit            *string      `json:"duration_unit"`
	DurationValue           *int         `json:"duration_value"`
	CustomSeconds           *int64       `json:"custom_seconds"`
	Enabled                 *bool        `json:"enabled"`
	SortOrder               *int         `json:"sort_order"`
	TotalAmount             *int64       `json:"total_amount"`
	QuotaResetPeriod        *string      `json:"quota_reset_period"`
	QuotaResetCustomSeconds *int64       `json:"quota_reset_custom_seconds"`
	CreatedAt               *int64       `json:"created_at"`
	UpdatedAt               *int64       `json:"updated_at"`
}

func (w *upstreamSubscriptionPlanWire) UnmarshalJSON(data []byte) error {
	type alias upstreamSubscriptionPlanWire
	return decodeDiscardingSensitiveFields(data, (*alias)(w), [][]string{{"stripe", "_", "price", "_", "id"}, {"creem", "_", "product", "_", "id"}, {"waffo", "_", "pancake", "_", "product", "_", "id"}, {"allow", "_", "balance", "_", "pay"}, {"allow", "_", "wallet", "_", "overflow"}, {"max", "_", "purchase", "_", "per", "_", "user"}, {"upgrade", "_", "group"}, {"downgrade", "_", "group"}, {"provider", "_", "payload"}})
}

type upstreamSubscriptionPlanDTO struct {
	Plan *upstreamSubscriptionPlanWire `json:"plan"`
}

type upstreamPricingItemWire struct {
	ModelName              *string      `json:"model_name"`
	Description            *string      `json:"description"`
	Icon                   *string      `json:"icon"`
	Tags                   *string      `json:"tags"`
	VendorID               *int64       `json:"vendor_id"`
	QuotaType              *int64       `json:"quota_type"`
	ModelRatio             *json.Number `json:"model_ratio"`
	ModelPrice             *json.Number `json:"model_price"`
	OwnerBy                *string      `json:"owner_by"`
	CompletionRatio        *json.Number `json:"completion_ratio"`
	CacheRatio             *json.Number `json:"cache_ratio"`
	CreateCacheRatio       *json.Number `json:"create_cache_ratio"`
	ImageRatio             *json.Number `json:"image_ratio"`
	AudioRatio             *json.Number `json:"audio_ratio"`
	AudioCompletionRatio   *json.Number `json:"audio_completion_ratio"`
	EnableGroups           *[]string    `json:"enable_groups"`
	SupportedEndpointTypes *[]string    `json:"supported_endpoint_types"`
}

func (wire *upstreamPricingItemWire) UnmarshalJSON(data []byte) error {
	type alias upstreamPricingItemWire
	if err := validateStrictJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode((*alias)(wire))
}

type upstreamPricingVendorWire struct {
	ID   *int64  `json:"id"`
	Name *string `json:"name"`
}

type upstreamPricingResponseWire struct {
	Success        *bool                        `json:"success"`
	Message        *string                      `json:"message"`
	Data           *[]upstreamPricingItemWire   `json:"data"`
	Vendors        *[]upstreamPricingVendorWire `json:"vendors"`
	GroupRatio     *map[string]json.Number      `json:"group_ratio"`
	UsableGroup    *map[string]string           `json:"usable_group"`
	PricingVersion *string                      `json:"pricing_version"`
}

func (wire *upstreamPricingResponseWire) UnmarshalJSON(data []byte) error {
	type alias upstreamPricingResponseWire
	if err := validateStrictJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode((*alias)(wire))
}

func (wire *upstreamPricingResponseWire) decodeUpstreamResponse(payload []byte) error {
	if err := validateStrictJSON(payload); err != nil {
		return invalidUpstreamResponse()
	}
	if err := json.Unmarshal(payload, wire); err != nil {
		return invalidUpstreamResponse()
	}
	if wire.Success == nil || !*wire.Success || wire.Data == nil || wire.Vendors == nil || wire.GroupRatio == nil || wire.UsableGroup == nil ||
		wire.PricingVersion == nil || !validUpstreamString(*wire.PricingVersion, 1, 64) {
		return invalidUpstreamResponse()
	}
	return nil
}

var upstreamPricingEndpointTypes = map[string]struct{}{
	"openai": {}, "openai-response": {}, "openai-response-compact": {}, "anthropic": {}, "gemini": {},
	"jina-rerank": {}, "image-generation": {}, "embeddings": {}, "openai-video": {},
}

func canonicalPricingDecimal(number *json.Number) (*string, bool) {
	if number == nil {
		return nil, true
	}
	raw := number.String()
	if strings.HasPrefix(raw, "-") {
		return nil, false
	}
	if strings.Trim(raw, "0.") == "" {
		value := "0"
		return &value, true
	}
	value, ok := canonicalPositiveDecimalWithPrecision(raw, 20, 18, 38)
	if !ok {
		return nil, false
	}
	return &value, true
}

func validatePricingStringList(values []string, limit int, allowed map[string]struct{}) ([]string, bool) {
	if len(values) > 1000 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !validUpstreamString(value, 1, limit) {
			return nil, false
		}
		if allowed != nil {
			if _, ok := allowed[value]; !ok {
				return nil, false
			}
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, true
}

func validatePricingGroups(groupNames []string) (dto.UpstreamPricingGroupSnapshot, error) {
	groups, ok := validatePricingStringList(groupNames, 128, nil)
	if !ok || len(groups) > 10000 {
		return dto.UpstreamPricingGroupSnapshot{}, invalidUpstreamResponse()
	}
	result := dto.UpstreamPricingGroupSnapshot{Groups: make([]dto.UpstreamPricingGroup, 0, len(groups))}
	for _, group := range groups {
		result.Groups = append(result.Groups, dto.UpstreamPricingGroup{Name: group})
	}
	return result, nil
}

func validatePricing(wire upstreamPricingResponseWire) (dto.UpstreamPricingOnlySnapshot, error) {
	if wire.Data == nil || wire.Vendors == nil || wire.GroupRatio == nil || wire.UsableGroup == nil || wire.PricingVersion == nil ||
		len(*wire.Data) > 100000 || len(*wire.Vendors) > 10000 || len(*wire.GroupRatio) > 10000 || len(*wire.UsableGroup) > 10000 {
		return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
	}
	vendors := make(map[int64]string, len(*wire.Vendors))
	for _, vendor := range *wire.Vendors {
		if vendor.ID == nil || vendor.Name == nil || *vendor.ID <= 0 {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		key := strings.TrimSpace(*vendor.Name)
		if !validUpstreamString(key, 1, 128) {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		if _, duplicate := vendors[*vendor.ID]; duplicate {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		vendors[*vendor.ID] = key
	}
	groupNames := make([]string, 0, len(*wire.GroupRatio)+len(*wire.UsableGroup))
	groupSet := make(map[string]struct{}, len(*wire.GroupRatio)+len(*wire.UsableGroup))
	for group := range *wire.GroupRatio {
		groupSet[group] = struct{}{}
		groupNames = append(groupNames, group)
	}
	for group := range *wire.UsableGroup {
		if _, exists := groupSet[group]; !exists {
			groupSet[group] = struct{}{}
			groupNames = append(groupNames, group)
		}
	}
	groups, ok := validatePricingStringList(groupNames, 128, nil)
	if !ok {
		return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
	}
	result := dto.UpstreamPricingOnlySnapshot{
		PricingVersion: *wire.PricingVersion,
		Items:          make([]dto.UpstreamPricingItem, 0, len(*wire.Data)),
		Groups:         make([]dto.UpstreamPricingGroup, 0, len(groups)),
	}
	for _, group := range groups {
		description, visible := (*wire.UsableGroup)[group]
		if visible && !validUpstreamString(description, 0, 255) {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		var ratio *string
		if raw, exists := (*wire.GroupRatio)[group]; exists {
			canonical, valid := canonicalPricingDecimal(&raw)
			if !valid || canonical == nil {
				return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
			}
			ratio = canonical
		}
		result.Groups = append(result.Groups, dto.UpstreamPricingGroup{Name: group, Ratio: ratio, Description: description, RootVisible: visible})
	}
	seenModels := make(map[string]struct{}, len(*wire.Data))
	for _, item := range *wire.Data {
		if item.ModelName == nil || item.QuotaType == nil || item.ModelRatio == nil || item.ModelPrice == nil || item.OwnerBy == nil ||
			item.CompletionRatio == nil || item.EnableGroups == nil || item.SupportedEndpointTypes == nil ||
			!validUpstreamString(*item.ModelName, 1, 255) || !validUpstreamString(*item.OwnerBy, 0, 64) ||
			(*item.QuotaType != 0 && *item.QuotaType != 1) {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		description, icon, tags := "", "", ""
		vendorID := int64(0)
		if item.Description != nil {
			description = *item.Description
		}
		if item.Icon != nil {
			icon = *item.Icon
		}
		if item.Tags != nil {
			tags = *item.Tags
		}
		if item.VendorID != nil {
			vendorID = *item.VendorID
		}
		if vendorID < 0 || !validUpstreamString(description, 0, 65535) || !validUpstreamString(icon, 0, 512) || !validUpstreamString(tags, 0, 255) {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		enableGroups, valid := validatePricingStringList(*item.EnableGroups, 128, nil)
		if !valid {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		endpointTypes, valid := validatePricingStringList(*item.SupportedEndpointTypes, 64, upstreamPricingEndpointTypes)
		if !valid {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		modelRatio, ok1 := canonicalPricingDecimal(item.ModelRatio)
		modelPrice, ok2 := canonicalPricingDecimal(item.ModelPrice)
		completionRatio, ok3 := canonicalPricingDecimal(item.CompletionRatio)
		cacheRatio, ok4 := canonicalPricingDecimal(item.CacheRatio)
		createCacheRatio, ok5 := canonicalPricingDecimal(item.CreateCacheRatio)
		imageRatio, ok6 := canonicalPricingDecimal(item.ImageRatio)
		audioRatio, ok7 := canonicalPricingDecimal(item.AudioRatio)
		audioCompletionRatio, ok8 := canonicalPricingDecimal(item.AudioCompletionRatio)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 || modelRatio == nil || modelPrice == nil || completionRatio == nil {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		vendorKey := "unknown"
		if vendorID > 0 {
			if mapped, exists := vendors[vendorID]; exists {
				vendorKey = mapped
			} else {
				vendorKey = "id:" + strconv.FormatInt(vendorID, 10)
			}
		}
		identity := *item.ModelName + "\x00" + vendorKey
		if _, duplicate := seenModels[identity]; duplicate {
			return dto.UpstreamPricingOnlySnapshot{}, invalidUpstreamResponse()
		}
		seenModels[identity] = struct{}{}
		result.Items = append(result.Items, dto.UpstreamPricingItem{
			ModelName: *item.ModelName, VendorKey: vendorKey, Description: description, Icon: icon, Tags: tags, OwnerBy: *item.OwnerBy,
			ModelRatio: *modelRatio, ModelPrice: *modelPrice, CompletionRatio: *completionRatio,
			CacheRatio: cacheRatio, CreateCacheRatio: createCacheRatio, ImageRatio: imageRatio,
			AudioRatio: audioRatio, AudioCompletionRatio: audioCompletionRatio, VendorID: vendorID,
			QuotaType: *item.QuotaType, RootVisible: true, EnableGroups: enableGroups, SupportedEndpointTypes: endpointTypes,
		})
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].ModelName != result.Items[j].ModelName {
			return result.Items[i].ModelName < result.Items[j].ModelName
		}
		return result.Items[i].VendorKey < result.Items[j].VendorKey
	})
	return result, nil
}

func validateSubscriptionPlans(w []upstreamSubscriptionPlanDTO) (dto.UpstreamSubscriptionPlanSnapshot, error) {
	if len(w) > 100000 {
		return dto.UpstreamSubscriptionPlanSnapshot{}, newUpstreamResponseTooLargeError(int64(len(w)), 100000)
	}
	seen := map[int64]struct{}{}
	out := dto.UpstreamSubscriptionPlanSnapshot{Items: make([]dto.UpstreamSubscriptionPlan, 0, len(w))}
	for _, x := range w {
		if x.Plan == nil {
			return out, invalidUpstreamResponse()
		}
		p := x.Plan
		if p.ID == nil || p.Title == nil || p.Subtitle == nil || p.PriceAmount == nil || p.Currency == nil || p.DurationUnit == nil || p.DurationValue == nil || p.CustomSeconds == nil || p.Enabled == nil || p.SortOrder == nil || p.TotalAmount == nil || p.QuotaResetPeriod == nil || p.QuotaResetCustomSeconds == nil || p.CreatedAt == nil || p.UpdatedAt == nil || *p.ID <= 0 || !validUpstreamString(*p.Title, 1, 128) {
			return out, invalidUpstreamResponse()
		}
		subtitle, currency, durationUnit, resetPeriod := *p.Subtitle, *p.Currency, *p.DurationUnit, *p.QuotaResetPeriod
		durationValue, customSeconds, enabled, sortOrder := *p.DurationValue, *p.CustomSeconds, *p.Enabled, *p.SortOrder
		totalAmount, resetSeconds, createdAt, updatedAt := *p.TotalAmount, *p.QuotaResetCustomSeconds, *p.CreatedAt, *p.UpdatedAt
		if currency != "USD" || customSeconds < 0 || totalAmount < 0 || resetSeconds < 0 || createdAt < 0 || updatedAt < 0 || !validUpstreamString(subtitle, 0, 255) {
			return out, invalidUpstreamResponse()
		}
		switch durationUnit {
		case "year", "month", "day", "hour":
			if durationValue <= 0 {
				return out, invalidUpstreamResponse()
			}
		case "custom":
			if customSeconds <= 0 {
				return out, invalidUpstreamResponse()
			}
		default:
			return out, invalidUpstreamResponse()
		}
		switch resetPeriod {
		case "never", "daily", "weekly", "monthly":
		case "custom":
			if resetSeconds <= 0 {
				return out, invalidUpstreamResponse()
			}
		default:
			return out, invalidUpstreamResponse()
		}
		if _, ok := seen[*p.ID]; ok {
			return out, invalidUpstreamResponse()
		}
		seen[*p.ID] = struct{}{}
		price, ok := canonicalNonNegativeMoneyDecimal(p.PriceAmount.String())
		if !ok {
			return out, invalidUpstreamResponse()
		}
		out.Items = append(out.Items, dto.UpstreamSubscriptionPlan{ID: *p.ID, Title: *p.Title, Subtitle: subtitle, PriceAmount: price, Currency: currency, DurationUnit: durationUnit, DurationValue: durationValue, CustomSeconds: customSeconds, Enabled: enabled, SortOrder: sortOrder, TotalAmount: totalAmount, QuotaResetPeriod: resetPeriod, QuotaResetCustomSeconds: resetSeconds, CreatedAt: createdAt, UpdatedAt: updatedAt})
	}
	return out, nil
}

func validateModelMetaPage(w upstreamModelMetaPageWire, expected int) (dto.UpstreamModelMetaPage, error) {
	if w.Page == nil || w.PageSize == nil || w.Total == nil || w.Items == nil || *w.Page != expected || *w.PageSize != upstreamPageSize || *w.Total < 0 || len(*w.Items) > upstreamPageSize || int64(len(*w.Items)) > *w.Total {
		return dto.UpstreamModelMetaPage{}, invalidUpstreamResponse()
	}
	out := dto.UpstreamModelMetaPage{Page: *w.Page, PageSize: *w.PageSize, Total: *w.Total, Items: make([]dto.UpstreamModelMeta, 0, len(*w.Items))}
	prev := int64(^uint64(0) >> 1)
	for _, x := range *w.Items {
		if x.ID == nil || x.ModelName == nil || x.Status == nil || x.SyncOfficial == nil || x.NameRule == nil || x.CreatedTime == nil || x.UpdatedTime == nil || *x.ID <= 0 || *x.ID >= prev || (*x.Status != 0 && *x.Status != 1) || (*x.SyncOfficial != 0 && *x.SyncOfficial != 1) || *x.NameRule < 0 || *x.NameRule > 3 || *x.CreatedTime < 0 || *x.UpdatedTime < 0 || !validUpstreamString(*x.ModelName, 1, 128) {
			return dto.UpstreamModelMetaPage{}, invalidUpstreamResponse()
		}
		description, icon, tags := "", "", ""
		vendorID := int64(0)
		if x.Description != nil {
			description = *x.Description
		}
		if x.Icon != nil {
			icon = *x.Icon
		}
		if x.Tags != nil {
			tags = *x.Tags
		}
		if x.VendorID != nil {
			vendorID = *x.VendorID
		}
		if vendorID < 0 || !validUpstreamString(description, 0, 65535) || !validUpstreamString(icon, 0, 512) || !validUpstreamString(tags, 0, 255) {
			return dto.UpstreamModelMetaPage{}, invalidUpstreamResponse()
		}
		prev = *x.ID
		out.Items = append(out.Items, dto.UpstreamModelMeta{ID: *x.ID, ModelName: *x.ModelName, Description: description, Icon: icon, Tags: tags, VendorID: vendorID, Status: *x.Status, SyncOfficial: *x.SyncOfficial, NameRule: *x.NameRule, CreatedTime: *x.CreatedTime, UpdatedTime: *x.UpdatedTime})
	}
	return out, nil
}

func decodeDiscardingSensitiveFields(data []byte, destination any, fields [][]string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, parts := range fields {
		delete(raw, strings.Join(parts, ""))
	}
	sanitized, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(sanitized))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func validateTaskPage(w upstreamTaskPageWire, expected int) (dto.UpstreamTaskPage, error) {
	if w.Page == nil || w.PageSize == nil || w.Total == nil || w.Items == nil || *w.Page != expected || *w.PageSize != upstreamPageSize || *w.Total < 0 || len(*w.Items) > upstreamPageSize || int64(len(*w.Items)) > *w.Total {
		return dto.UpstreamTaskPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamTask, 0, len(*w.Items))
	previous := int64(^uint64(0) >> 1)
	validStatus := func(v string) bool {
		switch v {
		case "NOT_START", "SUBMITTED", "QUEUED", "IN_PROGRESS", "FAILURE", "SUCCESS", "UNKNOWN":
			return true
		}
		return false
	}
	validProgress := func(v string) bool {
		if v == "" {
			return true
		}
		if !strings.HasSuffix(v, "%") {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSuffix(v, "%"))
		return err == nil && n >= 0 && n <= 100 && strconv.Itoa(n)+"%" == v
	}
	for _, raw := range *w.Items {
		if raw.ID == nil || raw.CreatedAt == nil || raw.UpdatedAt == nil || raw.TaskID == nil || raw.Platform == nil || raw.UserID == nil || raw.Group == nil || raw.ChannelID == nil || raw.Quota == nil || raw.Action == nil || raw.Status == nil || raw.SubmitTime == nil || raw.StartTime == nil || raw.FinishTime == nil || raw.Progress == nil || raw.Properties == nil || *raw.ID <= 0 || *raw.CreatedAt < 0 || *raw.UpdatedAt < *raw.CreatedAt || *raw.UserID < 0 || *raw.ChannelID < 0 || *raw.Quota < 0 || *raw.SubmitTime < 0 || *raw.StartTime < 0 || *raw.FinishTime < 0 || !validUpstreamString(*raw.TaskID, 1, 191) || !validUpstreamString(*raw.Platform, 0, 30) || !validUpstreamString(*raw.Group, 0, 50) || !validUpstreamString(*raw.Action, 0, 40) || !validUpstreamString(*raw.Progress, 0, 20) || !validProgress(*raw.Progress) || !validStatus(*raw.Status) || *raw.ID >= previous {
			return dto.UpstreamTaskPage{}, invalidUpstreamResponse()
		}
		modelName := ""
		if raw.Properties.OriginModelName != nil {
			modelName = *raw.Properties.OriginModelName
		} else if raw.Properties.UpstreamModelName != nil {
			modelName = *raw.Properties.UpstreamModelName
		}
		if !validUpstreamString(modelName, 0, 255) {
			return dto.UpstreamTaskPage{}, invalidUpstreamResponse()
		}
		previous = *raw.ID
		items = append(items, dto.UpstreamTask{ID: *raw.ID, CreatedAt: *raw.CreatedAt, UpdatedAt: *raw.UpdatedAt, TaskID: *raw.TaskID, Platform: *raw.Platform, UserID: *raw.UserID, Group: *raw.Group, ChannelID: *raw.ChannelID, Quota: *raw.Quota, Action: *raw.Action, Status: *raw.Status, SubmitTime: *raw.SubmitTime, StartTime: *raw.StartTime, FinishTime: *raw.FinishTime, Progress: *raw.Progress, Properties: dto.UpstreamTaskProperties{Model: modelName}})
	}
	return dto.UpstreamTaskPage{Page: *w.Page, PageSize: *w.PageSize, Total: *w.Total, Items: items}, nil
}

func decodeDiscardingSensitive(data []byte, destination any, parts []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	delete(fields, strings.Join(parts, "_"))
	sanitized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(sanitized))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values")
	}
	return nil
}

func validateTopupPage(wire upstreamTopupPageWire, expectedPage int) (dto.UpstreamTopupPage, error) {
	if wire.Page == nil || wire.PageSize == nil || wire.Total == nil || wire.Items == nil || *wire.Page != expectedPage || *wire.PageSize != upstreamPageSize || *wire.Total < 0 || len(*wire.Items) > upstreamPageSize || int64(len(*wire.Items)) > *wire.Total {
		return dto.UpstreamTopupPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamTopup, 0, len(*wire.Items))
	var previous int64
	for index, raw := range *wire.Items {
		if raw.ID == nil || raw.UserID == nil || raw.Amount == nil || raw.Money == nil || raw.PaymentMethod == nil || raw.PaymentProvider == nil || raw.CreateTime == nil || raw.CompleteTime == nil || raw.Status == nil || *raw.ID <= 0 || *raw.UserID < 0 || *raw.Amount < 0 || *raw.CreateTime < 0 || *raw.CompleteTime < 0 || !validUpstreamString(*raw.PaymentMethod, 0, 50) || !validUpstreamString(*raw.PaymentProvider, 0, 50) || !validUpstreamString(*raw.Status, 1, 32) {
			return dto.UpstreamTopupPage{}, invalidUpstreamResponse()
		}
		money, ok := canonicalNonNegativeMoneyDecimal(raw.Money.String())
		value, parsed := new(big.Rat).SetString(money)
		if !ok || !parsed || value.Sign() < 0 {
			return dto.UpstreamTopupPage{}, invalidUpstreamResponse()
		}
		if index > 0 && previous <= *raw.ID {
			return dto.UpstreamTopupPage{}, invalidUpstreamResponse()
		}
		previous = *raw.ID
		items = append(items, dto.UpstreamTopup{ID: *raw.ID, UserID: *raw.UserID, Amount: *raw.Amount, Money: money, PaymentMethod: *raw.PaymentMethod, PaymentProvider: *raw.PaymentProvider, CreateTime: *raw.CreateTime, CompleteTime: *raw.CompleteTime, Status: *raw.Status})
	}
	return dto.UpstreamTopupPage{Page: *wire.Page, PageSize: *wire.PageSize, Total: *wire.Total, Items: items}, nil
}

func validateRedemptionPage(wire upstreamRedemptionPageWire, expectedPage int) (dto.UpstreamRedemptionPage, error) {
	if wire.Page == nil || wire.PageSize == nil || wire.Total == nil || wire.Items == nil || *wire.Page != expectedPage || *wire.PageSize != upstreamPageSize || *wire.Total < 0 || len(*wire.Items) > upstreamPageSize || int64(len(*wire.Items)) > *wire.Total {
		return dto.UpstreamRedemptionPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamRedemption, 0, len(*wire.Items))
	var previous int64
	for index, raw := range *wire.Items {
		if raw.ID == nil || raw.UserID == nil || raw.Status == nil || raw.Name == nil || raw.Quota == nil || raw.CreatedTime == nil || raw.RedeemedTime == nil || raw.UsedUserID == nil || raw.ExpiredTime == nil || *raw.ID <= 0 || *raw.UserID < 0 || *raw.Status < 0 || *raw.Quota < 0 || *raw.CreatedTime < 0 || *raw.RedeemedTime < 0 || *raw.UsedUserID < 0 || *raw.ExpiredTime < 0 || !validUpstreamString(*raw.Name, 0, 255) {
			return dto.UpstreamRedemptionPage{}, invalidUpstreamResponse()
		}
		if index > 0 && previous <= *raw.ID {
			return dto.UpstreamRedemptionPage{}, invalidUpstreamResponse()
		}
		previous = *raw.ID
		items = append(items, dto.UpstreamRedemption{ID: *raw.ID, UserID: *raw.UserID, Status: *raw.Status, Name: *raw.Name, Quota: *raw.Quota, CreatedTime: *raw.CreatedTime, RedeemedTime: *raw.RedeemedTime, UsedUserID: *raw.UsedUserID, ExpiredTime: *raw.ExpiredTime})
	}
	return dto.UpstreamRedemptionPage{Page: *wire.Page, PageSize: *wire.PageSize, Total: *wire.Total, Items: items}, nil
}

type upstreamPerformanceBucketWire struct {
	Ts             *int64       `json:"ts"`
	AvgTTFTMS      *json.Number `json:"avg_ttft_ms"`
	AvgLatencyMS   *json.Number `json:"avg_latency_ms"`
	SuccessRate    *json.Number `json:"success_rate"`
	AvgTPS         *json.Number `json:"avg_tps"`
	RequestCount   *int64       `json:"request_count"`
	SuccessCount   *int64       `json:"success_count"`
	TotalLatencyMS *int64       `json:"total_latency_ms"`
	TTFTSumMS      *int64       `json:"ttft_sum_ms"`
	TTFTCount      *int64       `json:"ttft_count"`
	OutputTokens   *int64       `json:"output_tokens"`
	GenerationMS   *int64       `json:"generation_ms"`
}
type upstreamPerformanceGroupWire struct {
	Group  *string                          `json:"group"`
	Series *[]upstreamPerformanceBucketWire `json:"series"`
}
type upstreamPerformanceHistoryWire struct {
	ModelName    *string                         `json:"model_name"`
	SeriesSchema *string                         `json:"series_schema"`
	Groups       *[]upstreamPerformanceGroupWire `json:"groups"`
}

type upstreamFlowRowWire struct {
	UserID       *int64          `json:"user_id"`
	Username     *string         `json:"username"`
	ModelName    *string         `json:"model_name"`
	ChannelID    json.RawMessage `json:"channel_id"`
	UseGroup     *string         `json:"use_group"`
	TokenID      *int64          `json:"token_id"`
	TokenName    *string         `json:"token_name"`
	NodeName     *string         `json:"node_name"`
	RequestCount *int64          `json:"count"`
	Quota        *int64          `json:"quota"`
	TokenUsed    *int64          `json:"token_used"`
}

type upstreamLogRowWire struct {
	ID                *int64  `json:"id"`
	UserID            *int64  `json:"user_id"`
	CreatedAt         *int64  `json:"created_at"`
	Type              *int    `json:"type"`
	Content           *string `json:"content"`
	Username          *string `json:"username"`
	TokenName         *string `json:"token_name"`
	ModelName         *string `json:"model_name"`
	Quota             *int64  `json:"quota"`
	PromptTokens      *int64  `json:"prompt_tokens"`
	CompletionTokens  *int64  `json:"completion_tokens"`
	UseTime           *int64  `json:"use_time"`
	IsStream          *bool   `json:"is_stream"`
	ChannelID         *int64  `json:"channel"`
	TokenID           *int64  `json:"token_id"`
	Group             *string `json:"group"`
	IP                *string `json:"ip"`
	RequestID         *string `json:"request_id"`
	UpstreamRequestID *string `json:"upstream_request_id"`
}

type upstreamLogPageWire struct {
	Page     *int                  `json:"page"`
	PageSize *int                  `json:"page_size"`
	Total    *int64                `json:"total"`
	Items    *[]upstreamLogRowWire `json:"items"`
}

func validateLogPage(wire upstreamLogPageWire, expectedPage int) (dto.UpstreamLogPage, error) {
	if wire.Page == nil || wire.PageSize == nil || wire.Total == nil || wire.Items == nil ||
		*wire.Page != expectedPage || *wire.PageSize < 1 || *wire.PageSize > upstreamPageSize || *wire.Total < 0 || len(*wire.Items) > *wire.PageSize {
		return dto.UpstreamLogPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamLogRow, 0, len(*wire.Items))
	for _, raw := range *wire.Items {
		if raw.ID == nil || raw.UserID == nil || raw.CreatedAt == nil || raw.Type == nil || raw.Content == nil ||
			raw.Username == nil || raw.TokenName == nil || raw.ModelName == nil || raw.Quota == nil ||
			raw.PromptTokens == nil || raw.CompletionTokens == nil || raw.UseTime == nil || raw.IsStream == nil ||
			raw.ChannelID == nil || raw.TokenID == nil || raw.Group == nil || raw.IP == nil ||
			*raw.ID < 0 || *raw.UserID < 0 || *raw.CreatedAt <= 0 || *raw.Type < 0 || *raw.Type > 7 || *raw.Quota < 0 ||
			*raw.PromptTokens < 0 || *raw.CompletionTokens < 0 || *raw.UseTime < 0 || *raw.ChannelID < 0 || *raw.TokenID < 0 ||
			!validUpstreamString(*raw.Content, 0, 4096) || !validUpstreamString(*raw.Username, 0, 255) ||
			!validUpstreamString(*raw.TokenName, 0, 255) || !validUpstreamString(*raw.ModelName, 0, 255) ||
			!validUpstreamString(*raw.Group, 0, 128) || !validUpstreamString(*raw.IP, 0, 64) ||
			!validOptionalUpstreamString(raw.RequestID, 64) || !validOptionalUpstreamString(raw.UpstreamRequestID, 128) {
			return dto.UpstreamLogPage{}, invalidUpstreamResponse()
		}
		items = append(items, dto.UpstreamLogRow{
			ID: *raw.ID, UserID: *raw.UserID, CreatedAt: *raw.CreatedAt, Type: *raw.Type, Content: *raw.Content,
			Username: *raw.Username, TokenName: *raw.TokenName, ModelName: *raw.ModelName, Quota: *raw.Quota,
			PromptTokens: *raw.PromptTokens, CompletionTokens: *raw.CompletionTokens, UseTimeSeconds: *raw.UseTime,
			IsStream: *raw.IsStream, ChannelID: *raw.ChannelID, TokenID: *raw.TokenID, UseGroup: *raw.Group,
			IP: *raw.IP, RequestID: optionalUpstreamString(raw.RequestID), UpstreamRequestID: optionalUpstreamString(raw.UpstreamRequestID),
		})
	}
	return dto.UpstreamLogPage{Page: *wire.Page, PageSize: *wire.PageSize, Total: *wire.Total, Items: items}, nil
}

func validOptionalUpstreamString(value *string, limit int) bool {
	return value == nil || validUpstreamString(*value, 0, limit)
}

func optionalUpstreamString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type upstreamDataRowWire struct {
	ModelName    *string `json:"model_name"`
	CreatedAt    *int64  `json:"created_at"`
	RequestCount *int64  `json:"count"`
	Quota        *int64  `json:"quota"`
	TokenUsed    *int64  `json:"token_used"`
}

type upstreamInstanceWire struct {
	NodeName          *string               `json:"node_name"`
	Status            *string               `json:"status"`
	StaleAfterSeconds *int64                `json:"stale_after_seconds"`
	StartedAt         *int64                `json:"started_at"`
	LastSeenAt        *int64                `json:"last_seen_at"`
	Info              *upstreamInstanceInfo `json:"info"`
}

type upstreamInstanceInfo struct {
	Node      *upstreamInstanceNode      `json:"node"`
	Role      json.RawMessage            `json:"role"`
	Runtime   *upstreamInstanceRuntime   `json:"runtime"`
	Host      *upstreamInstanceHost      `json:"host"`
	Resources *upstreamInstanceResources `json:"resources"`
}

type upstreamInstanceNode struct {
	Name *string `json:"name"`
}

type upstreamInstanceRuntime struct {
	Version *string `json:"version"`
	GOOS    *string `json:"goos"`
	GOARCH  *string `json:"goarch"`
}

type upstreamInstanceHost struct {
	Hostname *string `json:"hostname"`
}

type upstreamInstanceResources struct {
	CPU     *upstreamInstanceUsage   `json:"cpu"`
	Memory  *upstreamInstanceUsage   `json:"memory"`
	Storage *upstreamInstanceStorage `json:"storage"`
}

type upstreamInstanceUsage struct {
	UsagePercent *float64 `json:"usage_percent"`
}

type upstreamInstanceStorage struct {
	TotalBytes  *int64   `json:"total_bytes"`
	UsedBytes   *int64   `json:"used_bytes"`
	FreeBytes   *int64   `json:"free_bytes"`
	UsedPercent *float64 `json:"used_percent"`
}

type upstreamLogStatWire struct {
	Quota *int64 `json:"quota"`
	RPM   *int64 `json:"rpm"`
	TPM   *int64 `json:"tpm"`
}

func (client *NewAPIClient) validateStatus(wire upstreamStatusWire) (dto.UpstreamStatus, error) {
	if wire.Version == nil || wire.SystemName == nil || wire.QuotaPerUnit == nil ||
		wire.USDExchangeRate == nil || wire.DataExportEnabled == nil ||
		!validUpstreamString(*wire.Version, 0, 64) || !validUpstreamString(*wire.SystemName, 0, 128) {
		return dto.UpstreamStatus{}, invalidUpstreamResponse()
	}
	quotaPerUnit, ok := canonicalPositiveDecimal(wire.QuotaPerUnit.String())
	if !ok {
		return dto.UpstreamStatus{}, invalidUpstreamResponse()
	}
	exchangeRate, ok := canonicalPositiveDecimal(wire.USDExchangeRate.String())
	if !ok {
		return dto.UpstreamStatus{}, invalidUpstreamResponse()
	}
	return dto.UpstreamStatus{
		Version:           *wire.Version,
		SystemName:        *wire.SystemName,
		QuotaPerUnit:      quotaPerUnit,
		USDExchangeRate:   exchangeRate,
		DataExportEnabled: *wire.DataExportEnabled,
	}, nil
}

func canonicalPositiveDecimal(raw string) (string, bool) {
	return canonicalPositiveDecimalWithPrecision(raw, 20, 10, 30)
}

func canonicalPositiveDecimalWithPrecision(raw string, maxIntegerDigits, maxFractionDigits, maxPrecision int) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "-") {
		return "", false
	}
	mantissa := raw
	exponent := int64(0)
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		mantissa = raw[:index]
		parsed, err := strconv.ParseInt(raw[index+1:], 10, 64)
		if err != nil || parsed < -1000 || parsed > 1000 {
			return "", false
		}
		exponent = parsed
	}
	parts := strings.Split(mantissa, ".")
	if len(parts) > 2 || parts[0] == "" {
		return "", false
	}
	integerPart := parts[0]
	fractionPart := ""
	if len(parts) == 2 {
		fractionPart = parts[1]
	}
	digits := integerPart + fractionPart
	if strings.Trim(digits, "0") == "" {
		return "", false
	}
	decimalPosition := int64(len(integerPart)) + exponent
	var canonicalInteger, canonicalFraction string
	switch {
	case decimalPosition <= 0:
		trimmedDigits := strings.TrimRight(digits, "0")
		scale := -decimalPosition + int64(len(trimmedDigits))
		if scale > int64(maxFractionDigits) {
			return "", false
		}
		canonicalInteger = "0"
		canonicalFraction = strings.Repeat("0", int(-decimalPosition)) + trimmedDigits
	case decimalPosition >= int64(len(digits)):
		leading := strings.TrimLeft(digits, "0")
		integerDigits := int64(len(leading)) + decimalPosition - int64(len(digits))
		if integerDigits > int64(maxIntegerDigits) {
			return "", false
		}
		canonicalInteger = leading + strings.Repeat("0", int(decimalPosition-int64(len(digits))))
	default:
		canonicalInteger = strings.TrimLeft(digits[:decimalPosition], "0")
		if canonicalInteger == "" {
			canonicalInteger = "0"
		}
		canonicalFraction = strings.TrimRight(digits[decimalPosition:], "0")
	}
	integerDigits := len(strings.TrimLeft(canonicalInteger, "0"))
	if integerDigits == 0 {
		integerDigits = 1
	}
	if integerDigits > maxIntegerDigits || len(canonicalFraction) > maxFractionDigits || integerDigits+len(canonicalFraction) > maxPrecision {
		return "", false
	}
	if canonicalFraction == "" {
		return canonicalInteger, true
	}
	return canonicalInteger + "." + canonicalFraction, true
}

func canonicalNonNegativeMoneyDecimal(raw string) (string, bool) {
	if strings.HasPrefix(raw, "-") {
		return "", false
	}
	if raw == "0" || raw == "0.0" || raw == "0.00" {
		return "0", true
	}
	return canonicalPositiveDecimalWithPrecision(raw, 28, 10, 38)
}

func canonicalSignedDecimal(raw string) (string, bool) {
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = strings.TrimPrefix(raw, "-")
	}
	if raw == "0" || raw == "0.0" || raw == "0.00" {
		return "0", true
	}
	value, ok := canonicalPositiveDecimal(raw)
	if !ok {
		return "", false
	}
	if negative {
		return "-" + value, true
	}
	return value, true
}

func canonicalNonNegativeDecimal(raw string) (string, bool) {
	if strings.HasPrefix(raw, "-") {
		return "", false
	}
	if raw == "0" || strings.Trim(raw, "0.") == "" {
		return "0", true
	}
	return canonicalPositiveDecimal(raw)
}

func validatePerformanceHistory(w upstreamPerformanceHistoryWire, expectedModel string, now int64) (dto.UpstreamPerformanceModelHistory, bool, error) {
	if w.ModelName == nil || w.SeriesSchema == nil || w.Groups == nil || *w.ModelName != expectedModel || !validUpstreamString(*w.ModelName, 1, 255) || !validUpstreamString(*w.SeriesSchema, 1, 64) {
		return dto.UpstreamPerformanceModelHistory{}, false, invalidUpstreamResponse()
	}
	out := dto.UpstreamPerformanceModelHistory{ModelName: *w.ModelName, SeriesSchema: *w.SeriesSchema, Groups: make([]dto.UpstreamPerformanceGroupHistory, 0, len(*w.Groups))}
	seenGroups := map[string]struct{}{}
	counterMode := -1
	for _, g := range *w.Groups {
		if g.Group == nil || g.Series == nil || !validUpstreamString(*g.Group, 0, 128) {
			return out, false, invalidUpstreamResponse()
		}
		if _, ok := seenGroups[*g.Group]; ok {
			return out, false, invalidUpstreamResponse()
		}
		seenGroups[*g.Group] = struct{}{}
		group := dto.UpstreamPerformanceGroupHistory{Group: *g.Group, Series: make([]dto.UpstreamPerformanceBucket, 0, len(*g.Series))}
		seenTS := map[int64]struct{}{}
		for _, b := range *g.Series {
			if b.Ts == nil || b.AvgTTFTMS == nil || b.AvgLatencyMS == nil || b.SuccessRate == nil || b.AvgTPS == nil || *b.Ts <= 0 || *b.Ts > now+5 {
				return out, false, invalidUpstreamResponse()
			}
			ttft, ok1 := canonicalNonNegativeDecimal(b.AvgTTFTMS.String())
			latency, ok2 := canonicalNonNegativeDecimal(b.AvgLatencyMS.String())
			rate, ok3 := canonicalNonNegativeDecimal(b.SuccessRate.String())
			tps, ok4 := canonicalNonNegativeDecimal(b.AvgTPS.String())
			if !ok1 || !ok2 || !ok3 || !ok4 {
				return out, false, invalidUpstreamResponse()
			}
			rateRat, ok := new(big.Rat).SetString(rate)
			if !ok || rateRat.Cmp(big.NewRat(1, 1)) > 0 {
				return out, false, invalidUpstreamResponse()
			}
			if _, ok := seenTS[*b.Ts]; ok {
				return out, false, invalidUpstreamResponse()
			}
			seenTS[*b.Ts] = struct{}{}
			pointers := []*int64{b.RequestCount, b.SuccessCount, b.TotalLatencyMS, b.TTFTSumMS, b.TTFTCount, b.OutputTokens, b.GenerationMS}
			present := 0
			for _, p := range pointers {
				if p != nil {
					present++
				}
			}
			if present != 0 && present != len(pointers) {
				return out, false, invalidUpstreamResponse()
			}
			mode := 0
			if present == len(pointers) {
				mode = 1
				for _, p := range pointers {
					if *p < 0 {
						return out, false, invalidUpstreamResponse()
					}
				}
				if *b.SuccessCount > *b.RequestCount {
					return out, false, invalidUpstreamResponse()
				}
			}
			if counterMode == -1 {
				counterMode = mode
			} else if counterMode != mode {
				return out, false, invalidUpstreamResponse()
			}
			group.Series = append(group.Series, dto.UpstreamPerformanceBucket{Timestamp: *b.Ts, AvgTTFTMS: ttft, AvgLatencyMS: latency, SuccessRate: rate, AvgTPS: tps, Counters: dto.UpstreamPerformanceCounters{RequestCount: b.RequestCount, SuccessCount: b.SuccessCount, TotalLatencyMS: b.TotalLatencyMS, TTFTSumMS: b.TTFTSumMS, TTFTCount: b.TTFTCount, OutputTokens: b.OutputTokens, GenerationMS: b.GenerationMS}})
		}
		out.Groups = append(out.Groups, group)
	}
	return out, counterMode == 1, nil
}

func validateUpstreamIdentity(wire upstreamIdentityWire) (dto.UpstreamIdentity, error) {
	if wire.ID == nil || wire.Username == nil || wire.DisplayName == nil || wire.Role == nil ||
		wire.Status == nil || wire.Group == nil || *wire.ID <= 0 ||
		!validUpstreamString(*wire.Username, 1, 255) ||
		!validUpstreamString(*wire.DisplayName, 0, 255) || !validUpstreamString(*wire.Group, 0, 128) {
		return dto.UpstreamIdentity{}, invalidUpstreamResponse()
	}
	return dto.UpstreamIdentity{
		ID:          *wire.ID,
		Username:    *wire.Username,
		DisplayName: *wire.DisplayName,
		Role:        *wire.Role,
		Status:      *wire.Status,
		Group:       *wire.Group,
	}, nil
}

func (client *NewAPIClient) validateUser(wire upstreamUserWire) (dto.UpstreamUser, error) {
	identity, err := validateUpstreamIdentity(upstreamIdentityWire{
		ID: wire.ID, Username: wire.Username, DisplayName: wire.DisplayName,
		Role: wire.Role, Status: wire.Status, Group: wire.Group,
	})
	if err != nil || wire.Quota == nil || wire.UsedQuota == nil || wire.RequestCount == nil ||
		wire.CreatedAt == nil || wire.LastLoginAt == nil || *wire.Quota < 0 || *wire.UsedQuota < 0 ||
		*wire.RequestCount < 0 || *wire.CreatedAt <= 0 || *wire.CreatedAt > client.now().Unix()+5 ||
		*wire.LastLoginAt < 0 || len(wire.DeletedAt) == 0 {
		return dto.UpstreamUser{}, invalidUpstreamResponse()
	}
	deleted, ok := parseUpstreamDeletedAt(wire.DeletedAt)
	if !ok {
		return dto.UpstreamUser{}, invalidUpstreamResponse()
	}
	return dto.UpstreamUser{
		UpstreamIdentity: identity,
		Quota:            *wire.Quota,
		UsedQuota:        *wire.UsedQuota,
		RequestCount:     *wire.RequestCount,
		CreatedAt:        *wire.CreatedAt,
		LastLoginAt:      *wire.LastLoginAt,
		Deleted:          deleted,
	}, nil
}

func parseUpstreamDeletedAt(raw json.RawMessage) (bool, bool) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, true
	}
	var value struct {
		Valid *bool `json:"Valid"`
	}
	if err := validateStrictJSONFor(raw, value); err != nil {
		return false, false
	}
	if err := json.Unmarshal(raw, &value); err != nil || value.Valid == nil {
		return false, false
	}
	return *value.Valid, true
}

func (client *NewAPIClient) validateUserPage(wire upstreamUserPageWire, expectedPage int) (dto.UpstreamUserPage, error) {
	if wire.Page == nil || wire.PageSize == nil || wire.Total == nil || wire.Items == nil ||
		*wire.Page != expectedPage || *wire.PageSize != upstreamPageSize || *wire.Total < 0 ||
		len(*wire.Items) > upstreamPageSize || int64(len(*wire.Items)) > *wire.Total {
		return dto.UpstreamUserPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamUser, 0, len(*wire.Items))
	seen := make(map[int64]struct{}, len(*wire.Items))
	for _, raw := range *wire.Items {
		item, err := client.validateUser(raw)
		if err != nil {
			return dto.UpstreamUserPage{}, err
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return dto.UpstreamUserPage{}, invalidUpstreamResponse()
		}
		seen[item.ID] = struct{}{}
		items = append(items, item)
	}
	return dto.UpstreamUserPage{Page: *wire.Page, PageSize: *wire.PageSize, Total: *wire.Total, Items: items}, nil
}

func validateChannelPage(wire upstreamChannelPageWire, expectedPage int) (dto.UpstreamChannelPage, error) {
	if wire.Page == nil || wire.PageSize == nil || wire.Total == nil || wire.Items == nil ||
		*wire.Page != expectedPage || *wire.PageSize != upstreamPageSize || *wire.Total < 0 ||
		len(*wire.Items) > upstreamPageSize || int64(len(*wire.Items)) > *wire.Total {
		return dto.UpstreamChannelPage{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamChannel, 0, len(*wire.Items))
	seen := make(map[int64]struct{}, len(*wire.Items))
	for _, raw := range *wire.Items {
		if raw.ID == nil || raw.Name == nil || raw.Type == nil || raw.Status == nil || raw.TestTime == nil ||
			raw.ResponseTime == nil || raw.Balance == nil || raw.BalanceUpdatedTime == nil || raw.Models == nil || raw.Group == nil ||
			raw.UsedQuota == nil || *raw.ID <= 0 || *raw.TestTime < 0 || *raw.ResponseTime < 0 ||
			*raw.BalanceUpdatedTime < 0 || *raw.UsedQuota < 0 || !validUpstreamString(*raw.Name, 0, 255) ||
			!validUpstreamString(*raw.Models, 0, 65535) || !validUpstreamString(*raw.Group, 0, 128) ||
			!validOptionalUpstreamString(raw.Tag, 255) {
			return dto.UpstreamChannelPage{}, invalidUpstreamResponse()
		}
		balance, ok := canonicalSignedDecimal(raw.Balance.String())
		if !ok {
			return dto.UpstreamChannelPage{}, invalidUpstreamResponse()
		}
		priority, weight, autoBan, tag := int64(0), int64(0), 1, ""
		if raw.Priority != nil {
			priority = *raw.Priority
		}
		if raw.Weight != nil {
			weight = *raw.Weight
		}
		if raw.AutoBan != nil {
			autoBan = *raw.AutoBan
		}
		if raw.Tag != nil {
			tag = *raw.Tag
		}
		if priority < 0 || weight < 0 {
			return dto.UpstreamChannelPage{}, invalidUpstreamResponse()
		}
		if _, duplicate := seen[*raw.ID]; duplicate {
			return dto.UpstreamChannelPage{}, invalidUpstreamResponse()
		}
		seen[*raw.ID] = struct{}{}
		items = append(items, dto.UpstreamChannel{ID: *raw.ID, Name: *raw.Name, Type: *raw.Type, Status: *raw.Status,
			TestTime: *raw.TestTime, ResponseTimeMS: *raw.ResponseTime, Balance: balance, BalanceUpdatedAt: *raw.BalanceUpdatedTime,
			Models: *raw.Models, Group: *raw.Group, UsedQuota: *raw.UsedQuota, Priority: priority, Weight: weight, AutoBan: autoBan, Tag: tag})
	}
	return dto.UpstreamChannelPage{Page: *wire.Page, PageSize: *wire.PageSize, Total: *wire.Total, Items: items}, nil
}

func validateAndAggregateFlowRows(wire []upstreamFlowRowWire) ([]dto.UpstreamFlowRow, error) {
	type flowKey struct {
		UserID    int64
		ModelName string
		ChannelID int64
		UseGroup  string
		TokenID   int64
		NodeName  string
	}
	result := make([]dto.UpstreamFlowRow, 0, len(wire))
	indices := make(map[flowKey]int, len(wire))
	for _, raw := range wire {
		channelID, ok := parseOptionalFlowChannelID(raw.ChannelID)
		useGroup, tokenName, nodeName := "", "", ""
		tokenID := int64(0)
		if raw.UseGroup != nil {
			useGroup = *raw.UseGroup
		}
		if raw.TokenID != nil {
			tokenID = *raw.TokenID
		}
		if raw.TokenName != nil {
			tokenName = *raw.TokenName
		}
		if raw.NodeName != nil {
			nodeName = *raw.NodeName
		}
		if !ok || raw.UserID == nil || raw.Username == nil || raw.ModelName == nil ||
			raw.RequestCount == nil || raw.Quota == nil || raw.TokenUsed == nil || *raw.UserID <= 0 ||
			tokenID < 0 || *raw.RequestCount < 0 || *raw.Quota < 0 || *raw.TokenUsed < 0 ||
			!validUpstreamString(*raw.Username, 0, 255) || !validUpstreamString(*raw.ModelName, 0, 255) ||
			!validUpstreamString(useGroup, 0, 128) || !validUpstreamString(tokenName, 0, 255) ||
			!validUpstreamString(nodeName, 0, 128) {
			return nil, invalidUpstreamResponse()
		}
		key := flowKey{UserID: *raw.UserID, ModelName: *raw.ModelName, ChannelID: channelID,
			UseGroup: useGroup, TokenID: tokenID, NodeName: nodeName}
		if index, duplicate := indices[key]; duplicate {
			result[index].Username = canonicalFlowUsername(result[index].Username, *raw.Username)
			result[index].TokenName = canonicalFlowUsername(result[index].TokenName, tokenName)
			var ok bool
			if result[index].RequestCount, ok = checkedAddInt64(result[index].RequestCount, *raw.RequestCount); !ok {
				return nil, invalidUpstreamResponse()
			}
			if result[index].Quota, ok = checkedAddInt64(result[index].Quota, *raw.Quota); !ok {
				return nil, invalidUpstreamResponse()
			}
			if result[index].TokenUsed, ok = checkedAddInt64(result[index].TokenUsed, *raw.TokenUsed); !ok {
				return nil, invalidUpstreamResponse()
			}
			continue
		}
		indices[key] = len(result)
		result = append(result, dto.UpstreamFlowRow{
			UserID: *raw.UserID, Username: *raw.Username, ModelName: *raw.ModelName,
			ChannelID: channelID, UseGroup: useGroup, TokenID: tokenID, TokenName: tokenName, NodeName: nodeName,
			RequestCount: *raw.RequestCount, Quota: *raw.Quota, TokenUsed: *raw.TokenUsed,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].UserID != result[right].UserID {
			return result[left].UserID < result[right].UserID
		}
		if result[left].ModelName != result[right].ModelName {
			return result[left].ModelName < result[right].ModelName
		}
		if result[left].ChannelID != result[right].ChannelID {
			return result[left].ChannelID < result[right].ChannelID
		}
		if result[left].UseGroup != result[right].UseGroup {
			return result[left].UseGroup < result[right].UseGroup
		}
		if result[left].TokenID != result[right].TokenID {
			return result[left].TokenID < result[right].TokenID
		}
		return result[left].NodeName < result[right].NodeName
	})
	return result, nil
}

func canonicalFlowUsername(current, candidate string) string {
	if current == "" {
		return candidate
	}
	if candidate == "" || current <= candidate {
		return current
	}
	return candidate
}

func parseOptionalFlowChannelID(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, false
	}
	var channelID int64
	if err := json.Unmarshal(raw, &channelID); err != nil || channelID < 0 {
		return 0, false
	}
	return channelID, true
}

func validateAndAggregateDataRows(wire []upstreamDataRowWire, hourStart int64) ([]dto.UpstreamDataRow, error) {
	result := make([]dto.UpstreamDataRow, 0, len(wire))
	indices := make(map[string]int, len(wire))
	for _, raw := range wire {
		if raw.ModelName == nil || raw.CreatedAt == nil || raw.RequestCount == nil || raw.Quota == nil ||
			raw.TokenUsed == nil || *raw.CreatedAt != hourStart || *raw.RequestCount < 0 || *raw.Quota < 0 ||
			*raw.TokenUsed < 0 || !validUpstreamString(*raw.ModelName, 0, 255) {
			return nil, invalidUpstreamResponse()
		}
		if index, duplicate := indices[*raw.ModelName]; duplicate {
			var ok bool
			if result[index].RequestCount, ok = checkedAddInt64(result[index].RequestCount, *raw.RequestCount); !ok {
				return nil, invalidUpstreamResponse()
			}
			if result[index].Quota, ok = checkedAddInt64(result[index].Quota, *raw.Quota); !ok {
				return nil, invalidUpstreamResponse()
			}
			if result[index].TokenUsed, ok = checkedAddInt64(result[index].TokenUsed, *raw.TokenUsed); !ok {
				return nil, invalidUpstreamResponse()
			}
			continue
		}
		indices[*raw.ModelName] = len(result)
		result = append(result, dto.UpstreamDataRow{
			ModelName: *raw.ModelName, CreatedAt: *raw.CreatedAt, RequestCount: *raw.RequestCount,
			Quota: *raw.Quota, TokenUsed: *raw.TokenUsed,
		})
	}
	return result, nil
}

func (client *NewAPIClient) validateInstances(wire []upstreamInstanceWire) ([]dto.UpstreamInstance, error) {
	result := make([]dto.UpstreamInstance, 0, len(wire))
	seen := make(map[string]struct{}, len(wire))
	maximumTimestamp := client.now().Unix() + 5
	for _, raw := range wire {
		if raw.NodeName == nil || raw.Status == nil || raw.StaleAfterSeconds == nil || raw.StartedAt == nil ||
			raw.LastSeenAt == nil || raw.Info == nil || !validUpstreamString(*raw.NodeName, 1, 128) ||
			(*raw.Status != "online" && *raw.Status != "stale") || *raw.StaleAfterSeconds < 1 ||
			*raw.StaleAfterSeconds > 3600 || *raw.StartedAt < 0 || *raw.StartedAt > maximumTimestamp ||
			*raw.LastSeenAt < 0 || *raw.LastSeenAt > maximumTimestamp {
			return nil, invalidUpstreamResponse()
		}
		if _, duplicate := seen[*raw.NodeName]; duplicate {
			return nil, invalidUpstreamResponse()
		}
		seen[*raw.NodeName] = struct{}{}
		info := raw.Info
		if info.Node == nil || info.Node.Name == nil || *info.Node.Name != *raw.NodeName ||
			info.Runtime == nil || info.Runtime.Version == nil || info.Runtime.GOOS == nil || info.Runtime.GOARCH == nil ||
			info.Host == nil || info.Host.Hostname == nil || len(info.Role) == 0 ||
			!validUpstreamString(*info.Runtime.Version, 0, 64) || !validUpstreamString(*info.Runtime.GOOS, 0, 32) ||
			!validUpstreamString(*info.Runtime.GOARCH, 0, 32) || !validUpstreamString(*info.Host.Hostname, 0, 255) {
			return nil, invalidUpstreamResponse()
		}
		isMaster, ok := parseInstanceRole(info.Role)
		if !ok {
			return nil, invalidUpstreamResponse()
		}
		instance := dto.UpstreamInstance{
			NodeName: *raw.NodeName, Status: *raw.Status, StaleAfterSeconds: *raw.StaleAfterSeconds,
			StartedAt: *raw.StartedAt, LastSeenAt: *raw.LastSeenAt, IsMaster: &isMaster,
			RuntimeVersion: *info.Runtime.Version, GOOS: *info.Runtime.GOOS, GOARCH: *info.Runtime.GOARCH,
			Hostname: *info.Host.Hostname,
		}
		if info.Resources != nil {
			if info.Resources.CPU != nil {
				instance.CPUPercent = info.Resources.CPU.UsagePercent
			}
			if info.Resources.Memory != nil {
				instance.MemoryPercent = info.Resources.Memory.UsagePercent
			}
			if info.Resources.Storage != nil {
				instance.StorageTotalBytes = info.Resources.Storage.TotalBytes
				instance.StorageUsedBytes = info.Resources.Storage.UsedBytes
				instance.StorageFreeBytes = info.Resources.Storage.FreeBytes
				instance.StorageUsedPercent = info.Resources.Storage.UsedPercent
			}
		}
		if !validOptionalPercent(instance.CPUPercent) || !validOptionalPercent(instance.MemoryPercent) ||
			!validOptionalPercent(instance.StorageUsedPercent) || !validOptionalNonnegative(instance.StorageTotalBytes) ||
			!validOptionalNonnegative(instance.StorageUsedBytes) || !validOptionalNonnegative(instance.StorageFreeBytes) ||
			(instance.StorageTotalBytes != nil && instance.StorageUsedBytes != nil && *instance.StorageUsedBytes > *instance.StorageTotalBytes) {
			return nil, invalidUpstreamResponse()
		}
		result = append(result, instance)
	}
	return result, nil
}

func parseInstanceRole(raw json.RawMessage) (bool, bool) {
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		switch legacy {
		case "master":
			return true, true
		case "worker":
			return false, true
		default:
			return false, false
		}
	}
	var structured struct {
		IsMaster *bool `json:"is_master"`
	}
	if err := validateStrictJSONFor(raw, structured); err != nil {
		return false, false
	}
	if err := json.Unmarshal(raw, &structured); err != nil || structured.IsMaster == nil {
		return false, false
	}
	return *structured.IsMaster, true
}

func validOptionalPercent(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 100)
}

func validOptionalNonnegative(value *int64) bool {
	return value == nil || *value >= 0
}

func validateLogStat(wire upstreamLogStatWire) (dto.UpstreamLogStat, error) {
	if wire.Quota == nil || wire.RPM == nil || wire.TPM == nil || *wire.Quota < 0 || *wire.RPM < 0 || *wire.TPM < 0 {
		return dto.UpstreamLogStat{}, invalidUpstreamResponse()
	}
	return dto.UpstreamLogStat{Quota: *wire.Quota, RPM: *wire.RPM, TPM: *wire.TPM}, nil
}

func invalidUpstreamResponse() error {
	return newUpstreamRequestError(UpstreamErrorResponseInvalid)
}
