package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCustomerNotFound            = errors.New("customer not found")
	ErrCustomerInvalid             = errors.New("customer is invalid")
	ErrCustomerInvalidState        = errors.New("customer state does not allow this operation")
	ErrCustomerDeleteRestricted    = errors.New("customer has deletion dependencies")
	ErrEntityStatisticsUnavailable = errors.New("entity statistics reader is unavailable")
	ErrEntityBackfillRunning       = errors.New("entity has a different active backfill range")
)

type EntityStatisticsReader interface {
	CustomerStatistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	AccountStatistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error)
}

type CustomerService struct {
	db         *gorm.DB
	clock      common.Clock
	statistics EntityStatisticsReader
	postCommit PostCommitNotifier
}

type CustomerServiceOptions struct {
	Database   *gorm.DB
	Clock      common.Clock
	Statistics EntityStatisticsReader
	PostCommit PostCommitNotifier
}

func NewCustomerService(options CustomerServiceOptions) (*CustomerService, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("customer service dependencies are required")
	}
	return &CustomerService{
		db: options.Database, clock: options.Clock, statistics: options.Statistics, postCommit: options.PostCommit,
	}, nil
}

func (service *CustomerService) Create(ctx context.Context, request dto.CustomerCreateRequest) (dto.CustomerDetail, error) {
	if request.Validate() != nil {
		return dto.CustomerDetail{}, ErrCustomerInvalid
	}
	if request.ContractAmount == "" {
		request.ContractAmount = "0"
	}
	if request.PaymentAmount == "" {
		request.PaymentAmount = "0"
	}
	now := service.clock.Now().Unix()
	customer := model.Customer{
		Name: request.Name, Contact: request.Contact, Remark: request.Remark,
		ContractAmount: request.ContractAmount, PaymentAmount: request.PaymentAmount, Status: request.Status,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewCustomerRepository(service.db).Create(ctx, &customer); err != nil {
		return dto.CustomerDetail{}, fmt.Errorf("create customer: %w", err)
	}
	return service.Get(ctx, customer.ID)
}

func (service *CustomerService) Get(ctx context.Context, id int64) (dto.CustomerDetail, error) {
	customer, err := model.NewCustomerRepository(service.db).FindByID(ctx, id)
	if err != nil {
		return dto.CustomerDetail{}, customerServiceError(err)
	}
	item, err := service.listItemFromModel(ctx, customer)
	if err != nil {
		return dto.CustomerDetail{}, err
	}
	return dto.CustomerDetail{
		CustomerListItem: item, StatisticsPausedAt: customer.StatisticsPausedAt,
		Completeness: entityCompleteness(customer.StatisticsPausedAt != nil, "customer_site_hour"),
		CreatedAt:    customer.CreatedAt,
	}, nil
}

func (service *CustomerService) List(ctx context.Context, query dto.CustomerListQuery) (common.PageData[dto.CustomerListItem], error) {
	customers, total, err := model.NewCustomerRepository(service.db).List(ctx, model.CustomerFilter{
		Keyword: query.Keyword, Status: query.Status, SortBy: query.SortBy, SortOrder: query.SortOrder,
		TodayDateKey: beijingDateKey(service.clock.Now()), Offset: query.Offset(), Limit: query.PageSize,
	})
	if err != nil {
		return common.PageData[dto.CustomerListItem]{}, fmt.Errorf("list customers: %w", err)
	}
	items := make([]dto.CustomerListItem, 0, len(customers))
	for _, customer := range customers {
		item, err := service.listItemFromModel(ctx, customer)
		if err != nil {
			return common.PageData[dto.CustomerListItem]{}, err
		}
		items = append(items, item)
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *CustomerService) Update(ctx context.Context, id int64, request dto.CustomerUpdateRequest) (dto.CustomerDetail, error) {
	if request.Validate() != nil {
		return dto.CustomerDetail{}, ErrCustomerInvalid
	}
	if request.ContractAmount == "" {
		request.ContractAmount = "0"
	}
	if request.PaymentAmount == "" {
		request.PaymentAmount = "0"
	}
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.NewCustomerRepository(tx).FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		current.Name = request.Name
		current.Contact = request.Contact
		current.Remark = request.Remark
		current.ContractAmount = request.ContractAmount
		current.PaymentAmount = request.PaymentAmount
		current.Status = request.Status
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), current.UpdatedAt)
		current.UpdatedAt = committedAt
		return model.NewCustomerRepository(tx).UpdateProfile(ctx, &current)
	})
	if err != nil {
		return dto.CustomerDetail{}, customerServiceError(err)
	}
	service.notifyCustomerLifecycleAfterCommit(ctx, id, committedAt)
	return service.Get(ctx, id)
}

