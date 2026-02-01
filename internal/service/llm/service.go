package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	clientllm "sayso-agent/internal/client/llm"
	"sayso-agent/internal/model"
)

// Service 调用大模型并解析为结构化动作
type Service struct {
	client *clientllm.Client
}

// NewService 创建 LLM 服务
func NewService(client *clientllm.Client) *Service {
	return &Service{client: client}
}

// ================== 任务规划类型 ==================

// SkillType 技能类型
type SkillType string

const (
	SkillCreateDoc    SkillType = "create_doc"
	SkillCreateFolder SkillType = "create_folder"
	SkillSendMessage  SkillType = "send_message"
)

// TaskSpec 单个任务规格
type TaskSpec struct {
	ID        string    `json:"id"`         // 任务ID（如 task_1）
	Skill     SkillType `json:"skill"`      // 技能类型
	Platform  string    `json:"platform"`   // 平台：feishu/slack
	Input     string    `json:"input"`      // 该任务相关的输入描述
	DependsOn []string  `json:"depends_on"` // 依赖的任务ID（需要等待的任务）
}

// TaskPlan 第一阶段任务规划结果
type TaskPlan struct {
	Summary string     `json:"summary"` // 整体意图摘要
	Tasks   []TaskSpec `json:"tasks"`   // 任务列表
}

// TaskResult 单个任务执行结果
type TaskResult struct {
	TaskID  string
	Action  *model.ActionSpec
	Error   error
	Outputs map[string]string // 输出变量（如 doc_url, folder_url）
}

// ================== 第一阶段：任务规划 ==================

const plannerPrompt = `分析用户输入，识别所有要执行的任务，返回 JSON：
{
  "summary": "整体意图摘要",
  "tasks": [
    {
      "id": "task_1",
      "skill": "create_doc|create_folder|send_message",
      "platform": "feishu|slack",
      "input": "该任务相关的输入描述",
      "depends_on": []
    }
  ]
}

技能类型：
- create_doc: 创建文档
- create_folder: 创建文件夹
- send_message: 发送消息

平台识别：
- feishu: 飞书、中文名字、ou_开头的ID、默认
- slack: slack、channel、#频道

## 依赖关系识别（非常重要）

以下情况必须设置 depends_on：

1. **顺序词**：出现以下词语时，后续任务依赖前面的任务
   - "然后"、"再"、"接着"、"之后"、"完了后"、"完成后"、"创建好后"

2. **引用前置任务结果**：
   - "把链接发给"、"发送链接"、"分享文档" → 依赖 create_doc
   - "发送文件夹链接" → 依赖 create_folder

3. **隐含依赖**：创建资源后发送给某人 = 先创建 + 再发送链接
   - "创建文档发给张三" = create_doc + send_message(depends_on create_doc)

## 示例

示例1 - "给张三发消息说开会"（无依赖）：
{"summary":"发送开会通知","tasks":[{"id":"task_1","skill":"send_message","platform":"feishu","input":"给张三发消息说开会","depends_on":[]}]}

示例2 - "给飞书和slack同时发消息"（并行，无依赖）：
{"summary":"多平台发送消息","tasks":[
  {"id":"task_1","skill":"send_message","platform":"feishu","input":"发消息","depends_on":[]},
  {"id":"task_2","skill":"send_message","platform":"slack","input":"发消息","depends_on":[]}
]}

示例3 - "创建周报，完了后把链接发给张三"（有依赖）：
{"summary":"创建文档并分享","tasks":[
  {"id":"task_1","skill":"create_doc","platform":"feishu","input":"创建周报文档","depends_on":[]},
  {"id":"task_2","skill":"send_message","platform":"feishu","input":"把文档链接发给张三（需要{{doc_url}}）","depends_on":["task_1"]}
]}

示例4 - "创建会议纪要然后发给ou_xxx"（有依赖）：
{"summary":"创建文档并分享","tasks":[
  {"id":"task_1","skill":"create_doc","platform":"feishu","input":"创建会议纪要","depends_on":[]},
  {"id":"task_2","skill":"send_message","platform":"feishu","input":"把文档链接发给ou_xxx（需要{{doc_url}}）","depends_on":["task_1"]}
]}

只返回 JSON。`

// ================== 第二阶段：各技能专用 Prompt ==================

var skillPrompts = map[SkillType]string{
	SkillCreateDoc: `提取创建文档参数，返回 JSON：
{"type":"feishu_create_doc","params":{"title":"标题","content":"内容","folder_name":"目录","collaborators":[{"member_id":"用户名","perm":"edit"}]}}

规则：
- title 必填，如果用户说"今天的日期"则使用实际日期格式如"2024-01-15"
- perm: full_access(默认)/edit/view

只返回 JSON。`,

	SkillCreateFolder: `提取创建文件夹参数，返回 JSON：
{"type":"feishu_create_folder","params":{"name":"名称","folder_name":"父目录"}}

规则：
- name 必填
- folder_name 可选

只返回 JSON。`,

	SkillSendMessage: `提取发送消息参数，返回 JSON：
{"type":"send_message","params":{"platform":"feishu|slack","message_type":"text|link_card","content":{"text":"消息","url":"链接"},"target_type":"user|chat|batch","targets":["目标"]}}

规则：
- platform: feishu(默认)/slack
- target_type: user(单人)/chat(群)/batch(多人)
- targets: 直接使用用户提供的ID（如ou_xxx）或用户名

占位符使用（重要）：
- 如果任务描述中包含"需要{{doc_url}}"，则：
  - message_type 设为 "link_card"
  - content.url 设为 "{{doc_url}}"
  - content.text 设为 "请查看文档"
- 如果包含"需要{{folder_url}}"，则 content.url 设为 "{{folder_url}}"

只返回 JSON。`,
}

