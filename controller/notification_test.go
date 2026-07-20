package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type notificationTesterFunc func(context.Context, string) (dto.NotificationTestResult, error)

func (function notificationTesterFunc) Test(ctx context.Context, requestID string) (dto.NotificationTestResult, error) {
	return function(ctx, requestID)
}

func TestNotificationControllerReturnsHTTP200ForExpectedTestFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	tester := notificationTesterFunc(func(_ context.Context, requestID string) (dto.NotificationTestResult, error) {
		called = true
		if requestID != "req_notification_test" {
			t.Fatalf("request ID = %q", requestID)
		}
		message := dto.MustMessageRef(constant.MessageNotificationNotConfigured, map[string]any{
			"alert_event_id": nil, "delivery_id": nil,
		}, "")
		return dto.NotificationTestResult{Status: "failed", Message: message}, nil
	})
	controller := NewNotificationController(tester)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(constant.ContextRequestID, "req_notification_test")
		c.Next()
	})
	engine.POST("/api/notifications/dingtalk/test", controller.TestDingTalk)

	request := httptest.NewRequest(http.MethodPost, "/api/notifications/dingtalk/test", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if !called || response.Code != http.StatusOK {
		t.Fatalf("called=%t status=%d body=%s", called, response.Code, response.Body.String())
	}
	var envelope struct {
		Success bool                       `json:"success"`
		Code    string                     `json:"code"`
		Data    dto.NotificationTestResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Success || envelope.Code != "" || envelope.Data.Status != "failed" ||
		envelope.Data.DeliveryID != nil || envelope.Data.Message.Code != constant.MessageNotificationNotConfigured {
		t.Fatalf("response envelope = %#v", envelope)
	}
}

func TestNewNotificationControllerRejectsMissingService(t *testing.T) {
	if controller := NewNotificationController(nil); controller != nil {
		t.Fatalf("controller = %#v", controller)
	}
}
