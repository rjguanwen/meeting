package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

type meetingReq struct {
	Title       string          `json:"title" binding:"required"`
	Description string          `json:"description"`
	MeetingTime *model.DateTime `json:"meeting_time"`
}

// CreateMeeting POST /api/meetings 创建会议（管理员）
func (h *Handler) CreateMeeting(c *gin.Context) {
	var req meetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	ctx := currentUser(c)
	mt := time.Now().Add(24 * time.Hour)
	if req.MeetingTime != nil {
		mt = req.MeetingTime.Time
	}
	meeting := &model.Meeting{
		Title:       req.Title,
		Description: req.Description,
		CreatorID:   ctx.ID,
		MeetingTime: mt,
		Status:      model.MeetingDraft,
	}
	if err := h.db.Create(meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建会议失败")
		return
	}
	h.logRecord(c, model.LogMeetingCreate, "meeting", meeting.ID, "创建会议："+meeting.Title)
	c.JSON(http.StatusCreated, meeting)
}

// ListMeetings GET /api/meetings 会议列表
// admin 看全部；leader 只看自己组织参会的会议。
// 支持查询参数：keyword（名称模糊）、start/end（会议时间范围，2006-01-02）、status（状态）。
// 分页：page（>0 时启用分页，返回 {items, total}）、page_size（默认 10）。
// 兼容：page=0 时返回数组，limit（默认 10，0 表示不限）控制条数。
func (h *Handler) ListMeetings(c *gin.Context) {
	ctx := currentUser(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	startDate := strings.TrimSpace(c.Query("start"))
	endDate := strings.TrimSpace(c.Query("end"))
	status := strings.TrimSpace(c.Query("status"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	q := h.db.Model(&model.Meeting{})
	if keyword != "" {
		q = q.Where("title LIKE ?", "%"+keyword+"%")
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if startDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", startDate, time.Local); err == nil {
			q = q.Where("meeting_time >= ?", t)
		}
	}
	if endDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", endDate, time.Local); err == nil {
			q = q.Where("meeting_time < ?", t.Add(24*time.Hour))
		}
	}
	if ctx.Role == model.RoleLeader {
		if ctx.OrgID == nil {
			c.JSON(http.StatusOK, []model.Meeting{})
			return
		}
		orgIDs := []uint{*ctx.OrgID}
		sub := h.db.Model(&model.MeetingOrg{}).
			Where("org_id IN ?", orgIDs).
			Select("meeting_id")
		q = q.Where("id IN (?)", sub)
	}

	if page > 0 {
		var total int64
		if err := q.Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, "查询会议失败")
			return
		}
		var meetings []model.Meeting
		err := q.Preload("Orgs.Org").
			Order("meeting_time desc").
			Offset((page - 1) * pageSize).
			Limit(pageSize).
			Find(&meetings).Error
		if err != nil {
			fail(c, http.StatusInternalServerError, "查询会议失败")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     meetings,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
		return
	}

	if limit > 0 {
		q = q.Limit(limit)
	}
	var meetings []model.Meeting
	if err := q.Preload("Orgs.Org").Order("meeting_time desc").Find(&meetings).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询会议失败")
		return
	}
	c.JSON(http.StatusOK, meetings)
}

// GetMeeting GET /api/meetings/:id 会议详情（含参会组织、汇报事项）
func (h *Handler) GetMeeting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议 ID 不合法")
		return
	}
	var meeting model.Meeting
	if err := h.db.Preload("Orgs.Org").Preload("Items.Org").Preload("Items.Creator").
		First(&meeting, id).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	c.JSON(http.StatusOK, meeting)
}

// UpdateMeeting PATCH /api/meetings/:id 修改会议（管理员）
func (h *Handler) UpdateMeeting(c *gin.Context) {
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
	if meeting.Status == model.MeetingArchived {
		badRequest(c, "会议已归档，不能修改会议信息")
		return
	}
	var req meetingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if req.Title != "" {
		meeting.Title = req.Title
	}
	meeting.Description = req.Description
	if req.MeetingTime != nil {
		meeting.MeetingTime = req.MeetingTime.Time
	}
	if err := h.db.Save(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存会议失败")
		return
	}
	h.logRecord(c, model.LogMeetingUpdate, "meeting", meeting.ID, "修改会议："+meeting.Title)
	c.JSON(http.StatusOK, meeting)
}

