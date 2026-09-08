package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meetingbackend/internal/model"
)

type meetingReq struct {
	Title           string          `json:"title" binding:"required"`
	Description     string          `json:"description"`
	MeetingTime     *model.DateTime `json:"meeting_time"`
	Location        string          `json:"location"`
	IsConfidential  *bool           `json:"is_confidential"` // 保密会议：仅参会组织负责人可查看
	OrgIDs          []uint          `json:"org_ids"`         // 参会组织（创建/编辑时设置）
}

// CreateMeeting POST /api/meetings 创建会议（管理员）
// 支持在创建时一并设置会议地点、会议时间与参会组织。
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
		Location:    req.Location,
		CreatorID:   ctx.ID,
		MeetingTime: mt,
		Status:      model.MeetingDraft,
	}
	if req.IsConfidential != nil {
		meeting.IsConfidential = *req.IsConfidential
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(meeting).Error; err != nil {
			return err
		}
		if len(req.OrgIDs) > 0 {
			return setMeetingOrgsTx(tx, meeting.ID, req.OrgIDs)
		}
		return nil
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "创建会议失败："+err.Error())
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
	if model.IsOrgRole(ctx.Role) {
		if ctx.OrgID == nil {
			c.JSON(http.StatusOK, []model.Meeting{})
			return
		}
		orgIDs := []uint{*ctx.OrgID}
		sub := h.db.Model(&model.MeetingOrg{}).
			Where("org_id IN ?", orgIDs).
			Select("meeting_id")
		q = q.Where("id IN (?)", sub)
		// 保密会议仅参会组织负责人可见，普通成员不可见
		if ctx.Role == model.RoleMember {
			q = q.Where("is_confidential = ?", false)
		}
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
	ctx := currentUser(c)
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "无权查看该会议")
		return
	}
	c.JSON(http.StatusOK, meeting)
}

// meetingVisible 判断会议对用户是否可见（读权限）：
// admin 全部可见；组织用户须为该会议参会组织；
// 保密会议（IsConfidential）仅参会组织负责人（dept_leader/team_leader）可见，普通成员不可见。
func (h *Handler) meetingVisible(role string, orgID *uint, meeting *model.Meeting) bool {
	if role == model.RoleAdmin {
		return true
	}
	if orgID == nil {
		return false
	}
	var count int64
	h.db.Model(&model.MeetingOrg{}).
		Where("meeting_id = ? AND org_id = ?", meeting.ID, *orgID).
		Count(&count)
	if count == 0 {
		return false
	}
	if meeting.IsConfidential && role == model.RoleMember {
		return false
	}
	return true
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
	// 参会组织仅筹备中的会议可修改
	if len(req.OrgIDs) > 0 && meeting.Status != model.MeetingDraft {
		badRequest(c, "仅筹备中的会议可以设置参会组织")
		return
	}
	if req.Title != "" {
		meeting.Title = req.Title
	}
	meeting.Description = req.Description
	meeting.Location = req.Location
	if req.IsConfidential != nil {
		meeting.IsConfidential = *req.IsConfidential
	}
	if req.MeetingTime != nil {
		meeting.MeetingTime = req.MeetingTime.Time
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&meeting).Error; err != nil {
			return err
		}
		if len(req.OrgIDs) > 0 {
			return setMeetingOrgsTx(tx, meeting.ID, req.OrgIDs)
		}
		return nil
	}); err != nil {
		fail(c, http.StatusInternalServerError, "保存会议失败："+err.Error())
		return
	}
	if len(req.OrgIDs) > 0 {
		var moList []*model.MeetingOrg
		h.db.Preload("Org").Where("meeting_id = ?", meeting.ID).Order("sort_order asc").Find(&moList)
		names := make([]string, 0, len(moList))
		for _, mo := range moList {
			if mo.Org != nil {
				names = append(names, mo.Org.Name)
			}
		}
		h.logRecord(c, model.LogMeetingOrgs, "meeting", meeting.ID,
			"设置会议「"+meeting.Title+"」参会组织："+strings.Join(names, "、"))
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

// setMeetingOrgsTx 在事务内覆盖式写入参会组织：校验组织存在、去重后写入。
func setMeetingOrgsTx(tx *gorm.DB, meetingID uint, orgIDs []uint) error {
	seen := make(map[uint]bool)
	for _, oid := range orgIDs {
		if seen[oid] {
			continue
		}
		seen[oid] = true
		var org model.Organization
		if err := tx.First(&org, oid).Error; err != nil {
			return fmt.Errorf("参会组织不存在: %d", oid)
		}
	}
	if err := tx.Where("meeting_id = ?", meetingID).Delete(&model.MeetingOrg{}).Error; err != nil {
		return err
	}
	sort := 0
	for _, oid := range orgIDs {
		if !seen[oid] {
			continue
		}
		seen[oid] = false
		mo := &model.MeetingOrg{MeetingID: meetingID, OrgID: oid, SortOrder: sort}
		if err := tx.Create(mo).Error; err != nil {
			return err
		}
		sort++
	}
	return nil
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
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		return setMeetingOrgsTx(tx, meeting.ID, req.OrgIDs)
	}); err != nil {
		if strings.HasPrefix(err.Error(), "参会组织不存在") {
			badRequest(c, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, "更新参会组织失败："+err.Error())
		return
	}
	var moList []*model.MeetingOrg
	h.db.Preload("Org").Where("meeting_id = ?", meeting.ID).Order("sort_order asc").Find(&moList)
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
	// 未录入任何汇报内容时禁止开始
	var itemCount int64
	h.db.Model(&model.ReportItem{}).Where("meeting_id = ?", id).Count(&itemCount)
	if itemCount == 0 {
		badRequest(c, "该会议尚未录入任何汇报内容，无法开始")
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
// 状态流转：仅 finished -> archived；未结束的会议不允许归档。
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
	if meeting.Status != model.MeetingFinished {
		badRequest(c, "仅已结束的会议可以归档")
		return
	}
	// 微盘自动上传：如需纪要且尚未生成，先自动生成一份（不阻塞归档）
	h.ensureMinutesForUpload(&meeting)

	meeting.Status = model.MeetingArchived
	if err := h.db.Save(&meeting).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新会议状态失败")
		return
	}
	h.logRecord(c, model.LogMeetingArchive, "meeting", meeting.ID, "归档会议："+meeting.Title)
	// 归档后自动上传最新纪要至微盘（失败仅记日志，不阻塞归档）
	h.uploadMinutesToWeDrive(c, &meeting)
	c.JSON(http.StatusOK, meeting)
}