// ================== 主处理流程 ==================

// Process 两阶段处理：规划 → 并行执行
func (s *Service) Process(ctx context.Context, userText string) (*model.LLMActionOutput, error) {
	// 第一阶段：任务规划
	plan, err := s.planTasks(ctx, userText)
	if err != nil {
		return nil, fmt.Errorf("plan tasks: %w", err)
	}
	if len(plan.Tasks) == 0 {
		return &model.LLMActionOutput{
			Intent: plan.Summary,
			Reply:  "抱歉，我不太理解您的意思。您可以尝试：创建文档、创建文件夹、发送消息。",
		}, nil
	}

	// 第二阶段：按依赖关系执行任务
	results, err := s.executeTasks(ctx, plan.Tasks)
	if err != nil {
		return nil, err
	}

	// 汇总结果
	return s.buildOutput(plan, results), nil
}

// planTasks 第一阶段：任务规划
func (s *Service) planTasks(ctx context.Context, userText string) (*TaskPlan, error) {
	raw, err := s.client.Chat(ctx, plannerPrompt, userText)
	if err != nil {
		return nil, err
	}
	raw = ExtractJSON(raw)
	var plan TaskPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	return &plan, nil
}

// executeTasks 按依赖关系执行任务（无依赖的并行，有依赖的等待）
func (s *Service) executeTasks(ctx context.Context, tasks []TaskSpec) (map[string]*TaskResult, error) {
	results := make(map[string]*TaskResult)
	var mu sync.Mutex

	// 构建依赖图
	pending := make(map[string]*TaskSpec)
	for i := range tasks {
		pending[tasks[i].ID] = &tasks[i]
	}

	// 循环执行直到所有任务完成
	for len(pending) > 0 {
		// 找出可执行的任务（依赖已完成）
		var ready []*TaskSpec
		for _, task := range pending {
			if s.canExecute(task, results) {
				ready = append(ready, task)
			}
		}

		if len(ready) == 0 {
			// 存在循环依赖或依赖任务失败
			return results, fmt.Errorf("无法继续执行：存在循环依赖或依赖任务失败")
		}

		// 并行执行就绪任务
		var wg sync.WaitGroup
		for _, task := range ready {
			wg.Add(1)
			go func(t *TaskSpec) {
				defer wg.Done()
				result := s.executeTask(ctx, t, results)
				mu.Lock()
				results[t.ID] = result
				delete(pending, t.ID)
				mu.Unlock()
			}(task)
		}
		wg.Wait()

		// 检查是否有任务失败
		for _, task := range ready {
			if results[task.ID].Error != nil {
				return results, fmt.Errorf("任务 %s 失败: %w", task.ID, results[task.ID].Error)
			}
		}
	}

	return results, nil
}

// canExecute 检查任务是否可执行（所有依赖已成功完成）
func (s *Service) canExecute(task *TaskSpec, results map[string]*TaskResult) bool {
	for _, depID := range task.DependsOn {
		result, exists := results[depID]
		if !exists || result.Error != nil {
			return false
		}
	}
	return true
}

// executeTask 执行单个任务
func (s *Service) executeTask(ctx context.Context, task *TaskSpec, depResults map[string]*TaskResult) *TaskResult {
	result := &TaskResult{
		TaskID:  task.ID,
		Outputs: make(map[string]string),
	}

	// 获取技能对应的 prompt
	prompt, ok := skillPrompts[task.Skill]
	if !ok {
		result.Error = fmt.Errorf("未知技能: %s", task.Skill)
		return result
	}

	// 替换输入中的占位符（引用依赖任务的输出）
	input := s.resolvePlaceholders(task.Input, depResults)

	// 调用 LLM 提取参数
	raw, err := s.client.Chat(ctx, prompt, input)
	if err != nil {
		result.Error = fmt.Errorf("LLM 调用失败: %w", err)
		return result
	}
	raw = ExtractJSON(raw)

	var action model.ActionSpec
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		result.Error = fmt.Errorf("解析参数失败: %w", err)
		return result
	}

	// 补充平台信息（send_message 需要）
	if task.Skill == SkillSendMessage && action.Params != nil {
		if _, ok := action.Params["platform"]; !ok {
			action.Params["platform"] = task.Platform
		}
	}

	result.Action = &action
	return result
}

// resolvePlaceholders 替换占位符为依赖任务的输出
func (s *Service) resolvePlaceholders(input string, depResults map[string]*TaskResult) string {
	for _, result := range depResults {
		if result.Outputs != nil {
			for key, value := range result.Outputs {
				placeholder := "{{" + key + "}}"
				input = strings.ReplaceAll(input, placeholder, value)
			}
		}
	}
	return input
}

// buildOutput 汇总所有任务结果
func (s *Service) buildOutput(plan *TaskPlan, results map[string]*TaskResult) *model.LLMActionOutput {
	out := &model.LLMActionOutput{
		Intent: plan.Summary,
	}

	// 按原始顺序收集 actions
	for _, task := range plan.Tasks {
		if result, ok := results[task.ID]; ok && result.Action != nil {
			out.Actions = append(out.Actions, *result.Action)
		}
	}

	return out
}

// ExtractJSON 从回复中提取 JSON
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "{"); start >= 0 {
		if end := strings.LastIndex(s, "}"); end > start {
			return s[start : end+1]
		}
	}
	return s
}
