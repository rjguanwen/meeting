package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/model"
)

type roomReq struct {
	Name      string `json:"name" binding:"required"`
	Location  string `json:"location"`
	Capacity  int    `json:"capacity"`
	Remark    string `json:"remark"`
	SortOrder int    `json:"sort_order"`
	IsActive  *bool  `json:"is_active"`
}

// ListRooms GET /api/rooms 会议室列表（所有登录用户可见，供会议选择地点）
func (h *Handler) ListRooms(c *gin.Context) {
	var rooms []model.MeetingRoom
	if err := h.db.Order("sort_order asc, id asc").Find(&rooms).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询会议室失败")
		return
	}
	c.JSON(http.StatusOK, rooms)
}

// CreateRoom POST /api/rooms 新增会议室（管理员）
func (h *Handler) CreateRoom(c *gin.Context) {
	var req roomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		badRequest(c, "会议室名称不能为空")
		return
	}
	room := &model.MeetingRoom{
		Name:      name,
		Location:  strings.TrimSpace(req.Location),
		Capacity:  req.Capacity,
		Remark:    strings.TrimSpace(req.Remark),
		SortOrder: req.SortOrder,
		IsActive:  true,
	}
	if req.IsActive != nil {
		room.IsActive = *req.IsActive
	}
	if err := h.db.Create(room).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建会议室失败")
		return
	}
	h.logRecord(c, model.LogRoomCreate, "room", room.ID, "新增会议室："+room.Name)
	c.JSON(http.StatusCreated, room)
}

// UpdateRoom PATCH /api/rooms/:id 修改会议室（管理员）
func (h *Handler) UpdateRoom(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议室 ID 不合法")
		return
	}
	var room model.MeetingRoom
	if err := h.db.First(&room, id).Error; err != nil {
		notFound(c, "会议室不存在")
		return
	}
	var req roomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数不合法："+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		badRequest(c, "会议室名称不能为空")
		return
	}
	oldName := room.Name
	room.Name = name
	room.Location = strings.TrimSpace(req.Location)
	room.Capacity = req.Capacity
	room.Remark = strings.TrimSpace(req.Remark)
	room.SortOrder = req.SortOrder
	if req.IsActive != nil {
		room.IsActive = *req.IsActive
	}
	if err := h.db.Save(&room).Error; err != nil {
		fail(c, http.StatusInternalServerError, "保存会议室失败")
		return
	}
	h.logRecord(c, model.LogRoomUpdate, "room", room.ID, "修改会议室："+oldName+" → "+room.Name)
	c.JSON(http.StatusOK, room)
}

// DeleteRoom DELETE /api/rooms/:id 删除会议室（管理员）
func (h *Handler) DeleteRoom(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "会议室 ID 不合法")
		return
	}
	var room model.MeetingRoom
	if err := h.db.First(&room, id).Error; err != nil {
		notFound(c, "会议室不存在")
		return
	}
	if err := h.db.Delete(&room).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除会议室失败")
		return
	}
	h.logRecord(c, model.LogRoomDelete, "room", room.ID, "删除会议室："+room.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
