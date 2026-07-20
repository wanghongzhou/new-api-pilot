package worker

import (
	"context"
	"testing"

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
				claim, model.CollectionRunWindow{},
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
