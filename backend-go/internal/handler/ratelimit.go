package handler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 认证类接口的失败限流：防止对登录与「安全问答找回密码」的暴力尝试。
// 计数保存在进程内（本系统为单实例 SQLite 部署，不引入额外表结构），
// 重启即清空；采用固定窗口：从窗口内第一次失败起计时，到期后自动解除。
const (
	loginMaxFails   = 10               // 同一来源对同一账号，窗口内允许的登录失败次数
	loginFailWindow = 15 * time.Minute // 登录失败计数窗口
	resetMaxFails   = 5                // 找回密码（取问题与校验答案）允许的失败次数
	resetFailWindow = 30 * time.Minute // 找回密码计数窗口
	limiterMaxKeys  = 4096             // 超过该数量时顺带清理过期条目，避免键无界增长
)

type failEntry struct {
	count int
	first time.Time
}

// failLimiter 按 key 统计时间窗内的失败次数。非线程安全的调用方已全部经 mutex 保护。
type failLimiter struct {
	mu      sync.Mutex
	entries map[string]*failEntry
}

func newFailLimiter() *failLimiter {
	return &failLimiter{entries: make(map[string]*failEntry)}
}

// blocked 报告该 key 在当前窗口内的失败次数是否已达上限。
func (l *failLimiter) blocked(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return false
	}
	if time.Since(e.first) > window {
		delete(l.entries, key) // 窗口已过，计数作废
		return false
	}
	return e.count >= max
}

// addFail 记录一次失败；窗口起点保持首次失败时间（固定窗口）。
func (l *failLimiter) addFail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) > limiterMaxKeys {
		l.purgeLocked()
	}
	if e, ok := l.entries[key]; ok {
		e.count++
		return
	}
	l.entries[key] = &failEntry{count: 1, first: time.Now()}
}

// clear 在验证成功后重置计数，避免历史失败累计。
func (l *failLimiter) clear(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// purgeLocked 清理已满窗口的条目；调用方需持有锁。
func (l *failLimiter) purgeLocked() {
	// 以最长的窗口为界，超过它的条目不可能再影响判定
	for key, e := range l.entries {
		if time.Since(e.first) > resetFailWindow {
			delete(l.entries, key)
		}
	}
}

// failKey 限流维度：来源 IP + 账号（小写去空），避免单一账号被跨来源累加误伤。
func failKey(c *gin.Context, username string) string {
	return c.ClientIP() + "|" + strings.ToLower(strings.TrimSpace(username))
}

// tooManyFails 若该 key 的失败次数已达上限，则直接写入 429 响应并返回 true，调用方据此提前 return。
func (h *Handler) tooManyFails(c *gin.Context, key string, max int, window time.Duration, msg string) bool {
	if !h.rateLimit.blocked(key, max, window) {
		return false
	}
	c.JSON(http.StatusTooManyRequests, gin.H{"detail": msg})
	return true
}
