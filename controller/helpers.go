package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/service"
)

func decodeJSON(c *gin.Context, target any, maxBytes int64) bool {
	if err := common.DecodeJSON(c.Request.Body, target, maxBytes); err != nil {
		if errors.Is(err, common.ErrPayloadTooLarge) {
			common.AbortError(c, http.StatusRequestEntityTooLarge, constant.CodePayloadTooLarge, "Request body is too large", nil)
			return false
		}
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid request body", map[string]string{"body": err.Error()})
		return false
	}
	return true
}

func parsePositiveID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Platform user not found", nil)
	case errors.Is(err, service.ErrUsernameConflict):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Username already exists", map[string]string{"username": "already exists"})
	case errors.Is(err, service.ErrLastAdmin):
		common.AbortError(c, http.StatusConflict, constant.CodeLastAdmin, "The last enabled admin cannot be disabled or downgraded", nil)
	case errors.Is(err, service.ErrDisableSelf):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "You cannot disable your own account", nil)
	case errors.Is(err, service.ErrResetSelf):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Use the change password endpoint for your own account", nil)
	default:
		common.AbortInternalError(c)
	}
}
