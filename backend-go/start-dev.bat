@echo off
rem meeting 后端开发启动脚本（Go + Gin + SQLite，端口 8002）
cd /d "%~dp0"
set "GOTOOLCHAIN=local"
go run ./cmd/server
