package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"sayso-agent/internal/model"
)

// ASRService 编排：接收 ASR 文本 -> 调大模型 -> 执行动作（飞书/Slack 等）
type ASRService struct {
	llm      *LLMService
	executor *Executor
}

// NewASRService 创建 ASR 编排服务
func NewASRService(llm *LLMService, executor *Executor) *ASRService {
	return &ASRService{
		llm:      llm,
		executor: executor,
	}
}

// Process 处理内部传入的 ASR 文本，完成大模型理解与外部动作执行
func (s *ASRService) Process(ctx context.Context, req model.ASRRequest) (model.ASRResponse, error) {
	taskID := strconv.FormatInt(time.Now().UnixNano(), 10)
	resp := model.ASRResponse{
		TaskID:  taskID,
		Success: false,
	}

	// 1. 大模型理解文本，得到结构化动作
	llmOut, err := s.llm.Process(ctx, req.Text, req.UserID, req.Contacts)
	if err != nil {
		resp.Message = fmt.Sprintf("大模型处理失败: %v", err)
		return resp, err
	}

	// 2. 逐条执行动作（本服务代表大模型调用外部 API）；传入 req 以便未指定接收人时用 user_id/context 作为默认
	var summaries []model.ActionSummary
	for _, spec := range llmOut.Actions {
		summary, err := s.executor.Execute(ctx, spec, &req)
		if err != nil {
			resp.Message = fmt.Sprintf("执行动作 %s 失败: %v", spec.Type, err)
			resp.Actions = summaries
			return resp, err
		}
		summaries = append(summaries, summary)
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
