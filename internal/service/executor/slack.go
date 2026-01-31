package executor

import (
	"context"

	"sayso-agent/internal/client/slack"
	"sayso-agent/internal/model"
)

// SlackExecutor Slack 相关动作执行器
type SlackExecutor struct {
	Client *slack.Client
	Cfg    slack.Config
}

// NewSlackExecutor 创建 Slack 执行器
func NewSlackExecutor(client *slack.Client, cfg slack.Config) *SlackExecutor {
	return &SlackExecutor{Client: client, Cfg: cfg}
}

// ExecuteSendMessage 发送 Slack 消息
func (e *SlackExecutor) ExecuteSendMessage(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrSlackDisabled
	}
	channel, _ := spec.Params["channel"].(string)
	if channel == "" {
		channel = spec.TargetChatID
	}
	if channel == "" && req != nil && req.Context != nil {
		channel = req.Context["slack_channel"]
	}
	text, _ := spec.Params["text"].(string)
	err := e.Client.SendMessage(ctx, channel, text)
	if err != nil {
		return model.ActionSummary{}, err
	}
	return model.ActionSummary{
		Type:   "slack_message",
		Target: channel,
	}, nil
}
