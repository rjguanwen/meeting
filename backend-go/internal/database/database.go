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
		&model.MeetingRoom{},
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

// MigrateLegacyRoles 迁移旧版角色：把历史 leader 账号按所属组织类型转为 dept_leader / team_leader。
// 组织类型在 organizations 表，通过 JOIN 判断。
func MigrateLegacyRoles(db *gorm.DB) {
	var users []model.User
	if err := db.Where("role = ?", model.RoleLeader).Find(&users).Error; err != nil {
		log.Printf("migrate legacy roles: find users: %v", err)
		return
	}
	for i := range users {
		u := &users[i]
		if u.OrgID == nil {
			// 无组织的旧 leader 降级为成员
			u.Role = model.RoleMember
		} else {
			var org model.Organization
			if err := db.First(&org, *u.OrgID).Error; err != nil {
				u.Role = model.RoleMember
			} else if org.Type == model.OrgTypeDept {
				u.Role = model.RoleDeptLeader
			} else {
				u.Role = model.RoleTeamLeader
			}
		}
		if err := db.Save(u).Error; err != nil {
			log.Printf("migrate legacy roles: save user %d: %v", u.ID, err)
		}
	}
	if len(users) > 0 {
		log.Printf("已迁移 %d 个旧 leader 账号角色", len(users))
	}
}

func hashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed)
}