func (service *CustomerService) Delete(ctx context.Context, id int64) error {
	err := model.NewCustomerRepository(service.db).DeleteByID(ctx, id)
	return customerServiceError(err)
}

func (service *CustomerService) Disable(ctx context.Context, id int64) (dto.CustomerDetail, error) {
	now := service.clock.Now().Unix()
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockCustomerOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		committedAt = monotonicMutationTime(now, scope.Customer.UpdatedAt)
		return model.NewCustomerRepository(tx).DisableInTransaction(ctx, id, floorHour(now), committedAt)
	})
	if err != nil {
		return dto.CustomerDetail{}, customerServiceError(err)
	}
	service.notifyCustomerLifecycleAfterCommit(ctx, id, committedAt)
	return service.Get(ctx, id)
}

func (service *CustomerService) Enable(ctx context.Context, id int64, requestID string) (dto.CollectionRunItem, error) {
	var result dto.CollectionRunItem
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockCustomerOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		if scope.Customer.Status != dto.CustomerStatusDisabled || scope.Customer.StatisticsPausedAt == nil {
			return ErrCustomerInvalidState
		}
		now := service.clock.Now().Unix()
		committedAt = monotonicMutationTime(now, scope.Customer.UpdatedAt)
		start := floorHour(*scope.Customer.StatisticsPausedAt)
		end := floorHour(now)
		if end < start {
			end = start
		}
		if existing, found, err := activeLocalRebuildRun(ctx, tx, "customer", id, constant.TaskTypeCustomerRebuild); err != nil {
			return err
		} else if found {
			if !sameRunRange(existing, start, end) {
				return ErrEntityBackfillRunning
			}
			result = collectionRunItemFromModel(existing, true)
			return nil
		}
		if err := model.NewCustomerRepository(tx).BeginEnableInTransaction(ctx, id, committedAt); err != nil {
			return err
		}
		run, err := newLocalRebuildRun("customer", id, constant.TaskTypeCustomerRebuild,
			*scope.Customer.StatisticsPausedAt, floorHour(now), requestID, committedAt)
		if err != nil {
			return err
		}
		created, deduplicated, err := model.NewSiteRepository(tx).CreateOrGetRun(ctx, &run)
		if err != nil {
			return err
		}
		result = collectionRunItemFromModel(created, deduplicated)
		return nil
	})
	if err != nil {
		return dto.CollectionRunItem{}, customerServiceError(err)
	}
	service.notifyCustomerLifecycleAfterCommit(ctx, id, committedAt)
	return result, nil
}

// CompleteEnable is the worker-only lifecycle completion boundary.
func (service *CustomerService) CompleteEnable(ctx context.Context, id int64) error {
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockCustomerOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), scope.Customer.UpdatedAt)
		return model.NewCustomerRepository(tx).CompleteEnableInTransaction(ctx, id, committedAt)
	})
	if err != nil {
		return customerServiceError(err)
	}
	service.notifyCustomerLifecycleAfterCommit(ctx, id, committedAt)
	return nil
}

func (service *CustomerService) notifyCustomerLifecycleAfterCommit(ctx context.Context, id int64, observedAt int64) {
	if service.postCommit == nil || observedAt <= 0 {
		return
	}
	service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
		Source: AlertSampleSourceLifecycle, ScopeType: "customer", ScopeID: id, ObservedAt: observedAt,
	})
}

func (service *CustomerService) Statistics(ctx context.Context, id int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	if _, err := model.NewCustomerRepository(service.db).FindByID(ctx, id); err != nil {
		return dto.StatisticsResponse{}, customerServiceError(err)
	}
	if service.statistics == nil {
		return dto.StatisticsResponse{}, ErrEntityStatisticsUnavailable
	}
	query.CustomerIDs = []int64{id}
	return service.statistics.CustomerStatistics(ctx, id, query)
}

type customerAccountCounts struct {
	AccountCount         int `gorm:"column:account_count"`
	ActiveAccountCount   int `gorm:"column:active_account_count"`
	ArchivedAccountCount int `gorm:"column:archived_account_count"`
	SiteCount            int `gorm:"column:site_count"`
}

