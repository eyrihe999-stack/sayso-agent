package service

import (
	"context"
	"fmt"
	"strings"

	"sayso-agent/internal/client/feishu"
	"sayso-agent/internal/client/slack"
	"sayso-agent/internal/model"
)

// Executor 根据大模型返回的动作规格，调用外部 API（飞书、Slack 等）
type Executor struct {
	feishu        *feishu.Client
	slack         *slack.Client
	feishuCfg     feishu.Config
	slackCfg      slack.Config
	folderMatcher *FolderMatcher
}

// NewExecutor 创建执行器
func NewExecutor(feishuClient *feishu.Client, slackClient *slack.Client, feishuCfg feishu.Config, slackCfg slack.Config, folderMatcher *FolderMatcher) *Executor {
	return &Executor{
		feishu:        feishuClient,
		slack:         slackClient,
		feishuCfg:     feishuCfg,
		slackCfg:      slackCfg,
		folderMatcher: folderMatcher,
	}
}

// Execute 执行单条动作，返回摘要。req 可为 nil；非 nil 时用于未指定接收人时回退到请求中的 user_id/context。
func (e *Executor) Execute(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	switch spec.Type {
	case "feishu_create_doc":
		return e.executeFeishuCreateDoc(ctx, spec, req)
	case "feishu_send_im":
		return e.executeFeishuSendIM(ctx, spec, req)
	case "slack_send_message":
		return e.executeSlackSendMessage(ctx, spec, req)
	default:
		return model.ActionSummary{}, fmt.Errorf("%w: %s", model.ErrActionNotSupport, spec.Type)
	}
}

func (e *Executor) executeFeishuCreateDoc(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	if !e.feishuCfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.feishu.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}
	folderToken, _ := spec.Params["folder_token"].(string)
	folderNameParam, _ := spec.Params["folder_name"].(string) // 用户指定的目录名称
	title, _ := spec.Params["title"].(string)
	content, _ := spec.Params["content"].(string)
	if title == "" {
		title = "未命名文档"
	}

	var folderName string
	var folders []feishu.FolderInfo

	// 获取目录树（用于名称匹配或智能选择）
	if folderToken == "" {
		folders, _ = e.feishu.GetFolderTree(ctx, token, 2)
	}

	// 1. 如果指定了目录名称，按名称匹配
	if folderToken == "" && folderNameParam != "" && len(folders) > 0 {
		folderToken, folderName = e.matchFolderByName(folderNameParam, folders)
	}

	// 2. 如果未指定目录或名称匹配失败，使用智能选择
	if folderToken == "" && e.folderMatcher != nil && len(folders) > 0 {
		folderToken, folderName, _ = e.folderMatcher.MatchFolder(ctx, title, folders)
	}

	// 3. 如果仍未找到，回退到根目录
	if folderToken == "" {
		rootToken, err := e.feishu.GetRootFolderToken(ctx, token)
		if err == nil {
			folderToken = rootToken
			folderName = "我的空间"
		}
	}

	fileToken, err := e.feishu.CreateDoc(ctx, token, folderToken, title, content)
	if err != nil {
		return model.ActionSummary{}, err
	}

	// 添加协作者
	e.addDocCollaborators(ctx, token, fileToken, spec, req)

	summary := model.ActionSummary{
		Type:   "feishu_doc",
		Target: title,
		ID:     fileToken,
	}
	// 生成文档链接
	if e.feishuCfg.Domain != "" {
		summary.URL = fmt.Sprintf("https://%s/docx/%s", e.feishuCfg.Domain, fileToken)
	}
	if folderName != "" {
		summary.Note = fmt.Sprintf("已存放至「%s」目录", folderName)
	}
	return summary, nil
}

