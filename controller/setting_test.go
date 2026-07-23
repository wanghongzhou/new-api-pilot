package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
