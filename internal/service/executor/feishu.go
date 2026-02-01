package executor

import (
	"context"
	"fmt"
	"strings"

	"sayso-agent/internal/client/feishu"
	"sayso-agent/internal/model"
)

// FeishuExecutor 飞书相关动作执行器
type FeishuExecutor struct {
	Client        *feishu.Client
	Cfg           feishu.Config
	FolderMatcher FolderMatcher // 可选，用于按标题智能选目录
}

// FolderMatcher 目录匹配器（由 llm.FolderMatcher 等实现，避免循环依赖）
type FolderMatcher interface {
	MatchFolder(ctx context.Context, title string, folders []feishu.FolderInfo) (token, name string, err error)
}

// NewFeishuExecutor 创建飞书执行器
func NewFeishuExecutor(client *feishu.Client, cfg feishu.Config, folderMatcher FolderMatcher) *FeishuExecutor {
	return &FeishuExecutor{Client: client, Cfg: cfg, FolderMatcher: folderMatcher}
}

// ExecuteCreateDoc 创建飞书云文档
func (e *FeishuExecutor) ExecuteCreateDoc(ctx context.Context, spec model.ActionSpec, _ *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.Client.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}
	folderToken, _ := spec.Params["folder_token"].(string)
	folderNameParam, _ := spec.Params["folder_name"].(string)
	title, _ := spec.Params["title"].(string)
	content, _ := spec.Params["content"].(string)
	if title == "" {
		title = "未命名文档"
	}

	var folderName string
	var folders []feishu.FolderInfo
	if folderToken == "" {
		folders, _ = e.Client.GetFolderTree(ctx, token, 2)
	}
	if folderToken == "" && folderNameParam != "" && len(folders) > 0 {
		folderToken, folderName = matchFolderByName(folderNameParam, folders)
	}
	if folderToken == "" && e.FolderMatcher != nil && len(folders) > 0 {
		folderToken, folderName, _ = e.FolderMatcher.MatchFolder(ctx, title, folders)
	}
	if folderToken == "" {
		rootToken, err := e.Client.GetRootFolderToken(ctx, token)
		if err == nil {
			folderToken = rootToken
			folderName = "我的空间"
		}
	}

	fileToken, err := e.Client.CreateDoc(ctx, token, folderToken, title, content)
	if err != nil {
		return model.ActionSummary{}, err
	}
	e.addDocCollaborators(ctx, token, fileToken, spec)

	summary := model.ActionSummary{Type: "feishu_doc", Target: title, ID: fileToken}
	if e.Cfg.Domain != "" {
		summary.URL = fmt.Sprintf("https://%s/docx/%s", e.Cfg.Domain, fileToken)
	}
	if folderName != "" {
		summary.Note = fmt.Sprintf("已存放至「%s」目录", folderName)
	}
	return summary, nil
}

// ExecuteCreateFolder 创建飞书云空间文件夹
func (e *FeishuExecutor) ExecuteCreateFolder(ctx context.Context, spec model.ActionSpec, _ *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.Client.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}
	name, _ := spec.Params["name"].(string)
	if name == "" {
		return model.ActionSummary{}, fmt.Errorf("feishu_create_folder: name is required")
	}
	folderToken, _ := spec.Params["folder_token"].(string)
	folderNameParam, _ := spec.Params["folder_name"].(string)
	var parentName string
	if folderToken == "" {
		folders, _ := e.Client.GetFolderTree(ctx, token, 2)
		if folderNameParam != "" && len(folders) > 0 {
			folderToken, parentName = matchFolderByName(folderNameParam, folders)
		}
		if folderToken == "" {
			rootToken, err := e.Client.GetRootFolderToken(ctx, token)
			if err != nil {
				return model.ActionSummary{}, fmt.Errorf("feishu create folder: get root folder: %w", err)
			}
			folderToken = rootToken
			parentName = "我的空间"
		}
	}
	newFolderToken, err := e.Client.CreateFolder(ctx, token, folderToken, name)
	if err != nil {
		return model.ActionSummary{}, err
	}
	summary := model.ActionSummary{Type: "feishu_folder", Target: name, ID: newFolderToken}
	if e.Cfg.Domain != "" {
		summary.URL = fmt.Sprintf("https://%s/drive/folder/%s", e.Cfg.Domain, newFolderToken)
	}
	if parentName != "" {
		summary.Note = fmt.Sprintf("已创建在「%s」下", parentName)
	}
	return summary, nil
}

func (e *FeishuExecutor) addDocCollaborators(ctx context.Context, accessToken, docToken string, spec model.ActionSpec) {
	collaborators, ok := spec.Params["collaborators"].([]any)
	if !ok {
		return
	}
	for _, c := range collaborators {
		collab, ok := c.(map[string]any)
		if !ok {
			continue
		}
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
		resolvedID := memberID
		resolvedType := memberType
		// 如果不是 open_id 格式，尝试按名字搜索
		if !isOpenID(memberID) {
			user, err := e.Client.SearchUserByName(ctx, accessToken, memberID)
			if err == nil && user != nil && user.UserID != "" {
				resolvedID = user.UserID
				resolvedType = "userid"
			} else {
				continue
			}
		}
		_ = e.Client.AddCollaborator(ctx, accessToken, docToken, "docx", feishu.Collaborator{
			MemberType: resolvedType,
			MemberID:   resolvedID,
			Perm:       perm,
		})
	}
}

