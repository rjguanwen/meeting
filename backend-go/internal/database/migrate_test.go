package database

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"meetingbackend/internal/model"
)

// TestMigrateAddsTokenVersionWithoutDataLoss 模拟生产库平滑升级：
// 旧库的 users 表没有 token_version 列且已有账号数据，AutoMigrate 必须只补列、
// 给存量行填入默认值 0，且不改动任何既有字段。
func TestMigrateAddsTokenVersionWithoutDataLoss(t *testing.T) {
	db := openRawSQLite(t, filepath.Join(t.TempDir(), "legacy.db"))

	// 升级前的 users 结构（刻意缺少 token_version）
	const legacySchema = `CREATE TABLE users (
		id integer PRIMARY KEY AUTOINCREMENT,
		username text UNIQUE NOT NULL,
		hashed_password text NOT NULL,
		name text,
		role text DEFAULT 'member',
		org_id integer,
		is_active numeric DEFAULT true,
		avatar text,
		password_hint text,
		security_question text,
		security_answer text,
		created_at datetime)`
	if err := db.Exec(legacySchema).Error; err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (username, hashed_password, name, role, is_active)
		VALUES ('zhang', '$2a$10$legacyhash', '张三', 'member', 1)`).Error; err != nil {
		t.Fatalf("写入存量账号失败: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatalf("升级后 users 表应已有 token_version 列")
	}

	var version int
	var name, hashed string
	err := db.Table("users").Select("token_version, name, hashed_password").
		Where("username = ?", "zhang").Row().Scan(&version, &name, &hashed)
	if err != nil {
		t.Fatalf("读取存量账号失败: %v", err)
	}
	if version != 0 {
		t.Fatalf("存量账号 token_version 应为默认值 0，实际 %d", version)
	}
	if name != "张三" || hashed != "$2a$10$legacyhash" {
		t.Fatalf("升级不应改动既有数据，实际 name=%q hashed=%q", name, hashed)
	}

	// 重复执行（每次启动都会跑）必须保持幂等
	if err := Migrate(db); err != nil {
		t.Fatalf("二次 Migrate: %v", err)
	}
	if n := countUsers(t, db); n != 1 {
		t.Fatalf("重复迁移不应复制数据，当前 %d 行", n)
	}

	// 版本号可正常自增并持久化
	if err := db.Model(&model.User{}).Where("username = ?", "zhang").
		Update("token_version", 1).Error; err != nil {
		t.Fatalf("更新 token_version 失败: %v", err)
	}
	var user model.User
	if err := db.Where("username = ?", "zhang").First(&user).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if user.TokenVersion != 1 {
		t.Fatalf("token_version 应持久化为 1，实际 %d", user.TokenVersion)
	}
}

func openRawSQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Windows 下需先释放文件句柄，否则 t.TempDir() 清理会失败
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db
}

func countUsers(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Table("users").Count(&n).Error; err != nil {
		t.Fatalf("统计 users 失败: %v", err)
	}
	return n
}
