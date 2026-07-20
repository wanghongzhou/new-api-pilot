package dto

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"new-api-pilot/common"
	"new-api-pilot/constant"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginUser struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	Status             int    `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
}

type ChangePasswordRequest struct {
	OriginalPassword string `json:"original_password"`
	NewPassword      string `json:"new_password"`
}

type CreatePlatformUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

type UpdatePlatformUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type PlatformUserItem struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	Status             int    `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	LastLoginAt        *int64 `json:"last_login_at"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

type PlatformUserListQuery struct {
	Page      int
	PageSize  int
	Keyword   string
	SortBy    string
	SortOrder string
	Role      string
	Status    *int
}

func (request *LoginRequest) Normalize() {
	request.Username = strings.TrimSpace(request.Username)
}

func (request LoginRequest) Validate() map[string]string {
	errors := map[string]string{}
	if !usernamePattern.MatchString(request.Username) {
		errors["username"] = "must be 3-64 lowercase ASCII letters, numbers, dots, underscores, or hyphens"
	}
	if request.Password == "" {
		errors["password"] = "is required"
	}
	return nilIfEmpty(errors)
}

func (request ChangePasswordRequest) Validate() map[string]string {
	errors := map[string]string{}
	if request.OriginalPassword == "" {
		errors["original_password"] = "is required"
	}
	if err := common.ValidatePassword(request.NewPassword); err != nil {
		errors["new_password"] = err.Error()
	}
	return nilIfEmpty(errors)
}

func (request *CreatePlatformUserRequest) Normalize() {
	request.Username = strings.TrimSpace(request.Username)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
}

func (request CreatePlatformUserRequest) Validate() map[string]string {
	errors := validateUserFields(request.Username, request.DisplayName, request.Role)
	if err := common.ValidatePassword(request.Password); err != nil {
		errors["password"] = err.Error()
	}
	return nilIfEmpty(errors)
}

func (request *UpdatePlatformUserRequest) Normalize() {
	request.Username = strings.TrimSpace(request.Username)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
}

func (request UpdatePlatformUserRequest) Validate() map[string]string {
	return nilIfEmpty(validateUserFields(request.Username, request.DisplayName, request.Role))
}

func (request ResetPasswordRequest) Validate() map[string]string {
	errors := map[string]string{}
	if err := common.ValidatePassword(request.NewPassword); err != nil {
		errors["new_password"] = err.Error()
	}
	return nilIfEmpty(errors)
}

func validateUserFields(username, displayName, role string) map[string]string {
	errors := map[string]string{}
	if !usernamePattern.MatchString(username) {
		errors["username"] = "must be 3-64 lowercase ASCII letters, numbers, dots, underscores, or hyphens"
	}
	if !utf8.ValidString(displayName) || utf8.RuneCountInString(displayName) < 1 || utf8.RuneCountInString(displayName) > 128 {
		errors["display_name"] = "must contain 1-128 Unicode characters"
	}
	if role != constant.RoleAdmin && role != constant.RoleViewer {
		errors["role"] = "must be admin or viewer"
	}
	return errors
}

func nilIfEmpty(errors map[string]string) map[string]string {
	if len(errors) == 0 {
		return nil
	}
	return errors
}
