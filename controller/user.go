package controller

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/service"
)

type PlatformUserController struct {
	users *service.PlatformUserService
}

func NewPlatformUserController(users *service.PlatformUserService) *PlatformUserController {
	return &PlatformUserController{users: users}
}

func (controller *PlatformUserController) List(c *gin.Context) {
	query, fieldErrors := parsePlatformUserListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid list query", fieldErrors)
		return
	}
	page, err := controller.users.List(c.Request.Context(), query)
	if err != nil {
		common.AbortInternalError(c)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *PlatformUserController) Create(c *gin.Context) {
	var request dto.CreatePlatformUserRequest
	if !decodeJSON(c, &request, common.CredentialBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid platform user", fieldErrors)
		return
	}
	user, err := controller.users.Create(c.Request.Context(), request)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, service.PlatformUserItemFromModel(user))
}

func (controller *PlatformUserController) Update(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var request dto.UpdatePlatformUserRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid platform user", fieldErrors)
		return
	}
	user, err := controller.users.Update(c.Request.Context(), id, request)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, service.PlatformUserItemFromModel(user))
}

func (controller *PlatformUserController) Enable(c *gin.Context) {
	controller.setStatus(c, true)
}

func (controller *PlatformUserController) Disable(c *gin.Context) {
	controller.setStatus(c, false)
}

func (controller *PlatformUserController) ResetPassword(c *gin.Context) {
	targetID, ok := parsePathID(c)
	if !ok {
		return
	}
	actorID, ok := actorID(c)
	if !ok {
		return
	}
	var request dto.ResetPasswordRequest
	if !decodeJSON(c, &request, common.CredentialBodyLimit) {
		return
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid password", fieldErrors)
		return
	}
	if err := controller.users.ResetPassword(c.Request.Context(), actorID, targetID, request.NewPassword); err != nil {
		writeServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func (controller *PlatformUserController) setStatus(c *gin.Context, enabled bool) {
	targetID, ok := parsePathID(c)
	if !ok {
		return
	}
	actorID, ok := actorID(c)
	if !ok {
		return
	}
	if err := controller.users.SetStatus(c.Request.Context(), actorID, targetID, enabled); err != nil {
		writeServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func parsePathID(c *gin.Context) (int64, bool) {
	id, ok := parsePositiveID(c.Param("id"))
	if !ok {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid platform user ID", map[string]string{"id": "must be a positive decimal integer"})
	}
	return id, ok
}

func actorID(c *gin.Context) (int64, bool) {
	identity, exists := middleware.CurrentIdentity(c)
	if !exists {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthRequired, "Authentication required", nil)
		return 0, false
	}
	id, ok := parsePositiveID(identity.ID)
	if !ok {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
		return 0, false
	}
	return id, true
}

func parsePlatformUserListQuery(c *gin.Context) (dto.PlatformUserListQuery, map[string]string) {
	query := dto.PlatformUserListQuery{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}
	errors := map[string]string{}
	if raw := c.Query("p"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			errors["p"] = "must be at least 1"
		} else {
			query.Page = value
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			errors["page_size"] = "must be between 1 and 100"
		} else {
			query.PageSize = value
		}
	}
	query.Keyword = strings.TrimSpace(c.Query("keyword"))
	if !utf8.ValidString(query.Keyword) || utf8.RuneCountInString(query.Keyword) > 128 {
		errors["keyword"] = "must not exceed 128 Unicode characters"
	}
	if raw := c.Query("sort_by"); raw != "" {
		query.SortBy = raw
	}
	if query.SortBy != "created_at" && query.SortBy != "username" && query.SortBy != "last_login_at" {
		errors["sort_by"] = "must be created_at, username, or last_login_at"
	}
	if raw := c.Query("sort_order"); raw != "" {
		query.SortOrder = strings.ToLower(raw)
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		errors["sort_order"] = "must be asc or desc"
	}
	query.Role = c.Query("role")
	if query.Role != "" && query.Role != constant.RoleAdmin && query.Role != constant.RoleViewer {
		errors["role"] = "must be admin or viewer"
	}
	if raw := c.Query("status"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || (value != constant.UserStatusEnabled && value != constant.UserStatusDisabled) {
			errors["status"] = "must be 1 or 2"
		} else {
			query.Status = &value
		}
	}
	if len(errors) != 0 {
		return dto.PlatformUserListQuery{}, errors
	}
	return query, nil
}
