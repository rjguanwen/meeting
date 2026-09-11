package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

// MaterialSlide 单页材料（PPT 形式）
type MaterialSlide struct {
	Index       int                      `json:"index"`
	OrgID       uint                     `json:"org_id"`
	OrgName     string                   `json:"org_name"`
	OrgType     string                   `json:"org_type"`
	DeptName    string                   `json:"dept_name"` // 所属部门名（小组事项时展示）
	ItemID      uint                     `json:"item_id"`
	Title       string                   `json:"title"`
	Content     string                   `json:"content"`
	Status      string                   `json:"status"`
	Reporter    string                   `json:"reporter"` // 录入人
	Conclusions []model.Conclusion       `json:"conclusions"`
	Attachments []model.ReportAttachment `json:"attachments,omitempty"`
}

// MaterialData 会议材料
type MaterialData struct {
	MeetingID   uint            `json:"meeting_id"`
	Title       string          `json:"title"`
	Location    string          `json:"location"`     // 会议地点
	MeetingTime string          `json:"meeting_time"` // 会议时间
	Status      string          `json:"status"`       // 会议状态，供展示页直接判断只读/录入
	Slides      []MaterialSlide `json:"slides"`
	Total       int             `json:"total"`
}

// buildMaterialData 组装会议材料（按小组→部门→事项组织成逐条页）
// allowedOrgIDs 为空(nil)时展示全部组织事项；非空时仅展示指定组织（用于负责人只读查看本部门）。
func (h *Handler) buildMaterialData(meeting *model.Meeting, allowedOrgIDs map[uint]bool) MaterialData {
	orgs, orgName, deptOf := h.meetingOrgs(meeting.ID)
	_ = orgs

	var items []model.ReportItem
	h.db.Preload("Creator").
		Where("meeting_id = ?", meeting.ID).
		Order("org_id asc, sort_order asc, id asc").
		Find(&items)

	if allowedOrgIDs != nil {
		filtered := items[:0]
		for _, it := range items {
			if allowedOrgIDs[it.OrgID] {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	var conclusions []model.Conclusion
	h.db.Where("meeting_id = ?", meeting.ID).Order("id asc").Find(&conclusions)
	concByItem := make(map[uint][]model.Conclusion)
	for _, cc := range conclusions {
		concByItem[cc.ReportItemID] = append(concByItem[cc.ReportItemID], cc)
	}

	var attachments []model.ReportAttachment
	h.db.Where("meeting_id = ?", meeting.ID).Order("id asc").Find(&attachments)
	attByItem := make(map[uint][]model.ReportAttachment)
	for _, at := range attachments {
		attByItem[at.ReportItemID] = append(attByItem[at.ReportItemID], at)
	}

	var slides []MaterialSlide
	idx := 0
	for _, org := range orgs {
		for _, item := range items {
			if item.OrgID != org.ID {
				continue
			}
			idx++
			reporter := ""
			if item.Creator != nil {
				reporter = item.Creator.Name
			}
			deptName := ""
			if org.Type == model.OrgTypeTeam {
				if did, ok := deptOf[org.ID]; ok {
					deptName = orgName[did]
				}
			}
			slides = append(slides, MaterialSlide{
				Index:       idx,
				OrgID:       org.ID,
				OrgName:     org.Name,
				OrgType:     org.Type,
				DeptName:    deptName,
				ItemID:      item.ID,
				Title:       item.Title,
				Content:     item.Content,
				Status:      item.Status,
				Reporter:    reporter,
				Conclusions: concByItem[item.ID],
				Attachments: attByItem[item.ID],
			})
		}
	}
	return MaterialData{
		MeetingID:   meeting.ID,
		Title:       meeting.Title,
		Location:    meeting.Location,
		MeetingTime: meeting.MeetingTime.Format("2006-01-02 15:04"),
		Status:      meeting.Status,
		Slides:      slides,
		Total:       len(slides),
	}
}

// meetingOrgs 返回参会组织列表及名称/部门归属映射。
// 展示顺序为小组在前、部门在后（type 取值 team/dept，desc 即 team 优先）：
// 会上先逐条听各小组汇报，再由部门汇总；同一类型内按参会时的手动排序、再按组织 ID。
func (h *Handler) meetingOrgs(meetingID uint) ([]model.Organization, map[uint]string, map[uint]uint) {
	var orgs []model.Organization
	h.db.
		Joins("JOIN meeting_orgs mo ON mo.org_id = organizations.id AND mo.meeting_id = ?", meetingID).
		Order("organizations.type desc, mo.sort_order asc, organizations.id asc").
		Find(&orgs)
	orgName := make(map[uint]string, len(orgs))
	deptOf := make(map[uint]uint) // teamID -> deptID
	for _, o := range orgs {
		orgName[o.ID] = o.Name
		if o.ParentID != nil {
			deptOf[o.ID] = *o.ParentID
		}
	}
	return orgs, orgName, deptOf
}

// GenerateMaterial POST /api/meetings/:id/material 生成会议材料
// 规则：筹备中可多次生成（会前预览）；会议开始后仅可正式生成一次；
//
//	归档后禁止生成（只读）。
func (h *Handler) GenerateMaterial(c *gin.Context) {
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
		badRequest(c, "会议已归档，材料只读，不能再生成")
		return
	}
	// 会议开始后，正式材料只允许生成一次
	if meeting.Status != model.MeetingDraft && meeting.MaterialGenerated {
		badRequest(c, "会议已开始，正式会议材料仅可生成一次")
		return
	}
	if meeting.Status != model.MeetingDraft {
		// 条件更新保证「仅可生成一次」在并发下也成立：只有把标记由 false 改为 true 的请求算正式生成
		res := h.db.Model(&model.Meeting{}).
			Where("id = ? AND material_generated = ?", meeting.ID, false).
			Update("material_generated", true)
		if res.Error != nil {
			// 标记写入失败必须报错：否则「正式材料仅可生成一次」的约束会静默失效
			fail(c, http.StatusInternalServerError, "记录材料生成状态失败")
			return
		}
		if res.RowsAffected == 0 { // 并发的另一个请求已正式生成
			badRequest(c, "会议已开始，正式会议材料仅可生成一次")
			return
		}
		meeting.MaterialGenerated = true
	}
	h.logRecord(c, model.LogMaterial, "meeting", meeting.ID, "生成会议材料："+meeting.Title)

	c.JSON(http.StatusOK, h.buildMaterialData(&meeting, nil))
}

// GetMaterial GET /api/meetings/:id/material 查看会议材料（只读，不改变状态）
// 管理员展示全部汇报事项；组织负责人仅展示其所属部门（含部门下小组）的汇报事项。
func (h *Handler) GetMaterial(c *gin.Context) {
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
		forbidden(c, "无权查看该会议材料")
		return
	}
	var allowed map[uint]bool
	if model.IsOrgRole(ctx.Role) && ctx.OrgID != nil {
		allowed = h.selfOrgIDs(*ctx.OrgID)
	}
	c.JSON(http.StatusOK, h.buildMaterialData(&meeting, allowed))
}

// selfOrgIDs 返回负责人可查看的组织 ID 集合：
// 绑定的组织若是部门，则包含该部门及其下所有小组；若是小组，仅自身。
// 仅用于已通过 meetingVisible 门禁的会议**内部**材料过滤（部门汇总视角），
// 不会让负责人多看到任何一个会议：只有部门自身也参会时，下属小组的事项才会出现在材料里。
// （事项列表 ListItems 按 org_id 精确过滤、不做本展开，两者差异待产品确认。）
func (h *Handler) selfOrgIDs(orgID uint) map[uint]bool {
	ids := map[uint]bool{orgID: true}
	var org model.Organization
	if err := h.db.First(&org, orgID).Error; err != nil {
		return ids
	}
	if org.Type == model.OrgTypeDept {
		var teams []model.Organization
		h.db.Where("parent_id = ?", orgID).Find(&teams)
		for _, t := range teams {
			ids[t.ID] = true
		}
	}
	return ids
}
