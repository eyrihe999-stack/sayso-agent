package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Config Slack 客户端配置
type Config struct {
	BotToken string
	Enabled  bool
}

// Client Slack API 客户端
type Client struct {
	cfg    Config
	client *http.Client
}

// NewClient 创建 Slack 客户端
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{},
	}
}

const slackAPIBase = "https://slack.com/api"

// SendMessage 发送消息到频道或用户（chat.postMessage）
func (c *Client) SendMessage(ctx context.Context, channel, text string) error {
	url := slackAPIBase + "/chat.postMessage"
	reqBody := map[string]string{
		"channel": channel,
		"text":    text,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(b, &result)
	if !result.OK {
		return fmt.Errorf("slack send message: %s", result.Error)
	}
	return nil
}
