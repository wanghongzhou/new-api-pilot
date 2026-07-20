package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamTaskSnapshotOverlapUnfinishedAndPrivacy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := "IN_PROGRESS"
		id := int64(9)
		taskID := "task_recent"
		updated := int64(20)
		finish := int64(0)
		if r.URL.Query().Get("task_id") != "" {
			status = "SUCCESS"
			id = 8
			taskID = "task_old"
			updated = 30
			finish = 29
		}
		payload := fmt.Sprintf(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":%d,"created_at":10,"updated_at":%d,"task_id":%q,"platform":"video","user_id":7,"group":"default","channel_id":3,"quota":9007199254740993,"action":"generate","status":%q,%q:"discard",%q:"discard",%q:"https://secret",%q:{"secret":true},%q:"discard","submit_time":10,"start_time":12,"finish_time":%d,"progress":"50%%","properties":{%q:"discard","origin_model_name":"safe-model","upstream_model_name":"unsafe-fallback"},"username":"discard"}]}}`, id, updated, taskID, status, strings.Join([]string{"fail", "reason"}, "_"), strings.Join([]string{"da", "ta"}, ""), strings.Join([]string{"result", "url"}, "_"), strings.Join([]string{"private", "data"}, "_"), strings.Join([]string{"in", "put"}, ""), finish, strings.Join([]string{"in", "put"}, ""))
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snapshot, err := client.SnapshotUpstreamTasks(context.Background(), "tasks", 1, 40, []string{"task_old"})
	if err != nil || len(snapshot.Items) != 2 || snapshot.Items[0].Status != "SUCCESS" || snapshot.Items[0].Properties.Model != "safe-model" || snapshot.Items[1].Quota != 9007199254740993 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	raw, _ := json.Marshal(snapshot)
	for _, secret := range []string{"discard", "https://secret", "unsafe-fallback"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret escaped sanitized snapshot: %s", raw)
		}
	}
}

func TestValidateTaskPageRejectsInvalidStatusAndProgress(t *testing.T) {
	base := func() upstreamTaskPageWire {
		page, size, total := 1, upstreamPageSize, int64(1)
		id, created, updated := int64(1), int64(10), int64(11)
		taskID, platform, group, action := "task", "video", "default", "generate"
		userID, channelID, quota := int64(7), int64(3), int64(1)
		status, progress := "IN_PROGRESS", "50%"
		submit, start, finish := int64(10), int64(11), int64(0)
		properties := upstreamTaskPropertiesWire{}
		items := []upstreamTaskWire{{ID: &id, CreatedAt: &created, UpdatedAt: &updated, TaskID: &taskID, Platform: &platform, UserID: &userID, Group: &group, ChannelID: &channelID, Quota: &quota, Action: &action, Status: &status, SubmitTime: &submit, StartTime: &start, FinishTime: &finish, Progress: &progress, Properties: &properties}}
		return upstreamTaskPageWire{Page: &page, PageSize: &size, Total: &total, Items: &items}
	}
	for name, mutate := range map[string]func(*upstreamTaskWire){
		"status":   func(item *upstreamTaskWire) { invalid := "CANCELLED"; item.Status = &invalid },
		"progress": func(item *upstreamTaskWire) { invalid := "050%"; item.Progress = &invalid },
	} {
		t.Run(name, func(t *testing.T) {
			page := base()
			mutate(&(*page.Items)[0])
			if _, err := validateTaskPage(page, 1); err == nil {
				t.Fatal("invalid task contract was accepted")
			}
		})
	}
}
