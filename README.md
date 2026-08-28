# 技术部门会议系统（meeting）

用于召开技术部门会议的应用，覆盖三大场景：**汇报事项录入 → 会议展示与记录 → 自动生成会议纪要**。

## 功能简介

1. **汇报事项录入**：管理员创建会议并指定参会组织（部门 / 小组）。每个参会组织对应一个负责人账号，负责人登录后录入本组织的汇报事项。每个事项支持上传最多 **10 个附件**（单文件 ≤5MB，支持图片 / 视频 / PDF / Office / 文本等常用格式）。
2. **会议展示与记录**：会议筹备期主持人可多次生成会议材料（会前预览）；「开始会议」后系统按「部门 → 小组 → 事项」自动组织材料，以类似 PPT 的方式逐条全屏展示，正式材料仅可生成一次；主持人在每个事项下可直接录入讨论结论或形成新任务。
3. **自动生成会议纪要**：主持人一键生成 Markdown 格式会议纪要（含参会组织、议程、汇报内容、讨论结论、待办任务汇总），支持在线预览与下载；归档前可多次生成，归档后只读。
4. **会议归档**：会议结束后可归档，归档后会议材料与纪要只能查看，不再允许生成或修改。

## 技术栈

- 前端：Vue 3 + Element Plus + Vite + Pinia + Vue Router（端口 5174）
- 后端：Go（Gin + GORM），数据库 SQLite（端口 8002）
- 认证：JWT（Bearer Token，有效期 7 天）

## 目录结构

```
meeting/
├── backend-go/          # Go 后端
│   ├── cmd/server/      # 入口
│   └── internal/
│       ├── config/      # 配置
│       ├── database/    # SQLite 连接与迁移
│       ├── model/       # 数据模型
│       ├── middleware/  # JWT 认证、CORS
│       └── handler/     # 业务处理
└── frontend/            # Vue 前端
    └── src/
        ├── api/         # axios 封装
        ├── router/      # 路由
        ├── stores/      # Pinia
        ├── layout/      # 布局
        └── views/       # 页面
```

## 快速启动

### 1. 启动后端

```bash
cd backend-go
# Windows cmd 注意：设置 GOTOOLCHAIN=local 后执行
go run ./cmd/server
```

默认监听 `:8002`，首次启动自动建库建表并创建管理员 `admin / admin123`。

可用环境变量（`.env` 或系统环境变量）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `8002` | 后端端口 |
| `DATABASE_URL` | `sqlite:///./meeting.db` | SQLite 文件路径 |
| `SECRET_KEY` | 随机串 | JWT 签名密钥（生产必须修改） |
| `INIT_ADMIN_USERNAME` | `admin` | 初始管理员用户名 |
| `INIT_ADMIN_PASSWORD` | `admin123` | 初始管理员密码 |

### 2. 启动前端

```bash
cd frontend
npm install
npm run dev
```

访问 `http://localhost:5174`。前端通过 Vite 代理将 `/api` 转发至后端 `8002`（可用 `VITE_BACKEND_PORT` 覆盖）。

### 3. 使用流程

1. **管理员登录**（`admin / admin123`）
2. **组织管理**：先建部门，再在部门下建小组
3. **账号管理**：账号角色分四类——管理员、部门负责人、小组负责人、组织成员。部门负责人须绑定部门、小组负责人须绑定小组（一个部门/小组可设置多个负责人），成员可绑定任意组织；新增与编辑均可调整角色
4. **会议管理 → 创建会议**：在创建弹框中填写会议名称、会议地点、会议时间并选择参会组织；进入详情页可点「编辑」修改会议名称/地点/时间（参会组织仅筹备中可改）
5. **负责人登录**：进入「汇报事项录入」，选择筹备中的会议，录入本组织汇报事项
6. **管理员**：会议详情 → 「生成会议材料」（会前可多次生成预览）
7. **开始会议**：状态变为「进行中」；此时「生成正式会议材料」仅可执行一次
8. **会议展示**：全屏逐条展示，逐条录入讨论结论 / 新任务（讨论结论归档后只读）
9. **结束会议** → **会议归档**：归档前纪要可多次生成；归档时若未生成纪要会询问是否先生成；归档后材料与纪要只读
10. **生成纪要**：会议详情 → 「生成会议纪要」，在线预览或下载 `.md`

