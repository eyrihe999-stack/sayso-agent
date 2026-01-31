package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用总配置，按环境加载
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Feishu   FeishuConfig   `yaml:"feishu"`
	Slack    SlackConfig    `yaml:"slack"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug, release
}

type LLMConfig struct {
	Provider string `yaml:"provider"` // openai, dashscope, etc.
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"`
	Model    string `yaml:"model"`
}

type FeishuConfig struct {
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
	BotToken  string `yaml:"bot_token"` // 机器人 token（可选）
	Domain    string `yaml:"domain"`    // 飞书域名，如 example.feishu.cn，用于生成文档链接
	Enabled   bool   `yaml:"enabled"`
}

type SlackConfig struct {
	BotToken string `yaml:"bot_token"`
	Enabled  bool   `yaml:"enabled"`
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
}

// Load 根据环境变量 APP_ENV 加载对应配置文件
// 支持: local, dev, prod，默认 local
func Load() (*Config, error) {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	path := fmt.Sprintf("config/%s.yaml", env)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// 允许环境变量覆盖敏感配置
	overrideFromEnv(&cfg)
	return &cfg, nil
}

func overrideFromEnv(c *Config) {
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("FEISHU_APP_ID"); v != "" {
		c.Feishu.AppID = v
	}
	if v := os.Getenv("FEISHU_APP_SECRET"); v != "" {
		c.Feishu.AppSecret = v
	}
	if v := os.Getenv("FEISHU_DOMAIN"); v != "" {
		c.Feishu.Domain = v
	}
	if v := os.Getenv("SLACK_BOT_TOKEN"); v != "" {
		c.Slack.BotToken = v
	}
}
