package handler

import (
	"github.com/gin-gonic/gin"
	"sayso-agent/internal/middleware"
	"sayso-agent/internal/service"
)

// Router 注册路由与中间件
func Router(svc *service.ASRService) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery(), middleware.Logger())

	asrHandler := NewASRHandler(svc)
	v1 := r.Group("/api/v1")
	{
		v1.POST("/asr/process", asrHandler.Process)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	return r
}
