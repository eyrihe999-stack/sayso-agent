# sayso-agent

基于 Gin 的智能任务执行服务：接收自然语言输入（ASR 文本），通过两阶段 LLM 处理理解意图并编排任务，代为调用飞书、Slack 等外部平台 API 完成操作。

> **设计理念**：大模型负责理解和规划，本服务负责安全执行。大模型无外部 API 权限，所有操作由本服务把关后执行。

## 场景示例

用户输入：**「给飞书的张三和 Slack 的 bob 同时发消息说开会，然后创建一个周报文档」**

服务会：
1. 识别出 3 个任务：飞书发消息、Slack 发消息、创建文档
2. 并行执行这 3 个无依赖的任务
3. 返回所有任务的执行结果

---

## 系统架构

### 整体流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              sayso-agent                                    │
│                                                                             │
│  ┌─────────┐    ┌─────────────────────────────────────────────────────┐    │
│  │ Handler │───▶│                    ASR Service                      │    │
│  │ (HTTP)  │    │                                                     │    │
│  └─────────┘    │  ┌───────────────────────────────────────────────┐  │    │
│                 │  │              LLM Service                      │  │    │
│                 │  │                                               │  │    │
│                 │  │   ┌─────────────┐    ┌──────────────────┐    │  │    │
│                 │  │   │ 第一阶段    │    │ 第二阶段          │    │  │    │
│                 │  │   │ 任务规划    │───▶│ 并行执行 Skills   │    │  │    │
│                 │  │   └─────────────┘    └──────────────────┘    │  │    │
│                 │  │                                               │  │    │
│                 │  └───────────────────────────────────────────────┘  │    │
│                 │                           │                          │    │
│                 │                           ▼                          │    │
│                 │  ┌───────────────────────────────────────────────┐  │    │
│                 │  │                  Executor                      │  │    │
│                 │  │   ┌──────────┐  ┌──────────┐  ┌──────────┐   │  │    │
│                 │  │   │  Feishu  │  │  Slack   │  │  更多...  │   │  │    │
│                 │  │   │ Executor │  │ Executor │  │          │   │  │    │
│                 │  │   └────┬─────┘  └────┬─────┘  └──────────┘   │  │    │
│                 │  └────────┼─────────────┼────────────────────────┘  │    │
│                 └───────────┼─────────────┼────────────────────────────┘    │
└─────────────────────────────┼─────────────┼─────────────────────────────────┘
                              │             │
                              ▼             ▼
                        ┌──────────┐  ┌──────────┐
                        │ 飞书 API │  │ Slack API│
                        └──────────┘  └──────────┘
```

### 两阶段 LLM 处理

```
用户输入: "给飞书的张三和slack的bob发消息，然后创建周报把链接发给李四"
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     第一阶段：任务规划 (Planner)                      │
│                                                                     │
│  Prompt: 分析用户输入，识别所有任务及依赖关系                          │
│                                                                     │
│  输出 TaskPlan:                                                     │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ task_1: send_message (feishu, 张三)      depends_on: []       │ │
│  │ task_2: send_message (slack, bob)        depends_on: []       │ │
│  │ task_3: create_doc (周报)                depends_on: []       │ │
│  │ task_4: send_message (李四, 带链接)       depends_on: [task_3] │ │
│  └───────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    第二阶段：并行执行 Skills                          │
│                                                                     │
│  Wave 1 (无依赖，并行执行):                                          │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐       │
│  │ task_1          │ │ task_2          │ │ task_3          │       │
│  │ send_message    │ │ send_message    │ │ create_doc      │       │
│  │ Skill Prompt    │ │ Skill Prompt    │ │ Skill Prompt    │       │
│  └────────┬────────┘ └────────┬────────┘ └────────┬────────┘       │
│           │                   │                   │                 │
│           ▼                   ▼                   ▼                 │
│       ActionSpec          ActionSpec          ActionSpec            │
│                                                   │                 │
│  ─────────────────────────────────────────────────┼─────────────── │
│                                                   │                 │
│  Wave 2 (等待 task_3 完成):                        │                 │
│  ┌─────────────────┐◀─────────────────────────────┘                 │
│  │ task_4          │  获取 {{doc_url}}                              │
│  │ send_message    │                                                │
│  │ Skill Prompt    │                                                │
│  └────────┬────────┘                                                │
│           │                                                         │
│           ▼                                                         │
│       ActionSpec                                                    │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                          ┌─────────────────┐
                          │ Executor 执行   │
                          │ 调用外部 API    │
                          └─────────────────┘
