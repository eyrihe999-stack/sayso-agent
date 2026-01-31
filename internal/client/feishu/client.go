package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sayso-agent/internal/model"
)

// Config 飞书客户端配置
type Config struct {
	AppID     string
	AppSecret string
	BotToken  string
	Domain    string // 飞书域名，如 example.feishu.cn，用于生成文档链接
	Enabled   bool
}

// Client 飞书 API 客户端（含机器人/应用能力）
type Client struct {
	cfg    Config
	client *http.Client
}

// NewClient 创建飞书客户端
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{},
	}
}

const feishuAPIBase = "https://open.feishu.cn/open-apis"

// 鉴权接口响应：https://open.feishu.cn/document/server-docs/authentication-v3/tenant_access_token/internal
type tenantAccessTokenResp struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// GetTenantAccessToken 获取 tenant_access_token（应用维度）
func (c *Client) GetTenantAccessToken(ctx context.Context) (string, error) {
	url := feishuAPIBase + "/auth/v3/tenant_access_token/internal"
	body := map[string]string{
		"app_id":     c.cfg.AppID,
		"app_secret": c.cfg.AppSecret,
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result tenantAccessTokenResp
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("feishu auth parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu auth: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.TenantAccessToken, nil
}

// docx v1 创建文档接口响应：https://open.feishu.cn/document/server-docs/docs/docs/docx-v1/document/create
// POST /open-apis/docx/v1/documents，请求体 folder_token、title；响应 data.document 含 document_id、revision_id、title
type docxCreateDocumentResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Document struct {
			DocumentID string `json:"document_id"`
			RevisionID int64  `json:"revision_id"`
			Title      string `json:"title"`
		} `json:"document"`
	} `json:"data"`
}

// CreateDoc 创建云文档（docx v1：POST /open-apis/docx/v1/documents）
// 请求体仅 folder_token、title；返回新文档的 document_id，后续写入正文需调 docx 文档内容接口。
func (c *Client) CreateDoc(ctx context.Context, token, folderToken, title, content string) (string, error) {
	url := feishuAPIBase + "/docx/v1/documents"
	reqBody := map[string]string{
		"folder_token": folderToken,
		"title":        title,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result docxCreateDocumentResp
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("feishu create doc parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu create doc: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))
	}
	_ = content
	return result.Data.Document.DocumentID, nil
}

// Collaborator 协作者信息
type Collaborator struct {
	MemberType string // openid, userid, email, chat_id 等
	MemberID   string // 对应类型的 ID
	Perm       string // view, edit, full_access
}

// addPermissionMemberResp 添加协作者响应
type addPermissionMemberResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Member struct {
			MemberType string `json:"member_type"`
			MemberID   string `json:"member_id"`
			Perm       string `json:"perm"`
		} `json:"member"`
	} `json:"data"`
}

// AddCollaborator 添加文档协作者
// API: POST /open-apis/drive/v1/permissions/{token}/members?type={type}
// docType: docx, sheet, bitable, file 等
func (c *Client) AddCollaborator(ctx context.Context, accessToken, docToken, docType string, collaborator Collaborator) error {
	url := fmt.Sprintf("%s/drive/v1/permissions/%s/members?type=%s&need_notification=true", feishuAPIBase, docToken, docType)
	reqBody := map[string]string{
		"member_type": collaborator.MemberType,
		"member_id":   collaborator.MemberID,
		"perm":        collaborator.Perm,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result addPermissionMemberResp
	if err := json.Unmarshal(b, &result); err != nil {
		return fmt.Errorf("feishu add collaborator parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu add collaborator: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))
	}
	return nil
}

// UserInfo 用户信息
type UserInfo struct {
	OpenID string `json:"open_id"`
	UserID string `json:"user_id,omitempty"`
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

// searchUserResp 搜索用户响应
type searchUserResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Users     []UserInfo `json:"users"`
		PageToken string     `json:"page_token"`
		HasMore   bool       `json:"has_more"`
	} `json:"data"`
}

// SearchUser 根据关键词搜索用户
// API: POST /open-apis/directory/v1/employee/search
// 文档: https://open.feishu.cn/document/directory-v1/employee/search
func (c *Client) SearchUser(ctx context.Context, accessToken, query string) ([]UserInfo, error) {
	url := feishuAPIBase + "/directory/v1/employees/search?page_size=20"
	reqBody := map[string]string{
		"query": query,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feishu search user: http status %d, body: %s", resp.StatusCode, string(b))
	}

	// 检查响应是否为空或非 JSON
	if len(b) == 0 {
		return nil, fmt.Errorf("feishu search user: empty response")
	}
	if b[0] != '{' {
		return nil, fmt.Errorf("feishu search user: invalid response (not JSON), body: %.200s", string(b))
	}

	var result model.GetUserInfoAPIResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("feishu search user parse response: %w, body: %.500s", err, string(b))
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("feishu search user: code=%d msg=%s", result.Code, result.Msg)
	}
	// 将 Employee 转换为 UserInfo
	// 注意：employee_id 是 user_id 类型，不是 open_id
	var users []UserInfo
	for _, emp := range result.Data.Employees {
		users = append(users, UserInfo{
			UserID: emp.BaseInfo.EmployeeID, // employee_id 是 user_id 类型
			Name:   emp.BaseInfo.Name.Name.DefaultValue,
			Email:  emp.BaseInfo.Email,
			Avatar: emp.BaseInfo.Avatar.AvatarOrigin,
		})
	}
	return users, nil
}

