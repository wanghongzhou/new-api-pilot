package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingAlertEvaluator struct {
	seen []AlertEvaluation
}

func (evaluator *recordingAlertEvaluator) Evaluate(
	_ context.Context,
	evaluation AlertEvaluation,
) (AlertEvaluationResult, error) {
	evaluator.seen = append(evaluator.seen, evaluation)
	return AlertEvaluationResult{Transition: "unchanged"}, nil
}

func TestAlertSampleIdentityCanonicalizesAggregateEvidence(t *testing.T) {
	first, err := BuildResourceAlertSampleIdentity(7, "node-a", 100, "row:2", "row:1", "row:1")
	if err != nil {
		t.Fatalf("build first identity: %v", err)
	}
	second, err := BuildResourceAlertSampleIdentity(7, "node-a", 100, "row:1", "row:2")
	if err != nil {
		t.Fatalf("build second identity: %v", err)
	}
	changed, err := BuildResourceAlertSampleIdentity(7, "node-a", 100, "row:1", "row:3")
	if err != nil {
		t.Fatalf("build changed identity: %v", err)
	}
	if first != second || first.SampleKey == changed.SampleKey || first.ObservedAt != 100 {
		t.Fatalf("canonical identities first=%#v second=%#v changed=%#v", first, second, changed)
	}
	if len(first.SampleKey) > 255 {
		t.Fatalf("sample key length = %d", len(first.SampleKey))
	}
}

func TestAlertSampleIdentityBuildersRejectInvalidScopes(t *testing.T) {
	tests := []func() error{
		func() error { _, err := BuildProbeAlertSampleIdentity(0, 1, "row:1"); return err },
		func() error { _, err := BuildResourceAlertSampleIdentity(1, "", 1, "row:1"); return err },
		func() error { _, err := BuildUserAlertSampleIdentity(1, 0, 1, "row:1"); return err },
		func() error { _, err := BuildWindowAlertSampleIdentity(1, "", 1, "row:1"); return err },
		func() error { _, err := BuildAuthAlertSampleIdentity(0, 1, "row:1"); return err },
		func() error { _, err := BuildLifecycleAlertSampleIdentity("", 1, 1, "row:1"); return err },
	}
	for index, test := range tests {
		if err := test(); err == nil {
			t.Errorf("invalid identity builder case %d succeeded", index)
		}
	}
}

func TestAlertPostCommitBatchIsChronologicalAndDeduplicated(t *testing.T) {
	siteID := int64(7)
	value := "90"
	identity20, _ := BuildResourceAlertSampleIdentity(siteID, "node-a", 20, "row:20")
	identity100, _ := BuildResourceAlertSampleIdentity(siteID, "node-a", 100, "row:100")
	base := AlertEvaluation{
		RuleKey: "cpu_high", SiteID: &siteID, TargetType: "instance", TargetKey: "7/node-a",
		TargetName: "node-a", State: AlertSampleKnown, CurrentValue: &value,
	}
	at20 := base
	at20.ObservedAt, at20.SampleKey = identity20.ObservedAt, identity20.SampleKey
	at100 := base
	at100.ObservedAt, at100.SampleKey = identity100.ObservedAt, identity100.SampleKey
	evaluator := &recordingAlertEvaluator{}
	hook, err := NewAlertPostCommitHook(evaluator)
	if err != nil {
		t.Fatalf("create post-commit hook: %v", err)
	}
	results, err := hook.EvaluateBatchAfterCommit(context.Background(), []AlertEvaluation{at100, at20, at20})
	if err != nil {
		t.Fatalf("evaluate post-commit batch: %v", err)
	}
	if len(results) != 2 || len(evaluator.seen) != 2 ||
		!reflect.DeepEqual([]int64{evaluator.seen[0].ObservedAt, evaluator.seen[1].ObservedAt}, []int64{20, 100}) {
		t.Fatalf("post-commit evaluation order = %#v results=%#v", evaluator.seen, results)
	}
}

func TestAlertPostCommitBatchRejectsSameTimeConflictBeforeEvaluation(t *testing.T) {
	siteID := int64(7)
	value := "90"
	firstIdentity, _ := BuildResourceAlertSampleIdentity(siteID, "node-a", 100, "row:1")
	secondIdentity, _ := BuildResourceAlertSampleIdentity(siteID, "node-a", 100, "row:2")
	first := AlertEvaluation{
		RuleKey: "cpu_high", SiteID: &siteID, TargetType: "instance", TargetKey: "7/node-a",
		TargetName: "node-a", State: AlertSampleKnown, CurrentValue: &value,
		ObservedAt: firstIdentity.ObservedAt, SampleKey: firstIdentity.SampleKey,
	}
	second := first
	second.SampleKey = secondIdentity.SampleKey
	evaluator := &recordingAlertEvaluator{}
	hook, err := NewAlertPostCommitHook(evaluator)
	if err != nil {
		t.Fatalf("create post-commit hook: %v", err)
	}
	if _, err := hook.EvaluateBatchAfterCommit(context.Background(), []AlertEvaluation{first, second}); !errors.Is(err, ErrAlertSampleConflict) {
		t.Fatalf("same-time conflict error = %v", err)
	}
	if len(evaluator.seen) != 0 {
		t.Fatalf("conflicting batch changed state: %#v", evaluator.seen)
	}
}