```

---

## Skill 系统

每种操作封装为独立的 Skill，拥有专用的精简 Prompt：

| Skill | 类型 | 说明 | Prompt 规模 |
|-------|------|------|-------------|
| `create_doc` | 飞书 | 创建云文档 | ~8 行 |
| `create_folder` | 飞书 | 创建文件夹 | ~6 行 |
| `send_message` | 通用 | 发送消息（飞书/Slack） | ~10 行 |

### Skill Prompt 示例

```go
// send_message Skill
`提取发送消息参数，返回 JSON：
{"type":"send_message","params":{
  "platform":"feishu|slack",
  "message_type":"text",
  "content":{"text":"消息"},
  "target_type":"user|chat|batch",
  "targets":["目标"]
}}

规则：
- platform: feishu(默认)/slack
- target_type: user(单人)/chat(群)/batch(多人)
- 引用之前任务的结果用占位符：{{doc_url}} {{folder_url}}

只返回 JSON。`
```

### 扩展新 Skill

1. 在 `internal/service/llm/service.go` 添加 Skill 类型和 Prompt：
```go
const SkillNewAction SkillType = "new_action"

var skillPrompts = map[SkillType]string{
    SkillNewAction: `你的 Skill Prompt...`,
}
```

2. 在 `internal/service/executor/` 添加执行器实现

---

## 依赖关系处理

### 依赖识别

第一阶段 Planner 自动识别任务间的依赖：

```json
{
  "tasks": [
    {"id": "task_1", "skill": "create_doc", "depends_on": []},
    {"id": "task_2", "skill": "send_message", "input": "把文档链接发给张三", "depends_on": ["task_1"]}
  ]
}
```

### 执行调度算法

```go
for len(pending) > 0 {
    // 1. 找出所有依赖已满足的任务
    ready := findReadyTasks(pending, completed)

    // 2. 并行执行就绪任务
    parallel.Execute(ready)

    // 3. 任一任务失败则终止
    if hasFailure(ready) {
        return error
    }
}
```

### 占位符替换

后续任务可通过占位符引用前置任务的输出：

| 占位符 | 说明 |
|--------|------|
| `{{doc_url}}` | 创建的文档链接 |
| `{{folder_url}}` | 创建的文件夹链接 |
| `{{last_url}}` | 最近创建资源的链接 |

---

## 外部集成

### 飞书 (Feishu/Lark)

| 功能 | API |
|------|-----|
| 创建文档 | `POST /docx/v1/documents` |
| 创建文件夹 | `POST /drive/v1/files/create_folder` |
| 发送消息 | `POST /im/v1/messages` |
| 添加协作者 | `POST /drive/v1/permissions` |
| 搜索用户 | `POST /search/v1/user` |

配置：
```yaml
feishu:
  enabled: true
  app_id: "cli_xxx"
  app_secret: "xxx"
  domain: "your-company.feishu.cn"
```

### Slack

| 功能 | API |
|------|-----|
| 发送消息 | `POST /chat.postMessage` |
| 打开私聊 | `POST /conversations.open` |

配置：
```yaml
slack:
  enabled: true
  bot_token: "xoxb-xxx"
