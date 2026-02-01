package model

// SendMessageParams 统一发送消息参数
type SendMessageParams struct {
	Platform    string         `json:"platform"`     // feishu | slack
	MessageType string         `json:"message_type"` // text | rich_text | link_card
	Content     MessageContent `json:"content"`
	TargetType  string         `json:"target_type"` // user | chat | batch
	Targets     []string       `json:"targets"`
}

// MessageContent 统一消息内容结构
type MessageContent struct {
	Text        string `json:"text,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// SendResult 单个发送结果
type SendResult struct {
	TargetID string `json:"target_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	MsgID    string `json:"msg_id,omitempty"`
}

// ParseSendMessageParams 从 ActionSpec.Params 解析发送消息参数
func ParseSendMessageParams(params map[string]any) SendMessageParams {
	result := SendMessageParams{}

	if platform, ok := params["platform"].(string); ok {
		result.Platform = platform
	}
	if msgType, ok := params["message_type"].(string); ok {
		result.MessageType = msgType
	}
	if targetType, ok := params["target_type"].(string); ok {
		result.TargetType = targetType
	}

	// 解析 targets 数组
	if targets, ok := params["targets"].([]any); ok {
		for _, t := range targets {
			if s, ok := t.(string); ok {
				result.Targets = append(result.Targets, s)
			}
		}
	}

	// 解析 content 对象
	if content, ok := params["content"].(map[string]any); ok {
		if text, ok := content["text"].(string); ok {
			result.Content.Text = text
		}
		if title, ok := content["title"].(string); ok {
			result.Content.Title = title
		}
		if url, ok := content["url"].(string); ok {
			result.Content.URL = url
		}
		if desc, ok := content["description"].(string); ok {
			result.Content.Description = desc
		}
	}

	return result
}
