package router

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestAlertRoutesViewerAdminStrictContractAndDeepLink(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	now := int64(1_752_401_200)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	alerts, err := service.NewAlertService(service.AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	alertController := controller.NewAlertController(alerts)
	viewer := newAlertRoleEngine(t, alertController, constant.RoleViewer)
	admin := newAlertRoleEngine(t, alertController, constant.RoleAdmin)

	ruleKey := "b6_http_" + strconv.FormatInt(now, 10)
	result := tx.Exec(`INSERT INTO alert_rule
(rule_key, name, enabled, level, metric, compare_operator, threshold_value, for_times, scope_type, scope_id, created_at, updated_at)
VALUES (?, 'B6 HTTP', 1, 'warning', 'instance.cpu_percent', '>=', 85, 3, 'global', 0, ?, ?)`, ruleKey, now, now)
	if result.Error != nil {
		t.Fatalf("create HTTP alert rule: %v", result.Error)
	}
	var ruleID int64
	if err := tx.Raw("SELECT id FROM alert_rule WHERE rule_key = ?", ruleKey).Scan(&ruleID).Error; err != nil || ruleID <= 0 {
		t.Fatalf("read rule ID = %d, %v", ruleID, err)
	}

	params := `{"site_id":"1","target_type":"account","target_name":"HTTP Account","value":"90","threshold":"85"}`
	value, threshold := "90", "85"
	activeKey := "b6-http-active-" + strconv.FormatInt(now, 10)
	event := model.AlertEvent{
		RuleID: ruleID, RuleKey: ruleKey, TargetType: "account", TargetKey: "9007199254740993", ActiveKey: &activeKey,
		Level: dto.AlertLevelWarning, Status: dto.AlertStatusFiring, ConsecutiveCount: 3,
		CurrentValue: &value, ThresholdValue: &threshold, MessageCode: string(constant.MessageAlertCPUHigh),
		MessageParams: &params, Message: "request_id=req-http", FirstObservedAt: now - 180,
		FirstFiredAt: &now, LastFiredAt: &now, CreatedAt: now - 180, UpdatedAt: now,
	}
	if err := tx.Create(&event).Error; err != nil {
		t.Fatalf("create HTTP alert event: %v", err)
	}

	readRules := performSiteRequest(viewer, http.MethodGet, "/api/alert-rules?scope_type=global", "")
	readRulesEnvelope := decodeSiteEnvelope(t, readRules)
	if readRules.Code != http.StatusOK || !readRulesEnvelope.Success {
		t.Fatalf("viewer rules = %d %#v body=%s", readRules.Code, readRulesEnvelope, readRules.Body.String())
	}
	var rules []map[string]any
	if err := json.Unmarshal(readRulesEnvelope.Data, &rules); err != nil {
		t.Fatalf("decode rules: %v", err)
	}
	var foundRule bool
	for _, rule := range rules {
		if rule["id"] == strconv.FormatInt(ruleID, 10) {
			foundRule = true
			if _, ok := rule["threshold_value"].(string); !ok {
				t.Fatalf("threshold is not a string: %#v", rule)
			}
		}
	}
	if !foundRule {
		t.Fatalf("HTTP rule not returned: %#v", rules)
	}

	forbidden := performSiteRequest(viewer, http.MethodPut, "/api/alert-rules/"+strconv.FormatInt(ruleID, 10), `{"threshold_value":"90"}`)
	forbiddenEnvelope := decodeSiteEnvelope(t, forbidden)
	if forbidden.Code != http.StatusForbidden || forbiddenEnvelope.Code != constant.CodeForbidden {
		t.Fatalf("viewer update = %d %#v", forbidden.Code, forbiddenEnvelope)
	}

	updated := performSiteRequest(admin, http.MethodPut, "/api/alert-rules/"+strconv.FormatInt(ruleID, 10), `{"threshold_value":"90","for_times":4}`)
	updatedEnvelope := decodeSiteEnvelope(t, updated)
	if updated.Code != http.StatusOK || !updatedEnvelope.Success {
		t.Fatalf("admin update = %d %#v body=%s", updated.Code, updatedEnvelope, updated.Body.String())
	}
	var updatedRule map[string]any
	if err := json.Unmarshal(updatedEnvelope.Data, &updatedRule); err != nil || updatedRule["threshold_value"] != "90.0000000000" || updatedRule["for_times"] != float64(4) {
		t.Fatalf("updated rule = %#v, %v", updatedRule, err)
	}

	unknownField := performSiteRequest(admin, http.MethodPut, "/api/alert-rules/"+strconv.FormatInt(ruleID, 10), `{"enabled":true,"rule_key":"forbidden"}`)
	unknownEnvelope := decodeSiteEnvelope(t, unknownField)
	if unknownField.Code != http.StatusBadRequest || unknownEnvelope.Code != constant.CodeValidationError || unknownEnvelope.FieldErrors["body"] == "" {
		t.Fatalf("unknown JSON field = %d %#v", unknownField.Code, unknownEnvelope)
	}

	invalidQuery := performSiteRequest(viewer, http.MethodGet, "/api/alerts?sort_by=id%20DESC", "")
	invalidEnvelope := decodeSiteEnvelope(t, invalidQuery)
	if invalidQuery.Code != http.StatusBadRequest || invalidEnvelope.FieldErrors["sort_by"] == "" {
		t.Fatalf("invalid query = %d %#v", invalidQuery.Code, invalidEnvelope)
	}
	validSort := performSiteRequest(viewer, http.MethodGet, "/api/alerts?sort_by=first_fired_at&sort_order=asc", "")
	validSortEnvelope := decodeSiteEnvelope(t, validSort)
	if validSort.Code != http.StatusOK || !validSortEnvelope.Success {
		t.Fatalf("documented alert sort = %d %#v", validSort.Code, validSortEnvelope)
	}
	for _, sortBy := range []string{"first_observed_at", "resolved_at", "updated_at"} {
		rejected := performSiteRequest(viewer, http.MethodGet, "/api/alerts?sort_by="+sortBy, "")
		rejectedEnvelope := decodeSiteEnvelope(t, rejected)
		if rejected.Code != http.StatusBadRequest || rejectedEnvelope.FieldErrors["sort_by"] == "" {
			t.Errorf("undocumented alert sort %s = %d %#v", sortBy, rejected.Code, rejectedEnvelope)
		}
	}

	bodyOnRead := performSiteRequest(viewer, http.MethodGet, "/api/alerts", `{}`)
	bodyEnvelope := decodeSiteEnvelope(t, bodyOnRead)
	if bodyOnRead.Code != http.StatusBadRequest || bodyEnvelope.FieldErrors["body"] == "" {
		t.Fatalf("GET body = %d %#v", bodyOnRead.Code, bodyEnvelope)
	}

	list := performSiteRequest(viewer, http.MethodGet,
		"/api/alerts?status=firing&status=resolved&level=warning&level=critical&target_type=account&target_type=site&p=1&page_size=20", "")
	listEnvelope := decodeSiteEnvelope(t, list)
	if list.Code != http.StatusOK || !listEnvelope.Success {
		t.Fatalf("alert list = %d %#v body=%s", list.Code, listEnvelope, list.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listEnvelope.Data, &page); err != nil || len(page.Items) == 0 {
		t.Fatalf("decode alert page = %#v, %v", page, err)
	}
	var foundEvent bool
	for _, item := range page.Items {
		if item["id"] == strconv.FormatInt(event.ID, 10) {
			foundEvent = true
			if _, ok := item["rule_id"].(string); !ok {
				t.Fatalf("event rule_id is not a string: %#v", item)
			}
			if _, ok := item["current_value"].(string); !ok {
				t.Fatalf("event decimal is not a string: %#v", item)
			}
		}
	}
	if !foundEvent {
		t.Fatalf("event missing from list: %#v", page.Items)
	}

	detail := performSiteRequest(viewer, http.MethodGet, "/api/alerts/"+strconv.FormatInt(event.ID, 10), "")
	detailEnvelope := decodeSiteEnvelope(t, detail)
	if detail.Code != http.StatusOK || !detailEnvelope.Success {
		t.Fatalf("alert deep link detail = %d %#v", detail.Code, detailEnvelope)
	}
	var eventDetail map[string]any
	if err := json.Unmarshal(detailEnvelope.Data, &eventDetail); err != nil || eventDetail["id"] != strconv.FormatInt(event.ID, 10) {
		t.Fatalf("event detail = %#v, %v", eventDetail, err)
	}

	summary := performSiteRequest(viewer, http.MethodGet, "/api/alerts/summary", "")
	summaryEnvelope := decodeSiteEnvelope(t, summary)
	if summary.Code != http.StatusOK || !summaryEnvelope.Success {
		t.Fatalf("alert summary = %d %#v", summary.Code, summaryEnvelope)
	}

	deleteGlobal := performSiteRequest(admin, http.MethodDelete, "/api/alert-rules/"+strconv.FormatInt(ruleID, 10), "")
	deleteEnvelope := decodeSiteEnvelope(t, deleteGlobal)
	if deleteGlobal.Code != http.StatusBadRequest || deleteEnvelope.Code != constant.CodeValidationError {
		t.Fatalf("delete global rule = %d %#v", deleteGlobal.Code, deleteEnvelope)
	}
}

func newAlertRoleEngine(t *testing.T, alertController *controller.AlertController, role string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Recovery())
	RegisterAlertRoutes(engine, alertController, fixedSiteIdentityResolver{role: role})
	return engine
}
