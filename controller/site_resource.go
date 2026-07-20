package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

func (controller *SiteController) ResourceStatus(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	query, fieldErrors := parseResourceQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site resource query", fieldErrors)
		return
	}
	response, err := controller.sites.ResourceStatus(c.Request.Context(), siteID, query)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func parseResourceQuery(c *gin.Context) (dto.ResourceQuery, map[string]string) {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{
		"start_timestamp": {}, "end_timestamp": {}, "granularity": {}, "node_name": {},
	})
	query := dto.ResourceQuery{
		StartTimestamp: parseRequiredTimestampQuery(c, "start_timestamp", fieldErrors),
		EndTimestamp:   parseRequiredTimestampQuery(c, "end_timestamp", fieldErrors),
	}
	query.Granularity, _ = singletonQueryValue(c, "granularity", fieldErrors)
	if nodeName, exists := singletonQueryValue(c, "node_name", fieldErrors); exists {
		query.NodeName = &nodeName
	}
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.ResourceQuery{}, fieldErrors
	}
	return query, nil
}
