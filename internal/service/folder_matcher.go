package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sayso-agent/internal/client/feishu"
)

// FolderMatcher 智能目录匹配服务
type FolderMatcher struct {
	llm *LLMService
}

// NewFolderMatcher 创建目录匹配服务
func NewFolderMatcher(llm *LLMService) *FolderMatcher {
	return &FolderMatcher{llm: llm}
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
// 返回匹配的 folder_token 和目录名称
func (m *FolderMatcher) MatchFolder(ctx context.Context, docTitle string, folders []feishu.FolderInfo) (token, name string, err error) {
	if len(folders) == 0 {
		return "", "", fmt.Errorf("no folders available")
	}

	// 如果只有根目录，直接返回
	if len(folders) == 1 {
		return folders[0].Token, folders[0].Name, nil
	}

	// 构建目录列表描述
	var folderList strings.Builder
	var rootToken, rootName string
	for i, f := range folders {
		if f.Name == "我的空间" || f.ParentToken == "" {
			rootToken = f.Token
			rootName = f.Name
		}
		fmt.Fprintf(&folderList, "%d. token: %s, 名称: %s\n", i+1, f.Token, f.Name)
	}

	// 调用 LLM 进行匹配
	prompt := fmt.Sprintf(folderMatchPrompt, docTitle, folderList.String())
	raw, err := m.llm.client.Chat(ctx, "你是一个文件分类助手，只返回 JSON 格式的结果。", prompt)
	if err != nil {
		// LLM 调用失败，返回根目录
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	// 解析 LLM 返回结果
	raw = extractJSON(raw)
	var result folderMatchResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		// 解析失败，返回根目录
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	// 如果返回 root，使用根目录
	if result.Token == "root" {
		if rootToken != "" {
			return rootToken, rootName, nil
		}
		return folders[0].Token, folders[0].Name, nil
	}

	// 验证返回的 token 是否在可用列表中
	for _, f := range folders {
		if f.Token == result.Token {
			return result.Token, result.Name, nil
		}
	}

	// token 不在列表中，返回根目录
	if rootToken != "" {
		return rootToken, rootName, nil
	}
	return folders[0].Token, folders[0].Name, nil
}
