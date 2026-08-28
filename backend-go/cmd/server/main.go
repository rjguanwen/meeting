package main

import (
	"log"

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
	r.Use(middleware.CORS())

	auth := middleware.NewAuth(cfg, db)
	h := handler.New(db, cfg, auth)
	h.RegisterRoutes(r)

	addr := ":" + cfg.Port
	log.Printf("%s 已启动，监听 %s", cfg.ProjectName, addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
