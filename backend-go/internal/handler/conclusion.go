package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

type conclusionReq struct {
	ReportItemID uint            `json:"report_item_id" binding:"required"`
	Kind         string          `json:"kind" binding:"required"` // conclusion / action
	Content      string          `json:"content" binding:"required"`
	Owner        string          `json:"owner"`
	DueDate      *model.DateTime `json:"due_date"`
}

// ListConclusions GET /api/meetings/:id/conclusions 结论列表
func (h *Handler) ListConclusions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议 ID 不合法")
		return
	}
	var meeting model.Meeting
	if err := h.db.First(&meeting, id).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "无权查看该会议内容")
		return
	}
	var conclusions []model.Conclusion
	if err := h.db.Where("meeting_id = ?", id).Order("report_item_id asc, id asc").Find(&conclusions).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询讨论结论失败")
		return
	}
	c.JSON(http.StatusOK, conclusions)
}

// CreateConclusion POST /api/meetings/:id/conclusions 添加讨论结论/新任务
func (h *Handler) CreateConclusion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议 ID 不合法")
		return
	}
	var meeting model.Meeting
	if err := h.db.First(&meeting, id).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if meeting.Status != model.MeetingOngoing {
		badRequest(c, "仅进行中的会议可以录入讨论结论")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	var req conclusionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Kind != model.ConclusionKind && req.Kind != model.ActionKind {
		badRequest(c, "类型只能是 conclusion 或 action")
		return
	}
	// 校验挂载的汇报事项属于该会议
	var item model.ReportItem
	if err := h.db.Where("id = ? AND meeting_id = ?", req.ReportItemID, id).First(&item).Error; err != nil {
		badRequest(c, "汇报事项不存在或不属于该会议")
		return
	}
	conc := &model.Conclusion{
		MeetingID:    uint(id),
		ReportItemID: req.ReportItemID,
		Kind:         req.Kind,
		Content:      req.Content,
		Owner:        req.Owner,
	}
	if req.DueDate != nil {
		conc.DueDate = req.DueDate
	}
	if err := h.db.Create(conc).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存讨论结论失败")
		return
	}
	h.logRecord(c, model.LogConclusion, "conclusion", conc.ID,
		"添加"+conclusionKindText(req.Kind)+"："+conc.Content)
	c.JSON(http.StatusCreated, conc)
}

// UpdateConclusion PATCH /api/conclusions/:id 修改结论
func (h *Handler) UpdateConclusion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "结论 ID 不合法")
		return
	}
	var conc model.Conclusion
	if err := h.db.First(&conc, id).Error; err != nil {
		notFound(c, "讨论结论不存在")
		return
	}
	var meeting model.Meeting
	if err := h.db.First(&meeting, conc.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if !h.conclusionEditable(conc.MeetingID) {
		badRequest(c, "仅进行中的会议可以修改讨论结论")
		return
	}
	var req conclusionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Kind != model.ConclusionKind && req.Kind != model.ActionKind {
		badRequest(c, "类型只能是 conclusion 或 action")
		return
	}
	conc.Kind = req.Kind
	conc.Content = req.Content
	conc.Owner = req.Owner
	if req.DueDate != nil {
		conc.DueDate = req.DueDate
	} else {
		conc.DueDate = nil
	}
	if err := h.db.Save(&conc).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存讨论结论失败")
		return
	}
	h.logRecord(c, model.LogConclusionUpd, "conclusion", conc.ID,
		"修改"+conclusionKindText(conc.Kind)+"："+conc.Content)
	c.JSON(http.StatusOK, conc)
}

// DeleteConclusion DELETE /api/conclusions/:id 删除结论
func (h *Handler) DeleteConclusion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "结论 ID 不合法")
		return
	}
	var conc model.Conclusion
	if err := h.db.First(&conc, id).Error; err != nil {
		notFound(c, "讨论结论不存在")
		return
	}
	var meeting model.Meeting
	if err := h.db.First(&meeting, conc.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if !h.conclusionEditable(conc.MeetingID) {
		badRequest(c, "仅进行中的会议可以删除讨论结论")
		return
	}
	if err := h.db.Delete(&conc).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除讨论结论失败")
		return
	}
	h.logRecord(c, model.LogConclusionDel, "conclusion", conc.ID, "删除"+conclusionKindText(conc.Kind))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// conclusionKindText 结论类型中文名
func conclusionKindText(kind string) string {
	if kind == model.ActionKind {
		return "新任务"
	}
	return "讨论结论"
}

// conclusionEditable 结论是否可编辑：仅进行中允许
func (h *Handler) conclusionEditable(meetingID uint) bool {
	var meeting model.Meeting
	if err := h.db.First(&meeting, meetingID).Error; err != nil {
		return false
	}
	return meeting.Status == model.MeetingOngoing
}
