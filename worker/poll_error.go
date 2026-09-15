package worker

import (
	"context"
	"errors"
	"log"

	"new-api-pilot/model"
)

// A rolled-back transaction can be retried by the next normal polling tick.
// Do not cancel other in-flight jobs or log database error text here.
func fatalPollError(component string, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if model.IsRetryableTransactionError(err) {
		log.Printf("worker poll deferred component=%s reason=transaction_contention", component)
		return false
	}
	return true
}
