# 技术部门会议系统（meeting）— Nginx 生产部署配置

> 本文档说明如何将 meeting 系统部署到生产环境：Go 后端以服务方式常驻，Nginx 托管前端静态资源并反向代理后端 API。

## 一、部署架构

```
浏览器
   │ HTTPS (443)
   ▼
Nginx
   ├── /             → 前端静态资源（frontend/dist）
   │                     SPA history 回退到 index.html
   └── /api/**       → 反向代理到 Go 后端 127.0.0.1:8002
                        （含附件 /api/attachments/:id/file、头像 /api/avatar/:name）
```

- 前端为 SPA（Vue Router history 模式），需配置 `try_files ... /index.html` 回退。
- 附件与头像均通过后端 `/api` 接口下发（附件带鉴权、头像公开），**无需**在 Nginx 上托管 `uploads/` 目录。
- 数据库（SQLite）与上传文件位于后端进程工作目录（`backend-go/`），确保后端以其目录为工作目录启动。

## 二、目录规划

```text
/opt/meeting/
├── backend-go/
│   ├── meeting-server        # 编译后的后端可执行文件
│   ├── .env                  # 环境变量（含 SECRET_KEY 等）
│   └── uploads/              # 附件、头像上传目录（后端自动创建）
├── frontend/
│   └── dist/                 # 前端构建产物
└── deploy/                   # 部署脚本
```

## 三、后端部署

### 1. 编译后端

```bash
cd backend-go
go build -o meeting-server ./cmd/server
```

> Windows 下需先设置 `set "GOTOOLCHAIN=local"`。

### 2. 准备环境变量（`.env`）

```ini
PORT=8002
DATABASE_URL=sqlite:///./meeting.db
SECRET_KEY=请替换为随机强密钥
INIT_ADMIN_USERNAME=admin
INIT_ADMIN_PASSWORD=请修改为强密码
UPLOAD_DIR=uploads
# 可信代理：默认 127.0.0.1,::1 已适配下文“nginx 与应用同机”的部署
TRUSTED_PROXIES=127.0.0.1,::1
# 前后端同域反代（如下文配置）无需设置跨域白名单，此项留空即可
# CORS_ALLOWED_ORIGINS=
```

> 生产环境务必修改 `SECRET_KEY` 与 `INIT_ADMIN_PASSWORD`（后者仅首次建库时写入，已存在数据时不会覆盖）。
>
> 若 nginx 与后端不在同一台机器（或 nginx 跑在 Docker 网络里），必须把它的地址加入 `TRUSTED_PROXIES`，
> 否则操作日志里的 IP 会变成 nginx 的对端地址而不是真实客户端 IP。

### 3. 后端常驻服务（Linux systemd）

新建 `/etc/systemd/system/meeting.service`：

```ini
[Unit]
Description=Meeting Backend (Gin + SQLite)
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/meeting/backend-go
ExecStart=/opt/meeting/backend-go/meeting-server
Restart=always
RestartSec=3
# 建议用独立低权限用户运行
User=www-data
Group=www-data

[Install]
WantedBy=multi-user.target
```

启动并设置开机自启：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now meeting
sudo systemctl status meeting
```

> Windows 环境可改用 NSSM（`nssm install meeting`）或任务计划程序注册为服务，同样以 `backend-go` 为工作目录启动 `meeting-server.exe`。

## 四、前端构建

```bash
cd frontend
npm install
npm run build
# 产物输出到 frontend/dist
```

## 五、Nginx 配置

### 5.1 完整配置（HTTP）

将以下内容保存为 `/etc/nginx/conf.d/meeting.conf`：

```nginx
# 上传限制：附件单文件最大 5MB、头像 2MB，这里放宽到 20MB 留余量
client_max_body_size 20m;

upstream meeting_backend {
    server 127.0.0.1:8002;
    keepalive 16;
}

