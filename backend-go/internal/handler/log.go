package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

// logRecord 写入一条操作日志。
// 优先使用登录用户；登录类动作（login/login_fail）传入 userID=0、username 单独指定。
func (h *Handler) logRecord(c *gin.Context, action, targetType string, targetID uint, detail string) {
	user := currentUser(c)
	entry := &model.OperationLog{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     truncateLog(detail, 500),
		IP:         clientIP(c),
		CreatedAt:  time.Now(),
	}
	if user != nil {
		entry.UserID = user.ID
		entry.Username = user.Username
	} else {
		entry.Username = c.PostForm("username")
	}
	_ = h.db.Create(entry).Error
}

// logLoginRecord 登录类日志（可指定用户名，登录失败时无登录态）。
func (h *Handler) logLoginRecord(c *gin.Context, action, username string, detail string) {
	entry := &model.OperationLog{
		Action:    action,
		Username:  username,
		Detail:    truncateLog(detail, 500),
		IP:        clientIP(c),
		CreatedAt: time.Now(),
	}
	_ = h.db.Create(entry).Error
}

func truncateLog(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func clientIP(c *gin.Context) string {
	if ip := c.ClientIP(); ip != "" {
		return ip
	}
	return c.RemoteIP()
}

// ListLogs GET /api/logs 操作日志查询（管理员）
// 支持：keyword（用户名/详情模糊）、action（动作类型）、target_type（目标类型）、
//       start/end（时间范围）、page/page_size（分页，默认 20）。
func (h *Handler) ListLogs(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	action := strings.TrimSpace(c.Query("action"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	startDate := strings.TrimSpace(c.Query("start"))
	endDate := strings.TrimSpace(c.Query("end"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	q := h.db.Model(&model.OperationLog{})
	if keyword != "" {
		q = q.Where("username LIKE ? OR detail LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if targetType != "" {
		q = q.Where("target_type = ?", targetType)
	}
	if startDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", startDate, time.Local); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if endDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", endDate, time.Local); err == nil {
			q = q.Where("created_at < ?", t.Add(24*time.Hour))
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询日志失败")
		return
	}
	var logs []model.OperationLog
	if err := q.Order("id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询日志失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
