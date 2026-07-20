package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type SettingController struct{ settings *service.SettingService }

func NewSettingController(settings *service.SettingService) *SettingController {
	return &SettingController{settings: settings}
}

func (controller *SettingController) Get(c *gin.Context) {
	if !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	groups, err := controller.settings.Get(c.Request.Context())
	if err != nil {
		writeSettingServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, groups)
}

func (controller *SettingController) Update(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	var request dto.SettingPatchRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid setting patch", fieldErrors)
		return
	}
	groups, err := controller.settings.Update(c.Request.Context(), request)
	if err != nil {
		writeSettingServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, groups)
}

func writeSettingServiceError(c *gin.Context, err error) {
	var validation *service.SettingValidationError
	switch {
	case errors.As(err, &validation):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid setting patch", validation.Fields)
	case errors.Is(err, service.ErrSettingSLOForbidden):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeSLOConfigForbidden,
			"Setting patch would violate the production H+15 SLO", nil)
	default:
		common.AbortInternalError(c)
	}
}