server {
    listen 80;
    server_name inkpot.cn;          # 替换为你的域名或服务器 IP

    # 前端静态资源
    root /opt/meeting/frontend/dist;
    index index.html;

    # gzip 压缩（接口 JSON 与文本收益明显）
    gzip on;
    gzip_min_length 1k;
    gzip_comp_level 5;
    gzip_types
        text/plain
        text/css
        text/javascript
        application/json
        application/javascript
        application/xml
        image/svg+xml;
    gzip_vary on;

    # 带 hash 的静态资源：长缓存
    location /assets/ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        try_files $uri =404;
    }

    # 后端 API 反向代理
    location /api/ {
        proxy_pass http://meeting_backend;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection        "";
        # 附件上传/下载、材料接口可能耗时较长
        proxy_connect_timeout 10s;
        proxy_read_timeout    120s;
        proxy_send_timeout    120s;
        # 大文件上传（附件、头像）不缓存
        proxy_request_buffering on;
        proxy_buffering off;
    }

    # SPA 路由回退（history 模式）
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

### 5.2 HTTPS 配置（推荐）

生产环境强烈建议启用 HTTPS。使用 Let's Encrypt 证书：

```nginx
# HTTP 强制跳转 HTTPS
server {
    listen 80;
    server_name inkpot.cn;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name inkpot.cn;

    # 证书（路径按实际调整）
    ssl_certificate     /etc/letsencrypt/live/inkpot.cn/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/inkpot.cn/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    client_max_body_size 20m;

    root /opt/meeting/frontend/dist;
    index index.html;

    # gzip（同 5.1）
    gzip on;
    gzip_min_length 1k;
    gzip_comp_level 5;
    gzip_types text/plain text/css text/javascript application/json application/javascript application/xml image/svg+xml;
    gzip_vary on;

    location /assets/ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        try_files $uri =404;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8002;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection        "";
        proxy_connect_timeout 10s;
        proxy_read_timeout    120s;
        proxy_send_timeout    120s;
        proxy_buffering off;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

## 六、配置校验与启动

```bash
# 校验语法
sudo nginx -t

# 重载配置
sudo nginx -s reload

# 或重启
sudo systemctl restart nginx
```

## 七、关键配置项说明

| 配置项 | 值 | 说明 |
| --- | --- | --- |
| `client_max_body_size` | `20m` | 附件单文件上限 5MB、头像 2MB，放宽留余量 |
| `proxy_read_timeout` | `120s` | 材料/纪要接口与附件下载耗时可能较长 |
| `proxy_buffering off` | — | 大文件流式传输，避免 Nginx 缓冲占用内存 |
| `try_files ... /index.html` | — | SPA history 模式路由回退，必须保留 |
| `location /assets/` 长缓存 | `30d` | 前端构建产物带 hash，可安全长缓存 |
| gzip | on | 压缩 JSON/JS/CSS，降低传输体积（此前性能优化的补充） |

## 八、部署检查清单

- [ ] 后端 `meeting-server` 已编译，`backend-go` 为工作目录启动（否则 SQLite/上传目录路径错位）
- [ ] `.env` 已配置 `SECRET_KEY` 强随机值、`INIT_ADMIN_PASSWORD` 强密码
- [ ] 后端监听 `127.0.0.1:8002`（未暴露到公网，仅 Nginx 访问）
- [ ] 前端 `npm run build` 完成，`dist` 路径与 Nginx `root` 一致
- [ ] `nginx -t` 校验通过并 reload
- [ ] 防火墙仅放行 80/443，不放行 8002 与 5174
- [ ] 不要再用 `npm run dev`（Vite dev server）对外提供服务
- [ ] 首次登录后立即修改管理员初始密码

## 九、故障排查

| 现象 | 排查方向 |
| --- | --- |
| 前端页面刷新 404 | 确认 `try_files ... /index.html` 已配置 |
| 接口 502 | 后端是否已启动、`127.0.0.1:8002` 是否可达（`curl http://127.0.0.1:8002/api/auth/me`） |
| 附件上传失败 | `client_max_body_size` 是否过小、后端工作目录 `uploads/` 是否可写 |
| 图片/头像不显示 | 头像为公开接口 `/api/avatar/:name`，确认反代未拦截 |
| 数据库报 locked | 已启用 WAL + busy_timeout，检查后端是否多实例同时运行（SQLite 单实例运行） |
