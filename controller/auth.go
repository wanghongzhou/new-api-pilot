package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/service"
)

type AuthController struct {
	auth     *service.AuthService
	users    *service.PlatformUserService
	sessions *common.SessionStore
}

func NewAuthController(auth *service.AuthService, users *service.PlatformUserService, sessions *common.SessionStore) *AuthController {
	return &AuthController{auth: auth, users: users, sessions: sessions}
}

func (controller *AuthController) Login(c *gin.Context) {
	var request dto.LoginRequest
	if !decodeJSON(c, &request, common.CredentialBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid login request", fieldErrors)
		return
	}
	user, err := controller.auth.Login(c.Request.Context(), c.ClientIP(), request)
	if err != nil {
		if rateLimit, limited := service.IsRateLimited(err); limited {
			retryAfter := int(math.Ceil(rateLimit.RetryAfter.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			common.AbortError(c, http.StatusTooManyRequests, constant.CodeLoginRateLimited, "Too many failed login attempts", nil)
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Invalid username or password", nil)
			return
		}
		if errors.Is(err, service.ErrUserDisabled) {
			common.AbortError(c, http.StatusForbidden, constant.CodeUserDisabled, "User account is disabled", nil)
			return
		}
		common.AbortInternalError(c)
		return
	}
	if err := controller.sessions.Write(c.Writer, service.IdentityFromUser(user)); err != nil {
		common.AbortInternalError(c)
		return
	}
	common.WriteSuccess(c, http.StatusOK, service.LoginUserFromUser(user))
}

func (controller *AuthController) Logout(c *gin.Context) {
	controller.sessions.Clear(c.Writer)
	common.WriteSuccess(c, http.StatusOK, nil)
}

func (controller *AuthController) Self(c *gin.Context) {
	identity, ok := middleware.CurrentIdentity(c)
	if !ok {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthRequired, "Authentication required", nil)
		return
	}
	id, valid := parsePositiveID(identity.ID)
	if !valid {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
		return
	}
	user, err := controller.users.Get(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, service.LoginUserFromUser(user))
}

func (controller *AuthController) ChangePassword(c *gin.Context) {
	identity, ok := middleware.CurrentIdentity(c)
	if !ok {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthRequired, "Authentication required", nil)
		return
	}
	userID, valid := parsePositiveID(identity.ID)
	if !valid {
		common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
		return
	}
	var request dto.ChangePasswordRequest
	if !decodeJSON(c, &request, common.CredentialBodyLimit) {
		return
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid password", fieldErrors)
		return
	}
	user, err := controller.auth.ChangePassword(c.Request.Context(), userID, request)
	if err != nil {
		if errors.Is(err, service.ErrInvalidPassword) {
			common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Original password is incorrect", map[string]string{"original_password": "is incorrect"})
			return
		}
		writeServiceError(c, err)
		return
	}
	if err := controller.sessions.Write(c.Writer, service.IdentityFromUser(user)); err != nil {
		common.AbortInternalError(c)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}
