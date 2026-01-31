package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sayso-agent/internal/client/llm"
	"sayso-agent/internal/model"
)

// LLMService 调用大模型并解析为结构化动作
type LLMService struct {
	client *llm.Client
}

// NewLLMService 创建 LLM 服务
func NewLLMService(client *llm.Client) *LLMService {
	return &LLMService{client: client}
}

// 系统提示：让大模型返回 JSON 格式的动作列表
const systemPrompt = `你是一个任务执行助手。用户会给你一段文本（可能是语音转写），你需要理解意图并输出要执行的动作。
你必须以 JSON 格式回复，且只输出一个 JSON 对象，不要其他说明。格式如下：
{
  "intent": "用户意图一句话摘要",
  "reply": "给用户的自然语言回复（可选）",
  "actions": [
    {
      "type": "feishu_create_doc | feishu_send_im | slack_send_message",
      "params": { ... },
      "target_user_id": "目标用户ID（可选）",
      "target_chat_id": "目标会话/频道ID（可选）"
    }
  ]
}

动作类型说明：

1. feishu_create_doc - 创建飞书文档
   params:
   - title: 文档标题（必填）
   - content: 文档内容（可选）
   - folder_name: 目标文件夹名称（可选）。用户指定的目录名，系统会自动匹配最合适的目录
   - folder_token: 目标文件夹token（可选，优先级高于 folder_name）
   - collaborators: 协作者数组（可选），每个协作者包含：
     - member_id: 用户名或用户ID（必填）。可以直接使用用户名（如"张三"），系统会自动搜索并解析为飞书ID
     - member_type: ID类型，可选 openid/userid/email（默认 openid，使用名字时无需填写）
     - perm: 权限级别，可选 full_access(可管理)/edit(可编辑)/view(仅查看)（默认 full_access）

   示例 - 用户说"在工作文档目录下创建周报，给张三编辑权限"：
   {
     "type": "feishu_create_doc",
     "params": {
       "title": "周报",
       "folder_name": "工作文档",
       "collaborators": [
         {"member_id": "张三", "perm": "edit"}
       ]
     }
   }

2. feishu_send_im - 发送飞书私聊消息
   params:
   - content: 消息内容（必填）
   - receive_id: 接收者ID（可选，不填则用当前用户）
   - receive_id_type: ID类型，可选 open_id/user_id/chat_id（默认 open_id）

3. slack_send_message - 发送Slack消息
   params:
   - channel: 频道ID（可选，不填则用请求context中的默认频道）
   - text: 消息内容（必填）

重要提示：
- 请求中的「当前用户ID」是发起请求的用户，创建文档时会自动将其添加为协作者
- 协作者的 member_id 可以直接使用用户名（如"张三"），系统会自动通过飞书API搜索并解析为对应的open_id
- 权限关键词映射：管理/完全控制 -> full_access，编辑/修改 -> edit，查看/只读 -> view
`

// Process 将用户文本交给大模型，返回解析后的动作列表
func (s *LLMService) Process(ctx context.Context, userText, userID string, contacts []model.Contact) (*model.LLMActionOutput, error) {
	var contentBuilder strings.Builder
	if userID != "" {
		contentBuilder.WriteString("当前用户ID: ")
		contentBuilder.WriteString(userID)
		contentBuilder.WriteString("\n\n")
	}
	// 添加联系人列表供 LLM 映射名字到 ID
	if len(contacts) > 0 {
		contentBuilder.WriteString("已知联系人列表（用于将名字映射为飞书ID）:\n")
		for _, c := range contacts {
			contentBuilder.WriteString("- ")
			contentBuilder.WriteString(c.Name)
			if c.OpenID != "" {
				contentBuilder.WriteString(", open_id: ")
				contentBuilder.WriteString(c.OpenID)
			}
			if c.UserID != "" {
				contentBuilder.WriteString(", user_id: ")
				contentBuilder.WriteString(c.UserID)
			}
			if c.Email != "" {
				contentBuilder.WriteString(", email: ")
				contentBuilder.WriteString(c.Email)
			}
			contentBuilder.WriteString("\n")
		}
		contentBuilder.WriteString("\n")
	}
	contentBuilder.WriteString("用户输入: ")
	contentBuilder.WriteString(userText)

	raw, err := s.client.Chat(ctx, systemPrompt, contentBuilder.String())
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}
	// 尝试从回复中提取 JSON（大模型可能带 markdown 代码块）
	raw = extractJSON(raw)
	var out model.LLMActionOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse llm output: %w", err)
	}
	return &out, nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "{"); start >= 0 {
		if end := strings.LastIndex(s, "}"); end > start {
			return s[start : end+1]
		}
	}
	return s
}
