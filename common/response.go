package common

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
)

type APIResponse struct {
	Success     bool              `json:"success"`
	Message     string            `json:"message"`
	Code        string            `json:"code"`
	Data        any               `json:"data"`
	RequestID   string            `json:"request_id"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
	Params      map[string]any    `json:"params,omitempty"`
}

func RequestID(c *gin.Context) string {
	requestID, _ := c.Get(constant.ContextRequestID)
	value, _ := requestID.(string)
	return value
}

func WriteSuccess(c *gin.Context, status int, data any) {
	c.JSON(status, APIResponse{
		Success:   true,
		Message:   "",
		Code:      "",
		Data:      data,
		RequestID: RequestID(c),
	})
}

func WriteError(c *gin.Context, status int, code, message string, fieldErrors map[string]string) {
	WriteErrorWithParams(c, status, code, message, nil, fieldErrors)
}

func WriteErrorWithParams(c *gin.Context, status int, code, message string, params map[string]any, fieldErrors map[string]string) {
	c.JSON(status, APIResponse{
		Success:     false,
		Message:     message,
		Code:        code,
		Data:        nil,
		RequestID:   RequestID(c),
		FieldErrors: fieldErrors,
		Params:      params,
	})
}

func AbortError(c *gin.Context, status int, code, message string, fieldErrors map[string]string) {
	WriteError(c, status, code, message, fieldErrors)
	c.Abort()
}

func AbortErrorWithParams(c *gin.Context, status int, code, message string, params map[string]any, fieldErrors map[string]string) {
	WriteErrorWithParams(c, status, code, message, params, fieldErrors)
	c.Abort()
}

func AbortInternalError(c *gin.Context) {
	AbortError(c, http.StatusInternalServerError, constant.CodeInternalError, "Internal server error", nil)
}