```

---

## 项目结构

```
sayso-agent/
├── cmd/server/
│   └── main.go                 # 入口：配置加载、依赖注入、启动服务
├── config/
│   ├── config.go               # 配置结构与加载逻辑
│   └── {local,dev,prod}.yaml   # 环境配置
├── internal/
│   ├── handler/
│   │   ├── asr.go              # ASR 处理接口
│   │   └── router.go           # 路由注册
│   ├── service/
│   │   ├── asr.go              # 请求编排
│   │   ├── llm/
│   │   │   ├── service.go      # 两阶段 LLM 处理
│   │   │   └── folder_matcher.go
│   │   └── executor/
│   │       ├── executor.go     # 动作路由
│   │       ├── feishu.go       # 飞书执行器
│   │       └── slack.go        # Slack 执行器
│   ├── client/
│   │   ├── llm/client.go       # LLM API 客户端
│   │   ├── feishu/client.go    # 飞书 API 客户端
│   │   └── slack/client.go     # Slack API 客户端
│   ├── model/                  # 数据模型
│   └── middleware/             # HTTP 中间件
└── go.mod
```

---

## 运行

### 本地开发

```bash
export APP_ENV=local
go run ./cmd/server
```

### 构建部署

```bash
go build -o sayso-agent ./cmd/server
./sayso-agent
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `APP_ENV` | 环境：local/dev/prod |
| `LLM_API_KEY` | LLM API 密钥 |
| `FEISHU_APP_ID` | 飞书应用 ID |
| `FEISHU_APP_SECRET` | 飞书应用密钥 |
| `SLACK_BOT_TOKEN` | Slack Bot Token |

### API 接口

```bash
# 健康检查
GET /health

# ASR 处理
POST /api/v1/asr/process
Content-Type: application/json

{
  "text": "创建周报文档然后把链接发给张三",
  "user_id": "ou_xxx"
}
```

---

## 高并发扩展方案

### 当前瓶颈分析

```
请求处理耗时分布：

LLM API (规划)        ████████████████  500-2000ms
LLM API (Skills x N)  ████████████████  500-2000ms（并行）
外部 API              ████              50-200ms
本地计算              █                 <1ms
```

**结论**：瓶颈在 LLM API，本地服务 CPU 占用极低。

### 扩展路径

```
┌─────────────────────────────────────────────────────────────────────┐
│  阶段一：单实例 + 本地限流                                           │
│                                                                     │
│  ┌─────────────┐                                                   │
│  │ sayso-agent │ ── Rate Limiter ──▶ LLM API                       │
│  └─────────────┘                                                   │
│                                                                     │
│  适用：QPS < 100，LLM API 有 rate limit                            │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  阶段二：水平扩展                                                    │
│                                                                     │
│              ┌─────────────┐                                       │
│              │   Nginx     │                                       │
│              │ 负载均衡    │                                       │
│              └──────┬──────┘                                       │
│         ┌──────────┼──────────┐                                    │
│         ▼          ▼          ▼                                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                           │
│  │ 实例 1   │ │ 实例 2   │ │ 实例 3   │                           │
│  └──────────┘ └──────────┘ └──────────┘                           │
│                                                                     │
│  适用：QPS 100-1000，服务无状态可随时扩容                            │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  阶段三：异步队列（可选）                                             │
│                                                                     │
│  ┌────────┐    ┌─────────┐    ┌──────────┐    ┌─────────────┐     │
│  │ API GW │───▶│  Queue  │───▶│ Workers  │───▶│ Webhook/WS  │     │
│  └────────┘    └─────────┘    └──────────┘    └─────────────┘     │
│                                                                     │
│  适用：需要异步通知、削峰填谷、跨服务事务保证                          │
└─────────────────────────────────────────────────────────────────────┘
```

### 本地限流器实现

```go
type RateLimiter struct {
    sem chan struct{}
}

func NewRateLimiter(maxConcurrent int) *RateLimiter {
    return &RateLimiter{sem: make(chan struct{}, maxConcurrent)}
}

func (r *RateLimiter) Acquire(ctx context.Context) error {
    select {
    case r.sem <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (r *RateLimiter) Release() {
    <-r.sem
}
```

---

## 性能特性

| 特性 | 说明 |
|------|------|
| **并行 Skill 执行** | 无依赖任务同时调用 LLM，减少总延迟 |
| **精简 Prompt** | 每个 Skill 独立 Prompt（<10行），提高 LLM 响应速度和准确性 |
| **Goroutine 调度** | 利用 Go 原生并发，IO 等待不占 CPU |
| **无状态设计** | 便于水平扩展 |

---

## License

MIT
