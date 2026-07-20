package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type BootstrapResult struct {
	Created           bool
	GeneratedPassword string
}

type PlatformUserService struct {
	users *model.PlatformUserRepository
	clock common.Clock
}

func NewPlatformUserService(users *model.PlatformUserRepository, clock common.Clock) *PlatformUserService {
	if clock == nil {
		clock = common.SystemClock{}
	}
	return &PlatformUserService{users: users, clock: clock}
}

func (service *PlatformUserService) EnsureBootstrapAdmin(ctx context.Context, configuredPassword string) (BootstrapResult, error) {
	password := configuredPassword
	generated := ""
	if password == "" {
		var err error
		password, err = generateBootstrapPassword()
		if err != nil {
			return BootstrapResult{}, err
		}
		generated = password
	}
	hash, err := common.HashPassword(password)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("hash bootstrap password: %w", err)
	}
	result := BootstrapResult{}
	err = service.users.WithTransaction(ctx, func(repository *model.PlatformUserRepository) error {
		count, err := repository.Count(ctx)
		if err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		now := service.clock.Now().Unix()
		user := model.PlatformUser{
			Username:           "admin",
			PasswordHash:       hash,
			DisplayName:        "Administrator",
			Role:               constant.RoleAdmin,
			Status:             constant.UserStatusEnabled,
			MustChangePassword: true,
			SessionVersion:     1,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := repository.Create(ctx, &user); err != nil {
			if model.IsDuplicateKey(err) {
				return nil
			}
			return err
		}
		result.Created = true
		result.GeneratedPassword = generated
		return nil
	})
	return result, err
}

func (service *PlatformUserService) Get(ctx context.Context, id int64) (model.PlatformUser, error) {
	user, err := service.users.FindByID(ctx, id)
	if model.IsNotFound(err) {
		return model.PlatformUser{}, ErrUserNotFound
	}
	return user, err
}

func (service *PlatformUserService) List(ctx context.Context, query dto.PlatformUserListQuery) (common.PageData[dto.PlatformUserItem], error) {
	users, total, err := service.users.List(ctx, model.PlatformUserFilter{
		Keyword:   query.Keyword,
		Role:      query.Role,
		Status:    query.Status,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		Offset:    (query.Page - 1) * query.PageSize,
		Limit:     query.PageSize,
	})
	if err != nil {
		return common.PageData[dto.PlatformUserItem]{}, err
	}
	items := make([]dto.PlatformUserItem, 0, len(users))
	for _, user := range users {
		items = append(items, PlatformUserItemFromModel(user))
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *PlatformUserService) Create(ctx context.Context, request dto.CreatePlatformUserRequest) (model.PlatformUser, error) {
	hash, err := common.HashPassword(request.Password)
	if err != nil {
		return model.PlatformUser{}, err
	}
	now := service.clock.Now().Unix()
	user := model.PlatformUser{
		Username:           request.Username,
		PasswordHash:       hash,
		DisplayName:        request.DisplayName,
		Role:               request.Role,
		Status:             constant.UserStatusEnabled,
		MustChangePassword: true,
		SessionVersion:     1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := service.users.Create(ctx, &user); err != nil {
		if model.IsDuplicateKey(err) {
			return model.PlatformUser{}, ErrUsernameConflict
		}
		return model.PlatformUser{}, err
	}
	return user, nil
}

func (service *PlatformUserService) Update(ctx context.Context, id int64, request dto.UpdatePlatformUserRequest) (model.PlatformUser, error) {
	var updated model.PlatformUser
	err := service.users.WithTransaction(ctx, func(repository *model.PlatformUserRepository) error {
		admins, err := repository.LockEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		user, err := repository.FindByIDForUpdate(ctx, id)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrUserNotFound
			}
			return err
		}
		if user.Role == constant.RoleAdmin && user.Status == constant.UserStatusEnabled && request.Role != constant.RoleAdmin && len(admins) <= 1 {
			return ErrLastAdmin
		}
		roleChanged := user.Role != request.Role
		user.Username = request.Username
		user.DisplayName = request.DisplayName
		user.Role = request.Role
		if roleChanged {
			user.SessionVersion++
		}
		user.UpdatedAt = service.clock.Now().Unix()
		if err := repository.Save(ctx, &user); err != nil {
			if model.IsDuplicateKey(err) {
				return ErrUsernameConflict
			}
			return err
		}
		updated = user
		return nil
	})
	return updated, err
}

func (service *PlatformUserService) SetStatus(ctx context.Context, actorID, targetID int64, enabled bool) error {
	return service.users.WithTransaction(ctx, func(repository *model.PlatformUserRepository) error {
		admins, err := repository.LockEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		user, err := repository.FindByIDForUpdate(ctx, targetID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrUserNotFound
			}
			return err
		}
		if !enabled && actorID == targetID {
			return ErrDisableSelf
		}
		targetStatus := constant.UserStatusDisabled
		if enabled {
			targetStatus = constant.UserStatusEnabled
		}
		if user.Status == targetStatus {
			return nil
		}
		if !enabled && user.Role == constant.RoleAdmin && user.Status == constant.UserStatusEnabled && len(admins) <= 1 {
			return ErrLastAdmin
		}
		user.Status = targetStatus
		user.SessionVersion++
		user.UpdatedAt = service.clock.Now().Unix()
		return repository.Save(ctx, &user)
	})
}

func (service *PlatformUserService) ResetPassword(ctx context.Context, actorID, targetID int64, newPassword string) error {
	if actorID == targetID {
		return ErrResetSelf
	}
	hash, err := common.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return service.users.WithTransaction(ctx, func(repository *model.PlatformUserRepository) error {
		user, err := repository.FindByIDForUpdate(ctx, targetID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrUserNotFound
			}
			return err
		}
		user.PasswordHash = hash
		user.MustChangePassword = true
		user.SessionVersion++
		user.UpdatedAt = service.clock.Now().Unix()
		return repository.Save(ctx, &user)
	})
}

func PlatformUserItemFromModel(user model.PlatformUser) dto.PlatformUserItem {
	return dto.PlatformUserItem{
		ID:                 strconv.FormatInt(user.ID, 10),
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Role:               user.Role,
		Status:             user.Status,
		MustChangePassword: user.MustChangePassword,
		LastLoginAt:        user.LastLoginAt,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
}

func generateBootstrapPassword() (string, error) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate bootstrap password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