// SearchUserByName 根据名字搜索用户，返回最匹配的一个
func (c *Client) SearchUserByName(ctx context.Context, accessToken, name string) (*UserInfo, error) {
	users, err := c.SearchUser(ctx, accessToken, name)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", name)
	}
	// 优先返回名字完全匹配的
	for _, u := range users {
		if u.Name == name {
			return &u, nil
		}
	}
	// 否则返回第一个结果
	return &users[0], nil
}

// FolderInfo 文件夹/文件信息
type FolderInfo struct {
	Token       string `json:"token"`
	Name        string `json:"name"`
	Type        string `json:"type"`         // folder, docx, sheet, bitable, etc.
	ParentToken string `json:"parent_token"` // 父目录 token
}

// rootFolderMetaResp 根目录元信息响应
type rootFolderMetaResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Token  string `json:"token"`
		ID     string `json:"id"`
		UserID string `json:"user_id"`
	} `json:"data"`
}

// GetRootFolderToken 获取用户云空间根目录 token
// API: GET /open-apis/drive/explorer/v2/root_folder/meta
func (c *Client) GetRootFolderToken(ctx context.Context, token string) (string, error) {
	url := feishuAPIBase + "/drive/explorer/v2/root_folder/meta"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result rootFolderMetaResp
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("feishu get root folder parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu get root folder: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data.Token, nil
}

// listFilesResp 列出文件响应
type listFilesResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Files []struct {
			Token       string `json:"token"`
			Name        string `json:"name"`
			Type        string `json:"type"`
			ParentToken string `json:"parent_token"`
		} `json:"files"`
		NextPageToken string `json:"next_page_token"`
		HasMore       bool   `json:"has_more"`
	} `json:"data"`
}

// ListFolderChildren 列出指定目录下的子文件/文件夹
// API: GET /open-apis/drive/v1/files?folder_token=xxx
func (c *Client) ListFolderChildren(ctx context.Context, token, folderToken string) ([]FolderInfo, error) {
	url := feishuAPIBase + "/drive/v1/files?folder_token=" + folderToken
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result listFilesResp
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("feishu list folder parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("feishu list folder: code=%d msg=%s", result.Code, result.Msg)
	}
	var folders []FolderInfo
	for _, f := range result.Data.Files {
		folders = append(folders, FolderInfo{
			Token:       f.Token,
			Name:        f.Name,
			Type:        f.Type,
			ParentToken: f.ParentToken,
		})
	}
	return folders, nil
}

// GetFolderTree 递归获取目录树（只返回 folder 类型，限制深度）
func (c *Client) GetFolderTree(ctx context.Context, token string, maxDepth int) ([]FolderInfo, error) {
	rootToken, err := c.GetRootFolderToken(ctx, token)
	if err != nil {
		return nil, err
	}
	var allFolders []FolderInfo
	// 添加根目录
	allFolders = append(allFolders, FolderInfo{
		Token: rootToken,
		Name:  "我的空间",
		Type:  "folder",
	})
	// 递归获取子目录
	c.collectFolders(ctx, token, rootToken, 1, maxDepth, &allFolders)
	return allFolders, nil
}

// collectFolders 递归收集文件夹
func (c *Client) collectFolders(ctx context.Context, token, folderToken string, depth, maxDepth int, result *[]FolderInfo) {
	if depth > maxDepth {
		return
	}
	children, err := c.ListFolderChildren(ctx, token, folderToken)
	if err != nil {
		return
	}
	for _, child := range children {
		if child.Type == "folder" {
			*result = append(*result, child)
			c.collectFolders(ctx, token, child.Token, depth+1, maxDepth, result)
		}
	}
}

// 发送消息接口响应：https://open.feishu.cn/document/server-docs/docs/im-v1/message/create
type sendMessageResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		MessageID  string `json:"message_id"`
		RootID     string `json:"root_id"`
		ParentID   string `json:"parent_id"`
		ThreadID   string `json:"thread_id"`
		MsgType    string `json:"msg_type"`
		Content    string `json:"content"`
		CreateTime string `json:"create_time"`
		UpdateTime string `json:"update_time"`
		Deleted    bool   `json:"deleted"`
		Updated    bool   `json:"updated"`
		ChatID     string `json:"chat_id"`
		Sender     *struct {
			ID         string `json:"id"`
			IDType     string `json:"id_type"`
			SenderType string `json:"sender_type"`
			TenantKey  string `json:"tenant_key"`
		} `json:"sender"`
	} `json:"data"`
}

// SendIM 发送私聊消息（通过机器人或应用）
func (c *Client) SendIM(ctx context.Context, token, receiveIDType, receiveID, content string) error {
	url := feishuAPIBase + "/im/v1/messages"
	params := "?receive_id_type=" + receiveIDType
	reqBody := map[string]interface{}{
		"receive_id": receiveID,
		"msg_type":   "text",
		"content":    fmt.Sprintf(`{"text":"%s"}`, content),
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+params, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result sendMessageResp
	if err := json.Unmarshal(b, &result); err != nil {
		return fmt.Errorf("feishu send im parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu send im: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))
	}
	return nil
}
