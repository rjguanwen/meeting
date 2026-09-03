package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// WeComConfig 企业微信微盘自动上传配置（全部来自环境变量，私有化部署）。
type WeComConfig struct {
	AutoUpload        bool   // WECOM_AUTO_UPLOAD  归档会议时是否自动上传纪要至微盘
	APIBase           string // WECOM_API_BASE     API 基地址，如 https://weixin.moutai.com.cn:8443
	APIPrefix         string // WECOM_API_PREFIX   接口前缀，默认 /cgi-bin
	CorpID            string // WECOM_CORP_ID      企业 ID
	AgentID           string // WECOM_AGENT_ID     自建应用 ID（预留，gettoken 实际只需 corpid+secret）
	CorpSecret        string // WECOM_CORP_SECRET  应用密钥
	SpaceID           string // WECOM_SPACE_ID     目标共享空间 ID
	FolderID          string // WECOM_FOLDER_ID    目标文件夹 fileid，留空表示空间根目录
	GenerateIfMissing bool   // WECOM_GENERATE_MISSING  归档时若未生成纪要，先自动生成再上传
	FileNameTemplate  string // WECOM_FILE_NAME_TEMPLATE 上传文件名模板，支持 {title} {date} {id}
	TLSInsecure       bool   // WECOM_TLS_INSECURE 跳过证书校验（私有化环境自签证书）
	TimeoutSeconds    int    // WECOM_TIMEOUT_SECONDS 请求超时（秒）
}

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

	WeCom WeComConfig
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
		WeCom: WeComConfig{
			AutoUpload:        getEnvBool("WECOM_AUTO_UPLOAD", false),
			APIBase:           getEnv("WECOM_API_BASE", ""),
			APIPrefix:         getEnv("WECOM_API_PREFIX", "/cgi-bin"),
			CorpID:            getEnv("WECOM_CORP_ID", ""),
			AgentID:           getEnv("WECOM_AGENT_ID", ""),
			CorpSecret:        getEnv("WECOM_CORP_SECRET", ""),
			SpaceID:           getEnv("WECOM_SPACE_ID", ""),
			FolderID:          getEnv("WECOM_FOLDER_ID", ""),
			GenerateIfMissing: getEnvBool("WECOM_GENERATE_MISSING", true),
			FileNameTemplate:  getEnv("WECOM_FILE_NAME_TEMPLATE", "会议纪要-{title}-{date}.md"),
			TLSInsecure:       getEnvBool("WECOM_TLS_INSECURE", false),
			TimeoutSeconds:    getEnvInt("WECOM_TIMEOUT_SECONDS", 30),
		},
	}

	if cfg.WeCom.AutoUpload {
		if cfg.WeCom.APIBase == "" || cfg.WeCom.CorpID == "" || cfg.WeCom.CorpSecret == "" || cfg.WeCom.SpaceID == "" {
			log.Println("[warn] WECOM_AUTO_UPLOAD=true 但 API_BASE/CORP_ID/CORP_SECRET/SPACE_ID 配置不完整，自动上传将停用")
		}
	}
	if cfg.SecretKey == "please-change-me-to-a-random-secret" {
		log.Println("[warn] 请修改 SECRET_KEY 为强随机值")
	}
	return cfg
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
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
