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

// checkHTTPStatus 读取 body 并检查 HTTP 状态码；非 2xx 时直接返回错误（不解析 JSON），
// 避免网关/404 返回纯文本（如 "404 page not found"）时出现 "invalid character 'p' after top-level value"。
// 约定：本包内所有飞书 API 调用必须先通过 checkHTTPStatus 检查状态码，再对 body 做 json.Unmarshal。
func (c *Client) checkHTTPStatus(resp *http.Response, apiName string) ([]byte, error) {
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", apiName, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: http status %d, body: %s", apiName, resp.StatusCode, string(b))
	}
	return b, nil
}

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
	b, err := c.checkHTTPStatus(resp, "feishu auth")
	if err != nil {
		return "", err
	}
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
	b, err := c.checkHTTPStatus(resp, "feishu create doc")
	if err != nil {
		return "", err
	}
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

// 创建文件夹接口响应：https://open.feishu.cn/document/server-docs/docs/drive-v1/folder/create_folder
// POST /open-apis/drive/v1/folder/create_folder，请求体 name、folder_token；响应 data 含新文件夹 token
type driveCreateFolderResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	} `json:"data"`
}

// CreateFolder 创建云空间文件夹
// API: POST /open-apis/drive/v1/folder/create_folder
// 请求体：name（文件夹名称）、folder_token（父文件夹 token，不传则在根目录下创建需按文档确认是否必填）
func (c *Client) CreateFolder(ctx context.Context, accessToken, parentFolderToken, name string) (string, error) {
	url := feishuAPIBase + "/drive/v1/files/create_folder"
	reqBody := map[string]string{
		"name":         name,
		"folder_token": parentFolderToken,
	}
	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	b, err := c.checkHTTPStatus(resp, "feishu create folder")
	if err != nil {
		return "", err
	}
	var result driveCreateFolderResp
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("feishu create folder parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu create folder: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))
	}
	return result.Data.Token, nil
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
	b, err := c.checkHTTPStatus(resp, "feishu add collaborator")
	if err != nil {
		return err
	}
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
	b, err := c.checkHTTPStatus(resp, "feishu search user")
	if err != nil {
		return nil, err
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
	b, err := c.checkHTTPStatus(resp, "feishu get root folder")
	if err != nil {
		return "", err
	}
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
	b, err := c.checkHTTPStatus(resp, "feishu list folder")
	if err != nil {
		return nil, err
	}
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
// 若 content 中含 http/https 链接，会以 post 富文本发送，使链接可点击；否则以 text 发送
func (c *Client) SendIM(ctx context.Context, token, receiveIDType, receiveID, content string) error {
	url := feishuAPIBase + "/im/v1/messages"
	params := "?receive_id_type=" + receiveIDType
	var contentStr string
	if linkURL := extractFirstURL(content); linkURL != "" {
		// 使用 post 富文本，链接可点击
		contentStr = buildPostContentWithLink(content, linkURL)
		reqBody := map[string]any{
			"receive_id": receiveID,
			"msg_type":   "post",
			"content":    contentStr,
		}
		data, _ := json.Marshal(reqBody)
		return c.sendIMRequest(ctx, token, url+params, data)
	}
	// text 类型，对 content 做 JSON 转义，避免引号/换行导致发送失败或链接被截断
	textContent, _ := json.Marshal(map[string]string{"text": content})
	reqBody := map[string]any{
		"receive_id": receiveID,
		"msg_type":   "text",
		"content":    string(textContent),
	}
	data, _ := json.Marshal(reqBody)
	return c.sendIMRequest(ctx, token, url+params, data)
}

// sendIMRequest 发送飞书消息请求（公共逻辑）
func (c *Client) sendIMRequest(ctx context.Context, token, fullURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	b, err := c.checkHTTPStatus(resp, "feishu send im")
	if err != nil {
		return err
	}
	var result sendMessageResp
	if err := json.Unmarshal(b, &result); err != nil {
		return fmt.Errorf("feishu send im parse response: %w, body: %s", err, string(b))
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu send im: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))
	}
	return nil
}

// extractFirstURL 从文本中提取第一个 http(s) URL
func extractFirstURL(s string) string {
	const https = "https://"
	const http = "http://"
	rest := s
	for {
		i1 := bytes.Index([]byte(rest), []byte(https))
		i2 := bytes.Index([]byte(rest), []byte(http))
		start := -1
		if i1 >= 0 && (i2 < 0 || i1 <= i2) {
			start = i1
		} else if i2 >= 0 {
			start = i2
		}
		if start < 0 {
			return ""
		}
		idx := start
		if idx+len(https) <= len(rest) && rest[idx:idx+len(https)] == https {
			idx += len(https)
		} else if idx+len(http) <= len(rest) && rest[idx:idx+len(http)] == http {
			idx += len(http)
		} else {
			rest = rest[start+1:]
			continue
		}
		for idx < len(rest) && isURLChar(rest[idx]) {
			idx++
		}
		return rest[start:idx]
	}
}

func isURLChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
		b == '.' || b == '-' || b == '_' || b == '~' || b == ':' || b == '/' || b == '?' || b == '#' || b == '[' || b == ']' || b == '@' || b == '!'
}

// buildPostContentWithLink 构建飞书 post 富文本 content（zh_cn），一段：正文 + 可点击链接 + 链接后文字
// 飞书 post 格式：{"zh_cn":{"content":[[{"tag":"text","text":"..."},{"tag":"a","text":"显示文字","href":"url"},{"tag":"text","text":"..."}]]}}
func buildPostContentWithLink(fullText, linkURL string) string {
	idx := bytes.Index([]byte(fullText), []byte(linkURL))
	textBefore := fullText
	textAfter := ""
	if idx >= 0 {
		textBefore = fullText[:idx]
		if idx+len(linkURL) <= len(fullText) {
			textAfter = fullText[idx+len(linkURL):]
		}
	}
	paragraph := []any{
		map[string]string{"tag": "text", "text": textBefore},
		map[string]string{"tag": "a", "text": linkURL, "href": linkURL},
		map[string]string{"tag": "text", "text": textAfter},
	}
	zhCN := map[string]any{"content": [][]any{paragraph}}
	root := map[string]any{"zh_cn": zhCN}
	b, _ := json.Marshal(root)
	return string(b)
}

// SendMessageRequest 统一发送消息请求参数
type SendMessageRequest struct {
	ReceiveID     string // 接收者 ID
	ReceiveIDType string // open_id | user_id | chat_id
	MsgType       string // text | post | interactive
	Content       string // JSON 格式的消息内容
}

// SendMessageResult 发送消息结果
type SendMessageResult struct {
	MessageID string
	Error     error
}

// SendMessage 发送消息（统一入口，支持私聊和群聊）
func (c *Client) SendMessage(ctx context.Context, token string, req SendMessageRequest) SendMessageResult {
	url := feishuAPIBase + "/im/v1/messages?receive_id_type=" + req.ReceiveIDType
	reqBody := map[string]any{
		"receive_id": req.ReceiveID,
		"msg_type":   req.MsgType,
		"content":    req.Content,
	}
	data, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return SendMessageResult{Error: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return SendMessageResult{Error: err}
	}
	b, err := c.checkHTTPStatus(resp, "feishu send message")
	if err != nil {
		return SendMessageResult{Error: err}
	}
	var result sendMessageResp
	if err := json.Unmarshal(b, &result); err != nil {
		return SendMessageResult{Error: fmt.Errorf("feishu send message parse response: %w, body: %s", err, string(b))}
	}
	if result.Code != 0 {
		return SendMessageResult{Error: fmt.Errorf("feishu send message: code=%d msg=%s body=%s", result.Code, result.Msg, string(b))}
	}
	msgID := ""
	if result.Data != nil {
		msgID = result.Data.MessageID
	}
	return SendMessageResult{MessageID: msgID}
}

// BuildTextContent 构建纯文本消息内容
func BuildTextContent(text string) string {
	content, _ := json.Marshal(map[string]string{"text": text})
	return string(content)
}

// BuildPostContent 构建富文本消息内容（带可点击链接）
func BuildPostContent(title, text, linkURL string) string {
	var paragraph []any
	if text != "" {
		paragraph = append(paragraph, map[string]string{"tag": "text", "text": text})
	}
	if linkURL != "" {
		paragraph = append(paragraph, map[string]string{"tag": "a", "text": linkURL, "href": linkURL})
	}
	post := map[string]any{
		"zh_cn": map[string]any{
			"title":   title,
			"content": [][]any{paragraph},
		},
	}
	b, _ := json.Marshal(post)
	return string(b)
}

// BuildInteractiveCard 构建交互式卡片消息内容（链接卡片）
func BuildInteractiveCard(title, text, linkURL, description string) string {
	elements := []any{}
	if text != "" {
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":     "plain_text",
				"content": text,
			},
		})
	}
	if description != "" {
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":     "plain_text",
				"content": description,
			},
		})
	}
	if linkURL != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []any{
				map[string]any{
					"tag": "button",
					"text": map[string]any{
						"tag":     "plain_text",
						"content": "查看链接",
					},
					"type": "primary",
					"url":  linkURL,
				},
			},
		})
	}
	card := map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
		},
		"elements": elements,
	}
	b, _ := json.Marshal(card)
	return string(b)
}
