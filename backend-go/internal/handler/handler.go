package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meetingbackend/internal/config"
	"meetingbackend/internal/middleware"
)

// Handler 持有依赖：数据库、配置、认证
type Handler struct {
	db   *gorm.DB
	cfg  *config.Config
	auth *middleware.Auth
}

func New(db *gorm.DB, cfg *config.Config, auth *middleware.Auth) *Handler {
	return &Handler{db: db, cfg: cfg, auth: auth}
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

	// 需要登录
	user := api.Group("", h.auth.RequireUser())
	user.GET("/auth/me", h.Me)
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

	// 需要管理员
	admin := api.Group("", h.auth.RequireAdmin())
	admin.POST("/meetings", h.CreateMeeting)
	admin.PATCH("/meetings/:id", h.UpdateMeeting)
	admin.DELETE("/meetings/:id", h.DeleteMeeting)
	admin.POST("/meetings/:id/orgs", h.SetMeetingOrgs)
	admin.POST("/meetings/:id/material", h.GenerateMaterial)
	admin.POST("/meetings/:id/minutes", h.GenerateMinutes)
	admin.POST("/meetings/:id/start", h.StartMeeting)
	admin.POST("/meetings/:id/finish", h.FinishMeeting)
	admin.POST("/meetings/:id/archive", h.ArchiveMeeting)
	admin.GET("/logs", h.ListLogs)
	admin.POST("/orgs", h.CreateOrg)
	admin.PATCH("/orgs/:id", h.UpdateOrg)
	admin.DELETE("/orgs/:id", h.DeleteOrg)
	admin.GET("/users", h.ListUsers)
	admin.POST("/users", h.CreateUser)
	admin.PATCH("/users/:id", h.UpdateUser)
}
