package executor

import (
	"context"
	"fmt"

	"sayso-agent/internal/client/feishu"
	"sayso-agent/internal/client/slack"
	"sayso-agent/internal/model"
)

// Executor 根据大模型返回的动作规格，将具体执行委托给各 app 的执行器（飞书、Slack 等）
type Executor struct {
	feishu *FeishuExecutor
	slack  *SlackExecutor
}

// NewExecutor 创建执行器，组装各 app 的执行器；folderMatcher 为可选（llm.FolderMatcher 等实现 FolderMatcher 接口）
func NewExecutor(feishuClient *feishu.Client, slackClient *slack.Client, feishuCfg feishu.Config, slackCfg slack.Config, folderMatcher FolderMatcher) *Executor {
	return &Executor{
		feishu: NewFeishuExecutor(feishuClient, feishuCfg, folderMatcher),
		slack:  NewSlackExecutor(slackClient, slackCfg),
	}
}

// Execute 执行单条动作，按 type 路由到对应 app 执行器
func (e *Executor) Execute(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	switch spec.Type {
	case "feishu_create_doc":
		return e.feishu.ExecuteCreateDoc(ctx, spec, req)
	case "feishu_create_folder":
		return e.feishu.ExecuteCreateFolder(ctx, spec, req)
	case "feishu_send_im":
		return e.feishu.ExecuteSendIM(ctx, spec, req)
	case "slack_send_message":
		return e.slack.ExecuteSendMessage(ctx, spec, req)
	default:
		return model.ActionSummary{}, fmt.Errorf("%w: %s", model.ErrActionNotSupport, spec.Type)
	}
}
