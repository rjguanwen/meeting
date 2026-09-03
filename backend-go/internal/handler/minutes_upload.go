package handler

import (
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meetingbackend/internal/model"
)

// uploadMinutesToWeDrive 归档时把最新会议纪要上传到企业微信微盘。
// 未启用微盘配置（wedrive 为 nil）时直接返回，不阻塞归档流程。
// 上传失败仅记录日志，不阻塞归档。
func (h *Handler) uploadMinutesToWeDrive(c *gin.Context, meeting *model.Meeting) {
	if h.wedrive == nil {
		return
	}
	var minutes model.MeetingMinutes
	if err := h.db.Where("meeting_id = ?", meeting.ID).Order("id desc").First(&minutes).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			h.logRecord(c, model.LogMinutesUpload, "meeting", meeting.ID, "未生成会议纪要，跳过微盘上传："+meeting.Title)
			return
		}
		h.logRecord(c, model.LogMinutesUpload, "meeting", meeting.ID, "查询会议纪要失败，跳过微盘上传："+err.Error())
		return
	}

	fileName := buildWeComFileName(h.cfg.WeCom.FileNameTemplate, meeting)
	fileID, err := h.wedrive.UploadText(fileName, []byte(minutes.Content))
	if err != nil {
		h.logRecord(c, model.LogMinutesUpload, "meeting", meeting.ID, "微盘上传失败："+meeting.Title+" / "+err.Error())
		log.Printf("[wecom] 会议 %s 纪要上传微盘失败: %v", meeting.Title, err)
		return
	}
	h.logRecord(c, model.LogMinutesUpload, "meeting", meeting.ID, "微盘上传成功："+fileName+" (fileid="+fileID+")")
	log.Printf("[wecom] 会议 %s 纪要已上传微盘: %s (fileid=%s)", meeting.Title, fileName, fileID)
}

// ensureMinutesForUpload 归档时若尚未生成纪要且配置允许（WECOM_GENERATE_MISSING=true），
// 仅当会议处于 ongoing/finished 时自动生成一份纪要，保证微盘上传有内容。
func (h *Handler) ensureMinutesForUpload(meeting *model.Meeting) {
	if h.wedrive == nil || !h.cfg.WeCom.GenerateIfMissing {
		return
	}
	if meeting.Status != model.MeetingOngoing && meeting.Status != model.MeetingFinished {
		return
	}
	var minutes model.MeetingMinutes
	if err := h.db.Where("meeting_id = ?", meeting.ID).Order("id desc").First(&minutes).Error; err == nil {
		return // 已有纪要
	}
	content, err := h.composeMinutesContent(meeting)
	if err != nil {
		log.Printf("[wecom] 会议 %s 自动生成纪要失败: %v", meeting.Title, err)
		return
	}
	rec := model.MeetingMinutes{MeetingID: meeting.ID, Content: content}
	if err := h.db.Create(&rec).Error; err != nil {
		log.Printf("[wecom] 会议 %s 自动生成纪要保存失败: %v", meeting.Title, err)
	}
}

// buildWeComFileName 根据模板生成上传文件名，支持 {title}/{date}/{id} 占位。
// 默认：会议纪要-{title}-{date}.md，title 取会议标题（过滤非法字符），date 取会议时间日期。
func buildWeComFileName(template string, meeting *model.Meeting) string {
	if strings.TrimSpace(template) == "" {
		template = "会议纪要-{title}-{date}.md"
	}
	date := meeting.MeetingTime.Format("2006-01-02")
	if meeting.MeetingTime.IsZero() {
		date = "未定日期"
	}
	replacer := strings.NewReplacer(
		"{title}", sanitizeFileName(meeting.Title, 60),
		"{date}", date,
		"{id}", fmt.Sprintf("%d", meeting.ID),
	)
	name := replacer.Replace(template)
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	return name
}

// sanitizeFileName 去除 Windows/文件系统非法字符并限制长度。
func sanitizeFileName(s string, maxRunes int) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		case '\r', '\n', '\t':
			return ' '
		}
		return r
	}, s)
	runes := []rune(strings.TrimSpace(s))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	out := strings.TrimSpace(string(runes))
	if out == "" {
		out = "未命名会议"
	}
	return out
}
