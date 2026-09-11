package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meetingbackend/internal/model"
)

const (
	maxAttachmentSize   = 5 * 1024 * 1024 // 单个附件 5MB
	maxAttachmentsCount = 10              // 每个事项最多 10 个附件
	// multipartOverhead 请求体余量：boundary、各 part 头部与字段名等开销
	multipartOverhead = 1 << 20
)

const tooLargeMsg = "单个附件大小不能超过 5MB"

// limitUploadBody 为上传请求设置请求体硬上限。
// 不设上限时，超大 body 会在解析 multipart 阶段被整体读入内存/临时文件，构成资源耗尽风险；
// 上限为「单文件大小 + multipart 开销」，因此合法大小的文件行为完全不变。
func limitUploadBody(c *gin.Context, fileLimit int64) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, fileLimit+multipartOverhead)
}

// isBodyTooLarge 判断读取 multipart 时的错误是否由请求体超限引起，以便保持原有提示文案。
func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return strings.Contains(err.Error(), "message too large") ||
		strings.Contains(err.Error(), "request body too large")
}

// removeUploadFiles 删除附件对应的磁盘文件（仅在数据库记录已删除后调用）。
// 单个文件删除失败不影响业务主流程，忽略即可。
func removeUploadFiles(paths []string) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		_ = os.Remove(p)
	}
}

// attachmentPaths 取出待级联删除的附件文件路径。
func attachmentPaths(tx *gorm.DB, query string, args ...any) ([]string, error) {
	var paths []string
	if err := tx.Model(&model.ReportAttachment{}).Where(query, args...).Pluck("file_path", &paths).Error; err != nil {
		return nil, err
	}
	return paths, nil
}

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
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "无权查看该会议附件")
		return
	}
	if model.IsOrgRole(ctx.Role) && (ctx.OrgID == nil || *ctx.OrgID != item.OrgID) {
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
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if model.IsOrgRole(ctx.Role) {
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

	limitUploadBody(c, maxAttachmentSize)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			badRequest(c, tooLargeMsg)
			return
		}
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
		os.Remove(dest)
		if isBodyTooLarge(err) {
			badRequest(c, tooLargeMsg)
			return
		}
		fail(c, http.StatusInternalServerError, "保存附件失败")
		return
	}
	// header.Size 是客户端声明值，可能偏小；以磁盘实际写入大小为准再校验一次
	info, statErr := os.Stat(dest)
	if statErr != nil {
		os.Remove(dest)
		fail(c, http.StatusInternalServerError, "保存附件失败")
		return
	}
	if info.Size() > maxAttachmentSize {
		os.Remove(dest)
		badRequest(c, tooLargeMsg)
		return
	}

	att := &model.ReportAttachment{
		MeetingID:    item.MeetingID,
		ReportItemID: item.ID,
		FileName:     filepath.Base(header.Filename),
		StoredName:   storedName,
		FilePath:     dest,
		FileSize:     info.Size(),
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
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "保密会议仅参会组织负责人可操作")
		return
	}
	if model.IsOrgRole(ctx.Role) {
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
	var meeting model.Meeting
	if err := h.db.First(&meeting, item.MeetingID).Error; err != nil {
		notFound(c, "会议不存在")
		return
	}
	if !h.meetingVisible(ctx.Role, ctx.OrgID, &meeting) {
		forbidden(c, "无权查看该会议附件")
		return
	}
	if model.IsOrgRole(ctx.Role) {
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
