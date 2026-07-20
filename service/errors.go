package service

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("platform user is disabled")
	ErrInvalidPassword    = errors.New("original password is incorrect")
	ErrUserNotFound       = errors.New("platform user not found")
	ErrUsernameConflict   = errors.New("platform username already exists")
	ErrLastAdmin          = errors.New("cannot disable or downgrade the last enabled admin")
	ErrDisableSelf        = errors.New("a user cannot disable their own account")
	ErrResetSelf          = errors.New("an admin must change their own password through the self-service endpoint")
)
