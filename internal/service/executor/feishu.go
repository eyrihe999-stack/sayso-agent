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
func (e *FeishuExecutor) ExecuteCreateDoc(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
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
	e.addDocCollaborators(ctx, token, fileToken, spec, req)

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
func (e *FeishuExecutor) ExecuteCreateFolder(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
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

// ExecuteSendIM 发送飞书私聊消息
func (e *FeishuExecutor) ExecuteSendIM(ctx context.Context, spec model.ActionSpec, req *model.ASRRequest) (model.ActionSummary, error) {
	if !e.Cfg.Enabled {
		return model.ActionSummary{}, model.ErrFeishuDisabled
	}
	token, err := e.Client.GetTenantAccessToken(ctx)
	if err != nil {
		return model.ActionSummary{}, err
	}
	content, _ := spec.Params["content"].(string)
	receiveID, receiveIDType := resolveFeishuReceiveID(spec, req)
	if receiveID == "" {
		return model.ActionSummary{}, fmt.Errorf("feishu_send_im: receive_id is required")
	}
	err = e.Client.SendIM(ctx, token, receiveIDType, receiveID, content)
	if err != nil {
		return model.ActionSummary{}, err
	}
	summary := model.ActionSummary{Type: "feishu_im", Target: receiveID}
	if e.Cfg.Domain != "" && receiveID != "" {
		summary.URL = fmt.Sprintf("https://%s/messenger/?openChatId=%s", e.Cfg.Domain, receiveID)
	}
	return summary, nil
}

func (e *FeishuExecutor) addDocCollaborators(ctx context.Context, accessToken, docToken string, spec model.ActionSpec, req *model.ASRRequest) {
	callerID, callerIDType := getCallerFeishuID(req)
	if callerID != "" {
		_ = e.Client.AddCollaborator(ctx, accessToken, docToken, "docx", feishu.Collaborator{
			MemberType: callerIDType,
			MemberID:   callerID,
			Perm:       "full_access",
		})
	}
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
				resolvedID := memberID
				if !isOpenID(memberID) {
					user, err := e.Client.SearchUserByName(ctx, accessToken, memberID)
					if err == nil && user != nil && user.UserID != "" {
						resolvedID = user.UserID
						memberType = "userid"
					} else {
						continue
					}
				}
				if resolvedID != callerID {
					_ = e.Client.AddCollaborator(ctx, accessToken, docToken, "docx", feishu.Collaborator{
						MemberType: memberType,
						MemberID:   resolvedID,
						Perm:       perm,
					})
				}
			}
		}
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

func getCallerFeishuID(req *model.ASRRequest) (id, idType string) {
	if req == nil {
		return "", ""
	}
	if req.Context != nil {
		if id := req.Context["feishu_open_id"]; id != "" {
			return id, "openid"
		}
		if id := req.Context["feishu_user_id"]; id != "" {
			return id, "userid"
		}
	}
	if req.UserID != "" {
		return req.UserID, "openid"
	}
	return "", ""
}

// resolveFeishuReceiveID 解析飞书私聊接收人 ID 与类型
func resolveFeishuReceiveID(spec model.ActionSpec, req *model.ASRRequest) (receiveID, receiveIDType string) {
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
	receiveIDType = "open_id"
	if t, ok := spec.Params["receive_id_type"].(string); ok && t != "" {
		receiveIDType = t
	} else if usedFeishuUserID {
		receiveIDType = "user_id"
	}
	if isOpenID(receiveID) {
		receiveIDType = "open_id"
	}
	return receiveID, receiveIDType
}
