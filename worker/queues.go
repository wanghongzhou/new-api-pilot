package worker

import (
	"sync"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type QueueKind string

const (
	QueueProbe           QueueKind = "probe"
	QueueRealtime        QueueKind = "realtime"
	QueueResource        QueueKind = "resource"
	QueuePerformance     QueueKind = "performance"
	QueueTopup           QueueKind = "topup"
	QueueRedemption      QueueKind = "redemption"
	QueueMetadata        QueueKind = "metadata"
	QueueUsage           QueueKind = "usage"
	QueueBackfill        QueueKind = "backfill"
	QueueValidation      QueueKind = "validation"
	QueueAccountRebuild  QueueKind = "account_rebuild"
	QueueCustomerRebuild QueueKind = "customer_rebuild"
)

var independentQueueOrder = []QueueKind{
	QueueProbe, QueueRealtime, QueueResource, QueuePerformance, QueueTopup, QueueRedemption, QueueMetadata,
}

func sharedCollectionQueue(queue QueueKind) bool {
	return queue == QueueUsage || queue == QueueBackfill || queue == QueueValidation || queue == QueueAccountRebuild || queue == QueueCustomerRebuild
}

func queueCapacityKey(queue QueueKind) QueueKind {
	switch queue {
	case QueueValidation, QueueAccountRebuild, QueueCustomerRebuild:
		return QueueBackfill
	default:
		return queue
	}
}

func QueueForTask(taskType string) (QueueKind, bool) {
	switch taskType {
	case constant.TaskTypeSiteProbe:
		return QueueProbe, true
	case constant.TaskTypeRealtimeStat:
		return QueueRealtime, true
	case constant.TaskTypeResourceSnapshot:
		return QueueResource, true
	case constant.TaskTypePerformanceSync:
		return QueuePerformance, true
	case constant.TaskTypeTopupSync:
		return QueueTopup, true
	case constant.TaskTypeRedemptionSync:
		return QueueRedemption, true
	case constant.TaskTypeUserSync, constant.TaskTypeChannelSync, constant.TaskTypeUpstreamTaskSync, constant.TaskTypeModelMetaSync, constant.TaskTypePlanSync, constant.TaskTypePricingSync, constant.TaskTypeSystemTaskSync, constant.TaskTypeLogSync:
		return QueueMetadata, true
	case constant.TaskTypeUsageHour:
		return QueueUsage, true
	case constant.TaskTypeUsageBackfill:
		return QueueBackfill, true
	case constant.TaskTypeUsageValidation:
		return QueueValidation, true
	case constant.TaskTypeAccountRebuild:
		return QueueAccountRebuild, true
	case constant.TaskTypeCustomerRebuild:
		return QueueCustomerRebuild, true
	default:
		return "", false
	}
}

func queueConcurrency(queue QueueKind, settings model.CollectorSettings) int {
	switch queue {
	case QueueProbe:
		return settings.ProbeConcurrency
	case QueueRealtime:
		return settings.RealtimeConcurrency
	case QueueResource:
		return settings.ResourceConcurrency
	case QueuePerformance, QueueTopup, QueueRedemption:
		return settings.ResourceConcurrency
	case QueueMetadata:
		return settings.MetadataConcurrency
	case QueueUsage:
		return settings.UsageConcurrency
	case QueueBackfill, QueueValidation, QueueAccountRebuild, QueueCustomerRebuild:
		return settings.BackfillConcurrency
	default:
		return 0
	}
}

type queueLimiter struct {
	mu     sync.Mutex
	active map[QueueKind]int
}

func newQueueLimiter() *queueLimiter {
	return &queueLimiter{active: make(map[QueueKind]int)}
}

func (limiter *queueLimiter) tryAcquire(queue QueueKind, maximum int) bool {
	if maximum <= 0 {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.active[queue] >= maximum {
		return false
	}
	limiter.active[queue]++
	return true
}

func (limiter *queueLimiter) release(queue QueueKind) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.active[queue] <= 1 {
		delete(limiter.active, queue)
		return
	}
	limiter.active[queue]--
}

func (limiter *queueLimiter) count(queue QueueKind) int {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	return limiter.active[queue]
}
