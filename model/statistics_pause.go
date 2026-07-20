package model

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"
)

type statisticsPauseAccount struct {
	ID         int64
	SiteID     int64
	CustomerID int64
	PauseAt    int64
}

type statisticsPauseCustomer struct {
	ID      int64
	PauseAt int64
	SiteIDs []int64
}

type statisticsPauseAccountDailyKey struct {
	AccountID int64
	DateKey   int
}

type statisticsPauseCustomerHourlyKey struct {
	CustomerID int64
	SiteID     int64
	HourTS     int64
}

type statisticsPauseCustomerDailyKey struct {
	CustomerID int64
	SiteID     int64
	DateKey    int
}

type statisticsPauseDailyAggregate struct {
	AccountID    int64  `gorm:"column:account_id"`
	DateKey      int    `gorm:"column:date_key"`
	RequestCount int64  `gorm:"column:request_count"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	DataStatus   string `gorm:"column:data_status"`
}

type statisticsPauseCustomerHourlyAggregate struct {
	HourTS       int64  `gorm:"column:hour_ts"`
	RequestCount int64  `gorm:"column:request_count"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	ActiveUsers  int64  `gorm:"column:active_users"`
	DataStatus   string `gorm:"column:data_status"`
}

type statisticsPauseCustomerDailyAggregate struct {
	DateKey      int    `gorm:"column:date_key"`
	RequestCount int64  `gorm:"column:request_count"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	ActiveUsers  int64  `gorm:"column:active_users"`
	DataStatus   string `gorm:"column:data_status"`
}

type statisticsPauseCustomerSite struct {
	CustomerID int64
	SiteID     int64
	Boundary   int64
	PauseAt    *int64
}

func cleanupStatisticsForPause(
	ctx context.Context,
	tx *gorm.DB,
	accounts []statisticsPauseAccount,
	customers []statisticsPauseCustomer,
	now int64,
) error {
	if tx == nil || now <= 0 {
		return ErrAccountObservationInvalid
	}
	accounts, err := normalizeStatisticsPauseAccounts(accounts)
	if err != nil {
		return err
	}
	customers, err = normalizeStatisticsPauseCustomers(customers)
	if err != nil {
		return err
	}
	if len(accounts) == 0 && len(customers) == 0 {
		return nil
	}
	if err := cleanupPausedAccountStatistics(ctx, tx, accounts, now); err != nil {
		return err
	}
	return rebuildPausedCustomerStatistics(ctx, tx, accounts, customers, now)
}

func normalizeStatisticsPauseAccounts(accounts []statisticsPauseAccount) ([]statisticsPauseAccount, error) {
	byID := make(map[int64]statisticsPauseAccount, len(accounts))
	for _, account := range accounts {
		if account.ID <= 0 || account.SiteID <= 0 || account.CustomerID <= 0 || account.PauseAt <= 0 {
			return nil, ErrAccountObservationInvalid
		}
		if existing, exists := byID[account.ID]; exists {
			if existing.SiteID != account.SiteID || existing.CustomerID != account.CustomerID {
				return nil, ErrAccountOperationScopeChanged
			}
			if account.PauseAt < existing.PauseAt {
				existing.PauseAt = account.PauseAt
				byID[account.ID] = existing
			}
			continue
		}
		byID[account.ID] = account
	}
	result := make([]statisticsPauseAccount, 0, len(byID))
	for _, account := range byID {
		result = append(result, account)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func normalizeStatisticsPauseCustomers(customers []statisticsPauseCustomer) ([]statisticsPauseCustomer, error) {
	byID := make(map[int64]statisticsPauseCustomer, len(customers))
	for _, customer := range customers {
		if customer.ID <= 0 || customer.PauseAt <= 0 {
			return nil, ErrCustomerLifecycleContract
		}
		existing, exists := byID[customer.ID]
		if !exists {
			existing = statisticsPauseCustomer{ID: customer.ID, PauseAt: customer.PauseAt}
		} else if customer.PauseAt < existing.PauseAt {
			existing.PauseAt = customer.PauseAt
		}
		sites := make(map[int64]struct{}, len(existing.SiteIDs)+len(customer.SiteIDs))
		for _, siteID := range existing.SiteIDs {
			sites[siteID] = struct{}{}
		}
		for _, siteID := range customer.SiteIDs {
			if siteID <= 0 {
				return nil, ErrCustomerLifecycleContract
			}
			sites[siteID] = struct{}{}
		}
		existing.SiteIDs = existing.SiteIDs[:0]
		for siteID := range sites {
			existing.SiteIDs = append(existing.SiteIDs, siteID)
		}
		sort.Slice(existing.SiteIDs, func(left, right int) bool { return existing.SiteIDs[left] < existing.SiteIDs[right] })
		byID[customer.ID] = existing
	}
	result := make([]statisticsPauseCustomer, 0, len(byID))
	for _, customer := range byID {
		result = append(result, customer)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func cleanupPausedAccountStatistics(ctx context.Context, tx *gorm.DB, accounts []statisticsPauseAccount, now int64) error {
	for _, account := range accounts {
		dateKey, dateStart := statisticsPauseDateBoundary(account.PauseAt)
		var existing []AccountStatDaily
		if err := tx.WithContext(ctx).Where("account_id = ? AND date_key >= ?", account.ID, dateKey).
			Order("date_key ASC").Find(&existing).Error; err != nil {
			return err
		}
		existingByKey := make(map[statisticsPauseAccountDailyKey]AccountStatDaily, len(existing))
		for _, daily := range existing {
			existingByKey[statisticsPauseAccountDailyKey{AccountID: daily.AccountID, DateKey: daily.DateKey}] = daily
		}
		if err := tx.WithContext(ctx).Where("account_id = ? AND hour_ts >= ?", account.ID, account.PauseAt).
			Delete(&AccountStatHourly{}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("account_id = ? AND date_key >= ?", account.ID, dateKey).
			Delete(&AccountStatDaily{}).Error; err != nil {
			return err
		}
		var aggregates []statisticsPauseDailyAggregate
		if err := tx.WithContext(ctx).Raw(`SELECT h.account_id,
CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, h.hour_ts, '1970-01-01 08:00:00'), '%Y%m%d') AS UNSIGNED) AS date_key,
SUM(h.request_count) AS request_count, SUM(h.quota) AS quota, SUM(h.token_used) AS token_used,
CASE WHEN SUM(CASE WHEN h.data_status <> 'complete' THEN 1 ELSE 0 END) > 0 THEN 'partial' ELSE 'complete' END AS data_status
FROM account_stat_hourly h
WHERE h.account_id = ? AND h.hour_ts >= ?
GROUP BY h.account_id, date_key
ORDER BY date_key ASC`, account.ID, dateStart).Scan(&aggregates).Error; err != nil {
			return err
		}
		rows := make([]AccountStatDaily, 0, len(aggregates))
		for _, aggregate := range aggregates {
			row := AccountStatDaily{
				AccountID: aggregate.AccountID, DateKey: aggregate.DateKey,
				RequestCount: aggregate.RequestCount, Quota: aggregate.Quota, TokenUsed: aggregate.TokenUsed,
				DataStatus: aggregate.DataStatus, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
			}
			if previous, exists := existingByKey[statisticsPauseAccountDailyKey{AccountID: row.AccountID, DateKey: row.DateKey}]; exists {
				row.DataStatus = statisticsPauseDataStatus(row.DataStatus, previous.DataStatus)
				row.IsFinal = previous.IsFinal
				if previous.CreatedAt > 0 {
					row.CreatedAt = previous.CreatedAt
				}
			}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			if err := tx.WithContext(ctx).CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func rebuildPausedCustomerStatistics(
	ctx context.Context,
	tx *gorm.DB,
	accounts []statisticsPauseAccount,
	customers []statisticsPauseCustomer,
	now int64,
) error {
	keys := statisticsPauseCustomerSites(accounts, customers)
	for _, key := range keys {
		if err := rebuildPausedCustomerHourly(ctx, tx, key, now); err != nil {
			return err
		}
		if err := rebuildPausedCustomerDaily(ctx, tx, key, now); err != nil {
			return err
		}
	}
	return nil
}

func statisticsPauseCustomerSites(accounts []statisticsPauseAccount, customers []statisticsPauseCustomer) []statisticsPauseCustomerSite {
	type customerSiteKey struct {
		customerID int64
		siteID     int64
	}
	byKey := make(map[customerSiteKey]statisticsPauseCustomerSite)
	for _, account := range accounts {
		mapKey := customerSiteKey{customerID: account.CustomerID, siteID: account.SiteID}
		key, exists := byKey[mapKey]
		if !exists || account.PauseAt < key.Boundary {
			key = statisticsPauseCustomerSite{CustomerID: account.CustomerID, SiteID: account.SiteID, Boundary: account.PauseAt}
		}
		byKey[mapKey] = key
	}
	for _, customer := range customers {
		pauseAt := customer.PauseAt
		for _, siteID := range customer.SiteIDs {
			mapKey := customerSiteKey{customerID: customer.ID, siteID: siteID}
			key, exists := byKey[mapKey]
			if !exists {
				key = statisticsPauseCustomerSite{CustomerID: customer.ID, SiteID: siteID, Boundary: pauseAt}
			} else if pauseAt < key.Boundary {
				key.Boundary = pauseAt
			}
			key.PauseAt = &pauseAt
			byKey[mapKey] = key
		}
	}
	result := make([]statisticsPauseCustomerSite, 0, len(byKey))
	for _, key := range byKey {
		result = append(result, key)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].CustomerID != result[right].CustomerID {
			return result[left].CustomerID < result[right].CustomerID
		}
		return result[left].SiteID < result[right].SiteID
	})
	return result
}

func rebuildPausedCustomerHourly(ctx context.Context, tx *gorm.DB, key statisticsPauseCustomerSite, now int64) error {
	var existing []CustomerStatHourly
	if err := tx.WithContext(ctx).Where("customer_id = ? AND site_id = ? AND hour_ts >= ?", key.CustomerID, key.SiteID, key.Boundary).
		Order("hour_ts ASC").Find(&existing).Error; err != nil {
		return err
	}
	existingByKey := make(map[statisticsPauseCustomerHourlyKey]CustomerStatHourly, len(existing))
	for _, hourly := range existing {
		existingByKey[statisticsPauseCustomerHourlyKey{CustomerID: hourly.CustomerID, SiteID: hourly.SiteID, HourTS: hourly.HourTS}] = hourly
	}
	if err := tx.WithContext(ctx).Where("customer_id = ? AND site_id = ? AND hour_ts >= ?", key.CustomerID, key.SiteID, key.Boundary).
		Delete(&CustomerStatHourly{}).Error; err != nil {
		return err
	}
	query := `SELECT h.hour_ts, SUM(h.request_count) AS request_count, SUM(h.quota) AS quota,
