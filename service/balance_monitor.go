package service

import (
	"context"
	"new-api-pilot/dto"
	"time"
)

type balanceRepository interface {
	Ingest(context.Context, []dto.BalanceRecord, int64) error
	List(context.Context, dto.BalanceQuery) (dto.BalancePage, error)
}
type BalanceMonitorService struct{ repository balanceRepository }

func NewBalanceMonitorService(r balanceRepository) *BalanceMonitorService {
	return &BalanceMonitorService{repository: r}
}
func (s *BalanceMonitorService) Ingest(ctx context.Context, v dto.BalanceIngest) error {
	if e := v.Validate(); e != nil {
		return e
	}
	return s.repository.Ingest(ctx, v.Records, time.Now().Unix())
}
func (s *BalanceMonitorService) List(ctx context.Context, q dto.BalanceQuery) (dto.BalancePage, error) {
	if e := q.Validate(); e != nil {
		return dto.BalancePage{}, e
	}
	return s.repository.List(ctx, q)
}
