package model

// ASRRequest 内部传入的 ASR 文本请求
type ASRRequest struct {
	// Text 语音识别得到的文本
	Text string `json:"text" binding:"required"`
	// UserID 发起请求的用户标识。发飞书私聊时用作默认接收人：应传该用户的飞书 open_id（或与 open_id 一致的内部 ID）。
	// 发 Slack 时若动作未指定 channel，可配合 Context["slack_channel"] 使用。
	UserID string `json:"user_id,omitempty"`
	// Context 可选上下文，用于定向发送与租户等：
	//   feishu_open_id: 飞书接收人 open_id（优先于 UserID 用于 feishu_send_im）
	//   feishu_user_id: 飞书 user_id（若用 user_id 维度发私聊）
	//   slack_channel: Slack 频道 ID（用于 slack_send_message 未指定 channel 时的默认值）
	//   其他: 会话 ID、租户等
	Context map[string]string `json:"context,omitempty"`
	// Contacts 已知联系人列表，用于 LLM 将用户提到的名字映射为飞书 ID
	// 示例: [{"name": "张三", "open_id": "ou_xxx"}, {"name": "李四", "open_id": "ou_yyy"}]
	Contacts []Contact `json:"contacts,omitempty"`
}

// Contact 联系人信息
type Contact struct {
	Name   string `json:"name"`              // 联系人名称
	OpenID string `json:"open_id,omitempty"` // 飞书 open_id
	UserID string `json:"user_id,omitempty"` // 飞书 user_id
	Email  string `json:"email,omitempty"`   // 邮箱
}

// ASRResponse 处理结果响应
type ASRResponse struct {
	// TaskID 任务/请求 ID，便于追踪
	TaskID string `json:"task_id"`
	// Success 是否处理成功
	Success bool `json:"success"`
	// Message 结果说明
	Message string `json:"message,omitempty"`
	// Actions 已执行的动作摘要（如：已创建飞书文档、已发送私聊）
	Actions []ActionSummary `json:"actions,omitempty"`
}

// ActionSummary 已执行动作的简要信息
type ActionSummary struct {
	Type   string `json:"type"`           // feishu_doc, feishu_im, slack_message, etc.
	Target string `json:"target"`         // 目标描述
	ID     string `json:"id,omitempty"`   // 资源 ID
	URL    string `json:"url,omitempty"`  // 资源访问链接
	Note   string `json:"note,omitempty"` // 备注信息，如存放目录
}
