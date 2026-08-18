package database

import (
	"fmt"
	"log"
	"strings"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"meetingbackend/internal/config"
	"meetingbackend/internal/model"
)

// Open 打开 SQLite 数据库。
func Open(cfg *config.Config) (*gorm.DB, error) {
	dsn := strings.TrimPrefix(cfg.DatabaseURL, "sqlite:///")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	// SQLite 单文件：限制连接数避免写锁冲突
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// Migrate 自动建表。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Organization{},
		&model.User{},
		&model.Meeting{},
		&model.MeetingOrg{},
		&model.ReportItem{},
		&model.ReportAttachment{},
		&model.Conclusion{},
		&model.MeetingMinutes{},
		&model.OperationLog{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}

// InitAdmin 初始化默认管理员。
func InitAdmin(db *gorm.DB, cfg *config.Config) {
	var count int64
	db.Model(&model.User{}).Where("username = ?", cfg.AdminUsername).Count(&count)
	if count > 0 {
		return
	}
	admin := &model.User{
		Username:       cfg.AdminUsername,
		HashedPassword: hashPassword(cfg.AdminPassword),
		Name:           cfg.AdminName,
		Role:           model.RoleAdmin,
		IsActive:       true,
	}
	if err := db.Create(admin).Error; err != nil {
		log.Printf("init admin: %v", err)
		return
	}
	log.Printf("已创建默认管理员 %s", cfg.AdminUsername)
}

func hashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed)
}
