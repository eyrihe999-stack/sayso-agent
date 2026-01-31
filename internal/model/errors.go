package model

import "errors"

var (
	ErrLLMUnavailable   = errors.New("llm service unavailable")
	ErrFeishuDisabled   = errors.New("feishu integration disabled")
	ErrSlackDisabled    = errors.New("slack integration disabled")
	ErrActionNotSupport = errors.New("action type not supported")
	ErrInvalidParams    = errors.New("invalid action params")
)
