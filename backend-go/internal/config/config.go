package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config 应用配置，全部来自环境变量，默认值适合本地开发。
type Config struct {
	ProjectName string
	Port        string
	SecretKey   string
	DatabaseURL string
	UploadDir   string

	AdminUsername string
	AdminPassword string
	AdminName     string
}

// Load 读取 .env 与环境变量，构造配置。
func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		ProjectName:   getEnv("PROJECT_NAME", "技术部门会议系统"),
		Port:          getEnv("PORT", "8002"),
		SecretKey:     getEnv("SECRET_KEY", "please-change-me-to-a-random-secret"),
		DatabaseURL:   getEnv("DATABASE_URL", "sqlite:///./meeting.db"),
		UploadDir:     getEnv("UPLOAD_DIR", "uploads"),
		AdminUsername: getEnv("INIT_ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("INIT_ADMIN_PASSWORD", "admin123"),
		AdminName:     getEnv("INIT_ADMIN_NAME", "系统管理员"),
	}

	if cfg.SecretKey == "please-change-me-to-a-random-secret" {
		log.Println("[warn] 请修改 SECRET_KEY 为强随机值")
	}
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}
