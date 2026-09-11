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

// validateUserRole 校验角色合法性及组织绑定关系，返回错误信息（空串表示合法）。
func (h *Handler) validateUserRole(role string, orgID *uint, excludeUserID uint) string {
	switch role {
	case model.RoleAdmin:
		if orgID != nil {
			return "管理员不需要绑定所属组织"
		}
		return ""
	case model.RoleDeptLeader, model.RoleTeamLeader:
		if orgID == nil {
			return "负责人必须指定所属组织"
		}
		var org model.Organization
		if err := h.db.First(&org, *orgID).Error; err != nil {
			return "所属组织不存在"
		}
		// 部门负责人须绑定部门，小组负责人须绑定小组
		want := model.OrgTypeDept
		if role == model.RoleTeamLeader {
			want = model.OrgTypeTeam
		}
		if org.Type != want {
			if role == model.RoleDeptLeader {
				return "部门负责人必须选择部门"
			}
			return "小组负责人必须选择小组"
		}
		// 一个部门/小组可设置多个负责人，无需唯一性校验
		_ = excludeUserID
		return ""
	case model.RoleMember:
		if orgID == nil {
			return "组织成员必须指定所属组织"
		}
		var org model.Organization
		if err := h.db.First(&org, *orgID).Error; err != nil {
			return "所属组织不存在"
		}
		return ""
	default:
		return "角色只能是 admin / dept_leader / team_leader / member"
	}
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
		role = model.RoleMember
	}
	if msg := h.validateUserRole(role, req.OrgID, 0); msg != "" {
		badRequest(c, msg)
		return
	}
	var count int64
	h.db.Model(&model.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		badRequest(c, "用户名已存在")
		return
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
	// 角色与所属组织可调整
	if req.Role != "" {
		role := req.Role
		if msg := h.validateUserRole(role, req.OrgID, user.ID); msg != "" {
			badRequest(c, msg)
			return
		}
		user.Role = role
		user.OrgID = req.OrgID
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
	h.auth.InvalidateUser(user.ID) // 角色/状态变更立即生效，无需等待缓存 TTL
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
