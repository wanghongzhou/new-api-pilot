package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type AlertSampleSource string

const (
	AlertSampleSourceProbe     AlertSampleSource = "probe"
	AlertSampleSourceResource  AlertSampleSource = "resource"
	AlertSampleSourceUser      AlertSampleSource = "user"
	AlertSampleSourceWindow    AlertSampleSource = "window"
	AlertSampleSourceChannel   AlertSampleSource = "channel"
	AlertSampleSourceAuth      AlertSampleSource = "auth"
	AlertSampleSourceLifecycle AlertSampleSource = "lifecycle"
)

type AlertSampleIdentity struct {
	ObservedAt int64
	SampleKey  string
}

func BuildAlertSampleIdentity(
	source AlertSampleSource,
	observedAt int64,
	aggregateKeys ...string,
) (AlertSampleIdentity, error) {
	if observedAt <= 0 || !alertOneOfService(
		string(source),
		string(AlertSampleSourceProbe),
		string(AlertSampleSourceResource),
		string(AlertSampleSourceUser),
		string(AlertSampleSourceWindow),
		string(AlertSampleSourceChannel),
		string(AlertSampleSourceAuth),
		string(AlertSampleSourceLifecycle),
	) {
		return AlertSampleIdentity{}, errors.New("invalid alert sample identity source")
	}
	canonical := make([]string, 0, len(aggregateKeys))
	for _, key := range aggregateKeys {
		if key == "" || len(key) > 2048 || strings.ContainsAny(key, "\x00\r\n") {
			return AlertSampleIdentity{}, errors.New("invalid alert sample aggregate key")
		}
		canonical = append(canonical, key)
	}
	if len(canonical) == 0 {
		return AlertSampleIdentity{}, errors.New("alert sample aggregate key is required")
	}
	sort.Strings(canonical)
	unique := canonical[:0]
	for _, key := range canonical {
		if len(unique) == 0 || unique[len(unique)-1] != key {
			unique = append(unique, key)
		}
	}
	digest := sha256.New()
	writeAlertIdentityPart(digest.Write, string(source))
	for _, key := range unique {
		writeAlertIdentityPart(digest.Write, key)
	}
	return AlertSampleIdentity{
		ObservedAt: observedAt,
		SampleKey:  "v1:" + string(source) + ":" + hex.EncodeToString(digest.Sum(nil)),
	}, nil
}

func BuildChannelAlertSampleIdentity(
	siteID int64,
	hourTS int64,
	observedAt int64,
	evidenceKeys ...string,
) (AlertSampleIdentity, error) {
	if siteID <= 0 || hourTS <= 0 {
		return AlertSampleIdentity{}, errors.New("invalid channel alert sample target")
	}
	return buildScopedAlertSampleIdentity(
		AlertSampleSourceChannel,
		observedAt,
		[]string{"site:" + strconv.FormatInt(siteID, 10), "hour:" + strconv.FormatInt(hourTS, 10)},
		evidenceKeys,
	)
}

func BuildProbeAlertSampleIdentity(siteID, observedAt int64, evidenceKeys ...string) (AlertSampleIdentity, error) {
	if siteID <= 0 {
		return AlertSampleIdentity{}, errors.New("invalid probe alert sample site")
	}
	return buildScopedAlertSampleIdentity(AlertSampleSourceProbe, observedAt, []string{"site:" + strconv.FormatInt(siteID, 10)}, evidenceKeys)
}

func BuildResourceAlertSampleIdentity(
	siteID int64,
	targetKey string,
	observedAt int64,
	evidenceKeys ...string,
) (AlertSampleIdentity, error) {
	if siteID <= 0 || targetKey == "" {
		return AlertSampleIdentity{}, errors.New("invalid resource alert sample target")
	}
	return buildScopedAlertSampleIdentity(
		AlertSampleSourceResource,
		observedAt,
		[]string{"site:" + strconv.FormatInt(siteID, 10), "target:" + targetKey},
		evidenceKeys,
	)
}

func BuildUserAlertSampleIdentity(
	siteID int64,
	accountID int64,
	observedAt int64,
	evidenceKeys ...string,
) (AlertSampleIdentity, error) {
	if siteID <= 0 || accountID <= 0 {
		return AlertSampleIdentity{}, errors.New("invalid user alert sample target")
	}
	return buildScopedAlertSampleIdentity(
		AlertSampleSourceUser,
		observedAt,
		[]string{"site:" + strconv.FormatInt(siteID, 10), "account:" + strconv.FormatInt(accountID, 10)},
		evidenceKeys,
	)
}

func BuildWindowAlertSampleIdentity(
	siteID int64,
	windowKey string,
	observedAt int64,
	evidenceKeys ...string,
) (AlertSampleIdentity, error) {
	if siteID <= 0 || windowKey == "" {
		return AlertSampleIdentity{}, errors.New("invalid window alert sample target")
	}
	return buildScopedAlertSampleIdentity(
		AlertSampleSourceWindow,
		observedAt,
		[]string{"site:" + strconv.FormatInt(siteID, 10), "window:" + windowKey},
		evidenceKeys,
	)
}

