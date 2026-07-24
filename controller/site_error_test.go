package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/service"
)

func TestWriteSiteServiceErrorKeepsLoginRejectionDistinct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(constant.ContextRequestID, "req_login_rejected")

	writeSiteServiceError(context, service.ErrUpstreamLoginRejected)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response common.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success || response.Code != constant.CodeUpstreamLoginRejected || response.RequestID != "req_login_rejected" {
		t.Fatalf("response = %#v", response)
	}
}
