package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PlatformUser struct {
	ID                 int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Username           string `gorm:"column:username"`
	PasswordHash       string `gorm:"column:password_hash"`
	DisplayName        string `gorm:"column:display_name"`
	Role               string `gorm:"column:role"`
	Status             int    `gorm:"column:status"`
	MustChangePassword bool   `gorm:"column:must_change_password"`
	SessionVersion     int    `gorm:"column:session_version"`
	LastLoginAt        *int64 `gorm:"column:last_login_at"`
	CreatedAt          int64  `gorm:"column:created_at"`
	UpdatedAt          int64  `gorm:"column:updated_at"`
}

func (PlatformUser) TableName() string {
	return "platform_user"
}

type PlatformUserFilter struct {
	Keyword   string
	Role      string
	Status    *int
	SortBy    string
	SortOrder string
	Offset    int
	Limit     int
}

type PlatformUserRepository struct {
	db *gorm.DB
}

func NewPlatformUserRepository(db *gorm.DB) *PlatformUserRepository {
	return &PlatformUserRepository{db: db}
}

func (repository *PlatformUserRepository) WithTransaction(ctx context.Context, operation func(*PlatformUserRepository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(&PlatformUserRepository{db: tx})
	})
}

func (repository *PlatformUserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&PlatformUser{}).Count(&count).Error
	return count, err
}

func (repository *PlatformUserRepository) FindByID(ctx context.Context, id int64) (PlatformUser, error) {
	var user PlatformUser
	err := repository.db.WithContext(ctx).First(&user, id).Error
	return user, err
}

func (repository *PlatformUserRepository) FindByIDForUpdate(ctx context.Context, id int64) (PlatformUser, error) {
	var user PlatformUser
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error
	return user, err
}

func (repository *PlatformUserRepository) FindByUsername(ctx context.Context, username string) (PlatformUser, error) {
	var user PlatformUser
	err := repository.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	return user, err
}

func (repository *PlatformUserRepository) Create(ctx context.Context, user *PlatformUser) error {
	return repository.db.WithContext(ctx).Create(user).Error
}

func (repository *PlatformUserRepository) Save(ctx context.Context, user *PlatformUser) error {
	return repository.db.WithContext(ctx).Save(user).Error
}

func (repository *PlatformUserRepository) UpdateLastLogin(ctx context.Context, id, timestamp int64) error {
	return repository.db.WithContext(ctx).Model(&PlatformUser{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": timestamp,
		"updated_at":    timestamp,
	}).Error
}

func (repository *PlatformUserRepository) LockEnabledAdmins(ctx context.Context) ([]PlatformUser, error) {
	var admins []PlatformUser
	err := repository.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("role = ? AND status = ?", "admin", 1).
		Order("id ASC").
		Find(&admins).Error
	return admins, err
}

func (repository *PlatformUserRepository) List(ctx context.Context, filter PlatformUserFilter) ([]PlatformUser, int64, error) {
	query := repository.db.WithContext(ctx).Model(&PlatformUser{})
	if filter.Keyword != "" {
		keyword := "%" + escapeLike(filter.Keyword) + "%"
		query = query.Where("(username LIKE ? ESCAPE '\\\\' OR display_name LIKE ? ESCAPE '\\\\')", keyword, keyword)
	}
	if filter.Role != "" {
		query = query.Where("role = ?", filter.Role)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortColumns := map[string]string{"created_at": "created_at", "username": "username", "last_login_at": "last_login_at", "status": "status"}
	column, exists := sortColumns[filter.SortBy]
	if !exists {
		return nil, 0, fmt.Errorf("unsupported platform user sort %q", filter.SortBy)
	}
	order := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		order = "ASC"
	}
	var users []PlatformUser
	err := query.Order(column + " " + order).Order("id DESC").Offset(filter.Offset).Limit(filter.Limit).Find(&users).Error
	return users, total, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func IsDuplicateKey(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
