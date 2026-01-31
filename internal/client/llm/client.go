package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Config LLM 客户端配置
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Client 大模型客户端（OpenAI 兼容接口）
type Client struct {
	cfg    Config
	client *http.Client
}

// NewClient 创建 LLM 客户端
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// ChatRequest 聊天请求（OpenAI 兼容）
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// Chat 发送对话请求，返回大模型回复文本
func (c *Client) Chat(ctx context.Context, systemPrompt, userContent string) (string, error) {
	url := c.cfg.BaseURL + "/chat/completions"
	reqBody := ChatRequest{
		Model: c.cfg.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm api error: %s %s", resp.Status, string(data))
	}
	var chatResp ChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return chatResp.Choices[0].Message.Content, nil
}
