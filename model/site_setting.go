package model

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func (repository *SiteRepository) IntSetting(ctx context.Context, key string, minimum, maximum int) (int, error) {
	if strings.TrimSpace(key) != key || key == "" || minimum > maximum {
		return 0, errors.New("invalid integer setting request")
	}
	var value, valueType string
	var secret bool
	err := repository.db.WithContext(ctx).Raw(`SELECT setting_value, value_type, is_secret
FROM platform_setting WHERE setting_key = ?`, key).Row().Scan(&value, &valueType, &secret)
	if err != nil {
		return 0, fmt.Errorf("read setting %s: %w", key, err)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || valueType != "int" || secret || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("setting %s has an invalid integer contract", key)
	}
	return parsed, nil
}

func (repository *SiteRepository) EffectiveInstanceStaleSeconds(ctx context.Context, siteID int64) (int, error) {
	if siteID <= 0 {
		return 0, errors.New("invalid site instance stale scope")
	}
	var threshold string
	err := repository.db.WithContext(ctx).Raw(`SELECT CAST(threshold_value AS CHAR)
FROM alert_rule
WHERE rule_key = 'instance_stale' AND level = 'warning'
  AND ((scope_type = 'site' AND scope_id = ?) OR (scope_type = 'global' AND scope_id = 0))
ORDER BY CASE WHEN scope_type = 'site' THEN 0 ELSE 1 END, id DESC
LIMIT 1`, siteID).Row().Scan(&threshold)
	if err != nil {
		return 0, fmt.Errorf("read effective instance_stale rule for site %d: %w", siteID, err)
	}
	value, valid := new(big.Rat).SetString(threshold)
	if !valid || !value.IsInt() || !value.Num().IsInt64() {
		return 0, fmt.Errorf("effective instance_stale rule for site %d has an invalid threshold", siteID)
	}
	seconds := value.Num().Int64()
	if seconds < 1 || seconds > int64(^uint32(0)>>1) {
		return 0, fmt.Errorf("effective instance_stale rule for site %d is outside the supported range", siteID)
	}
	return int(seconds), nil
}
