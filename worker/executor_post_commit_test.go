package worker

import (
	"context"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

type recordingPostCommitNotifier struct {
	triggers []service.AlertPostCommitTrigger
}

func (notifier *recordingPostCommitNotifier) NotifyAfterCommit(_ context.Context, trigger service.AlertPostCommitTrigger) {
	notifier.triggers = append(notifier.triggers, trigger)
}

func TestLocalRebuildCompletionNotifiesLifecycleScope(t *testing.T) {
	testCases := []struct {
		taskType  string
		scopeType string
	}{
		{taskType: constant.TaskTypeAccountRebuild, scopeType: "account"},
		{taskType: constant.TaskTypeCustomerRebuild, scopeType: "customer"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.scopeType, func(t *testing.T) {
			notifier := &recordingPostCommitNotifier{}
			executor := &Executor{postCommit: notifier}
			claim := model.CollectionTaskClaim{Run: model.CollectionRun{
				TaskType: testCase.taskType, TargetID: 42,
			}}
			executor.notifyWindowAfterCommit(
				context.Background(), claim, model.CollectionRunWindow{},
				model.CollectionRun{Status: model.CollectionTaskStatusSuccess}, 101, false,
			)
			if len(notifier.triggers) != 1 {
				t.Fatalf("lifecycle triggers = %#v", notifier.triggers)
			}
			trigger := notifier.triggers[0]
			if trigger.Source != service.AlertSampleSourceLifecycle || trigger.ScopeType != testCase.scopeType ||
				trigger.ScopeID != 42 || trigger.ObservedAt != 101 {
				t.Fatalf("lifecycle trigger = %#v", trigger)
			}
		})
	}
}

type contextRecordingPostCommitNotifier struct {
	err       error
	deadline  time.Time
	hasExpiry bool
}

func (notifier *contextRecordingPostCommitNotifier) NotifyAfterCommit(ctx context.Context, _ service.AlertPostCommitTrigger) {
	notifier.err = ctx.Err()
	notifier.deadline, notifier.hasExpiry = ctx.Deadline()
}

func TestPostCommitFinalizationDetachesCancellationAndHasDeadline(t *testing.T) {
	notifier := &contextRecordingPostCommitNotifier{}
	executor := &Executor{postCommit: notifier}
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	executor.notifyWindowAfterCommit(
		parent,
		model.CollectionTaskClaim{Run: model.CollectionRun{TaskType: constant.TaskTypeAccountRebuild, TargetID: 42}},
		model.CollectionRunWindow{},
		model.CollectionRun{Status: model.CollectionTaskStatusSuccess},
		101,
		false,
	)
	if notifier.err != nil {
		t.Fatalf("post-commit context inherited cancellation: %v", notifier.err)
	}
	if !notifier.hasExpiry {
		t.Fatal("post-commit context has no deadline")
	}
	remaining := time.Until(notifier.deadline)
	if remaining <= 0 || remaining > executorFinalizationTimeout {
		t.Fatalf("post-commit deadline remaining = %s", remaining)
	}
}
