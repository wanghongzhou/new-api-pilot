package common

import (
	"context"
	"time"
)

// FinalizationContext preserves request values while allowing durable state
// finalization to outlive parent cancellation for a strictly bounded period.
func FinalizationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
