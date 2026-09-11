package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"meetingbackend/internal/config"
	"meetingbackend/internal/database"
	"meetingbackend/internal/handler"
	"meetingbackend/internal/middleware"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	database.MigrateLegacyRoles(db)
	database.InitAdmin(db, cfg)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	// multipart 内存缓冲上限（gin 默认 32MB）：超出部分落临时文件，降低并发上传时的内存峰值
	r.MaxMultipartMemory = 4 << 20
	// 只采信可信代理转发的 X-Forwarded-For，否则操作日志里的来源 IP 可被客户端随意伪造
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("set trusted proxies: %v", err)
	}
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))

	auth := middleware.NewAuth(cfg, db)
	h := handler.New(db, cfg, auth)
	h.RegisterRoutes(r)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("%s 已启动，监听 %s", cfg.ProjectName, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("run server: %v", err)
		}
	}()

	// 优雅退出：收到信号后停服并 flush 剩余日志
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
	h.Close() // 关闭日志 worker，flush 剩余日志
	log.Println("已退出")
}
