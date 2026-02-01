package model

import (
	"reflect"
	"testing"
)

func TestParseSendMessageParams(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		expected SendMessageParams
	}{
		{
			name: "basic text message to user",
			params: map[string]any{
				"platform":     "feishu",
				"message_type": "text",
				"target_type":  "user",
				"targets":      []any{"张三"},
				"content": map[string]any{
					"text": "你好",
				},
			},
			expected: SendMessageParams{
				Platform:    "feishu",
				MessageType: "text",
				TargetType:  "user",
				Targets:     []string{"张三"},
				Content: MessageContent{
					Text: "你好",
				},
			},
		},
		{
			name: "link card to chat",
			params: map[string]any{
				"platform":     "feishu",
				"message_type": "link_card",
				"target_type":  "chat",
				"targets":      []any{"oc_xxx"},
				"content": map[string]any{
					"title":       "项目文档",
					"text":        "请查看最新的项目文档",
					"url":         "https://example.com/doc",
					"description": "这是一个重要文档",
				},
			},
			expected: SendMessageParams{
				Platform:    "feishu",
				MessageType: "link_card",
				TargetType:  "chat",
				Targets:     []string{"oc_xxx"},
				Content: MessageContent{
					Title:       "项目文档",
					Text:        "请查看最新的项目文档",
					URL:         "https://example.com/doc",
					Description: "这是一个重要文档",
				},
			},
		},
		{
			name: "batch send to multiple users",
			params: map[string]any{
				"platform":     "feishu",
				"message_type": "text",
				"target_type":  "batch",
				"targets":      []any{"张三", "李四", "王五"},
				"content": map[string]any{
					"text": "会议通知",
				},
			},
			expected: SendMessageParams{
				Platform:    "feishu",
				MessageType: "text",
				TargetType:  "batch",
				Targets:     []string{"张三", "李四", "王五"},
				Content: MessageContent{
					Text: "会议通知",
				},
			},
		},
		{
			name: "slack message to channel",
			params: map[string]any{
				"platform":     "slack",
				"message_type": "text",
				"target_type":  "chat",
				"targets":      []any{"#general"},
				"content": map[string]any{
					"text": "Hello team!",
				},
			},
			expected: SendMessageParams{
				Platform:    "slack",
				MessageType: "text",
				TargetType:  "chat",
				Targets:     []string{"#general"},
				Content: MessageContent{
					Text: "Hello team!",
				},
			},
		},
		{
			name:   "empty params",
			params: map[string]any{},
			expected: SendMessageParams{
				Targets: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSendMessageParams(tt.params)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseSendMessageParams() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}
