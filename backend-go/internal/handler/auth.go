package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

const (
	maxAvatarSize      = 2 * 1024 * 1024 // 头像上限 2MB
	avatarExtWhitelist = ".jpg,.jpeg,.png,.webp"
)

// UserOut 用户输出（头像为公开信息，供列表/材料展示）
type UserOut struct {
	ID       uint                `json:"id"`
	Username string              `json:"username"`
	Name     string              `json:"name"`
	Role     string              `json:"role"`
	OrgID    *uint               `json:"org_id"`
	IsActive bool                `json:"is_active"`
	Avatar   string              `json:"avatar"` // 头像文件名
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
		Avatar:   u.Avatar,
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
	key := failKey(c, username)
	if h.tooManyFails(c, key, loginMaxFails, loginFailWindow, "登录失败次数过多，请 15 分钟后再试") {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：失败次数过多，已限流")
		return
	}
	if username == "" || password == "" {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：用户名或密码为空")
		badRequest(c, "用户名或密码错误")
		return
	}
	var user model.User
	if err := h.db.Preload("Org").Where("username = ?", username).First(&user).Error; err != nil {
		h.rateLimit.addFail(key)
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：用户不存在")
		badRequest(c, "用户名或密码错误")
		return
	}
	if !user.CheckPassword(password) {
		h.rateLimit.addFail(key)
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：密码错误")
		badRequest(c, "用户名或密码错误")
		return
	}
	if !user.IsActive {
		h.logLoginRecord(c, model.LogLoginFail, username, "登录失败：账号已停用")
		forbidden(c, "账号已停用，请联系管理员")
		return
	}
	h.rateLimit.clear(key)
	token, err := h.auth.CreateToken(user.ID, user.TokenVersion)
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

// Me GET /api/auth/me（返回本人信息，含密码提示词与安全问题）
func (h *Handler) Me(c *gin.Context) {
	ctx := currentUser(c)
	var user model.User
	if err := h.db.Preload("Org").First(&user, ctx.ID).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	out := toUserOut(&user)
	c.JSON(http.StatusOK, gin.H{
		"id":                out.ID,
		"username":          out.Username,
		"name":              out.Name,
		"role":              out.Role,
		"org_id":            out.OrgID,
		"is_active":         out.IsActive,
		"avatar":            out.Avatar,
		"org":               out.Org,
		"password_hint":     user.PasswordHint,
		"security_question": user.SecurityQuestion,
	})
}

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword PATCH /api/auth/password 修改本人密码（所有登录用户，需验证原密码）
func (h *Handler) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if len(req.NewPassword) < 6 {
		badRequest(c, "新密码至少 6 位")
		return
	}
	if req.OldPassword == req.NewPassword {
		badRequest(c, "新密码不能与原密码相同")
		return
	}
	ctx := currentUser(c)
	var user model.User
	if err := h.db.First(&user, ctx.ID).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	if !user.CheckPassword(req.OldPassword) {
		badRequest(c, "原密码错误")
		return
	}
	if err := user.SetPassword(req.NewPassword); err != nil {
		fail(c, http.StatusInternalServerError, "设置密码失败")
		return
	}
	user.RevokeTokens() // 旧令牌全部失效（其它设备需重新登录）
	if err := h.db.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存密码失败")
		return
	}
	h.auth.InvalidateUser(user.ID)
	out := gin.H{"ok": true}
	// 为当前设备换发新令牌，避免刚改完密码就被踢下线；换发失败不影响密码已生效
	if newToken, terr := h.auth.CreateToken(user.ID, user.TokenVersion); terr == nil {
		out["access_token"] = newToken
	}
	h.logRecord(c, model.LogPasswordChange, "user", user.ID, "修改密码："+user.Username)
	c.JSON(http.StatusOK, out)
}

// UpdateProfile PATCH /api/auth/profile 自助修改昵称（所有登录用户）
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(c, "昵称不能为空")
		return
	}
	ctx := currentUser(c)
	var user model.User
	if err := h.db.First(&user, ctx.ID).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	user.Name = req.Name
	if err := h.db.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存昵称失败")
		return
	}
	h.logRecord(c, model.LogProfileUpdate, "user", user.ID, "修改昵称："+user.Username)
	h.auth.InvalidateUser(user.ID) // 缓存中的昵称立即失效，操作日志不会还记旧昵称
	c.JSON(http.StatusOK, toUserOut(&user))
}

