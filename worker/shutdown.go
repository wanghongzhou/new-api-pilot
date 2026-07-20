package worker

import (
	"context"
	"errors"
	"time"
)

const runtimeForceJoinMax = 5 * time.Second

var ErrRuntimeStopTimeout = errors.New("runtime stop deadline exceeded")

func awaitRuntimeStop(
	ctx context.Context,
	done <-chan struct{},
	cancelExecution context.CancelFunc,
	drain bool,
	loadRunError func() error,
) error {
	if ctx == nil {
		cancelExecution()
		return ErrRuntimeStopTimeout
	}
	if drain {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			reserve := remaining / 5
			if reserve > runtimeForceJoinMax {
				reserve = runtimeForceJoinMax
			}
			drainFor := remaining - reserve
			if drainFor > 0 {
				timer := time.NewTimer(drainFor)
				select {
				case <-done:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return loadRunError()
				case <-timer.C:
					cancelExecution()
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					cancelExecution()
					return errors.Join(ErrRuntimeStopTimeout, ctx.Err())
				}
			} else {
				cancelExecution()
			}
		} else {
			select {
			case <-done:
				return loadRunError()
			case <-ctx.Done():
				cancelExecution()
				return errors.Join(ErrRuntimeStopTimeout, ctx.Err())
			}
		}
	} else {
		cancelExecution()
	}

	select {
	case <-done:
		return loadRunError()
	case <-ctx.Done():
		cancelExecution()
		return errors.Join(ErrRuntimeStopTimeout, ctx.Err())
	}
}
