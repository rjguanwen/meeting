package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meetingbackend/internal/config"
	"meetingbackend/internal/middleware"
	"meetingbackend/internal/model"
	"meetingbackend/internal/wedrive"
)

// Handler 持有依赖：数据库、配置、认证
type Handler struct {
	db   *gorm.DB
	cfg  *config.Config
	auth *middleware.Auth

	wedrive *wedrive.Client // 企业微信微盘客户端；未启用时为 nil

	logCh   chan *model.OperationLog // 操作日志异步写队列
	logDone chan struct{}            // 关闭信号：触发日志 worker 退出并 flush
}

// logQueueSize 日志异步队列容量；logBatchSize 批量写入条数；logFlushInterval 定时刷新间隔。
const (
	logQueueSize    = 1024
	logBatchSize    = 100
	logFlushInterval = 200 * time.Millisecond
)

func New(db *gorm.DB, cfg *config.Config, auth *middleware.Auth) *Handler {
	h := &Handler{
		db:      db,
		cfg:     cfg,
		auth:    auth,
		logCh:   make(chan *model.OperationLog, logQueueSize),
		logDone: make(chan struct{}),
	}
	if wc := cfg.WeCom; wc.AutoUpload && wc.APIBase != "" && wc.CorpID != "" && wc.CorpSecret != "" && wc.SpaceID != "" {
		h.wedrive = wedrive.New(wedrive.Config{
			APIBase:     wc.APIBase,
			APIPrefix:   wc.APIPrefix,
			CorpID:      wc.CorpID,
			CorpSecret:  wc.CorpSecret,
			SpaceID:     wc.SpaceID,
			FolderID:    wc.FolderID,
			TLSInsecure: wc.TLSInsecure,
			Timeout:     time.Duration(wc.TimeoutSeconds) * time.Second,
		})
	}
	go h.logWorker()
	return h
}

// Close 优雅关闭：停止日志 worker 并 flush 剩余日志（进程退出前调用）。
func (h *Handler) Close() {
	close(h.logDone)
}

// currentUser 从上下文获取当前登录用户
func currentUser(c *gin.Context) *middleware.UserContext {
	v, ok := c.Get("user")
	if !ok {
		return nil
	}
	return v.(*middleware.UserContext)
}

// RegisterRoutes 注册全部路由
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")

	// 公开接口
	api.POST("/auth/login", h.Login)
	api.GET("/avatar/:name", h.GetAvatar)
	api.POST("/auth/forgot-question", h.ForgotQuestion)
	api.POST("/auth/forgot-reset", h.ForgotReset)

	// 需要登录
	user := api.Group("", h.auth.RequireUser())
	user.GET("/auth/me", h.Me)
	user.PATCH("/auth/password", h.ChangePassword)
	user.PATCH("/auth/profile", h.UpdateProfile)
	user.PATCH("/auth/security", h.UpdateSecurity)
	user.POST("/auth/avatar", h.UploadAvatar)
	user.GET("/meetings", h.ListMeetings)
	user.GET("/meetings/:id", h.GetMeeting)
	user.GET("/meetings/:id/items", h.ListItems)
	user.POST("/meetings/:id/items", h.CreateItem)
	user.PATCH("/items/:id", h.UpdateItem)
	user.DELETE("/items/:id", h.DeleteItem)
	user.POST("/items/:id/attachments", h.UploadAttachment)
	user.DELETE("/attachments/:id", h.DeleteAttachment)
	user.POST("/meetings/:id/conclusions", h.CreateConclusion)
	user.GET("/meetings/:id/conclusions", h.ListConclusions)
	user.PATCH("/conclusions/:id", h.UpdateConclusion)
	user.DELETE("/conclusions/:id", h.DeleteConclusion)
	user.GET("/meetings/:id/minutes", h.GetMinutes)
	user.GET("/meetings/:id/material", h.GetMaterial) // 只读查看材料
	user.GET("/items/:id/attachments", h.ListAttachments)
	user.GET("/attachments/:id/file", h.GetAttachmentFile)
	user.GET("/orgs/tree", h.OrgTree)
	user.GET("/rooms", h.ListRooms)

	// 需要管理员
	admin := api.Group("", h.auth.RequireAdmin())
	admin.POST("/meetings", h.CreateMeeting)
	admin.PATCH("/meetings/:id", h.UpdateMeeting)
	admin.DELETE("/meetings/:id", h.DeleteMeeting)
	admin.POST("/meetings/:id/orgs", h.SetMeetingOrgs)
	admin.POST("/meetings/:id/material", h.GenerateMaterial)
	admin.POST("/meetings/:id/minutes", h.GenerateMinutes)
	admin.PATCH("/meetings/:id/minutes", h.UpdateMinutes)
	admin.POST("/meetings/:id/start", h.StartMeeting)
	admin.POST("/meetings/:id/finish", h.FinishMeeting)
	admin.POST("/meetings/:id/archive", h.ArchiveMeeting)
	admin.GET("/logs", h.ListLogs)
	admin.POST("/rooms", h.CreateRoom)
	admin.PATCH("/rooms/:id", h.UpdateRoom)
	admin.DELETE("/rooms/:id", h.DeleteRoom)
	admin.POST("/orgs", h.CreateOrg)
	admin.PATCH("/orgs/:id", h.UpdateOrg)
	admin.DELETE("/orgs/:id", h.DeleteOrg)
	admin.GET("/users", h.ListUsers)
	admin.POST("/users", h.CreateUser)
	admin.PATCH("/users/:id", h.UpdateUser)
}