// UpdateSecurity PATCH /api/auth/security 自助设置密码提示词与安全问答（所有登录用户）
// password_hint：密码提示词；security_question：安全问题；security_answer：答案（为空则清空安全问答）。
func (h *Handler) UpdateSecurity(c *gin.Context) {
	var req struct {
		PasswordHint     string `json:"password_hint"`
		SecurityQuestion string `json:"security_question"`
		SecurityAnswer   string `json:"security_answer"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	ctx := currentUser(c)
	var user model.User
	if err := h.db.First(&user, ctx.ID).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}

	question := strings.TrimSpace(req.SecurityQuestion)
	answer := req.SecurityAnswer

	// 若提供了答案则必须同时提供问题；反之仅更新提示词
	if answer != "" && question == "" {
		badRequest(c, "设置安全答案时必须先选择或填写安全问题")
		return
	}
	user.PasswordHint = strings.TrimSpace(req.PasswordHint)
	if question == "" {
		// 清空安全问答
		user.SecurityQuestion = ""
		user.SecurityAnswer = ""
	} else {
		user.SecurityQuestion = question
		if answer != "" {
			user.SetSecurityAnswer(answer)
		}
	}

	if err := h.db.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存安全设置失败")
		return
	}
	h.logRecord(c, model.LogProfileUpdate, "user", user.ID, "更新密码提示词/安全问答："+user.Username)
	c.JSON(http.StatusOK, gin.H{
		"ok":                true,
		"password_hint":     user.PasswordHint,
		"security_question": user.SecurityQuestion,
	})
}

// UploadAvatar POST /api/auth/avatar 上传头像（所有登录用户，multipart 字段 file）
func (h *Handler) UploadAvatar(c *gin.Context) {
	ctx := currentUser(c)
	limitUploadBody(c, maxAvatarSize)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			badRequest(c, "头像大小不能超过 2MB")
			return
		}
		badRequest(c, "未接收到文件或文件字段名应为 file")
		return
	}
	defer file.Close()

	if header.Size > maxAvatarSize {
		badRequest(c, "头像大小不能超过 2MB")
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !strings.Contains(avatarExtWhitelist, ext) {
		badRequest(c, "头像仅支持 jpg / jpeg / png / webp 格式")
		return
	}

	dir := filepath.Join(h.cfg.UploadDir, "avatars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, http.StatusInternalServerError, "创建头像目录失败")
		return
	}
	// 用用户 ID + 时间戳命名，避免浏览器缓存旧头像
	storedName := fmt.Sprintf("%d_%d%s", ctx.ID, time.Now().UnixNano(), ext)
	dest := filepath.Join(dir, storedName)
	if err := c.SaveUploadedFile(header, dest); err != nil {
		os.Remove(dest)
		if isBodyTooLarge(err) {
			badRequest(c, "头像大小不能超过 2MB")
			return
		}
		fail(c, http.StatusInternalServerError, "保存头像失败")
		return
	}
	// header.Size 是客户端声明值；以磁盘实际写入大小为准再校验一次
	info, statErr := os.Stat(dest)
	if statErr != nil {
		os.Remove(dest)
		fail(c, http.StatusInternalServerError, "保存头像失败")
		return
	}
	if info.Size() > maxAvatarSize {
		os.Remove(dest)
		badRequest(c, "头像大小不能超过 2MB")
		return
	}

	var user model.User
	if err := h.db.First(&user, ctx.ID).Error; err != nil {
		os.Remove(dest)
		notFound(c, "用户不存在")
		return
	}
	// 删除旧头像文件
	if user.Avatar != "" {
		oldPath := filepath.Join(dir, user.Avatar)
		_ = os.Remove(oldPath)
	}
	user.Avatar = storedName
	if err := h.db.Save(&user).Error; err != nil {
		os.Remove(dest)
		fail(c, http.StatusInternalServerError, "保存头像失败")
		return
	}
	h.auth.InvalidateUser(ctx.ID)
	h.logRecord(c, model.LogAvatarUpdate, "user", user.ID, "上传头像："+user.Username)
	c.JSON(http.StatusOK, gin.H{"avatar": storedName})
}

// GetAvatar GET /api/avatar/:name 读取头像文件（公开，头像非敏感）
func (h *Handler) GetAvatar(c *gin.Context) {
	name := filepath.Base(c.Param("name")) // 防路径穿越
	if name == "." || name == "" {
		notFound(c, "未设置头像")
		return
	}
	path := filepath.Join(h.cfg.UploadDir, "avatars", name)
	if _, err := os.Stat(path); err != nil {
		notFound(c, "头像文件不存在")
		return
	}
	c.File(path)
}

// ForgotQuestion POST /api/auth/forgot-question 找回密码第一步：获取密码提示词与安全问题（公开）
func (h *Handler) ForgotQuestion(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	username := strings.TrimSpace(req.Username)
	key := failKey(c, username)
	if h.tooManyFails(c, key, resetMaxFails, resetFailWindow, "操作过于频繁，请 30 分钟后再试或直接联系管理员重置密码") {
		return
	}
	var user model.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		h.rateLimit.addFail(key)
		// 不泄露用户是否存在，统一返回
		c.JSON(http.StatusOK, gin.H{"security_question": "", "password_hint": ""})
		return
	}
	h.rateLimit.clear(key)
	c.JSON(http.StatusOK, gin.H{
		"security_question": user.SecurityQuestion,
		"password_hint":     user.PasswordHint,
	})
}

// ForgotReset POST /api/auth/forgot-reset 找回密码第二步：校验安全答案并重置密码（公开）
func (h *Handler) ForgotReset(c *gin.Context) {
	var req struct {
		Username       string `json:"username" binding:"required"`
		SecurityAnswer string `json:"security_answer" binding:"required"`
		NewPassword    string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if len(req.NewPassword) < 6 {
		badRequest(c, "新密码至少 6 位")
		return
	}
	username := strings.TrimSpace(req.Username)
	key := failKey(c, username)
	if h.tooManyFails(c, key, resetMaxFails, resetFailWindow, "尝试次数过多，请 30 分钟后再试或直接联系管理员重置密码") {
		return
	}
	var user model.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		h.rateLimit.addFail(key)
		// 与「答案错误」使用同一文案，不泄露账号是否存在
		badRequest(c, "用户名或安全答案错误")
		return
	}
	if user.SecurityQuestion == "" || !user.CheckSecurityAnswer(req.SecurityAnswer) {
		h.rateLimit.addFail(key)
		badRequest(c, "用户名或安全答案错误")
		return
	}
	if err := user.SetPassword(req.NewPassword); err != nil {
		fail(c, http.StatusInternalServerError, "设置密码失败")
		return
	}
	user.RevokeTokens() // 重置成功后使旧令牌全部失效
	if err := h.db.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存密码失败")
		return
	}
	h.rateLimit.clear(key)
	h.auth.InvalidateUser(user.ID)
	h.logRecord(c, model.LogPasswordReset, "user", user.ID, "安全问答重置密码："+user.Username)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
