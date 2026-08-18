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
	MeetingID uint            `json:"meeting_id"`
	Title     string          `json:"title"`
	Slides    []MaterialSlide `json:"slides"`
	Total     int             `json:"total"`
}

// buildMaterialData 组装会议材料（按部门→小组→事项组织成逐条页）
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
		MeetingID: meeting.ID,
		Title:     meeting.Title,
		Slides:    slides,
		Total:     len(slides),
	}
}

// meetingOrgs 返回参会组织列表及名称/部门归属映射
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
		meeting.MaterialGenerated = true
		h.db.Save(&meeting)
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
	var allowed map[uint]bool
	if ctx.Role == model.RoleLeader && ctx.OrgID != nil {
		allowed = h.selfOrgIDs(*ctx.OrgID)
	}
	c.JSON(http.StatusOK, h.buildMaterialData(&meeting, allowed))
}

// selfOrgIDs 返回负责人可查看的组织 ID 集合：
// 绑定的组织若是部门，则包含该部门及其下所有小组；若是小组，仅自身。
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