SUM(h.token_used) AS token_used, COUNT(DISTINCT h.account_id) AS active_users,
CASE WHEN SUM(CASE WHEN h.data_status <> 'complete' THEN 1 ELSE 0 END) > 0 THEN 'partial' ELSE 'complete' END AS data_status
FROM account_stat_hourly h
JOIN account a ON a.id = h.account_id
WHERE a.customer_id = ? AND a.site_id = ? AND h.hour_ts >= ?
  AND (a.statistics_paused_at IS NULL OR h.hour_ts < a.statistics_paused_at)`
	args := []any{key.CustomerID, key.SiteID, key.Boundary}
	if key.PauseAt != nil {
		query += " AND h.hour_ts < ?"
		args = append(args, *key.PauseAt)
	}
	query += " GROUP BY h.hour_ts ORDER BY h.hour_ts ASC"
	var aggregates []statisticsPauseCustomerHourlyAggregate
	if err := tx.WithContext(ctx).Raw(query, args...).Scan(&aggregates).Error; err != nil {
		return err
	}
	rows := make([]CustomerStatHourly, 0, len(aggregates))
	for _, aggregate := range aggregates {
		row := CustomerStatHourly{
			CustomerID: key.CustomerID, SiteID: key.SiteID, HourTS: aggregate.HourTS,
			RequestCount: aggregate.RequestCount, Quota: aggregate.Quota, TokenUsed: aggregate.TokenUsed,
			ActiveUsers: aggregate.ActiveUsers, DataStatus: aggregate.DataStatus,
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if previous, exists := existingByKey[statisticsPauseCustomerHourlyKey{CustomerID: row.CustomerID, SiteID: row.SiteID, HourTS: row.HourTS}]; exists {
			row.DataStatus = statisticsPauseDataStatus(row.DataStatus, previous.DataStatus)
			if previous.CreatedAt > 0 {
				row.CreatedAt = previous.CreatedAt
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).CreateInBatches(rows, 500).Error
}

func rebuildPausedCustomerDaily(ctx context.Context, tx *gorm.DB, key statisticsPauseCustomerSite, now int64) error {
	dateKey, dateStart := statisticsPauseDateBoundary(key.Boundary)
	var existing []CustomerStatDaily
	if err := tx.WithContext(ctx).Where("customer_id = ? AND site_id = ? AND date_key >= ?", key.CustomerID, key.SiteID, dateKey).
		Order("date_key ASC").Find(&existing).Error; err != nil {
		return err
	}
	existingByKey := make(map[statisticsPauseCustomerDailyKey]CustomerStatDaily, len(existing))
	for _, daily := range existing {
		existingByKey[statisticsPauseCustomerDailyKey{CustomerID: daily.CustomerID, SiteID: daily.SiteID, DateKey: daily.DateKey}] = daily
	}
	if err := tx.WithContext(ctx).Where("customer_id = ? AND site_id = ? AND date_key >= ?", key.CustomerID, key.SiteID, dateKey).
		Delete(&CustomerStatDaily{}).Error; err != nil {
		return err
	}
	query := `SELECT
CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, h.hour_ts, '1970-01-01 08:00:00'), '%Y%m%d') AS UNSIGNED) AS date_key,
SUM(h.request_count) AS request_count, SUM(h.quota) AS quota, SUM(h.token_used) AS token_used,
COUNT(DISTINCT h.account_id) AS active_users,
CASE WHEN SUM(CASE WHEN h.data_status <> 'complete' THEN 1 ELSE 0 END) > 0 THEN 'partial' ELSE 'complete' END AS data_status
FROM account_stat_hourly h
JOIN account a ON a.id = h.account_id
WHERE a.customer_id = ? AND a.site_id = ? AND h.hour_ts >= ?
  AND (a.statistics_paused_at IS NULL OR h.hour_ts < a.statistics_paused_at)`
	args := []any{key.CustomerID, key.SiteID, dateStart}
	if key.PauseAt != nil {
		query += " AND h.hour_ts < ?"
		args = append(args, *key.PauseAt)
	}
	query += " GROUP BY date_key ORDER BY date_key ASC"
	var aggregates []statisticsPauseCustomerDailyAggregate
	if err := tx.WithContext(ctx).Raw(query, args...).Scan(&aggregates).Error; err != nil {
		return err
	}
	rows := make([]CustomerStatDaily, 0, len(aggregates))
	for _, aggregate := range aggregates {
		row := CustomerStatDaily{
			CustomerID: key.CustomerID, SiteID: key.SiteID, DateKey: aggregate.DateKey,
			RequestCount: aggregate.RequestCount, Quota: aggregate.Quota, TokenUsed: aggregate.TokenUsed,
			ActiveUsers: aggregate.ActiveUsers, DataStatus: aggregate.DataStatus,
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if previous, exists := existingByKey[statisticsPauseCustomerDailyKey{CustomerID: row.CustomerID, SiteID: row.SiteID, DateKey: row.DateKey}]; exists {
			row.DataStatus = statisticsPauseDataStatus(row.DataStatus, previous.DataStatus)
			row.IsFinal = previous.IsFinal
			if previous.CreatedAt > 0 {
				row.CreatedAt = previous.CreatedAt
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).CreateInBatches(rows, 500).Error
}

func statisticsPauseDateBoundary(timestamp int64) (int, int64) {
	local := time.Unix(timestamp, 0).In(usageAggregationLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, usageAggregationLocation)
	return local.Year()*10000 + int(local.Month())*100 + local.Day(), start.Unix()
}

func statisticsPauseDataStatus(aggregate, previous string) string {
	if aggregate == UsageAggregationStatusPartial || previous == UsageAggregationStatusPartial {
		return UsageAggregationStatusPartial
	}
	if aggregate == UsageAggregationStatusComplete {
		return UsageAggregationStatusComplete
	}
	if previous != "" {
		return previous
	}
	return UsageAggregationStatusComplete
}