// DeleteMeeting DELETE /api/meetings/:id 删除会议（管理员）
func (h *Handler) DeleteMeeting(c *gin.Context) {
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
	if meeting.Status == model.MeetingArchived {
		badRequest(c, "会议已归档，不允许删除")
		return
	}
	// 级联清理
	h.db.Where("meeting_id = ?", id).Delete(&model.MeetingOrg{})
	h.db.Where("meeting_id = ?", id).Delete(&model.ReportItem{})
	h.db.Where("meeting_id = ?", id).Delete(&model.Conclusion{})
	h.db.Where("meeting_id = ?", id).Delete(&model.MeetingMinutes{})
	if err := h.db.Delete(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除会议失败")
		return
	}
	h.logRecord(c, model.LogMeetingDelete, "meeting", meeting.ID, "删除会议："+meeting.Title)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type meetingOrgsReq struct {
	OrgIDs []uint `json:"org_ids" binding:"required"`
}

// SetMeetingOrgs POST /api/meetings/:id/orgs 设置参会组织（管理员，覆盖式）
func (h *Handler) SetMeetingOrgs(c *gin.Context) {
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
	if meeting.Status != model.MeetingDraft {
		badRequest(c, "仅筹备中的会议可以设置参会组织")
		return
	}
	var req meetingOrgsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	if len(req.OrgIDs) == 0 {
		badRequest(c, "请至少指定一个参会组织")
		return
	}
	// 校验组织存在且唯一
	seen := make(map[uint]bool)
	for _, oid := range req.OrgIDs {
		if seen[oid] {
			continue
		}
		seen[oid] = true
		var org model.Organization
		if err := h.db.First(&org, oid).Error; err != nil {
			badRequest(c, "参会组织不存在")
			return
		}
	}
	if err := h.db.Where("meeting_id = ?", id).Delete(&model.MeetingOrg{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新参会组织失败")
		return
	}
	sort := 0
	for _, oid := range req.OrgIDs {
		if !seen[oid] {
			continue
		}
		seen[oid] = false
		mo := &model.MeetingOrg{MeetingID: uint(id), OrgID: oid, SortOrder: sort}
		if err := h.db.Create(mo).Error; err != nil {
			fail(c, http.StatusInternalServerError, "保存参会组织失败")
			return
		}
		sort++
	}
	var moList []*model.MeetingOrg
	h.db.Preload("Org").Where("meeting_id = ?", id).Order("sort_order asc").Find(&moList)
	names := make([]string, 0, len(moList))
	for _, mo := range moList {
		if mo.Org != nil {
			names = append(names, mo.Org.Name)
		}
	}
	h.logRecord(c, model.LogMeetingOrgs, "meeting", meeting.ID,
		"设置会议「"+meeting.Title+"」参会组织："+strings.Join(names, "、"))
	c.JSON(http.StatusOK, moList)
}

// StartMeeting POST /api/meetings/:id/start 开始会议（管理员）
// 状态流转：draft -> ongoing
func (h *Handler) StartMeeting(c *gin.Context) {
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
	if meeting.Status != model.MeetingDraft {
		badRequest(c, "仅筹备中的会议可以开始")
		return
	}
	meeting.Status = model.MeetingOngoing
	if err := h.db.Save(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新会议状态失败")
		return
	}
	h.logRecord(c, model.LogMeetingStart, "meeting", meeting.ID, "开始会议："+meeting.Title)
	c.JSON(http.StatusOK, meeting)
}

// FinishMeeting POST /api/meetings/:id/finish 结束会议（管理员）
// 状态流转：ongoing -> finished
func (h *Handler) FinishMeeting(c *gin.Context) {
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
		badRequest(c, "仅进行中的会议可以结束")
		return
	}
	meeting.Status = model.MeetingFinished
	if err := h.db.Save(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新会议状态失败")
		return
	}
	h.logRecord(c, model.LogMeetingFinish, "meeting", meeting.ID, "结束会议："+meeting.Title)
	c.JSON(http.StatusOK, meeting)
}

// ArchiveMeeting POST /api/meetings/:id/archive 归档会议（管理员）
// 状态流转：draft / ongoing / finished -> archived
// 归档后材料与纪要只读。
func (h *Handler) ArchiveMeeting(c *gin.Context) {
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
	if meeting.Status == model.MeetingArchived {
		badRequest(c, "会议已归档")
		return
	}
	meeting.Status = model.MeetingArchived
	if err := h.db.Save(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新会议状态失败")
		return
	}
	h.logRecord(c, model.LogMeetingArchive, "meeting", meeting.ID, "归档会议："+meeting.Title)
	c.JSON(http.StatusOK, meeting)
}
