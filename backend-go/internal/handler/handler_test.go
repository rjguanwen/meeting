package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"meetingbackend/internal/config"
	"meetingbackend/internal/database"
	"meetingbackend/internal/middleware"
	"meetingbackend/internal/model"
)

// testEnv 一套跑在临时 SQLite 文件上的真实路由（含 JWT 管理员身份）。
type testEnv struct {
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
	auth   *middleware.Auth
	token  string
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		ProjectName:   "测试",
		Port:          "0",
		SecretKey:     "unit-test-secret",
		DatabaseURL:   filepath.Join(t.TempDir(), "test.db"),
		UploadDir:     filepath.Join(t.TempDir(), "uploads"),
		AdminUsername: "admin",
		AdminPassword: "admin123",
		AdminName:     "管理员",
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.InitAdmin(db, cfg)

	// Windows 下必须先释放文件句柄，否则 t.TempDir() 清理时会因文件被占用而失败
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	auth := middleware.NewAuth(cfg, db)
	h := New(db, cfg, auth)
	t.Cleanup(h.Close)

	router := gin.New()
	h.RegisterRoutes(router)

	// InitAdmin 创建的第一个账号即管理员
	token, err := auth.CreateToken(1, 0)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return &testEnv{router: router, db: db, cfg: cfg, auth: auth, token: "Bearer " + token}
}

// jsonReq 发送带默认鉴权头的 JSON 请求。
func (e *testEnv) jsonReq(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	return e.jsonReqAs(t, e.token, method, path, body)
}

// jsonReqAs 以指定 token 发送 JSON 请求（token 为空表示不鉴权，用于公开接口）。
func (e *testEnv) jsonReqAs(t *testing.T, token, method, path string, body any) (int, map[string]any) {
	t.Helper()
	w := e.do(t, token, method, path, body)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out) // 数组等非对象响应跳过解析
	return w.Code, out
}

// arrayReqAs 同上，但把响应按 JSON 数组解析（用于直接返回数组的接口）。
func (e *testEnv) arrayReqAs(t *testing.T, token, method, path string, body any) (int, []map[string]any) {
	t.Helper()
	w := e.do(t, token, method, path, body)
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON 数组：%s", w.Body.String())
	}
	return w.Code, out
}

