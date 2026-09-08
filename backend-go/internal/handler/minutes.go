package handler

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

// atxHeadingRE 匹配 Markdown ATX 标题行（# 开头，前面最多 3 个空格）。
var atxHeadingRE = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.*)$`)

// flattenReportHeadings 将汇报内容里的 Markdown 标题行转成加粗文本，
// 避免内容中的 #/## 破坏会议纪要自身的标题层级（纪要结构：# 会议 → ## 章节 → ### 组织）。
// 保留列表、表格、引用、行内加粗等正文格式；代码块内的 # 不会被误处理。
func flattenReportHeadings(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var out []string
	inCode := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCode = !inCode
		}
		if !inCode {
			if m := atxHeadingRE.FindStringSubmatch(ln); m != nil {
				text := strings.TrimSpace(m[1])
				if text != "" {
					out = append(out, "**"+text+"**")
					continue
				}
			}
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// buildMinutes 根据会议信息生成 Markdown 纪要
func buildMinutes(meeting *model.Meeting, orgs []model.Organization, items []model.ReportItem, conclusions []model.Conclusion) string {
	var b strings.Builder

	// 标题
	fmt.Fprintf(&b, "# %s\n\n", meeting.Title)
	fmt.Fprintf(&b, "- 会议时间：%s\n", meeting.MeetingTime.Format("2006-01-02 15:04"))
	if strings.TrimSpace(meeting.Location) != "" {
		fmt.Fprintf(&b, "- 会议地点：%s\n", meeting.Location)
	}
	fmt.Fprintf(&b, "- 会议状态：%s\n", meetingStatusText(meeting.Status))
	fmt.Fprintf(&b, "- 生成时间：%s\n\n", time.Now().Format("2006-01-02 15:04"))

	// 参会组织
	orgNames := make([]string, 0, len(orgs))
	for _, o := range orgs {
		orgNames = append(orgNames, o.Name)
	}
	fmt.Fprintf(&b, "## 参会组织\n\n%s\n\n", strings.Join(orgNames, "、"))

	// 按组织分组事项
	type orgBlock struct {
		name  string
		items []model.ReportItem
	}
	var blocks []orgBlock
	itemByOrg := make(map[uint][]model.ReportItem)
	for _, it := range items {
		itemByOrg[it.OrgID] = append(itemByOrg[it.OrgID], it)
	}
	orgByID := make(map[uint]model.Organization, len(orgs))
	for _, o := range orgs {
		orgByID[o.ID] = o
	}
	for _, o := range orgs {
		if list, ok := itemByOrg[o.ID]; ok {
			blocks = append(blocks, orgBlock{name: o.Name, items: list})
		}
	}

	// 结论按事项分组
	concByItem := make(map[uint][]model.Conclusion)
	for _, cc := range conclusions {
		concByItem[cc.ReportItemID] = append(concByItem[cc.ReportItemID], cc)
	}

	b.WriteString("## 会议议程与汇报\n\n")
	for i, blk := range blocks {
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, blk.name)
		for j, it := range blk.items {
			fmt.Fprintf(&b, "**%d.%d %s**\n\n", i+1, j+1, it.Title)
			if strings.TrimSpace(it.Content) != "" {
				// 汇报内容支持 Markdown，但其中标题行需扁平化为加粗，
				// 以免破坏纪要自身层级
				fmt.Fprintf(&b, "%s\n\n", flattenReportHeadings(it.Content))
			}
			concs := concByItem[it.ID]
			if len(concs) > 0 {
				b.WriteString("**讨论结论：**\n\n")
				for _, cc := range concs {
					if cc.Kind == model.ActionKind {
						owner := strings.TrimSpace(cc.Owner)
						due := ""
						if cc.DueDate != nil {
							due = "，" + cc.DueDate.Time.Format("2006-01-02")
						}
						fmt.Fprintf(&b, "- 【任务】%s（负责人：%s%s）\n", cc.Content, owner, due)
					} else {
						fmt.Fprintf(&b, "- %s\n", cc.Content)
					}
				}
				b.WriteString("\n")
			}
		}
	}

	// 任务汇总
	var tasks []model.Conclusion
	for _, cc := range conclusions {
		if cc.Kind == model.ActionKind {
			tasks = append(tasks, cc)
		}
	}
	if len(tasks) > 0 {
		b.WriteString("## 待办任务汇总\n\n")
		for i, t := range tasks {
			owner := strings.TrimSpace(t.Owner)
			if owner == "" {
				owner = "待定"
			}
			due := "未定"
			if t.DueDate != nil {
				due = t.DueDate.Time.Format("2006-01-02")
			}
			fmt.Fprintf(&b, "%d. %s（负责人：%s，截止：%s）\n", i+1, t.Content, owner, due)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func meetingStatusText(status string) string {
	switch status {
	case model.MeetingDraft:
		return "筹备中"
	case model.MeetingOngoing:
		return "进行中"
	case model.MeetingFinished:
		return "已结束"
	default:
		return status
	}
}

// GetMinutes GET /api/meetings/:id/minutes 获取已生成的纪要
func (h *Handler) GetMinutes(c *gin.Context) {
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
		forbidden(c, "无权查看该会议纪要")
		return
	}
	var minutes model.MeetingMinutes
	if err := h.db.Where("meeting_id = ?", id).Order("id desc").Limit(1).Find(&minutes).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询会议纪要失败")
		return
	}
	if minutes.ID == 0 {
		// 未生成纪要时返回空对象，避免 GORM First 的 ErrRecordNotFound 日志噪音
		c.JSON(http.StatusOK, model.MeetingMinutes{MeetingID: uint(id)})
		return
	}
	c.JSON(http.StatusOK, minutes)
}

// GenerateMinutes POST /api/meetings/:id/minutes 生成会议纪要（管理员）
func (h *Handler) GenerateMinutes(c *gin.Context) {
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
	if meeting.Status != model.MeetingOngoing && meeting.Status != model.MeetingFinished {
		badRequest(c, "仅会议开始后且未归档时可以生成会议纪要")
		return
	}

	content, err := h.composeMinutesContent(&meeting)
	if err != nil {
		fail(c, http.StatusInternalServerError, "生成会议纪要失败")
		return
	}
	minutes := model.MeetingMinutes{MeetingID: meeting.ID, Content: content}
	if err := h.db.Create(&minutes).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存会议纪要失败")
		return
	}
	h.logRecord(c, model.LogMinutes, "meeting", meeting.ID, "生成会议纪要："+meeting.Title)
	c.JSON(http.StatusOK, minutes)
}

// composeMinutesContent 加载会议相关数据并生成 Markdown 纪要文本。
func (h *Handler) composeMinutesContent(meeting *model.Meeting) (string, error) {
	var orgs []model.Organization
	if err := h.db.
		Joins("JOIN meeting_orgs mo ON mo.org_id = organizations.id AND mo.meeting_id = ?", meeting.ID).
		Order("organizations.type desc, mo.sort_order asc").
		Find(&orgs).Error; err != nil {
		return "", fmt.Errorf("查询参会组织失败: %w", err)
	}
	var items []model.ReportItem
	if err := h.db.Where("meeting_id = ?", meeting.ID).Order("org_id asc, sort_order asc").Find(&items).Error; err != nil {
		return "", fmt.Errorf("查询汇报事项失败: %w", err)
	}
	var conclusions []model.Conclusion
	if err := h.db.Where("meeting_id = ?", meeting.ID).Order("id asc").Find(&conclusions).Error; err != nil {
		return "", fmt.Errorf("查询讨论结论失败: %w", err)
	}
	return buildMinutes(meeting, orgs, items, conclusions), nil
}

// UpdateMinutes PATCH /api/meetings/:id/minutes 编辑会议纪要（会议结束后可编辑，归档后只读）
func (h *Handler) UpdateMinutes(c *gin.Context) {
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
		badRequest(c, "会议已归档，纪要只读")
		return
	}
	if meeting.Status == model.MeetingDraft {
		badRequest(c, "会议开始后才能编辑会议纪要")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求参数不合法")
		return
	}
	var minutes model.MeetingMinutes
	if err := h.db.Where("meeting_id = ?", id).Order("id desc").Limit(1).Find(&minutes).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询会议纪要失败")
		return
	}
	if minutes.ID == 0 {
		badRequest(c, "纪要尚未生成，不能编辑")
		return
	}
	minutes.Content = req.Content
	if err := h.db.Save(&minutes).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存会议纪要失败")
		return
	}
	h.logRecord(c, model.LogMinutes, "meeting", meeting.ID, "编辑会议纪要："+meeting.Title)
	c.JSON(http.StatusOK, minutes)
}
