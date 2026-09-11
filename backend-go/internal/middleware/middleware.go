package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"meetingbackend/internal/config"
	"meetingbackend/internal/model"
)

// UserContext 认证后写入上下文的用户信息
type UserContext struct {
	ID       uint
	Username string
	Name     string
	Role     string
	OrgID    *uint
	// tokenVersion 签发令牌时用户的版本号，与令牌中的 tv 声明不一致即失效（改密码后踢下线）
	tokenVersion int
}

// cacheTTL 用户信息缓存有效期：缓存命中可避免每请求查库。
// 角色 / 停用 / 改密码等关键变更处会主动 InvalidateUser，最坏情况下也只延迟一个 TTL 生效。
const cacheTTL = 30 * time.Second

type cachedUser struct {
	uc      *UserContext
	expires time.Time
}

type Auth struct {
	cfg *config.Config
	db  *gorm.DB

	mu    sync.RWMutex
	cache map[uint]cachedUser
}

func NewAuth(cfg *config.Config, db *gorm.DB) *Auth {
	return &Auth{cfg: cfg, db: db, cache: make(map[uint]cachedUser)}
}

// InvalidateUser 主动失效指定用户的鉴权缓存（角色变更 / 停用时调用）。
func (a *Auth) InvalidateUser(userID uint) {
	a.mu.Lock()
	delete(a.cache, userID)
	a.mu.Unlock()
}

// CORS 跨域中间件。
// allowed 为空时保持通配 Access-Control-Allow-Origin: *（默认行为，兼容现有前后端分离部署）；
// 配置白名单后仅回显命中的 Origin，并补 Vary: Origin 防止缓存串扰。
func CORS(allowed []string) gin.HandlerFunc {
	if len(allowed) == 0 {
		return func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
		}
	}
	set := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		set[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			if _, ok := set[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// CreateToken 生成 JWT，有效期 7 天。
// tokenVersion 写入 tv 声明：用户改密码后库内版本自增，此前签发的令牌随之失效。
// 升级前签发的令牌没有 tv 声明，校验时按 0 处理，部署后已登录用户不会掉线。
func (a *Auth) CreateToken(userID uint, tokenVersion int) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"tv":  tokenVersion,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.cfg.SecretKey))
}

// RequireUser 要求登录
func (a *Auth) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := a.currentUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "登录凭证无效或已过期"})
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

// RequireAdmin 要求管理员
func (a *Auth) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := a.currentUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "登录凭证无效或已过期"})
			return
		}
		if user.Role != model.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"detail": "需要管理员权限"})
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func (a *Auth) currentUser(c *gin.Context) (*UserContext, bool) {
	header := c.GetHeader("Authorization")
	tokenStr := strings.TrimPrefix(header, "Bearer ")
	if tokenStr == "" || tokenStr == header {
		return nil, false
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(a.cfg.SecretKey), nil
	})
	if err != nil || !token.Valid {
		return nil, false
	}
	sub, ok := claims["sub"].(float64)
	if !ok {
		return nil, false
	}
	userID := uint(sub)
	// 旧版令牌无 tv 声明，按 0 处理，保证平滑升级
	var tokenVersion int
	if v, ok := claims["tv"].(float64); ok {
		tokenVersion = int(v)
	}

	// 优先读缓存，命中且未过期直接返回，避免每请求查库
	a.mu.RLock()
	if cu, ok := a.cache[userID]; ok && time.Now().Before(cu.expires) {
		a.mu.RUnlock()
		if cu.uc.tokenVersion != tokenVersion {
			return nil, false // 令牌版本陈旧：密码已修改，需重新登录
		}
		return cu.uc, true
	}
	a.mu.RUnlock()

	var user model.User
	if err := a.db.First(&user, userID).Error; err != nil {
		return nil, false
	}
	if !user.IsActive {
		return nil, false
	}
	if user.TokenVersion != tokenVersion {
		return nil, false
	}
	uc := &UserContext{
		ID:           user.ID,
		Username:     user.Username,
		Name:         user.Name,
		Role:         user.Role,
		OrgID:        user.OrgID,
		tokenVersion: user.TokenVersion,
	}
	a.mu.Lock()
	a.cache[userID] = cachedUser{uc: uc, expires: time.Now().Add(cacheTTL)}
	a.mu.Unlock()
	return uc, true
}
