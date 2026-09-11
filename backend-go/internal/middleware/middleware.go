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
}

// cacheTTL 用户信息缓存有效期：缓存命中可避免每请求查库，同时保证角色/停用变更最多延迟一个 TTL 生效。
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

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
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

// CreateToken 生成 JWT，有效期 7 天
func (a *Auth) CreateToken(userID uint) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
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

	// 优先读缓存，命中且未过期直接返回，避免每请求查库
	a.mu.RLock()
	if cu, ok := a.cache[userID]; ok && time.Now().Before(cu.expires) {
		a.mu.RUnlock()
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
	uc := &UserContext{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		Role:     user.Role,
		OrgID:    user.OrgID,
	}
	a.mu.Lock()
	a.cache[userID] = cachedUser{uc: uc, expires: time.Now().Add(cacheTTL)}
	a.mu.Unlock()
	return uc, true
}