func BuildAuthAlertSampleIdentity(siteID, observedAt int64, evidenceKeys ...string) (AlertSampleIdentity, error) {
	if siteID <= 0 {
		return AlertSampleIdentity{}, errors.New("invalid auth alert sample site")
	}
	return buildScopedAlertSampleIdentity(AlertSampleSourceAuth, observedAt, []string{"site:" + strconv.FormatInt(siteID, 10)}, evidenceKeys)
}

func BuildLifecycleAlertSampleIdentity(
	scopeType string,
	scopeID int64,
	observedAt int64,
	evidenceKeys ...string,
) (AlertSampleIdentity, error) {
	if scopeID <= 0 || strings.TrimSpace(scopeType) == "" {
		return AlertSampleIdentity{}, errors.New("invalid lifecycle alert sample scope")
	}
	return buildScopedAlertSampleIdentity(
		AlertSampleSourceLifecycle,
		observedAt,
		[]string{"scope_type:" + scopeType, "scope_id:" + strconv.FormatInt(scopeID, 10)},
		evidenceKeys,
	)
}

func buildScopedAlertSampleIdentity(
	source AlertSampleSource,
	observedAt int64,
	dimensions []string,
	evidenceKeys []string,
) (AlertSampleIdentity, error) {
	for _, dimension := range dimensions {
		parts := strings.SplitN(dimension, ":", 2)
		if len(parts) != 2 || parts[1] == "" || parts[1] == "0" {
			return AlertSampleIdentity{}, errors.New("invalid alert sample identity scope")
		}
	}
	keys := append(append([]string(nil), dimensions...), evidenceKeys...)
	return BuildAlertSampleIdentity(source, observedAt, keys...)
}

func writeAlertIdentityPart(write func([]byte) (int, error), value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = write(size[:])
	_, _ = write([]byte(value))
}

type AlertPostCommitHook struct {
	evaluator AlertEvaluator
}

func NewAlertPostCommitHook(evaluator AlertEvaluator) (*AlertPostCommitHook, error) {
	if evaluator == nil {
		return nil, errors.New("alert post-commit evaluator is required")
	}
	return &AlertPostCommitHook{evaluator: evaluator}, nil
}

// EvaluateAfterCommit must be called only after the source transaction commits.
func (hook *AlertPostCommitHook) EvaluateAfterCommit(
	ctx context.Context,
	evaluation AlertEvaluation,
) (AlertEvaluationResult, error) {
	if hook == nil || hook.evaluator == nil {
		return AlertEvaluationResult{}, errors.New("alert post-commit hook is required")
	}
	return hook.evaluator.Evaluate(ctx, evaluation)
}

// EvaluateBatchAfterCommit rejects ambiguous same-target samples before changing alert state.
func (hook *AlertPostCommitHook) EvaluateBatchAfterCommit(
	ctx context.Context,
	evaluations []AlertEvaluation,
) ([]AlertEvaluationResult, error) {
	if hook == nil || hook.evaluator == nil {
		return nil, errors.New("alert post-commit hook is required")
	}
	canonical := append([]AlertEvaluation(nil), evaluations...)
	for index := range canonical {
		canonical[index].RuleKey = strings.TrimSpace(canonical[index].RuleKey)
		canonical[index].TargetType = strings.ToLower(strings.TrimSpace(canonical[index].TargetType))
		canonical[index].TargetKey = strings.TrimSpace(canonical[index].TargetKey)
		if fields := validateAlertEvaluation(canonical[index]); fields != nil {
			return nil, &AlertValidationError{Fields: fields}
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		leftGroup := alertPostCommitTargetKey(canonical[left])
		rightGroup := alertPostCommitTargetKey(canonical[right])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		if canonical[left].ObservedAt != canonical[right].ObservedAt {
			return canonical[left].ObservedAt < canonical[right].ObservedAt
		}
		return canonical[left].SampleKey < canonical[right].SampleKey
	})
	deduplicated := canonical[:0]
	for _, evaluation := range canonical {
		if len(deduplicated) == 0 {
			deduplicated = append(deduplicated, evaluation)
			continue
		}
		previous := deduplicated[len(deduplicated)-1]
		if alertPostCommitGroupKey(previous) != alertPostCommitGroupKey(evaluation) {
			deduplicated = append(deduplicated, evaluation)
			continue
		}
		if previous.SampleKey != evaluation.SampleKey || !reflect.DeepEqual(previous, evaluation) {
			return nil, fmt.Errorf(
				"%w: target=%s observed_at=%d",
				ErrAlertSampleConflict,
				alertPostCommitGroupKey(evaluation),
				evaluation.ObservedAt,
			)
		}
	}
	results := make([]AlertEvaluationResult, 0, len(deduplicated))
	for _, evaluation := range deduplicated {
		result, err := hook.evaluator.Evaluate(ctx, evaluation)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func alertPostCommitGroupKey(evaluation AlertEvaluation) string {
	return alertPostCommitTargetKey(evaluation) + "\x00" +
		strconv.FormatInt(evaluation.ObservedAt, 10)
}

func alertPostCommitTargetKey(evaluation AlertEvaluation) string {
	return evaluation.RuleKey + "\x00" + evaluation.TargetType + "\x00" + evaluation.TargetKey
}
