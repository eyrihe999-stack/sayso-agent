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
	_, err := c.SendMessageWithBlocks(ctx, channel, text, nil)
	return err
}

// Block Slack Block Kit 元素
type Block struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
	// 用于 section 中的附件
	Accessory *Accessory `json:"accessory,omitempty"`
	// 用于 actions block
	Elements []Element `json:"elements,omitempty"`
}

// Text Slack 文本对象
type Text struct {
	Type string `json:"type"` // plain_text | mrkdwn
	Text string `json:"text"`
}

// Accessory Slack 附件对象
type Accessory struct {
	Type     string `json:"type"` // button
	Text     *Text  `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	ActionID string `json:"action_id,omitempty"`
}

// Element Slack 元素（用于 actions block）
type Element struct {
	Type     string `json:"type"`
	Text     *Text  `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	ActionID string `json:"action_id,omitempty"`
}

// SendMessageResult 发送消息结果
type SendMessageResult struct {
	Timestamp string // 消息 ts，可用作消息 ID
	Channel   string
	Error     error
}

// SendMessageWithBlocks 发送消息，支持 Block Kit
func (c *Client) SendMessageWithBlocks(ctx context.Context, channel, text string, blocks []Block) (SendMessageResult, error) {
	url := slackAPIBase + "/chat.postMessage"
	reqBody := map[string]any{
		"channel": channel,
		"text":    text,
	}
	if len(blocks) > 0 {
		reqBody["blocks"] = blocks
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return SendMessageResult{Error: err}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return SendMessageResult{Error: err}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Ts      string `json:"ts"`
		Channel string `json:"channel"`
	}
	_ = json.Unmarshal(b, &result)
	if !result.OK {
		err := fmt.Errorf("slack send message: %s", result.Error)
		return SendMessageResult{Error: err}, err
	}
	return SendMessageResult{Timestamp: result.Ts, Channel: result.Channel}, nil
}

// OpenConversation 打开与用户的私聊会话（conversations.open）
// 返回 DM channel ID
func (c *Client) OpenConversation(ctx context.Context, userID string) (string, error) {
	url := slackAPIBase + "/conversations.open"
	reqBody := map[string]string{
		"users": userID,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	_ = json.Unmarshal(b, &result)
	if !result.OK {
		return "", fmt.Errorf("slack open conversation: %s", result.Error)
	}
	return result.Channel.ID, nil
}

// BuildRichTextBlocks 构建富文本 blocks（带链接）
func BuildRichTextBlocks(title, text, linkURL, description string) []Block {
	var blocks []Block

	// 标题（如果有）
	if title != "" {
		blocks = append(blocks, Block{
			Type: "header",
			Text: &Text{Type: "plain_text", Text: title},
		})
	}

	// 正文
	if text != "" {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: text},
		})
	}

	// 描述
	if description != "" {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: description},
		})
	}

	// 链接按钮
	if linkURL != "" {
		blocks = append(blocks, Block{
			Type: "actions",
			Elements: []Element{
				{
					Type:     "button",
					Text:     &Text{Type: "plain_text", Text: "查看链接"},
					URL:      linkURL,
					ActionID: "link_button",
				},
			},
		})
	}

	return blocks
}