func (service *CustomerService) listItemFromModel(ctx context.Context, customer model.Customer) (dto.CustomerListItem, error) {
	var counts customerAccountCounts
	err := service.db.WithContext(ctx).Model(&model.Account{}).
		Select(`COUNT(*) AS account_count,
			COALESCE(SUM(CASE WHEN managed_status = 'active' THEN 1 ELSE 0 END), 0) AS active_account_count,
			COALESCE(SUM(CASE WHEN managed_status = 'archived' THEN 1 ELSE 0 END), 0) AS archived_account_count,
			COUNT(DISTINCT site_id) AS site_count`).
		Where("customer_id = ?", customer.ID).Scan(&counts).Error
	if err != nil {
		return dto.CustomerListItem{}, fmt.Errorf("read customer account counts: %w", err)
	}
	backfill, err := service.localBackfillSummary(ctx, "customer", customer.ID, customer.StatisticsBackfillStatus)
	if err != nil {
		return dto.CustomerListItem{}, err
	}
	return dto.CustomerListItem{
		ID: strconv.FormatInt(customer.ID, 10), Name: customer.Name, Contact: customer.Contact,
		Remark: customer.Remark, ContractAmount: customer.ContractAmount, PaymentAmount: customer.PaymentAmount,
		Status: customer.Status, AccountCount: counts.AccountCount,
		ActiveAccountCount: counts.ActiveAccountCount, ArchivedAccountCount: counts.ArchivedAccountCount,
		SiteCount: counts.SiteCount,
		Today:     dto.CustomerUsageSummary{UsageSummary: missingUsageSummary(), SiteBreakdown: []dto.SiteQuotaBreakdown{}},
		Backfill:  backfill, UpdatedAt: customer.UpdatedAt,
	}, nil
}

func (service *CustomerService) localBackfillSummary(ctx context.Context, targetType string, targetID int64, status string) (dto.BackfillSummary, error) {
	var run model.CollectionRun
	err := service.db.WithContext(ctx).Where("target_type = ? AND target_id = ? AND task_type IN ?",
		targetType, targetID, []string{constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild}).
		Order("id DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.BackfillSummary{Status: normalizedBackfillStatus(status)}, nil
	}
	if err != nil {
		return dto.BackfillSummary{}, fmt.Errorf("read local rebuild run: %w", err)
	}
	return backfillSummaryFromRun(run), nil
}

func normalizedBackfillStatus(status string) string {
	if status == "pending" || status == "running" || status == "failed" {
		return status
	}
	return "none"
}

func activeLocalRebuildRun(ctx context.Context, tx *gorm.DB, targetType string, targetID int64, taskType string) (model.CollectionRun, bool, error) {
	var run model.CollectionRun
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("target_type = ? AND target_id = ? AND task_type = ? AND status IN ?",
			targetType, targetID, taskType, []string{"pending", "running"}).Order("id DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.CollectionRun{}, false, nil
	}
	return run, err == nil, err
}

func sameRunRange(run model.CollectionRun, start, end int64) bool {
	return run.StartTimestamp != nil && run.EndTimestamp != nil && *run.StartTimestamp == start && *run.EndTimestamp == end
}

func newLocalRebuildRun(targetType string, targetID int64, taskType string, start, end int64, requestID string, now int64) (model.CollectionRun, error) {
	start = floorHour(start)
	end = floorHour(end)
	if end < start {
		end = start
	}
	scope, err := model.CanonicalCollectionRunScope(taskType, []byte("{}"))
	if err != nil {
		return model.CollectionRun{}, err
	}
	activeKey, err := model.CollectionRunActiveKey(taskType, targetType, targetID, &start, &end)
	if err != nil {
		return model.CollectionRun{}, err
	}
	return model.CollectionRun{
		TaskType: taskType, TargetType: targetType, TargetID: targetID,
		TriggerType: constant.CollectionTriggerDependency, StartTimestamp: &start, EndTimestamp: &end,
		Scope: scope, ActiveKey: &activeKey, Status: "pending", Priority: constant.CollectionPriorityLocalRebuild,
		NextAttemptAt: now, CreatedRequestID: requestID, LastRequestID: requestID, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func customerServiceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case model.IsNotFound(err):
		return ErrCustomerNotFound
	case errors.Is(err, model.ErrDeleteHasDependencies):
		return ErrCustomerDeleteRestricted
	case errors.Is(err, model.ErrCustomerLifecycleContract), errors.Is(err, model.ErrCustomerEnableNotReady):
		return ErrCustomerInvalidState
	default:
		return err
	}
}

func missingUsageSummary() dto.UsageSummary {
	return dto.UsageSummary{DataStatus: "missing"}
}

func entityCompleteness(paused bool, unitType string) dto.Completeness {
	status := "missing"
	if paused {
		status = "paused"
	}
	return dto.Completeness{
		DataStatus: status, UnitType: unitType, MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{},
	}
}

func beijingDateKey(now time.Time) int {
	beijing := now.UTC().Add(8 * time.Hour)
	return beijing.Year()*10000 + int(beijing.Month())*100 + beijing.Day()
}
