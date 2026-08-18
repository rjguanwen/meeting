package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

// UserOut 用户输出
type UserOut struct {
	ID       uint                `json:"id"`
	Username string              `json:"username"`
	Name     string              `json:"name"`
	Role     string              `json:"role"`
	OrgID    *uint               `json:"org_id"`
	IsActive bool                `json:"is_active"`
	Org      *model.Organization `json:"org,omitempty"`
}

type TokenOut struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	User        UserOut `json:"user"`
}

func toUserOut(u *model.User) UserOut {
	out := UserOut{
		ID:       u.ID,
		Username: u.Username,
		Name:     u.Name,
		Role:     u.Role,
		OrgID:    u.OrgID,
		IsActive: u.IsActive,
	}
	if u.Org != nil {
		out.Org = u.Org
	}
	return out
}

// Login POST /api/auth/login (OAuth2 form)
func (h *Handler) Login(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	if username == "" || password == "" {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：用户名或密码为空")
		badRequest(c, "用户名或密码错误")
		return
	}
	var user model.User
	if err := h.db.Preload("Org").Where("username = ?", username).First(&user).Error; err != nil {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：用户不存在")
		badRequest(c, "用户名或密码错误")
		return
	}
	if !user.CheckPassword(password) {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：密码错误")
		badRequest(c, "用户名或密码错误")
		return
	}
	if !user.IsActive {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：账号已停用")
		forbidden(c, "账号已停用，请联系管理员")
		return
	}
	token, err := h.auth.CreateToken(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	h.logLoginRecord(c, model.LogLogin, username, "登录成功")
	c.JSON(http.StatusOK, TokenOut{
		AccessToken: token,
		TokenType:   "bearer",
		User:        toUserOut(&user),
	})
}

// Me GET /api/auth/me
func (h *Handler) Me(c *gin.Context) {
	ctx := currentUser(c)
	var user model.User
	if err := h.db.Preload("Org").First(&user, ctx.ID).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	c.JSON(http.StatusOK, toUserOut(&user))
}
