package executor

import (
	"context"
	"fmt"
	"strings"

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

// ExecuteSendMessage 统一发送消息（支持用户、频道、批量）
func (e *SlackExecutor) ExecuteSendMessage(ctx context.Context, spec model.ActionSpec, _ *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrSlackDisabled
	}

	params := model.ParseSendMessageParams(spec.Params)

	// 构建消息内容
	text, blocks := e.buildSlackMessage(params)

	var results []model.SendResult

	switch params.TargetType {
	case "user":
		if len(params.Targets) == 0 {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required for user type")
		}
		result := e.sendToUser(ctx, params.Targets[0], text, blocks)
		results = append(results, result)

	case "chat":
		if len(params.Targets) == 0 {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required for chat type")
		}
		result := e.sendToChannel(ctx, params.Targets[0], text, blocks)
		results = append(results, result)

	case "batch":
		for _, target := range params.Targets {
			result := e.sendToUser(ctx, target, text, blocks)
			results = append(results, result)
		}

	default:
		// 默认按频道处理
		if len(params.Targets) > 0 {
			result := e.sendToChannel(ctx, params.Targets[0], text, blocks)
			results = append(results, result)
		} else {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required")
		}
	}

	return e.buildSendMessageSummary(results), nil
}

// buildSlackMessage 根据消息类型构建 Slack 消息内容
func (e *SlackExecutor) buildSlackMessage(params model.SendMessageParams) (text string, blocks []slack.Block) {
	text = params.Content.Text

	switch params.MessageType {
	case "rich_text", "link_card":
		blocks = slack.BuildRichTextBlocks(
			params.Content.Title,
			params.Content.Text,
			params.Content.URL,
			params.Content.Description,
		)
	default:
		// text 类型不需要 blocks
	}

	return text, blocks
}

// sendToUser 发送私聊消息给用户
func (e *SlackExecutor) sendToUser(ctx context.Context, userID, text string, blocks []slack.Block) model.SendResult {
	// 先打开私聊会话
	channelID, err := e.Client.OpenConversation(ctx, userID)
	if err != nil {
		return model.SendResult{
			TargetID: userID,
			Success:  false,
			Error:    fmt.Sprintf("open conversation failed: %s", err.Error()),
		}
	}

	// 发送消息
	result, err := e.Client.SendMessageWithBlocks(ctx, channelID, text, blocks)
	if err != nil {
		return model.SendResult{
			TargetID: userID,
			Success:  false,
			Error:    err.Error(),
		}
	}

	return model.SendResult{
		TargetID: userID,
		Success:  true,
		MsgID:    result.Timestamp,
	}
}

// sendToChannel 发送消息到频道
func (e *SlackExecutor) sendToChannel(ctx context.Context, channel, text string, blocks []slack.Block) model.SendResult {
	result, err := e.Client.SendMessageWithBlocks(ctx, channel, text, blocks)
	if err != nil {
		return model.SendResult{
			TargetID: channel,
			Success:  false,
			Error:    err.Error(),
		}
	}

	return model.SendResult{
		TargetID: channel,
		Success:  true,
		MsgID:    result.Timestamp,
	}
}

// buildSendMessageSummary 构建发送消息摘要
func (e *SlackExecutor) buildSendMessageSummary(results []model.SendResult) model.ActionSummary {
	successCount := 0
	var failedTargets []string
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedTargets = append(failedTargets, r.TargetID)
		}
	}

	summary := model.ActionSummary{
		Type: "slack_message",
	}

	if len(results) == 1 {
		summary.Target = results[0].TargetID
		if results[0].Success {
			summary.ID = results[0].MsgID
		} else {
			summary.Note = results[0].Error
		}
	} else {
		summary.Target = fmt.Sprintf("%d/%d targets", successCount, len(results))
		if len(failedTargets) > 0 {
			summary.Note = fmt.Sprintf("failed: %s", strings.Join(failedTargets, ", "))
		}
	}

	return summary
}
