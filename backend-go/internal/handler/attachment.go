package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

const (
	maxAttachmentSize   = 5 * 1024 * 1024 // 单个附件 5MB
	maxAttachmentsCount = 10              // 每个事项最多 10 个附件
)

// allowedExts 允许的附件扩展名（图片/视频/pdf/word/文本等常用格式）
var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".svg": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true, ".wmv": true, ".flv": true,
	".pdf": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".txt": true, ".md": true, ".log": true, ".csv": true, ".rtf": true,
}

// ListAttachments GET /api/items/:id/attachments 附件列表
func (h *Handler) ListAttachments(c *gin.Context) {
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
	ctx := currentUser(c)
	if ctx.Role == model.RoleLeader && (ctx.OrgID == nil || *ctx.OrgID != item.OrgID) {
		forbidden(c, "无权查看其他组织的附件")
		return
	}
	var atts []model.ReportAttachment
	if err := h.db.Where("report_item_id = ?", id).Order("id asc").Find(&atts).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询附件失败")
		return
	}
	c.JSON(http.StatusOK, atts)
}

// UploadAttachment POST /api/items/:id/attachments 上传附件
// 规则：仅筹备中会议可上传；每事项最多 10 个；单文件 ≤5MB；扩展名白名单。
func (h *Handler) UploadAttachment(c *gin.Context) {
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
	// 会议须处于筹备中
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if meeting.Status != model.MeetingDraft {
		badRequest(c, "会议已开始或已结束，不能再上传附件")
		return
	}
	ctx := currentUser(c)
	if ctx.Role == model.RoleLeader {
		if ctx.OrgID == nil || *ctx.OrgID != item.OrgID {
			forbidden(c, "只能为本人所属组织的汇报事项上传附件")
			return
		}
	}

	// 数量限制
	var count int64
	h.db.Model(&model.ReportAttachment{}).Where("report_item_id = ?", id).Count(&count)
	if count >= maxAttachmentsCount {
		badRequest(c, fmt.Sprintf("每个汇报事项最多上传 %d 个附件", maxAttachmentsCount))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "未接收到文件或文件字段名应为 file")
		return
	}
	defer file.Close()

	if header.Size > maxAttachmentSize {
		badRequest(c, "单个附件大小不能超过 5MB")
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		badRequest(c, "不支持的附件格式，仅支持图片、视频、PDF、Office 文档、文本等常用格式")
		return
	}

	// 保存到 uploads/meetings/{meetingID}/items/{itemID}/
	dir := filepath.Join(h.cfg.UploadDir, "meetings", strconv.FormatUint(uint64(item.MeetingID), 10),
		"items", strconv.FormatUint(uint64(id), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, http.StatusInternalServerError, "创建上传目录失败")
		return
	}
	storedName := fmt.Sprintf("%d_%d%s", time.Now().UnixNano(), item.ID, ext)
	dest := filepath.Join(dir, storedName)
	if err := c.SaveUploadedFile(header, dest); err != nil {
		fail(c, http.StatusInternalServerError, "保存附件失败")
		return
	}

	att := &model.ReportAttachment{
		MeetingID:    item.MeetingID,
		ReportItemID: item.ID,
		FileName:     filepath.Base(header.Filename),
		StoredName:   storedName,
		FilePath:     dest,
		FileSize:     header.Size,
		MimeType:     header.Header.Get("Content-Type"),
	}
	if err := h.db.Create(att).Error; err != nil {
		os.Remove(dest)
		fail(c, http.StatusInternalServerError, "保存附件记录失败")
		return
	}
	h.logRecord(c, model.LogAttachmentUp, "attachment", att.ID, "上传附件："+att.FileName)
	c.JSON(http.StatusCreated, att)
}

// DeleteAttachment DELETE /api/attachments/:id 删除附件
func (h *Handler) DeleteAttachment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "附件 ID 不合法")
		return
	}
	var att model.ReportAttachment
	if err := h.db.First(&att, id).Error; err != nil {
		notFound(c, "附件不存在")
		return
	}
	var item model.ReportItem
	if err := h.db.First(&item, att.ReportItemID).Error; err != nil {
		notFound(c, "汇报事项不存在")
		return
	}
	// 会议须处于筹备中
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if meeting.Status != model.MeetingDraft {
		badRequest(c, "会议已开始或已结束，不能再删除附件")
		return
	}
	ctx := currentUser(c)
	if ctx.Role == model.RoleLeader {
		if ctx.OrgID == nil || *ctx.OrgID != item.OrgID {
			forbidden(c, "只能删除本人所属组织的汇报事项附件")
			return
		}
	}
	_ = os.Remove(att.FilePath)
	if err := h.db.Delete(&att).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除附件失败")
		return
	}
	h.logRecord(c, model.LogAttachmentDel, "attachment", att.ID, "删除附件："+att.FileName)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetAttachmentFile GET /api/attachments/:id/file 下载/查看附件
func (h *Handler) GetAttachmentFile(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "附件 ID 不合法")
		return
	}
	var att model.ReportAttachment
	if err := h.db.First(&att, id).Error; err != nil {
		notFound(c, "附件不存在")
		return
	}
	var item model.ReportItem
	if err := h.db.First(&item, att.ReportItemID).Error; err != nil {
		notFound(c, "汇报事项不存在")
		return
	}
	ctx := currentUser(c)
	if ctx.Role == model.RoleLeader {
		if ctx.OrgID == nil || *ctx.OrgID != item.OrgID {
			forbidden(c, "无权查看其他组织的附件")
			return
		}
	}
	if _, err := os.Stat(att.FilePath); err != nil {
		notFound(c, "附件文件已丢失")
		return
	}
	c.FileAttachment(att.FilePath, att.FileName)
}
