package common

import (
	"context"
	"errors"
	"testing"
	"time"
)

type finalizationContextKey struct{}

func TestFinalizationContextDetachesCancellationAndRemainsBounded(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), finalizationContextKey{}, "request-value"))
	cancelParent()

	ctx, cancel := FinalizationContext(parent, 20*time.Millisecond)
	defer cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("finalization context inherited parent cancellation: %v", err)
	}
	if value := ctx.Value(finalizationContextKey{}); value != "request-value" {
		t.Fatalf("finalization context value = %v", value)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("finalization context did not enforce its deadline")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("finalization context error = %v", ctx.Err())
	}
}
