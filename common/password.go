package common

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	PasswordMinRunes = 8
	PasswordMaxBytes = 72
	BcryptCost       = 10
)

var (
	ErrPasswordTooShort = errors.New("password must contain at least 8 Unicode characters")
	ErrPasswordTooLong  = errors.New("password UTF-8 encoding must not exceed 72 bytes")
	ErrPasswordInvalid  = errors.New("password must be valid UTF-8")
)

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordInvalid
	}
	if utf8.RuneCountInString(password) < PasswordMinRunes {
		return ErrPasswordTooShort
	}
	if len([]byte(password)) > PasswordMaxBytes {
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
