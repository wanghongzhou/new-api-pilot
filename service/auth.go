package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type AuthService struct {
	users             *model.PlatformUserRepository
	limiter           *LoginLimiter
	clock             common.Clock
	dummyPasswordHash string
}

func NewAuthService(users *model.PlatformUserRepository, limiter *LoginLimiter, clock common.Clock) (*AuthService, error) {
	if clock == nil {
		clock = common.SystemClock{}
	}
	dummyHash, err := common.HashPassword("invalid-password-placeholder")
	if err != nil {
		return nil, fmt.Errorf("create login timing hash: %w", err)
	}
	return &AuthService{users: users, limiter: limiter, clock: clock, dummyPasswordHash: dummyHash}, nil
}

func (service *AuthService) Login(ctx context.Context, clientIP string, request dto.LoginRequest) (model.PlatformUser, error) {
	key := clientIP + ":" + strings.ToLower(strings.TrimSpace(request.Username))
	if err := service.limiter.Check(key); err != nil {
		return model.PlatformUser{}, err
	}
	user, err := service.users.FindByUsername(ctx, request.Username)
	if err != nil {
		if !model.IsNotFound(err) {
			return model.PlatformUser{}, fmt.Errorf("find login user: %w", err)
		}
		_ = common.CheckPassword(service.dummyPasswordHash, request.Password)
		service.limiter.RecordFailure(key)
		return model.PlatformUser{}, ErrInvalidCredentials
	}
	if err := common.CheckPassword(user.PasswordHash, request.Password); err != nil {
		service.limiter.RecordFailure(key)
		return model.PlatformUser{}, ErrInvalidCredentials
	}
	if user.Status != constant.UserStatusEnabled {
		return model.PlatformUser{}, ErrUserDisabled
	}
	now := service.clock.Now().Unix()
	if err := service.users.UpdateLastLogin(ctx, user.ID, now); err != nil {
		return model.PlatformUser{}, fmt.Errorf("record login: %w", err)
	}
	user.LastLoginAt = &now
	user.UpdatedAt = now
	service.limiter.Reset(key)
	return user, nil
}

func (service *AuthService) LoadIdentity(ctx context.Context, id string) (common.Identity, error) {
	parsedID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || parsedID <= 0 {
		return common.Identity{}, ErrUserNotFound
	}
	user, err := service.users.FindByID(ctx, parsedID)
	if err != nil {
		if model.IsNotFound(err) {
			return common.Identity{}, ErrUserNotFound
		}
		return common.Identity{}, err
	}
	if user.Status != constant.UserStatusEnabled {
		return common.Identity{}, ErrUserNotFound
	}
	return IdentityFromUser(user), nil
}

func (service *AuthService) ChangePassword(ctx context.Context, userID int64, request dto.ChangePasswordRequest) (model.PlatformUser, error) {
	newHash, err := common.HashPassword(request.NewPassword)
	if err != nil {
		return model.PlatformUser{}, err
	}
	var updated model.PlatformUser
	err = service.users.WithTransaction(ctx, func(repository *model.PlatformUserRepository) error {
		user, err := repository.FindByIDForUpdate(ctx, userID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrUserNotFound
			}
			return err
		}
		if err := common.CheckPassword(user.PasswordHash, request.OriginalPassword); err != nil {
			return ErrInvalidPassword
		}
		user.PasswordHash = newHash
		user.MustChangePassword = false
		user.SessionVersion++
		user.UpdatedAt = service.clock.Now().Unix()
		if err := repository.Save(ctx, &user); err != nil {
			return err
		}
		updated = user
		return nil
	})
	if err != nil {
		return model.PlatformUser{}, err
	}
	return updated, nil
}

func IdentityFromUser(user model.PlatformUser) common.Identity {
	return common.Identity{
		ID:                 strconv.FormatInt(user.ID, 10),
		Username:           user.Username,
		Role:               user.Role,
		Status:             user.Status,
		MustChangePassword: user.MustChangePassword,
		SessionVersion:     user.SessionVersion,
	}
}

func LoginUserFromUser(user model.PlatformUser) dto.LoginUser {
	return dto.LoginUser{
		ID:                 strconv.FormatInt(user.ID, 10),
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Role:               user.Role,
		Status:             user.Status,
		MustChangePassword: user.MustChangePassword,
	}
}

func IsRateLimited(err error) (LoginRateLimitedError, bool) {
	var result LoginRateLimitedError
	matched := errors.As(err, &result)
	return result, matched
}