// addDocCollaborators 添加文档协作者
// 1. 默认将调用者添加为协作者（full_access）
// 2. 如果 params 中指定了 collaborators，也添加进来
func (e *Executor) addDocCollaborators(ctx context.Context, accessToken, docToken string, spec model.ActionSpec, req *model.ASRRequest) {
	// 1. 添加调用者为协作者
	callerID, callerIDType := e.getCallerFeishuID(req)
	if callerID != "" {
		_ = e.feishu.AddCollaborator(ctx, accessToken, docToken, "docx", feishu.Collaborator{
			MemberType: callerIDType,
			MemberID:   callerID,
			Perm:       "full_access",
		})
	}

	// 2. 添加额外指定的协作者
	if collaborators, ok := spec.Params["collaborators"].([]interface{}); ok {
		for _, c := range collaborators {
			if collab, ok := c.(map[string]interface{}); ok {
				memberType, _ := collab["member_type"].(string)
				memberID, _ := collab["member_id"].(string)
				perm, _ := collab["perm"].(string)
				if memberType == "" {
					memberType = "openid"
				}
				if perm == "" {
					perm = "full_access"
				}
				if memberID == "" {
					continue
				}
				// 如果 memberID 不是 open_id 格式（ou_ 开头），尝试按名字搜索
				resolvedID := memberID
				if !isOpenID(memberID) {
					user, err := e.feishu.SearchUserByName(ctx, accessToken, memberID)
					if err == nil && user != nil && user.UserID != "" {
						resolvedID = user.UserID
						memberType = "userid" // employee_id 是 user_id 类型
					} else {
						// 搜索失败，跳过此协作者
						continue
					}
				}
				if resolvedID != callerID {
					_ = e.feishu.AddCollaborator(ctx, accessToken, docToken, "docx", feishu.Collaborator{
						MemberType: memberType,
						MemberID:   resolvedID,
						Perm:       perm,
					})
				}
			}
		}
	}
}

// isOpenID 检查是否是飞书 open_id 格式
func isOpenID(id string) bool {
	return len(id) > 3 && id[:3] == "ou_"
}

// matchFolderByName 根据名称匹配目录
// 返回 (folderToken, folderName)
func (e *Executor) matchFolderByName(name string, folders []feishu.FolderInfo) (string, string) {
	// 优先完全匹配
	for _, f := range folders {
		if f.Name == name {
			return f.Token, f.Name
		}
	}
	// 其次模糊匹配（包含关系）
	for _, f := range folders {
		if strings.Contains(f.Name, name) || strings.Contains(name, f.Name) {
			return f.Token, f.Name
		}
	}
	return "", ""
}

// getCallerFeishuID 获取调用者的飞书 ID
// 返回 (id, id_type)，优先使用 open_id
func (e *Executor) getCallerFeishuID(req *model.ASRRequest) (string, string) {
	if req == nil {
		return "", ""
	}
	// 优先使用 context 中的 feishu_open_id
	if req.Context != nil {
		if id := req.Context["feishu_open_id"]; id != "" {
			return id, "openid"
		}
		if id := req.Context["feishu_user_id"]; id != "" {
			return id, "userid"
		}
	}
	// 回退到 user_id（假设是 open_id 格式）
	if req.UserID != "" {
		return req.UserID, "openid"
	}
	return "", ""
}

func (e *Executor) executeFeishuSendIM(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	if !e.feishuCfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.feishu.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}
	content, _ := spec.Params["content"].(string)
	// 优先使用调用方显式传入的飞书身份，再使用 LLM 返回的 target，最后回退到 user_id
	var receiveID string
	usedFeishuUserID := false
	if req != nil && req.Context != nil {
		if id := req.Context["feishu_open_id"]; id != "" {
			receiveID = id
		}
		if receiveID == "" {
			if id := req.Context["feishu_user_id"]; id != "" {
				receiveID = id
				usedFeishuUserID = true
			}
		}
	}
	if receiveID == "" {
		receiveID = spec.TargetUserID
	}
	if receiveID == "" {
		receiveID, _ = spec.Params["receive_id"].(string)
	}
	if receiveID == "" && req != nil && req.UserID != "" {
		receiveID = req.UserID
	}
	receiveIDType := "open_id"
	if t, ok := spec.Params["receive_id_type"].(string); ok && t != "" {
		receiveIDType = t
	} else if usedFeishuUserID {
		receiveIDType = "user_id"
	}
	// 如果 receiveID 是 open_id 格式（ou_ 开头），强制使用 open_id 类型
	if isOpenID(receiveID) {
		receiveIDType = "open_id"
	}
	err = e.feishu.SendIM(ctx, token, receiveIDType, receiveID, content)
	if err != nil {
		return model.ActionSummary{}, err
	}
	summary := model.ActionSummary{
		Type:   "feishu_im",
		Target: receiveID,
	}
	// 生成私聊链接
	if e.feishuCfg.Domain != "" && receiveID != "" {
		summary.URL = fmt.Sprintf("https://%s/messenger/?openChatId=%s", e.feishuCfg.Domain, receiveID)
	}
	return summary, nil
}

func (e *Executor) executeSlackSendMessage(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	if !e.slackCfg.Enabled {
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
	err := e.slack.SendMessage(ctx, channel, text)
	if err != nil {
		return model.ActionSummary{}, err
	}
	return model.ActionSummary{
		Type:   "slack_message",
		Target: channel,
	}, nil
}
