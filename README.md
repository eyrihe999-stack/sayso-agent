# sayso-agent

基于 Gin 的 API 服务：接收内部 ASR 文本，经大模型理解后由本服务代为调用飞书、Slack 等外部能力（大模型无外部 app 权限，由本服务执行动作）。

## 场景示例

内部传入文本：**「帮我整理我今天的所有会话记录，并整理成一个飞书文档或在飞书私聊发给我」**

1. 本服务接收 ASR 文本；
2. 调用大模型理解意图，大模型返回结构化动作（如：`feishu_create_doc`、`feishu_send_im`）及参数；
3. 本服务根据动作类型调用飞书 API（创建文档、发送私聊等）完成操作。

## 项目结构（分层参考 Gin 最佳实践）

```
sayso-agent/
├── cmd/
│   └── server/
│       └── main.go          # 入口：按环境加载配置、组装依赖、启动 HTTP
├── config/
│   ├── config.go            # 配置结构体与按环境加载逻辑
│   ├── local.yaml           # 本地环境
│   ├── dev.yaml             # 开发环境
│   └── prod.yaml             # 生产环境
├── internal/
│   ├── handler/             # HTTP 层（Gin）
│   │   ├── asr.go           # ASR 处理接口
│   │   └── router.go        # 路由与中间件注册
│   ├── service/             # 业务服务层
│   │   ├── asr.go           # 编排：ASR -> LLM -> 执行动作
│   │   ├── llm.go           # 调用大模型并解析为结构化动作
│   │   └── executor.go      # 根据动作类型调用飞书/Slack 等
│   ├── client/              # 外部服务客户端
│   │   ├── llm/             # 大模型（OpenAI 兼容）
│   │   ├── feishu/          # 飞书开放平台
│   │   └── slack/           # Slack API
│   ├── model/               # 领域模型与请求/响应结构
│   └── middleware/          # 日志、恢复等中间件
├── .env.example
├── go.mod
└── README.md
```

## 配置说明（按环境区分）

- 通过环境变量 **`APP_ENV`** 选择配置：`local` | `dev` | `prod`，对应加载 `config/<env>.yaml`。
- 敏感项建议用环境变量覆盖（见 `config/config.go` 中 `overrideFromEnv`）：
  - `LLM_API_KEY`、`FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`SLACK_BOT_TOKEN`。

复制 `.env.example` 为 `.env` 并按需填写，运行前 `source .env` 或由部署系统注入。

## 运行

```bash
# 在项目根目录执行（保证 config/*.yaml 可被找到）
export APP_ENV=local
go run ./cmd/server
```

默认监听 `:8080`。健康检查：`GET /health`。ASR 处理：`POST /api/v1/asr/process`。

### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/asr/process \
  -H "Content-Type: application/json" \
  -d '{"text":"帮我整理今天的会话并发到飞书私聊","user_id":"ou_xxx"}'
```

## 大模型与动作执行

- 大模型负责理解用户文本并输出 **JSON 动作列表**（类型、参数、目标用户/会话）。
- 本服务 **不把外部系统权限交给大模型**，而是根据大模型返回的 `type` 和 `params` 调用内部封装的飞书/Slack 客户端，完成创建文档、发私聊、发 Slack 消息等。

当前支持的动作类型示例：

- `feishu_create_doc`：创建飞书云文档（params: folder_token, title, content）
- `feishu_send_im`：飞书私聊消息（params: content；target_user_id 或 receive_id）
- `slack_send_message`：Slack 发消息（params: channel, text）

扩展新动作时：在 `internal/model/action.go` 增加类型约定，在 `internal/service/executor.go` 中实现对应分支，并在 `internal/service/llm.go` 的系统提示中说明格式即可。
