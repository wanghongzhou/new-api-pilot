package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const runtimeRollbackTimeout = 10 * time.Second

var errRuntimeGroupStopTimeout = errors.New("runtime group stop deadline exceeded")

type runtimeGroup struct {
	components []runtimeLifecycle
}

func newRuntimeGroup(components ...runtimeLifecycle) (*runtimeGroup, error) {
	for _, component := range components {
		if component == nil {
			return nil, errors.New("runtime group component is required")
		}
	}
	if len(components) == 0 {
		return nil, errors.New("runtime group requires at least one component")
	}
	return &runtimeGroup{components: append([]runtimeLifecycle(nil), components...)}, nil
}

func (group *runtimeGroup) Start(ctx context.Context) error {
	started := make([]runtimeLifecycle, 0, len(group.components))
	for index, component := range group.components {
		if err := component.Start(ctx); err != nil {
			return errors.Join(fmt.Errorf("start runtime component %d: %w", index, err), rollbackRuntimeComponents(started))
		}
		started = append(started, component)
		if !component.Ready() {
			return errors.Join(fmt.Errorf("runtime component %d did not become ready", index), rollbackRuntimeComponents(started))
		}
	}
	return nil
}

func rollbackRuntimeComponents(components []runtimeLifecycle) error {
	ctx, cancel := context.WithTimeout(context.Background(), runtimeRollbackTimeout)
	defer cancel()
	var result error
	for index := len(components) - 1; index >= 0; index-- {
		result = errors.Join(result, components[index].Quiesce())
	}
	for index := len(components) - 1; index >= 0; index-- {
		result = errors.Join(result, components[index].Stop(ctx))
	}
	return result
}

func (group *runtimeGroup) Quiesce() error {
	if group == nil {
		return nil
	}
	var result error
	for index := len(group.components) - 1; index >= 0; index-- {
		result = errors.Join(result, group.components[index].Quiesce())
	}
	return result
}

func (group *runtimeGroup) Stop(ctx context.Context) error {
	if group == nil {
		return nil
	}
	if ctx == nil {
		return errRuntimeGroupStopTimeout
	}
	results := make(chan error, len(group.components))
	var result error
	for index := len(group.components) - 1; index >= 0; index-- {
		component := group.components[index]
		go func() {
			results <- component.Stop(ctx)
		}()
	}
	for range group.components {
		select {
		case err := <-results:
			result = errors.Join(result, err)
		case <-ctx.Done():
			return errors.Join(result, errRuntimeGroupStopTimeout, ctx.Err())
		}
	}
	return result
}

func (group *runtimeGroup) Ready() bool {
	if group == nil || len(group.components) == 0 {
		return false
	}
	for _, component := range group.components {
		if !component.Ready() {
			return false
		}
	}
	return true
}
