package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

type itemReq struct {
	OrgID     uint   `json:"org_id" binding:"required"`
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content"`
	SortOrder int    `json:"sort_order"`
}

// ListItems GET /api/meetings/:id/items 汇报事项列表
// leader 只能看自己组织的；admin 看全部。
func (h *Handler) ListItems(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议 ID 不合法")
		return
	}
	ctx := currentUser(c)
	var meeting model.Meeting
	if err := h.db.First(&meeting, id).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "无权查看该会议内容")
		return
	}
	q := h.db.Preload("Org").Preload("Creator").Where("meeting_id = ?", id)
	if model.IsOrgRole(ctx.Role) {
		if ctx.OrgID == nil {
			c.JSON(http.StatusOK, []model.ReportItem{})
			return
		}
		q = q.Where("org_id = ?", *ctx.OrgID)
	}
	var items []model.ReportItem
	if err := q.Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询汇报事项失败")
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateItem POST /api/meetings/:id/items 录入汇报事项
// leader 只能给自己组织录；admin 可给任意参会组织录。
func (h *Handler) CreateItem(c *gin.Context) {
	mid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议 ID 不合法")
		return
	}
	var meeting model.Meeting
	if err := h.db.First(&meeting, mid).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if meeting.Status != model.MeetingDraft {
		badRequest(c, "会议材料已生成或会议已结束，不能再录入汇报事项")
		return
	}
	var req itemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	// 会议创建者可为其创建的会议录入汇报（可挂任意参会组织）
	isCreator := meeting.CreatorID == ctx.ID
	if model.IsOrgRole(ctx.Role) && !isCreator {
		if ctx.OrgID == nil || *ctx.OrgID != req.OrgID {
			forbidden(c, "只能为本人所属组织录入汇报事项")
			return
		}
	}
	// 校验组织参会
	var mo model.MeetingOrg
	if err := h.db.Where("meeting_id = ? AND org_id = ?", mid, req.OrgID).First(&mo).Error; err != nil {
		badRequest(c, "该组织未参加此会议")
		return
	}
	item := &model.ReportItem{
		MeetingID: uint(mid),
		OrgID:     req.OrgID,
		CreatorID: ctx.ID,
		Title:     req.Title,
		Content:   req.Content,
		Status:    model.ItemSubmitted,
		SortOrder: req.SortOrder,
	}
	if err := h.db.Create(item).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存汇报事项失败")
		return
	}
	h.db.Preload("Org").Preload("Creator").First(item, item.ID)
	h.logRecord(c, model.LogItemCreate, "item", item.ID, "录入汇报事项：「"+item.Title+"」")
	c.JSON(http.StatusCreated, item)
}

// UpdateItem PATCH /api/items/:id 修改汇报事项
func (h *Handler) UpdateItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "事项 ID 不合法")
		return
	}
	var item model.ReportItem
	if err := h.db.First(&item, id).Error; err != nil {
		notFound(c, "汇报事项不存在")
		return
	}
	// 校验所属会议仍处于草稿状态
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err == nil && meeting.Status != model.MeetingDraft {
		badRequest(c, "会议材料已生成或会议已结束，不能再修改汇报事项")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if model.IsOrgRole(ctx.Role) {
		if ctx.OrgID == nil || *ctx.OrgID != item.OrgID {
			forbidden(c, "只能修改本人所属组织的汇报事项")
			return
		}
	}
	var req itemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if model.IsOrgRole(ctx.Role) {
		req.OrgID = item.OrgID
	}
	item.Title = req.Title
	item.Content = req.Content
	item.SortOrder = req.SortOrder
	if err := h.db.Save(&item).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存汇报事项失败")
		return
	}
	h.db.Preload("Org").Preload("Creator").First(&item, item.ID)
	h.logRecord(c, model.LogItemUpdate, "item", item.ID, "修改汇报事项：「"+item.Title+"」")
	c.JSON(http.StatusOK, item)
}

// DeleteItem DELETE /api/items/:id 删除汇报事项
func (h *Handler) DeleteItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "事项 ID 不合法")
		return
	}
	var item model.ReportItem
	if err := h.db.First(&item, id).Error; err != nil {
		notFound(c, "汇报事项不存在")
		return
	}
	// 校验所属会议仍处于草稿状态
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err == nil && meeting.Status != model.MeetingDraft {
		badRequest(c, "会议材料已生成或会议已结束，不能再删除汇报事项")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if model.IsOrgRole(ctx.Role) {
		if ctx.OrgID == nil || *ctx.OrgID != item.OrgID {
			forbidden(c, "只能删除本人所属组织的汇报事项")
			return
		}
	}
	h.db.Where("report_item_id = ?", id).Delete(&model.Conclusion{})
	if err := h.db.Delete(&item).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除汇报事项失败")
		return
	}
	h.logRecord(c, model.LogItemDelete, "item", item.ID, "删除汇报事项：「"+item.Title+"」")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
