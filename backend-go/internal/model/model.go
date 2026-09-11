package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 组织类型
const (
	OrgTypeDept = "dept" // 部门
	OrgTypeTeam = "team" // 小组
)

// 用户角色
const (
	RoleAdmin      = "admin"       // 系统管理员/会议主持人
	RoleDeptLeader = "dept_leader" // 部门负责人
	RoleTeamLeader = "team_leader" // 小组负责人
	RoleMember     = "member"      // 组织成员
	// RoleLeader 兼容旧数据（迁移后会替换），避免历史代码引用出错
	RoleLeader = "leader"
)

// IsOrgRole 判断是否为绑定组织的普通用户角色（非管理员）
func IsOrgRole(role string) bool {
	return role != RoleAdmin
}

// 会议状态
const (
	MeetingDraft    = "draft"    // 筹备中：事项录入中
	MeetingOngoing  = "ongoing"  // 进行中：会议已开始
	MeetingFinished = "finished" // 已结束
	MeetingArchived = "archived" // 已归档：材料与纪要只读
)

// 汇报事项状态
const (
	ItemSubmitted = "submitted" // 已录入
	ItemPresented = "presented" // 已展示
)

// 结论类型
const (
	ConclusionKind = "conclusion" // 讨论结论
	ActionKind     = "action"     // 新任务
)

// Organization 组织（部门/小组），支持树形结构。
type Organization struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Type      string    `gorm:"size:16;not null" json:"type"` // dept / team
	ParentID  *uint     `gorm:"index" json:"parent_id"`       // 部门为 nil，小组指向所属部门
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`

	Children []*Organization `gorm:"-" json:"children,omitempty"`
}

// User 用户账号。每个参会组织对应一个 leader 账号。
type User struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Username       string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	HashedPassword string    `gorm:"size:255;not null" json:"-"`
	Name           string    `gorm:"size:128" json:"name"`
	Role           string    `gorm:"size:16;default:member" json:"role"`
	OrgID          *uint     `gorm:"index" json:"org_id"` // 非管理员所属组织，admin 为 nil
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	Avatar         string    `gorm:"size:255" json:"avatar"`         // 头像文件名（存于 uploads/avatars，空则显示默认）
	PasswordHint   string    `gorm:"size:255" json:"password_hint"`  // 密码提示词（用户自助设置）
	SecurityQuestion string  `gorm:"size:255" json:"security_question"` // 安全找回问题
	SecurityAnswer string    `gorm:"size:255" json:"-"`              // 安全答案哈希（bcrypt）
	CreatedAt      time.Time `json:"created_at"`

	Org *Organization `gorm:"foreignKey:OrgID" json:"org,omitempty"`
}

// CheckPassword 校验密码。
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.HashedPassword), []byte(password)) == nil
}

// SetPassword 更新密码。
func (u *User) SetPassword(password string) error {
	u.HashedPassword = hashPassword(password)
	return nil
}

// SetSecurityAnswer 设置安全答案（哈希存储）。
func (u *User) SetSecurityAnswer(answer string) {
	u.SecurityAnswer = hashPassword(answer)
}

// CheckSecurityAnswer 校验安全答案。
func (u *User) CheckSecurityAnswer(answer string) bool {
	if u.SecurityAnswer == "" || answer == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.SecurityAnswer), []byte(answer)) == nil
}

func hashPassword(password string) string {
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed)
}

// Meeting 会议。
type Meeting struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	Title             string    `gorm:"size:255;not null" json:"title"`
	Description       string    `gorm:"type:text" json:"description"`
	Location          string    `gorm:"size:255" json:"location"`         // 会议地点
	IsConfidential    bool      `gorm:"default:false" json:"is_confidential"` // 保密会议：仅参会组织负责人可查看
	CreatorID         uint      `gorm:"index" json:"creator_id"`
	MeetingTime       time.Time `json:"meeting_time"` // 会议时间
	Status            string    `gorm:"size:16;default:draft" json:"status"`
	MaterialGenerated bool      `gorm:"default:false" json:"material_generated"` // 是否已正式生成材料（开始后仅一次）
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	Creator *User         `gorm:"foreignKey:CreatorID" json:"-"`
	Orgs    []*MeetingOrg `gorm:"foreignKey:MeetingID" json:"orgs,omitempty"`
	Items   []*ReportItem `gorm:"foreignKey:MeetingID" json:"items,omitempty"`
}

// MeetingOrg 会议参会组织。
type MeetingOrg struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	MeetingID uint `gorm:"index" json:"meeting_id"`
	OrgID     uint `gorm:"index" json:"org_id"`
	SortOrder int  `json:"sort_order"`

	Org *Organization `gorm:"foreignKey:OrgID" json:"org,omitempty"`
}

// ReportItem 汇报事项。由参会组织负责人录入。
type ReportItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	MeetingID uint      `gorm:"index;not null" json:"meeting_id"`
	OrgID     uint      `gorm:"index;not null" json:"org_id"`
	CreatorID uint      `gorm:"index" json:"creator_id"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Status    string    `gorm:"size:16;default:submitted" json:"status"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`

	Org     *Organization `gorm:"foreignKey:OrgID" json:"org,omitempty"`
	Creator *User         `gorm:"foreignKey:CreatorID" json:"creator,omitempty"`
}

