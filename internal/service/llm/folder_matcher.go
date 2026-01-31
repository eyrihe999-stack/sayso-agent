package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sayso-agent/internal/client/feishu"
	clientllm "sayso-agent/internal/client/llm"
)

// FolderMatcher 智能目录匹配服务（依赖大模型）
type FolderMatcher struct {
	client *clientllm.Client
}

// NewFolderMatcher 创建目录匹配服务
func NewFolderMatcher(client *clientllm.Client) *FolderMatcher {
	return &FolderMatcher{client: client}
}

// folderMatchResult LLM 返回的匹配结果
type folderMatchResult struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

const folderMatchPrompt = `你是一个文件分类助手。根据文档标题，从以下目录列表中选择最合适的存放目录。

文档标题: %s

可用目录:
%s

请选择最合适的目录来存放这个文档。如果没有明确匹配的目录，返回根目录（token 为 "root"）。

只返回 JSON，格式如下：
{"token": "目录token", "name": "目录名称"}`

// MatchFolder 根据文档标题和目录列表，选择最合适的目录
func (m *FolderMatcher) MatchFolder(ctx context.Context, docTitle string, folders []feishu.FolderInfo) (token, name string, err error) {
	if len(folders) == 0 {
		return "", "", fmt.Errorf("no folders available")
	}
	if len(folders) == 1 {
		return folders[0].Token, folders[0].Name, nil
	}

	var folderList strings.Builder
	var rootToken, rootName string
	for i, f := range folders {
		if f.Name == "我的空间" || f.ParentToken == "" {
			rootToken = f.Token
			rootName = f.Name
		}
		fmt.Fprintf(&folderList, "%d. token: %s, 名称: %s\n", i+1, f.Token, f.Name)
	}

	prompt := fmt.Sprintf(folderMatchPrompt, docTitle, folderList.String())
	raw, err := m.client.Chat(ctx, "你是一个文件分类助手，只返回 JSON 格式的结果。", prompt)
	if err != nil {
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	raw = ExtractJSON(raw)
	var result folderMatchResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	if result.Token == "root" {
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	for _, f := range folders {
		if f.Token == result.Token {
			return result.Token, result.Name, nil
		}
	}

	if rootToken != "" {
		return rootToken, rootName, nil
	}
	return folders[0].Token, folders[0].Name, nil
}
