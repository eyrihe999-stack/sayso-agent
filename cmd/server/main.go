package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"sayso-agent/config"
	"sayso-agent/internal/client/feishu"
	"sayso-agent/internal/client/llm"
	"sayso-agent/internal/client/slack"
	"sayso-agent/internal/handler"
	"sayso-agent/internal/service"
)

func main() {
	// 按环境加载配置（APP_ENV=local|dev|prod）
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ginMode := cfg.Server.Mode
	if os.Getenv("GIN_MODE") != "" {
		ginMode = os.Getenv("GIN_MODE")
	}
	gin.SetMode(ginMode)

	// 构建 LLM 客户端
	llmClient := llm.NewClient(llm.Config{
		APIKey:  cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL,
		Model:   cfg.LLM.Model,
	})

	// 构建飞书客户端
	feishuCfg := feishu.Config{
		AppID:     cfg.Feishu.AppID,
		AppSecret: cfg.Feishu.AppSecret,
		BotToken:  cfg.Feishu.BotToken,
		Domain:    cfg.Feishu.Domain,
		Enabled:   cfg.Feishu.Enabled,
	}
	feishuClient := feishu.NewClient(feishuCfg)

	// 构建 Slack 客户端
	slackCfg := slack.Config{
		BotToken: cfg.Slack.BotToken,
		Enabled:  cfg.Slack.Enabled,
	}
	slackClient := slack.NewClient(slackCfg)

	// 服务层
	llmSvc := service.NewLLMService(llmClient)
	folderMatcher := service.NewFolderMatcher(llmSvc)
	executor := service.NewExecutor(feishuClient, slackClient, feishuCfg, slackCfg, folderMatcher)
	asrSvc := service.NewASRService(llmSvc, executor)

	// 路由
	r := handler.Router(asrSvc)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("server starting at %s (env=%s)", addr, getEnv())
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func getEnv() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		return "local"
	}
	return env
}
