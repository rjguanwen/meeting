package database

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"meetingbackend/internal/config"
	"meetingbackend/internal/model"
)

// Open 打开 SQLite 数据库。
// 启用 WAL 模式：读不阻塞写、写不阻塞读，配合多连接池提升并发吞吐；
// busy_timeout 让并发写排队等待而非立即报 "database is locked"。
func Open(cfg *config.Config) (*gorm.DB, error) {
	dsn := strings.TrimPrefix(cfg.DatabaseURL, "sqlite:///")
	// 通过 DSN pragma 设置 busy_timeout 与 synchronous（glebarez/sqlite 支持 _pragma 查询参数）
	if !strings.Contains(dsn, "?") {
		dsn += "?"
	} else {
		dsn += "&"
	}
	dsn += "_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"

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
	// 开启 WAL（持久化，一次生效即可）
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("set journal_mode=WAL: %v", err)
	}
	// 多连接并发读；写仍由 SQLite 单写者串行，busy_timeout 负责排队
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// Migrate 自动建表并补齐性能索引（幂等，不影响已有数据）。
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
	if err := ensureIndexes(db); err != nil {
		return fmt.Errorf("ensure indexes: %w", err)
	}
	return nil
}

// ensureIndexes 补充常用查询的复合索引（CREATE INDEX IF NOT EXISTS，幂等安全）。
func ensureIndexes(db *gorm.DB) error {
	stmts := []string{
		// 会议列表按状态过滤 + 按时间排序
		`CREATE INDEX IF NOT EXISTS idx_meetings_status_time ON meetings(status, meeting_time)`,
		// 会议可见性判断 / 组织维度查询
		`CREATE INDEX IF NOT EXISTS idx_meeting_orgs_meeting_org ON meeting_orgs(meeting_id, org_id)`,
		// 负责人会议列表：按自己的 org_id 反查 meeting_id（与上面的索引方向不同，两者各自覆盖一侧查询）
		`CREATE INDEX IF NOT EXISTS idx_meeting_orgs_org_meeting ON meeting_orgs(org_id, meeting_id)`,
		// 材料组装：按会议取事项并按组织、排序分组
		`CREATE INDEX IF NOT EXISTS idx_report_items_meeting_org_sort ON report_items(meeting_id, org_id, sort_order)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
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
