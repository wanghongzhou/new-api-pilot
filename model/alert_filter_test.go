package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestAlertEventWhereUsesArrayFiltersWithStableArguments(t *testing.T) {
	where, args := alertEventWhere(AlertEventFilter{
		Statuses:    []string{"firing", "resolved"},
		Levels:      []string{"critical", "warning"},
		TargetTypes: []string{"site", "account"},
	})
	for _, clause := range []string{"e.status IN ?", "e.level IN ?", "e.target_type IN ?"} {
		if !strings.Contains(where, clause) {
			t.Fatalf("alert where %q is missing %q", where, clause)
		}
	}
	want := []any{
		[]string{"firing", "resolved"},
		[]string{"critical", "warning"},
		[]string{"site", "account"},
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("alert filter args = %#v, want %#v", args, want)
	}
}
