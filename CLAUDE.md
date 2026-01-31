# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run

```bash
# Run locally (requires config/local.yaml)
export APP_ENV=local
go run ./cmd/server

# Build binary
go build -o sayso-agent ./cmd/server
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/service/...

# Run a single test
go test ./internal/service -run TestASRService
```

## Configuration

Environment selection via `APP_ENV` (local/dev/prod) loads `config/<env>.yaml`. Sensitive values can be overridden with environment variables: `LLM_API_KEY`, `FEISHU_APP_ID`, `FEISHU_APP_SECRET`, `SLACK_BOT_TOKEN`.

## Architecture

This is a Gin-based API service that receives ASR (speech-to-text) input, interprets user intent via LLM, and executes actions on external platforms (Feishu, Slack). The LLM does NOT have direct access to external APIs; this service acts as the executor.

### Request Flow

1. **Handler** (`internal/handler/asr.go`) receives POST `/api/v1/asr/process` with `{text, user_id, context}`
2. **ASRService** (`internal/service/asr.go`) orchestrates the pipeline
3. **LLMService** (`internal/service/llm.go`) calls LLM with a system prompt, returns structured `LLMActionOutput` (intent + actions array)
4. **Executor** (`internal/service/executor.go`) dispatches actions by type (`feishu_create_doc`, `feishu_send_im`, `slack_send_message`) to the appropriate client

### Key Types

- `model.ActionSpec` - Single action with type, params, target_user_id, target_chat_id
- `model.LLMActionOutput` - LLM response containing intent, reply, and actions array

### Adding New Actions

1. Add action type constant in `internal/model/action.go`
2. Add case in `Executor.Execute()` switch in `internal/service/executor.go`
3. Update the system prompt in `internal/service/llm.go` to document the new action format
