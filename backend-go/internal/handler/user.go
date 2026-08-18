package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"meetingbackend/internal/model"
)

type userReq struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
	OrgID    *uint  `json:"org_id"`
	IsActive *bool  `json:"is_active"`
}

// ListUsers GET /api/users 用户列表（管理员）
func (h *Handler) ListUsers(c *gin.Context) {
	var users []model.User
	if err := h.db.Preload("Org").Order("id asc").Find(&users).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询用户失败")
		return
	}
	out := make([]UserOut, 0, len(users))
	for i := range users {
		out = append(out, toUserOut(&users[i]))
	}
	c.JSON(http.StatusOK, out)
}

// CreateUser POST /api/users 创建用户（管理员）
func (h *Handler) CreateUser(c *gin.Context) {
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		badRequest(c, "用户名不能为空")
		return
	}
	if req.Password == "" || len(req.Password) < 6 {
		badRequest(c, "密码至少 6 位")
		return
	}
	role := req.Role
	if role == "" {
		role = model.RoleLeader
	}
	if role != model.RoleAdmin && role != model.RoleLeader {
		badRequest(c, "角色只能是 admin 或 leader")
		return
	}
	if role == model.RoleLeader && req.OrgID == nil {
		badRequest(c, "组织负责人必须指定所属组织")
		return
	}
	var count int64
	h.db.Model(&model.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		badRequest(c, "用户名已存在")
		return
	}
	// 组织负责人账号必须绑定存在的部门或小组
	if req.OrgID != nil {
		var org model.Organization
		if err := h.db.First(&org, *req.OrgID).Error; err != nil {
			badRequest(c, "所属组织不存在")
			return
		}
		h.db.Model(&model.User{}).Where("org_id = ? AND role = ?", *req.OrgID, model.RoleLeader).Count(&count)
		if count > 0 {
			badRequest(c, "该组织已存在负责人账号")
			return
		}
	}
	user := &model.User{
		Username:       req.Username,
		HashedPassword: hashPassword(req.Password),
		Name:           req.Name,
		Role:           role,
		OrgID:          req.OrgID,
		IsActive:       true,
	}
	if err := h.db.Create(user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建用户失败")
		return
	}
	h.db.Preload("Org").First(user, user.ID)
	h.logRecord(c, model.LogUserCreate, "user", user.ID, "创建账号："+user.Username+"（"+user.Name+"）")
	c.JSON(http.StatusCreated, toUserOut(user))
}

// UpdateUser PATCH /api/users/:id 更新用户（管理员）
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "用户 ID 不合法")
		return
	}
	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		notFound(c, "用户不存在")
		return
	}
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Password != "" {
		if len(req.Password) < 6 {
			badRequest(c, "密码至少 6 位")
			return
		}
		if err := user.SetPassword(req.Password); err != nil {
			fail(c, http.StatusInternalServerError, "设置密码失败")
			return
		}
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if err := h.db.Save(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存用户失败")
		return
	}
	h.db.Preload("Org").First(&user, user.ID)
	h.logRecord(c, model.LogUserUpdate, "user", user.ID, "修改账号："+user.Username)
	c.JSON(http.StatusOK, toUserOut(&user))
}

func hashPassword(password string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hashed)
}