func (e *testEnv) do(t *testing.T, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// loginReq 发送登录表单（公开接口，不需 token）。
func (e *testEnv) loginReq(t *testing.T, username, password string) (int, map[string]any) {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// tokenForUser 直接落库一个账号并返回其令牌（避开登录接口，只服务被测逻辑）。
func (e *testEnv) tokenForUser(t *testing.T, user *model.User) string {
	t.Helper()
	user.IsActive = true
	if user.HashedPassword == "" {
		user.SetPassword("pass123456")
	}
	if err := e.db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := e.auth.CreateToken(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return "Bearer " + token
}

// upload 发送 multipart 上传（字段名固定 file）。
func (e *testEnv) upload(t *testing.T, path, fileName string, content []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", e.token)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// newOrg 直接建一个部门组织（绕开组织接口的字段校验，只服务被测逻辑）。
func (e *testEnv) newOrg(t *testing.T, name string) uint {
	t.Helper()
	org := &model.Organization{Name: name, Type: model.OrgTypeDept}
	if err := e.db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return org.ID
}

// newTeam 在指定部门下建一个小组。
func (e *testEnv) newTeam(t *testing.T, name string, deptID uint) uint {
	t.Helper()
	parent := deptID
	org := &model.Organization{Name: name, Type: model.OrgTypeTeam, ParentID: &parent}
	if err := e.db.Create(org).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return org.ID
}

// draftMeeting 建一个含参会组织的筹备中会议，返回会议 ID。
func (e *testEnv) draftMeeting(t *testing.T, orgID uint, body map[string]any) uint {
	t.Helper()
	code, out := e.jsonReq(t, http.MethodPost, "/api/meetings", body)
	if code != http.StatusCreated {
		t.Fatalf("创建会议期望 201，实际 %d：%v", code, out)
	}
	id, _ := out["id"].(float64)
	if id == 0 {
		t.Fatalf("创建会议未返回 id：%v", out)
	}
	return uint(id)
}

// addItem 为会议录入一个汇报事项，返回事项 ID。
func (e *testEnv) addItem(t *testing.T, meetingID, orgID uint) uint {
	t.Helper()
	path := fmt.Sprintf("/api/meetings/%d/items", meetingID)
	code, out := e.jsonReq(t, http.MethodPost, path, map[string]any{
		"org_id": orgID, "title": "季度进展", "content": "内容",
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("创建事项期望 200/201，实际 %d：%v", code, out)
	}
	id, _ := out["id"].(float64)
	if id == 0 {
		t.Fatalf("创建事项未返回 id：%v", out)
	}
	return uint(id)
}

// addAttachment 为事项上传一个附件，返回附件 ID 与磁盘路径。
func (e *testEnv) addAttachment(t *testing.T, itemID uint, content []byte) (uint, string) {
	t.Helper()
	path := fmt.Sprintf("/api/items/%d/attachments", itemID)
	code, out := e.upload(t, path, "说明.txt", content)
	if code != http.StatusCreated {
		t.Fatalf("上传附件期望 201，实际 %d：%v", code, out)
	}
	id, _ := out["id"].(float64)
	var att model.ReportAttachment
	if err := e.db.First(&att, uint(id)).Error; err != nil {
		t.Fatalf("查询附件失败: %v", err)
	}
	if _, err := os.Stat(att.FilePath); err != nil {
		t.Fatalf("附件文件应已落盘: %v", err)
	}
	return att.ID, att.FilePath
}

// TestUpdateMeetingKeepsUnsentFields 覆盖 PATCH 语义：
// 编辑会议时未传的 description 不应被清空（前端编辑表单不提交该字段）。
func TestUpdateMeetingKeepsUnsentFields(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	mid := e.draftMeeting(t, orgID, map[string]any{
		"title":       "月度例会",
		"description": "重要背景说明",
		"location":    "301 会议室",
		"org_ids":     []uint{orgID},
	})

	// 与前端编辑表单一致：只传 title / location / is_confidential
	code, out := e.jsonReq(t, http.MethodPatch, fmt.Sprintf("/api/meetings/%d", mid), map[string]any{
		"title": "月度例会（改）", "location": "302 会议室", "is_confidential": false,
	})
	if code != http.StatusOK {
		t.Fatalf("修改会议期望 200，实际 %d：%v", code, out)
	}
	if got := out["description"]; got != "重要背景说明" {
		t.Fatalf("未提交的 description 被覆盖为 %q，期望保持「重要背景说明」", got)
	}
	if got := out["location"]; got != "302 会议室" {
		t.Fatalf("location 期望更新为「302 会议室」，实际 %q", got)
	}

	// 显式传空字符串仍应清空，PATCH 的两种语义都要保留
	code, out = e.jsonReq(t, http.MethodPatch, fmt.Sprintf("/api/meetings/%d", mid), map[string]any{
		"title": "月度例会（改）", "description": "",
	})
	if code != http.StatusOK {
		t.Fatalf("清空描述期望 200，实际 %d：%v", code, out)
	}
	if got := out["description"]; got != "" {
		t.Fatalf("显式传空应清空 description，实际 %q", got)
	}
	if got := out["location"]; got != "302 会议室" {
		t.Fatalf("未提交的 location 不应被清空，实际 %q", got)
	}
}

// TestDeleteMeetingCascadesAttachments 删除会议时，附件记录与磁盘文件都要清理。
func TestDeleteMeetingCascadesAttachments(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	mid := e.draftMeeting(t, orgID, map[string]any{
		"title": "月度例会", "org_ids": []uint{orgID},
	})
	itemID := e.addItem(t, mid, orgID)
	_, filePath := e.addAttachment(t, itemID, []byte("附件内容"))

	conclusion := &model.Conclusion{MeetingID: mid, ReportItemID: itemID, Content: "结论"}
	if err := e.db.Create(conclusion).Error; err != nil {
		t.Fatalf("创建结论失败: %v", err)
	}

	code, out := e.jsonReq(t, http.MethodDelete, fmt.Sprintf("/api/meetings/%d", mid), nil)
	if code != http.StatusOK {
		t.Fatalf("删除会议期望 200，实际 %d：%v", code, out)
	}

	for name, count := range map[string]int64{
		"meetings":           countRows(e.db, "meetings", "id = ?", mid),
		"report_items":       countRows(e.db, "report_items", "meeting_id = ?", mid),
		"report_attachments": countRows(e.db, "report_attachments", "meeting_id = ?", mid),
		"conclusions":        countRows(e.db, "conclusions", "meeting_id = ?", mid),
		"meeting_orgs":       countRows(e.db, "meeting_orgs", "meeting_id = ?", mid),
	} {
		if count != 0 {
			t.Fatalf("%s 仍有 %d 条残留记录", name, count)
		}
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("附件文件未被删除：%s", filePath)
	}
}

// TestDeleteItemCascadesAttachments 删除汇报事项时，其附件记录与文件都要清理。
func TestDeleteItemCascadesAttachments(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	mid := e.draftMeeting(t, orgID, map[string]any{
		"title": "月度例会", "org_ids": []uint{orgID},
	})
	itemID := e.addItem(t, mid, orgID)
	attID, filePath := e.addAttachment(t, itemID, []byte("附件内容"))

	code, out := e.jsonReq(t, http.MethodDelete, fmt.Sprintf("/api/items/%d", itemID), nil)
	if code != http.StatusOK {
		t.Fatalf("删除事项期望 200，实际 %d：%v", code, out)
	}
	if n := countRows(e.db, "report_attachments", "id = ?", attID); n != 0 {
		t.Fatalf("附件记录残留 %d 条", n)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("附件文件未被删除：%s", filePath)
	}
	// 同会议的其它数据不受影响
	if n := countRows(e.db, "meetings", "id = ?", mid); n != 1 {
		t.Fatalf("删除事项不应影响会议本身，当前会议数 %d", n)
	}
}

// TestUploadAttachmentOversized 超限时仍返回原有的明确提示（新增的请求体上限不应改变文案）。
func TestUploadAttachmentOversized(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	mid := e.draftMeeting(t, orgID, map[string]any{
		"title": "月度例会", "org_ids": []uint{orgID},
	})
	itemID := e.addItem(t, mid, orgID)

	// 边界外但仍在请求体上限内：由声明大小检查拦截
	oversized := bytes.Repeat([]byte("x"), maxAttachmentSize+1024)
	code, out := e.upload(t, fmt.Sprintf("/api/items/%d/attachments", itemID), "超大文件.txt", oversized)
	if code != http.StatusBadRequest {
		t.Fatalf("超大附件期望 400，实际 %d：%v", code, out)
	}
	if got, _ := out["detail"].(string); got != tooLargeMsg {
		t.Fatalf("期望提示 %q，实际 %q", tooLargeMsg, got)
	}
	if n := countRows(e.db, "report_attachments", "report_item_id = ?", itemID); n != 0 {
		t.Fatalf("超限上传不应留下记录，当前 %d 条", n)
	}

	// 超出请求体硬上限（单文件上限 + multipart 余量）：必须被早于落盘处拒绝，且提示文案不变
	huge := bytes.Repeat([]byte("x"), maxAttachmentSize+3*1024*1024)
	code, out = e.upload(t, fmt.Sprintf("/api/items/%d/attachments", itemID), "巨大文件.txt", huge)
	if code != http.StatusBadRequest {
		t.Fatalf("超出请求体上限期望 400，实际 %d：%v", code, out)
	}
	if got, _ := out["detail"].(string); got != tooLargeMsg {
		t.Fatalf("超出请求体上限的提示应为 %q，实际 %q", tooLargeMsg, got)
	}
	if n := countRows(e.db, "report_attachments", "report_item_id = ?", itemID); n != 0 {
		t.Fatalf("被拒的上传不应留下记录，当前 %d 条", n)
	}

	// 边界内的大小正常接受
	code, out = e.upload(t, fmt.Sprintf("/api/items/%d/attachments", itemID), "正常.txt", []byte("正常内容"))
	if code != http.StatusCreated {
		t.Fatalf("正常附件期望 201，实际 %d：%v", code, out)
	}
}

// TestMaterialSlidesTeamsBeforeDepts 锁定材料展示顺序：小组汇报在部门汇报之前（设计决定，非缺陷）。
// 构造时故意让部门先入会，以区分「按类型优先」与「按入会顺序」。
func TestMaterialSlidesTeamsBeforeDepts(t *testing.T) {
	e := newEnv(t)
	dept := e.newOrg(t, "综合部")
	team := e.newTeam(t, "运维组", dept)
	mid := e.draftMeeting(t, dept, map[string]any{
		"title": "月度例会", "org_ids": []uint{dept, team},
	})
	e.addItem(t, mid, dept)
	e.addItem(t, mid, team)

	code, out := e.jsonReq(t, http.MethodGet, fmt.Sprintf("/api/meetings/%d/material", mid), nil)
	if code != http.StatusOK {
		t.Fatalf("查看材料期望 200，实际 %d：%v", code, out)
	}
	raw, _ := out["slides"].([]any)
	if len(raw) != 2 {
		t.Fatalf("期望 2 页材料，实际 %d", len(raw))
	}
	first, _ := raw[0].(map[string]any)
	second, _ := raw[1].(map[string]any)
	if first["org_type"] != model.OrgTypeTeam {
		t.Fatalf("首页应为小组汇报，实际 org_type=%v", first["org_type"])
	}
	if second["org_type"] != model.OrgTypeDept {
		t.Fatalf("次页应为部门汇报，实际 org_type=%v", second["org_type"])
	}
	if first["dept_name"] != "综合部" {
		t.Fatalf("小组页应展示所属部门名，实际 %v", first["dept_name"])
	}
	if idx, _ := first["index"].(float64); idx != 1 {
		t.Fatalf("首页 index 应为 1，实际 %v", first["index"])
	}
}

// TestConclusionWritesRequireAdminOrCreator 结论/任务的增删改仅限管理员（或会议创建者），
// 参会组织负责人仍可查看。
func TestConclusionWritesRequireAdminOrCreator(t *testing.T) {
	e := newEnv(t)
	dept := e.newOrg(t, "综合部")
	mid := e.draftMeeting(t, dept, map[string]any{
		"title": "月度例会", "org_ids": []uint{dept},
	})
	itemID := e.addItem(t, mid, dept)
	orgID := dept
	leaderToken := e.tokenForUser(t, &model.User{
		Username: "dept_leader", Name: "部门负责人", Role: model.RoleDeptLeader, OrgID: &orgID,
	})

	// 结论仅进行中可写，先把会议推到进行中
	if code, out := e.jsonReq(t, http.MethodPost, fmt.Sprintf("/api/meetings/%d/start", mid), nil); code != http.StatusOK {
		t.Fatalf("开始会议期望 200，实际 %d：%v", code, out)
	}

	concPath := fmt.Sprintf("/api/meetings/%d/conclusions", mid)
	body := map[string]any{"report_item_id": itemID, "kind": model.ConclusionKind, "content": "下周一提交方案"}

	// 负责人可读，不泄露权限；但不可写
	if code, _ := e.jsonReqAs(t, leaderToken, http.MethodGet, concPath, nil); code != http.StatusOK {
		t.Fatalf("负责人查看结论期望 200")
	}
	code, out := e.jsonReqAs(t, leaderToken, http.MethodPost, concPath, body)
	if code != http.StatusForbidden {
		t.Fatalf("负责人新增结论期望 403，实际 %d：%v", code, out)
	}
	if got, _ := out["detail"].(string); !strings.Contains(got, "仅管理员或会议创建者") {
		t.Fatalf("提示应说明写权限归属，实际 %q", got)
	}

	code, created := e.jsonReq(t, http.MethodPost, concPath, body)
	if code != http.StatusCreated {
		t.Fatalf("管理员新增结论期望 201，实际 %d：%v", code, created)
	}
	concID := uint(created["id"].(float64))

	patch := map[string]any{"report_item_id": itemID, "kind": model.ActionKind, "content": "负责人跟进"}
	itemPath := fmt.Sprintf("/api/conclusions/%d", concID)
	if code, out := e.jsonReqAs(t, leaderToken, http.MethodPatch, itemPath, patch); code != http.StatusForbidden {
		t.Fatalf("负责人修改结论期望 403，实际 %d：%v", code, out)
	}
	if code, out := e.jsonReqAs(t, leaderToken, http.MethodDelete, itemPath, nil); code != http.StatusForbidden {
		t.Fatalf("负责人删除结论期望 403，实际 %d：%v", code, out)
	}
	if n := countRows(e.db, "conclusions", "id = ?", concID); n != 1 {
		t.Fatalf("被拒的写操作不应改变数据，当前结论数 %d", n)
	}

	if code, out := e.jsonReq(t, http.MethodPatch, itemPath, patch); code != http.StatusOK {
		t.Fatalf("管理员修改结论期望 200，实际 %d：%v", code, out)
	}
	if code, out := e.jsonReq(t, http.MethodDelete, itemPath, nil); code != http.StatusOK {
		t.Fatalf("管理员删除结论期望 200，实际 %d：%v", code, out)
	}

	// 会议结束后，连管理员也不能再写（原有状态限定不变）
	if code, out := e.jsonReq(t, http.MethodPost, fmt.Sprintf("/api/meetings/%d/finish", mid), nil); code != http.StatusOK {
		t.Fatalf("结束会议期望 200，实际 %d：%v", code, out)
	}
	if code, out := e.jsonReq(t, http.MethodPost, concPath, body); code != http.StatusBadRequest {
		t.Fatalf("已结束的会议新增结论期望 400，实际 %d：%v", code, out)
	}
}

// TestPasswordChangeRevokesOldTokens 改密后旧令牌失效，同时接口为当前设备换发新令牌。
func TestPasswordChangeRevokesOldTokens(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	user := &model.User{Username: "zhang", Name: "张三", Role: model.RoleMember, OrgID: &orgID}
	user.SetPassword("old123456")
	if err := e.db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldToken := "Bearer " + mustToken(t, e.auth, user.ID, user.TokenVersion)

	if code, _ := e.jsonReqAs(t, oldToken, http.MethodGet, "/api/auth/me", nil); code != http.StatusOK {
		t.Fatalf("改密前旧令牌应可用，实际 %d", code)
	}

	code, out := e.jsonReqAs(t, oldToken, http.MethodPatch, "/api/auth/password", map[string]any{
		"old_password": "old123456", "new_password": "new123456",
	})
	if code != http.StatusOK {
		t.Fatalf("修改密码期望 200，实际 %d：%v", code, out)
	}
	newToken, _ := out["access_token"].(string)
	if newToken == "" {
		t.Fatalf("改密接口应换发新令牌，实际响应：%v", out)
	}

	if code, _ := e.jsonReqAs(t, oldToken, http.MethodGet, "/api/auth/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("改密后旧令牌应被拒（401），实际 %d", code)
	}
	if code, _ := e.jsonReqAs(t, "Bearer "+newToken, http.MethodGet, "/api/auth/me", nil); code != http.StatusOK {
		t.Fatalf("新令牌应可用，实际 %d", code)
	}
	var reloaded model.User
	if err := e.db.First(&reloaded, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.TokenVersion != 1 {
		t.Fatalf("token_version 应由 0 变为 1，实际 %d", reloaded.TokenVersion)
	}

	// 平滑升级：无 tv 声明的旧版令牌在版本未变时仍应有效
	legacy := mustLegacyToken(t, e.cfg, reloaded.ID)
	if code, _ := e.jsonReqAs(t, "Bearer "+legacy, http.MethodGet, "/api/auth/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("库内 token_version=1 时旧版令牌应被拒，实际 %d", code)
	}
	fresh := &model.User{Username: "li", Name: "李四", Role: model.RoleMember, OrgID: &orgID}
	fresh.SetPassword("pass123456")
	if err := e.db.Create(fresh).Error; err != nil {
		t.Fatalf("create fresh user: %v", err)
	}
	legacy2 := mustLegacyToken(t, e.cfg, fresh.ID)
	if code, _ := e.jsonReqAs(t, "Bearer "+legacy2, http.MethodGet, "/api/auth/me", nil); code != http.StatusOK {
		t.Fatalf("升级前签发的令牌（无 tv、库内 version=0）应保持有效，实际 %d", code)
	}
}

// TestLoginFailRateLimit 同一来源对同一账号连续失败后限流，不影响其它账号。
func TestLoginFailRateLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < loginMaxFails; i++ {
		code, out := e.loginReq(t, "ghost", fmt.Sprintf("wrong-%d", i))
		if code != http.StatusBadRequest {
			t.Fatalf("第 %d 次失败登录期望 400，实际 %d：%v", i+1, code, out)
		}
	}
	// 达上限后即使凭证正确也需先等窗口过去（防爆破的真实语义）
	code, out := e.loginReq(t, "ghost", "whatever")
	if code != http.StatusTooManyRequests {
		t.Fatalf("超过阀值后期望 429，实际 %d：%v", code, out)
	}
	if got, _ := out["detail"].(string); got == "" {
		t.Fatalf("429 应带可读提示")
	}
	// 其它账号不受影响
	if code, out := e.loginReq(t, "admin", "admin123"); code != http.StatusOK {
		t.Fatalf("未被限流的账号应正常登录，实际 %d：%v", code, out)
	}
}

// TestForgotResetHidesAccountExistence 账号不存在与安全答案错误必须返回同一文案。
func TestForgotResetHidesAccountExistence(t *testing.T) {
	e := newEnv(t)
	orgID := e.newOrg(t, "综合部")
	noQuestion := &model.User{Username: "wang", Name: "王五", Role: model.RoleMember, OrgID: &orgID}
	noQuestion.SetPassword("pass123456")
	if err := e.db.Create(noQuestion).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	body := func(username string) map[string]any {
		return map[string]any{"username": username, "security_answer": "答案", "new_password": "new123456"}
	}
	codeA, outA := e.jsonReqAs(t, "", http.MethodPost, "/api/auth/forgot-reset", body("definitely-not-exists"))
	codeB, outB := e.jsonReqAs(t, "", http.MethodPost, "/api/auth/forgot-reset", body("wang"))
	if codeA != http.StatusBadRequest || codeB != http.StatusBadRequest {
		t.Fatalf("两种失败应同为 400，实际 %d / %d", codeA, codeB)
	}
	detailA, _ := outA["detail"].(string)
	detailB, _ := outB["detail"].(string)
	if detailA != detailB || detailA == "" {
		t.Fatalf("错误文案应一致且不泄露账号是否存在，实际 %q / %q", detailA, detailB)
	}
}

// TestDeptLeaderMeetingScope 锁定会议可见范围口径（业务裁定）：
// 部门负责人只能看到本组织直接参会的会议，下属小组单独参会的会议一律不外扩。
func TestDeptLeaderMeetingScope(t *testing.T) {
	e := newEnv(t)
	dept := e.newOrg(t, "综合部")
	team := e.newTeam(t, "运维组", dept)
	deptID, teamID := dept, team
	deptLeader := e.tokenForUser(t, &model.User{
		Username: "dept_boss", Name: "综合部主任", Role: model.RoleDeptLeader, OrgID: &deptID,
	})
	teamLeader := e.tokenForUser(t, &model.User{
		Username: "team_boss", Name: "运维组长", Role: model.RoleTeamLeader, OrgID: &teamID,
	})

	// M1 仅下属小组参会；M2 部门与小组同时参会
	m1 := e.draftMeeting(t, team, map[string]any{"title": "小组专题会", "org_ids": []uint{team}})
	e.addItem(t, m1, team)
	m2 := e.draftMeeting(t, dept, map[string]any{"title": "月度例会", "org_ids": []uint{dept, team}})
	e.addItem(t, m2, dept)
	e.addItem(t, m2, team)

	listed := func(token string) map[uint]bool {
		t.Helper()
		code, out := e.jsonReqAs(t, token, http.MethodGet, "/api/meetings?page=1&page_size=50", nil)
		if code != http.StatusOK {
			t.Fatalf("会议列表期望 200，实际 %d：%v", code, out)
		}
		got := map[uint]bool{}
		raw, _ := out["items"].([]any)
		for _, m := range raw {
			row, _ := m.(map[string]any)
			id, _ := row["id"].(float64)
			got[uint(id)] = true
		}
		return got
	}

	ds := listed(deptLeader)
	if ds[m1] {
		t.Fatalf("部门负责人不应看到仅由下属小组参会的会议（id=%d）", m1)
	}
	if !ds[m2] {
		t.Fatalf("部门负责人应看到本部门直接参会的会议（id=%d），实际列表 %v", m2, ds)
	}
	// 反向约束：小组负责人当然能看到自己参会的会议
	if ts := listed(teamLeader); !ts[m1] {
		t.Fatalf("小组负责人应看到本组参会的会议（id=%d）", m1)
	}

	// 对 M1 的任何入口都必须拒绝
	for _, path := range []string{
		fmt.Sprintf("/api/meetings/%d", m1),
		fmt.Sprintf("/api/meetings/%d/items", m1),
		fmt.Sprintf("/api/meetings/%d/material", m1),
		fmt.Sprintf("/api/meetings/%d/conclusions", m1),
		fmt.Sprintf("/api/meetings/%d/minutes", m1),
	} {
		if code, out := e.jsonReqAs(t, deptLeader, http.MethodGet, path, nil); code != http.StatusForbidden {
			t.Fatalf("GET %s 期望 403，实际 %d：%v", path, code, out)
		}
	}
	if code, out := e.jsonReqAs(t, deptLeader, http.MethodPost, fmt.Sprintf("/api/meetings/%d/items", m1),
		map[string]any{"org_id": team, "title": "代录", "content": "内容"}); code != http.StatusForbidden {
		t.Fatalf("部门负责人代下属小组录入事项期望 403，实际 %d：%v", code, out)
	}
	if n := countRows(e.db, "report_items", "meeting_id = ? AND org_id = ?", m1, team); n != 1 {
		t.Fatalf("被拒的录入不应落库，当前 %d 条", n)
	}

	// M2 详情不得夹带事项正文（前端一律走 /items，那里按组织过滤）
	code, det := e.jsonReqAs(t, deptLeader, http.MethodGet, fmt.Sprintf("/api/meetings/%d", m2), nil)
	if code != http.StatusOK {
		t.Fatalf("会议详情期望 200，实际 %d：%v", code, det)
	}
	if its, ok := det["items"]; ok {
		t.Fatalf("详情不应返回 items：%v", its)
	}

	// M2 材料页保留部门汇总视角（含已参会的下属小组事项）
	code, mat := e.jsonReqAs(t, deptLeader, http.MethodGet, fmt.Sprintf("/api/meetings/%d/material", m2), nil)
	if code != http.StatusOK {
		t.Fatalf("部门负责人查看本部门参会会议的材料期望 200，实际 %d：%v", code, mat)
	}
	if slides, _ := mat["slides"].([]any); len(slides) != 2 {
		t.Fatalf("材料页应含本部门与下属小组共 2 页，实际 %d", len(slides))
	}
	// 事项列表仍精确到本部门（与材料页的差异是已知现状，见 selfOrgIDs 注释）
	code, items := e.arrayReqAs(t, deptLeader, http.MethodGet, fmt.Sprintf("/api/meetings/%d/items", m2), nil)
	if code != http.StatusOK {
		t.Fatalf("事项列表期望 200，实际 %d", code)
	}
	if len(items) != 1 || uint(items[0]["org_id"].(float64)) != dept {
		t.Fatalf("事项列表应只含本部门 1 条，实际 %d 条：%v", len(items), items)
	}
}

// pngBytes PNG 文件头。后端不解析像素，只看扩展名与大小，所以无需一张真图。
var pngBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// uploadAvatar 以当前 token 发送 multipart 头像上传。
func (e *testEnv) uploadAvatar(t *testing.T, filename string, content []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/avatar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", e.token)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	detail, _ := out["detail"].(string)
	return w.Code, detail
}

// TestUploadAvatarFlow 头像上传闭环：能存能读、换图后旧文件被清理、会话仍然有效、非法扩展名被拒。
func TestUploadAvatarFlow(t *testing.T) {
	e := newEnv(t)

	code, detail := e.uploadAvatar(t, "avatar.png", pngBytes)
	if code != http.StatusOK {
		t.Fatalf("上传 PNG 头像期望 200，实际 %d：%s", code, detail)
	}
	avatarOf := func() string {
		t.Helper()
		var user model.User
		if err := e.db.First(&user, 1).Error; err != nil {
			t.Fatalf("load user: %v", err)
		}
		return user.Avatar
	}
	first := avatarOf()
	if !strings.HasPrefix(first, "1_") || !strings.HasSuffix(first, ".png") {
		t.Fatalf("头像存储命名不符合预期：%q", first)
	}
	stored := filepath.Join(e.cfg.UploadDir, "avatars", first)
	if _, err := os.Stat(stored); err != nil {
		t.Fatalf("头像文件未落盘：%v", err)
	}

	// 读取接口公开可用（无 token）
	if w := e.do(t, "", http.MethodGet, "/api/avatar/"+first, nil); w.Code != http.StatusOK || w.Body.Len() == 0 {
		t.Fatalf("读取头像期望 200 且非空，实际 %d / %d 字节", w.Code, w.Body.Len())
	}
	// 上传后缓存失效不应误伤当前会话（否则用户传完头像就被踢）
	if code, _ := e.jsonReqAs(t, e.token, http.MethodGet, "/api/auth/me", nil); code != http.StatusOK {
		t.Fatalf("上传头像后 /auth/me 期望 200，实际 %d", code)
	}

	// 再传一次：新文件写入、旧文件删除，不留垃圾
	time.Sleep(2 * time.Millisecond)
	if code, detail := e.uploadAvatar(t, "avatar.png", pngBytes); code != http.StatusOK {
		t.Fatalf("替换头像期望 200，实际 %d：%s", code, detail)
	}
	second := avatarOf()
	if second == first {
		t.Fatalf("替换后头像文件名未变：%q", second)
	}
	if _, err := os.Stat(filepath.Join(e.cfg.UploadDir, "avatars", second)); err != nil {
		t.Fatalf("新头像未落盘：%v", err)
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Fatalf("旧头像应被删除，实际仍在：%v", err)
	}

	// 扩展名校验：旧的“拼接后 Contains”写法会放行 .web / .jpe 这类子串
	for _, name := range []string{"evil.web", "evil.jpe", "page.html", "noext"} {
		if code, _ := e.uploadAvatar(t, name, pngBytes); code != http.StatusBadRequest {
			t.Fatalf("上传 %s 期望 400，实际 %d", name, code)
		}
	}
	if n := countRows(e.db, "users", "1 = 1"); n != 1 {
		t.Fatalf("失败请求不应影响数据，当前用户数 %d", n)
	}
}

func mustToken(t *testing.T, auth *middleware.Auth, userID uint, version int) string {
	t.Helper()
	token, err := auth.CreateToken(userID, version)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

// mustLegacyToken 签一个不含 tv 声明的令牌，等价于本次升级前已发出的凭证。
func mustLegacyToken(t *testing.T, cfg *config.Config, userID uint) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.SecretKey))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}
	return token
}

func countRows(db *gorm.DB, table string, where string, args ...any) int64 {
	var n int64
	q := db.Table(table)
	if where != "" {
		q = q.Where(where, args...)
	}
	q.Count(&n)
	return n
}
