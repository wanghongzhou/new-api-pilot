package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemTaskSnapshotListCurrentTypedPrivacyAndPartial(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.RequestURI())
		if r.Header.Get("Authorization") != "test-root-token" || r.Header.Get("New-Api-User") != "1" {
			t.Errorf("missing management headers")
		}
		switch r.URL.Path {
		case "/api/system-task/list":
			fmt.Fprint(w, `{"success":true,"message":"","data":[{"id":3,"task_id":"systask_channel","type":"channel_test","status":"succeeded","active_key":"secret","payload":{"mode":"scheduled_all","notify":true,"secret":"discard"},"state":{"total":5,"processed":5,"progress":100,"remaining":0,"private":"discard"},"result":{"tested":5,"succeeded":4,"failed":1,"disabled":1,"enabled":4,"private":"discard"},"error":"raw secret error","locked_by":"runner-secret","created_at":10,"updated_at":20},{"id":1,"task_id":"systask_async","type":"async_task_poll","status":"succeeded","payload":null,"state":null,"result":{"unfinished_tasks":2,"platforms_scanned":1,"null_tasks_failed":0},"error":"","created_at":1,"updated_at":2}]}`)
		case "/api/system-task/current":
			if r.URL.Query().Get("type") == "log_cleanup" {
				fmt.Fprint(w, `{"success":true,"message":"","data":{"id":4,"task_id":"systask_cleanup","type":"log_cleanup","status":"running","active_key":"secret","payload":{"target_timestamp":1,"batch_size":1000},"state":{"total":10,"processed":3,"progress":30,"remaining":7},"result":null,"error":"","locked_by":"runner","created_at":21,"updated_at":22}}`)
			} else {
				fmt.Fprint(w, `{"success":true,"message":"","data":null}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snapshot, err := client.SnapshotSystemTasks(context.Background(), "system-tasks")
	if err != nil {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
	if !snapshot.Partial || snapshot.Truncated || !snapshot.IDGap || len(snapshot.Items) != 3 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if snapshot.Items[0].TaskID != "systask_cleanup" || snapshot.Items[1].ErrorCode != "UPSTREAM_SYSTEM_TASK_FAILED" || snapshot.Items[1].Tested == nil || *snapshot.Items[1].Tested != 5 || snapshot.Items[2].PlatformsScanned == nil {
		t.Fatalf("items=%#v", snapshot.Items)
	}
	if len(calls) != 6 || calls[0] != "/api/system-task/list?limit=100" {
		t.Fatalf("calls=%#v", calls)
	}
	for _, call := range calls {
		if strings.Contains(call, "systask_") {
			t.Fatalf("per-task N+1 call: %s", call)
		}
	}
}

func TestSystemTaskSnapshotRejectsUnknownTypeAndRawShape(t *testing.T) {
	for _, body := range []string{`{"success":true,"message":"","data":[{"id":1,"task_id":"x","type":"unknown","status":"running","payload":null,"state":null,"result":null,"error":"","created_at":1,"updated_at":1}]}`, `{"success":true,"message":"","data":[{"id":1,"task_id":"x","type":"channel_test","status":"running","payload":null,"state":"raw","result":null,"error":"","created_at":1,"updated_at":1}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/system-task/list" {
				fmt.Fprint(w, body)
			} else {
				fmt.Fprint(w, `{"success":true,"message":"","data":null}`)
			}
		}))
		client := testClientForServer(t, server, true, testClientSettings{})
		_, err := client.SnapshotSystemTasks(context.Background(), "invalid-system-task")
		server.Close()
		if err == nil {
			t.Fatalf("invalid body accepted: %s", body)
		}
	}
}

func TestSystemTaskCurrentFailureProducesPartialWithoutPerTaskLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system-task/list" {
			fmt.Fprint(w, `{"success":true,"message":"","data":[]}`)
			return
		}
		if r.URL.Query().Get("type") == "model_update" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"success":true,"message":"","data":null}`)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snapshot, err := client.SnapshotSystemTasks(context.Background(), "current-failure")
	if err != nil || !snapshot.Partial || len(snapshot.CurrentFailures) != 1 || snapshot.CurrentFailures[0] != "model_update" {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
}