// Conclusion 讨论结论 / 新任务。挂在汇报事项下。
type Conclusion struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	MeetingID    uint      `gorm:"index;not null" json:"meeting_id"`
	ReportItemID uint      `gorm:"index;not null" json:"report_item_id"`
	Kind         string    `gorm:"size:16;default:conclusion" json:"kind"` // conclusion / action
	Content      string    `gorm:"type:text;not null" json:"content"`
	Owner        string    `gorm:"size:128" json:"owner"` // 负责人（新任务用）
	DueDate      *DateTime `json:"due_date"`              // 截止日期（新任务用）
	CreatedAt    time.Time `json:"created_at"`
}

// ReportAttachment 汇报事项附件。
type ReportAttachment struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	MeetingID    uint      `gorm:"index;not null" json:"meeting_id"`
	ReportItemID uint      `gorm:"index;not null" json:"report_item_id"`
	FileName     string    `gorm:"size:255;not null" json:"file_name"` // 原始文件名
	StoredName   string    `gorm:"size:255;not null" json:"-"`         // 磁盘存储名
	FilePath     string    `gorm:"size:512;not null" json:"-"`         // 存储路径
	FileSize     int64     `json:"file_size"`
	MimeType     string    `gorm:"size:128" json:"mime_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// MeetingMinutes 会议纪要。
type MeetingMinutes struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	MeetingID uint      `gorm:"index;not null" json:"meeting_id"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MeetingRoom 会议室。
type MeetingRoom struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"` // 会议室名称
	Location  string    `gorm:"size:255" json:"location"`      // 所在位置（楼栋/楼层）
	Capacity  int       `json:"capacity"`                      // 可容纳人数
	Remark    string    `gorm:"size:512" json:"remark"`        // 备注
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `gorm:"default:true" json:"is_active"` // 是否启用
	CreatedAt time.Time `json:"created_at"`
}

// OperationLog 操作日志。记录登录与关键操作，供管理员查询。
type OperationLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	Username   string    `gorm:"size:64" json:"username"`
	Action     string    `gorm:"size:64;index;not null" json:"action"` // 动作类型，如 meeting.create
	TargetType string    `gorm:"size:32" json:"target_type"`           // 目标类型：meeting/org/user/item/conclusion/attachment
	TargetID   uint      `json:"target_id"`
	Detail     string    `gorm:"size:512" json:"detail"` // 简要描述
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

// 操作日志动作常量
const (
	LogLogin          = "login"
	LogLoginFail      = "login_fail"
	LogOrgCreate      = "org.create"
	LogOrgUpdate      = "org.update"
	LogOrgDelete      = "org.delete"
	LogUserCreate     = "user.create"
	LogUserUpdate     = "user.update"
	LogMeetingCreate  = "meeting.create"
	LogMeetingUpdate  = "meeting.update"
	LogMeetingDelete  = "meeting.delete"
	LogMeetingOrgs    = "meeting.set_orgs"
	LogMeetingStart   = "meeting.start"
	LogMeetingFinish  = "meeting.finish"
	LogMeetingArchive = "meeting.archive"
	LogMinutesUpload  = "meeting.minutes.upload" // 归档时自动上传纪要至微盘
	LogMaterial       = "meeting.material"
	LogMinutes        = "meeting.minutes"
	LogItemCreate     = "item.create"
	LogItemUpdate     = "item.update"
	LogItemDelete     = "item.delete"
	LogConclusion     = "conclusion.create"
	LogConclusionUpd  = "conclusion.update"
	LogConclusionDel  = "conclusion.delete"
	LogAttachmentUp   = "attachment.upload"
	LogAttachmentDel  = "attachment.delete"
	LogRoomCreate     = "room.create"
	LogRoomUpdate     = "room.update"
	LogRoomDelete     = "room.delete"
	LogPasswordChange = "password.change"
	LogProfileUpdate  = "profile.update"   // 自助修改昵称/密码提示词/安全问答
	LogAvatarUpdate   = "avatar.update"    // 自助上传头像
	LogPasswordReset  = "password.reset"   // 安全问答找回密码
)

// WithOrganizationTree 将组织列表按部门-小组组装为树。
func WithOrganizationTree(orgs []*Organization) []*Organization {
	byID := make(map[uint]*Organization, len(orgs))
	for _, o := range orgs {
		byID[o.ID] = o
	}
	var roots []*Organization
	for _, o := range orgs {
		if o.ParentID != nil {
			if parent, ok := byID[*o.ParentID]; ok {
				parent.Children = append(parent.Children, o)
				continue
			}
		}
		roots = append(roots, o)
	}
	return roots
}

// BeforeCreate 保留钩子，占位以提示 GORM 不处理额外逻辑。
func (c *Conclusion) BeforeCreate(*gorm.DB) error { return nil }