## 会议状态流转

```
draft(筹备) --开始会议--> ongoing(进行中) --结束会议--> finished(已结束) --会议归档--> archived(已归档)
```

| 状态 | 材料 | 纪要 | 讨论结论 | 汇报事项 |
| --- | --- | --- | --- | --- |
| draft 筹备 | 可多次生成 | 可生成 | 可录入 | 可录入/修改 |
| ongoing 进行中 | 可正式生成一次 | 可生成 | 可录入 | 只读 |
| finished 已结束 | 可正式生成一次 | 可生成 | 可录入 | 只读 |
| archived 已归档 | 只读查看 | 只读查看 | 只读 | 只读 |

「会议展示」始终可进入查看；「生成材料 / 生成纪要」不再自动改变会议状态，状态只能通过「开始会议 / 结束会议 / 会议归档」按钮变更。

### 会议保密属性

会议可标记为**保密**（创建/编辑会议时通过「是否保密」开关设置）：

- **保密会议**：仅参会组织的**负责人**（部门负责人 / 小组负责人）可以查看会议相关内容（列表、详情、材料、纪要、汇报事项、附件），普通组织成员不可见。
- **非保密会议**：相关会议材料向所有参会组织成员公开（成员按所属部门范围查看）。
- **管理员**对所有会议均可见、可操作。

## 数据模型

| 表 | 说明 |
| --- | --- |
| `organizations` | 组织（部门 / 小组树形结构） |
| `users` | 用户账号（角色：管理员 / 部门负责人 / 小组负责人 / 组织成员，非管理员绑定组织） |
| `meetings` | 会议（draft / ongoing / finished / archived，含 `location` 地点与 `is_confidential` 保密标记） |
| `meeting_orgs` | 会议参会组织 |
| `report_items` | 汇报事项（挂在组织下） |
| `report_attachments` | 汇报事项附件（≤10 个/事项，≤5MB/个，存 `uploads/`） |
| `meeting_rooms` | 会议室（名称/位置/容量/启用状态，供会议地点选择） |
| `operation_logs` | 操作日志（登录/关键操作，供管理员查询） |
| `conclusions` | 讨论结论 / 新任务（挂在汇报事项下） |
| `meeting_minutes` | 会议纪要（Markdown） |

## 主要接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/auth/login` | 登录（form-data） |
| GET | `/api/orgs/tree` | 组织树 |
| GET | `/api/rooms` | 会议室列表（所有登录用户可见） |
| POST | `/api/rooms` | 新增会议室（管理员） |
| GET | `/api/meetings` | 会议列表（支持 `keyword` 名称模糊、`start`/`end` 日期范围、`limit`，默认最近 10 条，0 不限） |
| POST | `/api/meetings` | 创建会议（管理员，支持 `location`、`meeting_time`、`is_confidential`、`org_ids`） |
| POST | `/api/meetings/:id/orgs` | 设置参会组织（管理员） |
| GET | `/api/meetings/:id/items` | 汇报事项列表 |
| POST | `/api/meetings/:id/items` | 录入汇报事项 |
| POST | `/api/meetings/:id/material` | 生成会议材料（会前可多次；开始后仅一次） |
| GET | `/api/meetings/:id/material` | 只读查看会议材料 |
| GET | `/api/items/:id/attachments` | 附件列表 |
| POST | `/api/items/:id/attachments` | 上传附件（multipart，字段 `file`，≤5MB） |
| GET | `/api/attachments/:id/file` | 下载/查看附件（带 token） |
| DELETE | `/api/attachments/:id` | 删除附件 |
| POST | `/api/meetings/:id/conclusions` | 添加讨论结论 / 新任务 |
| POST | `/api/meetings/:id/minutes` | 生成会议纪要（归档前可多次） |
| GET | `/api/meetings/:id/minutes` | 获取会议纪要 |
| POST | `/api/meetings/:id/start` | 开始会议（draft → ongoing） |
| POST | `/api/meetings/:id/finish` | 结束会议（ongoing → finished） |
| POST | `/api/meetings/:id/archive` | 归档会议（→ archived） |
| GET | `/api/logs` | 操作日志查询（管理员，支持 keyword/action/target_type/start/end 过滤 + 分页） |

完整接口见 `backend-go/internal/handler/handler.go`。