func isOpenID(id string) bool {
	return len(id) > 3 && id[:3] == "ou_"
}

func matchFolderByName(name string, folders []feishu.FolderInfo) (token, folderName string) {
	for _, f := range folders {
		if f.Name == name {
			return f.Token, f.Name
		}
	}
	for _, f := range folders {
		if strings.Contains(f.Name, name) || strings.Contains(name, f.Name) {
			return f.Token, f.Name
		}
	}
	return "", ""
}

// ExecuteSendMessage 统一发送消息（支持用户、群聊、批量）
func (e *FeishuExecutor) ExecuteSendMessage(ctx context.Context, spec model.ActionSpec, _ *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.Client.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}

	params := model.ParseSendMessageParams(spec.Params)

	// 构建消息内容
	msgType, content := e.buildFeishuMessage(params)

	var results []model.SendResult

	switch params.TargetType {
	case "user":
		if len(params.Targets) == 0 {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required for user type")
		}
		result := e.sendToTarget(ctx, token, params.Targets[0], "user", msgType, content)
		results = append(results, result)

	case "chat":
		if len(params.Targets) == 0 {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required for chat type")
		}
		result := e.sendToTarget(ctx, token, params.Targets[0], "chat", msgType, content)
		results = append(results, result)

	case "batch":
		for _, target := range params.Targets {
			result := e.sendToTarget(ctx, token, target, "user", msgType, content)
			results = append(results, result)
		}

	default:
		// 默认按用户处理
		if len(params.Targets) > 0 {
			result := e.sendToTarget(ctx, token, params.Targets[0], "user", msgType, content)
			results = append(results, result)
		} else {
			return model.ActionSummary{}, fmt.Errorf("send_message: targets is required")
		}
	}

	return e.buildSendMessageSummary(results, params), nil
}

// buildFeishuMessage 根据消息类型构建飞书消息内容
func (e *FeishuExecutor) buildFeishuMessage(params model.SendMessageParams) (msgType, content string) {
	switch params.MessageType {
	case "rich_text", "post":
		msgType = "post"
		content = feishu.BuildPostContent(params.Content.Title, params.Content.Text, params.Content.URL)

	case "link_card", "interactive":
		msgType = "interactive"
		content = feishu.BuildInteractiveCard(
			params.Content.Title,
			params.Content.Text,
			params.Content.URL,
			params.Content.Description,
		)

	default: // text
		msgType = "text"
		content = feishu.BuildTextContent(params.Content.Text)
	}
	return msgType, content
}

// sendToTarget 发送消息到指定目标
func (e *FeishuExecutor) sendToTarget(ctx context.Context, token, target, targetType, msgType, content string) model.SendResult {
	receiveIDType := "open_id"
	resolvedTarget := target

	// 根据目标类型确定 receive_id_type
	switch targetType {
	case "chat":
		receiveIDType = "chat_id"
	case "user":
		// 尝试识别 ID 类型
		if isOpenID(target) {
			receiveIDType = "open_id"
		} else if isChatID(target) {
			receiveIDType = "chat_id"
		} else {
			// 可能是用户名，尝试搜索
			user, err := e.Client.SearchUserByName(ctx, token, target)
			if err == nil && user != nil {
				if user.OpenID != "" {
					resolvedTarget = user.OpenID
					receiveIDType = "open_id"
				} else if user.UserID != "" {
					resolvedTarget = user.UserID
					receiveIDType = "user_id"
				}
			} else {
				return model.SendResult{
					TargetID: target,
					Success:  false,
					Error:    fmt.Sprintf("user not found: %s", target),
				}
			}
		}
	}

	result := e.Client.SendMessage(ctx, token, feishu.SendMessageRequest{
		ReceiveID:     resolvedTarget,
		ReceiveIDType: receiveIDType,
		MsgType:       msgType,
		Content:       content,
	})

	if result.Error != nil {
		return model.SendResult{
			TargetID: target,
			Success:  false,
			Error:    result.Error.Error(),
		}
	}

	return model.SendResult{
		TargetID: target,
		Success:  true,
		MsgID:    result.MessageID,
	}
}

// buildSendMessageSummary 构建发送消息摘要
func (e *FeishuExecutor) buildSendMessageSummary(results []model.SendResult, _ model.SendMessageParams) model.ActionSummary {
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
		Type: "feishu_message",
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

// isChatID 判断是否是群聊 ID
func isChatID(id string) bool {
	return len(id) > 3 && id[:3] == "oc_"
}
