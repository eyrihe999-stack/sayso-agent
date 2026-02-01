package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sayso-agent/internal/model"
	"sayso-agent/internal/service/executor"
	servicellm "sayso-agent/internal/service/llm"
)

// ASRService 编排：接收 ASR 文本 -> 调大模型 -> 执行动作（飞书/Slack 等）
type ASRService struct {
	llm      *servicellm.Service
	executor *executor.Executor
}

// NewASRService 创建 ASR 编排服务
func NewASRService(llm *servicellm.Service, exec *executor.Executor) *ASRService {
	return &ASRService{
		llm:      llm,
		executor: exec,
	}
}

// 占位符：大模型在生成时不知道前序动作结果，用 {{doc_url}} 等占位，执行时用真实值替换
// 支持: doc_url, doc_id, folder_url, folder_id, last_url, last_note
var placeholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Process 处理内部传入的 ASR 文本，完成大模型理解与外部动作执行
func (s *ASRService) Process(ctx context.Context, req model.ASRRequest) (model.ASRResponse, error) {
	taskID := strconv.FormatInt(time.Now().UnixNano(), 10)
	resp := model.ASRResponse{
		TaskID:  taskID,
		Success: false,
	}

	// 1. 大模型理解文本，从自然语言中提取平台、目标、消息内容等
	llmOut, err := s.llm.Process(ctx, req.Text)
	if err != nil {
		resp.Message = fmt.Sprintf("大模型处理失败: %v", err)
		return resp, err
	}

	// 2. 逐条执行动作；用前序动作结果替换 {{doc_url}} 等占位符（大模型不知道真实 URL）
	placeholders := make(map[string]string)
	var summaries []model.ActionSummary
	for _, spec := range llmOut.Actions {
		spec := applyPlaceholders(spec, placeholders)
		summary, err := s.executor.Execute(ctx, spec, &req)
		if err != nil {
			resp.Message = fmt.Sprintf("执行动作 %s 失败: %v", spec.Type, err)
			resp.Actions = summaries
			return resp, err
		}
		summaries = append(summaries, summary)
		updatePlaceholders(placeholders, spec.Type, summary)
	}

	resp.Success = true
	resp.Actions = summaries
	if llmOut.Reply != "" {
		resp.Message = llmOut.Reply
	} else {
		resp.Message = "处理完成"
	}
	return resp, nil
}

// applyPlaceholders 将 spec 中 Params 里的字符串值中的 {{key}} 替换为 placeholders[key]
func applyPlaceholders(spec model.ActionSpec, placeholders map[string]string) model.ActionSpec {
	if len(placeholders) == 0 {
		return spec
	}
	out := spec
	if spec.Params != nil {
		out.Params = replacePlaceholdersInMap(spec.Params, placeholders)
	}
	return out
}

// replacePlaceholdersInMap 递归替换 map 中所有字符串值的占位符
func replacePlaceholdersInMap(m map[string]any, placeholders map[string]string) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = replacePlaceholdersInValue(v, placeholders)
	}
	return result
}

// replacePlaceholdersInValue 递归替换任意值中的占位符
func replacePlaceholdersInValue(v any, placeholders map[string]string) any {
	switch val := v.(type) {
	case string:
		return replacePlaceholdersInString(val, placeholders)
	case map[string]any:
		return replacePlaceholdersInMap(val, placeholders)
	case map[string]string:
		result := make(map[string]any)
		for k, s := range val {
			result[k] = replacePlaceholdersInString(s, placeholders)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = replacePlaceholdersInValue(item, placeholders)
		}
		return result
	default:
		return v
	}
}

func replacePlaceholdersInString(s string, placeholders map[string]string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if v, ok := placeholders[key]; ok {
			return v
		}
		return match
	})
}

// updatePlaceholders 根据刚执行完的动作类型与结果，更新占位符供后续动作使用
func updatePlaceholders(m map[string]string, actionType string, summary model.ActionSummary) {
	switch actionType {
	case "feishu_create_doc":
		if summary.URL != "" {
			m["doc_url"] = summary.URL
			m["last_url"] = summary.URL
		}
		if summary.ID != "" {
			m["doc_id"] = summary.ID
		}
		if summary.Note != "" {
			m["last_note"] = summary.Note
		}
	case "feishu_create_folder":
		if summary.URL != "" {
			m["folder_url"] = summary.URL
			m["last_url"] = summary.URL
		}
		if summary.ID != "" {
			m["folder_id"] = summary.ID
		}
		if summary.Note != "" {
			m["last_note"] = summary.Note
		}
	}
}
