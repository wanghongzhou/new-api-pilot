package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/service"
)

func TestSettingControllerRejectsUnknownAndDuplicateJSONFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewSettingController(nil)
	for _, body := range []string{
		`{"items":[],"unknown":true}`,
		`{"items":[{"key":"collector.usage_delay_minutes","value":5,"value":6}]}`,
	} {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
		controller.Update(context)
		if context.Writer.Status() != http.StatusBadRequest {
			t.Fatalf("setting body %s status = %d", body, context.Writer.Status())
		}
	}
}

func TestWriteSettingServiceErrorUsesStableSLOCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	writeSettingServiceError(context, service.ErrSettingSLOForbidden)
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode setting error: %v", err)
	}
	if response.Code != http.StatusUnprocessableEntity || envelope.Code != constant.CodeSLOConfigForbidden {
		t.Fatalf("setting SLO response = %d %#v", response.Code, envelope)
	}
}
