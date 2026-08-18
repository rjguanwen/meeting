package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

type orgReq struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type" binding:"required"` // dept / team
	ParentID  *uint  `json:"parent_id"`
	SortOrder int    `json:"sort_order"`
}

// OrgTree GET /api/orgs/tree 组织树
func (h *Handler) OrgTree(c *gin.Context) {
	var orgs []*model.Organization
	if err := h.db.Order("type desc, sort_order asc, id asc").Find(&orgs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询组织失败")
		return
	}
	c.JSON(http.StatusOK, model.WithOrganizationTree(orgs))
}

// CreateOrg POST /api/orgs 创建组织（管理员）
func (h *Handler) CreateOrg(c *gin.Context) {
	var req orgReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Type != model.OrgTypeDept && req.Type != model.OrgTypeTeam {
		badRequest(c, "组织类型只能是 dept 或 team")
		return
	}
	if req.Type == model.OrgTypeDept && req.ParentID != nil {
		badRequest(c, "部门不能挂在其他组织下")
		return
	}
	if req.Type == model.OrgTypeTeam && req.ParentID == nil {
		badRequest(c, "小组必须指定所属部门")
		return
	}
	org := &model.Organization{
		Name:      req.Name,
		Type:      req.Type,
		ParentID:  req.ParentID,
		SortOrder: req.SortOrder,
	}
	if err := h.db.Create(org).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建组织失败")
		return
	}
	h.logRecord(c, model.LogOrgCreate, "org", org.ID, "创建组织："+org.Name)
	c.JSON(http.StatusCreated, org)
}

// UpdateOrg PATCH /api/orgs/:id 修改组织（管理员）
func (h *Handler) UpdateOrg(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "组织 ID 不合法")
		return
	}
	var org model.Organization
	if err := h.db.First(&org, id).Error; err != nil {
		notFound(c, "组织不存在")
		return
	}
	var req orgReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Type != model.OrgTypeDept && req.Type != model.OrgTypeTeam {
		badRequest(c, "组织类型只能是 dept 或 team")
		return
	}
	if org.Type == model.OrgTypeDept && req.ParentID != nil {
		badRequest(c, "部门不能挂在其他组织下")
		return
	}
	oldName := org.Name
	org.Name = req.Name
	org.SortOrder = req.SortOrder
	if err := h.db.Save(&org).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存组织失败")
		return
	}
	h.logRecord(c, model.LogOrgUpdate, "org", org.ID, "修改组织："+oldName+" → "+org.Name)
	c.JSON(http.StatusOK, org)
}

// DeleteOrg DELETE /api/orgs/:id 删除组织（管理员）
func (h *Handler) DeleteOrg(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "组织 ID 不合法")
		return
	}
	var org model.Organization
	if err := h.db.First(&org, id).Error; err != nil {
		notFound(c, "组织不存在")
		return
	}
	// 禁止删除已有子组织的部门
	var children int64
	h.db.Model(&model.Organization{}).Where("parent_id = ?", id).Count(&children)
	if children > 0 {
		badRequest(c, "请先删除该部门下的所有小组")
		return
	}
	// 禁止删除已有账号的组织
	var users int64
	h.db.Model(&model.User{}).Where("org_id = ?", id).Count(&users)
	if users > 0 {
		badRequest(c, "该组织已有关联账号，无法删除")
		return
	}
	if err := h.db.Delete(&org).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除组织失败")
		return
	}
	h.logRecord(c, model.LogOrgDelete, "org", org.ID, "删除组织："+org.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
