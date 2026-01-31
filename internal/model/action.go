package model

// LLMActionOutput 大模型返回的结构化动作（由本服务解析后调用外部 API）
// 大模型无外部 app 权限，由本服务代为执行
type LLMActionOutput struct {
	// Intent 用户意图摘要
	Intent string `json:"intent"`
	// Actions 待执行的动作列表
	Actions []ActionSpec `json:"actions"`
	// Reply 给用户的自然语言回复（可选）
	Reply string `json:"reply,omitempty"`
}

// ActionSpec 单条动作规格：调哪个 API、参数、发给谁
type ActionSpec struct {
	// Type 动作类型: feishu_create_doc, feishu_send_im, slack_send_message, etc.
	Type string `json:"type"`
	// Params 调用该 API 所需的参数（由 executor 按 type 解析）
	Params map[string]interface{} `json:"params"`
	// TargetUserID 目标用户 ID（飞书 open_id、Slack user_id 等）
	TargetUserID string `json:"target_user_id,omitempty"`
	// TargetChatID 目标群/会话 ID（可选）
	TargetChatID string `json:"target_chat_id,omitempty"`
}
