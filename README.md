# llm-mock-provider

这是一个用于模拟 OpenAI-compatible 大模型服务的 mock provider 工程，并配套 k6 脚本用于压测 new-api 大模型网关。它包含：

- OpenAI-compatible `mock-provider`
- k6 压测脚本
- docker compose 启动配置
- new-api mock 渠道接入说明
- pprof / Pyroscope 性能分析配置说明

压测链路：

```text
k6 -> new-api -> mock-openai-provider
```

也可以先让 k6 直连 `mock-provider` 验证 mock 服务行为。

## 项目结构

```text
llm-mock-provider/
├── mock-provider/
│   ├── main.go
│   ├── go.mod
│   ├── Dockerfile
│   └── README.md
├── k6/
│   ├── smoke.js
│   ├── chat-non-stream.js
│   ├── chat-stream.js
│   ├── stress.js
│   └── soak.js
├── docker-compose.yml
└── README.md
```

Kubernetes 部署入口已经合入 `new-api-saas/deploy/helm/new-api-saas`。本仓库只保留 mock provider 本身和本地/k6 压测资产。

## 快速启动

```bash
docker compose up --build
```

启动后 mock provider 监听：

```text
http://localhost:3001
```

验证：

```bash
curl -s http://localhost:3001/healthz
curl -s http://localhost:3001/v1/models
```

## mock-provider 接口

### `GET /healthz`

返回：

```json
{"status":"ok"}
```

### `GET /v1/models`

返回模型名来自 `DEFAULT_MODEL`，默认是 `mock-gpt`。

### `POST /v1/chat/completions`

支持 `stream=false` 和 `stream=true`。

非流式示例：

```bash
curl -s http://localhost:3001/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":false}'
```

流式示例：

```bash
curl -N 'http://localhost:3001/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=10' \
  -H 'Content-Type: application/json' \
  -d '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

### `POST /v1/embeddings`

返回 OpenAI-compatible 格式的固定 embedding，不调用真实模型。

## Mock 控制参数

这些参数通过 query string 控制：

| 参数 | 说明 | 示例 |
| --- | --- | --- |
| `delay_ms` | 非流式整体延迟 | `delay_ms=1000` |
| `ttft_ms` | 流式首 token 延迟 | `ttft_ms=300` |
| `chunk_delay_ms` | 流式 chunk 间隔 | `chunk_delay_ms=50` |
| `prompt_tokens` | 模拟输入 token | `prompt_tokens=100` |
| `completion_tokens` | 模拟输出 token；流式模式下控制 content chunk 数 | `completion_tokens=200` |
| `error_rate` | 随机错误概率 | `error_rate=0.05` |
| `error_status` | 错误状态码，默认 `500` | `error_status=429` |
| `timeout_rate` | 随机超时概率 | `timeout_rate=0.01` |
| `timeout_ms` | 超时等待时间，默认 `30000` | `timeout_ms=30000` |

示例：

```text
/v1/chat/completions?delay_ms=1000
/v1/chat/completions?error_rate=0.05&error_status=429
/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200
/v1/chat/completions?timeout_rate=0.01&timeout_ms=30000
```

错误响应格式：

```json
{
  "error": {
    "message": "mock upstream error",
    "type": "mock_error",
    "code": "mock_error"
  }
}
```

## 在 new-api 添加 mock 渠道

在 new-api 后台添加渠道：

| 配置项 | 值 |
| --- | --- |
| 类型 | `OpenAI` |
| Base URL | `http://mock-provider:3001/v1` 或 `http://localhost:3001/v1` |
| API Key | `mock-key` |
| 模型 | `mock-gpt` |

如果 new-api 与 mock-provider 在同一个 docker compose 网络中，使用：

```text
http://mock-provider:3001/v1
```

如果 new-api 跑在宿主机，使用：

```text
http://localhost:3001/v1
```

## k6 脚本

所有脚本都支持：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:3001` | 压测目标地址；压 new-api 时改为 new-api 地址 |
| `API_KEY` | `mock-key` | Bearer token |
| `MODEL` | `mock-gpt` | 请求模型名 |
| `QUERY` | 脚本内默认值 | 追加到 `/v1/chat/completions` 的 query string |

### Smoke Test

验证链路是否通，1 个虚拟用户，持续 30 秒。

```bash
k6 run k6/smoke.js
```

经由 new-api：

```bash
BASE_URL=http://localhost:3000 API_KEY=你的-new-api-key MODEL=mock-gpt k6 run k6/smoke.js
```

### 非流式压测

```bash
k6 run k6/chat-non-stream.js
```

模拟上游延迟：

```bash
QUERY='?delay_ms=1000' k6 run k6/chat-non-stream.js
```

### 流式压测

默认使用：

```text
?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200
```

运行：

```bash
k6 run k6/chat-stream.js
```

### 阶梯压测

`stress.js` 阶梯：

```text
10 VUs -> 50 VUs -> 100 VUs -> 200 VUs -> 500 VUs -> 1000 VUs
```

每档 2 分钟。

```bash
k6 run k6/stress.js
```

### 长稳压测

100 VUs，持续 1 小时。

```bash
k6 run k6/soak.js
```

## 推荐压测流程

1. Smoke Test

目标：验证链路是否通。

并发：1
持续：30 秒

2. Load Test

目标：验证正常负载能力。

并发：50 / 100 / 200
持续：10-20 分钟

3. Stress Test

目标：找到系统瓶颈。

并发：100 / 200 / 500 / 1000

4. Streaming Test

目标：验证 SSE 长连接能力。

并发连接：100 / 500 / 1000 / 3000

建议 mock 参数：

```text
ttft_ms=300
chunk_delay_ms=50
completion_tokens=200
```

5. Fault Injection

目标：验证异常处理、重试、计费、日志和熔断策略。

建议参数：

```text
error_rate=0.01&error_status=429
error_rate=0.02&error_status=500
timeout_rate=0.01&timeout_ms=30000
```

6. Soak Test

目标：长稳测试。

并发：目标生产并发的 50%-70%
持续：1-6 小时

## 压测时必须观察的指标

new-api：

- QPS
- P50 / P95 / P99
- 错误率
- HTTP 429 / 500 / 502 / 504
- SSE 连接数
- 请求日志写入耗时
- 额度扣减是否正确
- 失败请求是否重复扣费

系统：

- CPU
- 内存
- goroutine / thread 数
- 文件描述符 fd
- 网络连接数
- MySQL 连接池
- Redis 连接池
- 慢 SQL
- 日志表增长

性能分析：

- pprof CPU profile
- pprof heap profile
- pprof goroutine profile
- Pyroscope 火焰图
- mutex / block profile

## new-api 性能分析配置

new-api 支持 pprof 和 Pyroscope：

- pprof 用于临时诊断和离线分析。
- Pyroscope 用于持续 profiling 和火焰图可视化。

docker compose 部署 new-api 时建议增加：

```yaml
environment:
  - ENABLE_PPROF=true
  - PYROSCOPE_URL=http://pyroscope:4040
  - PYROSCOPE_APP_NAME=new-api
  - PYROSCOPE_MUTEX_RATE=5
  - PYROSCOPE_BLOCK_RATE=5
  - HOSTNAME=new-api-local
```

说明：

| 变量 | 说明 |
| --- | --- |
| `ENABLE_PPROF=true` | 启用 `/debug/pprof/` |
| `PYROSCOPE_URL` | 配置 Pyroscope 地址 |
| `PYROSCOPE_APP_NAME` | 区分应用 |
| `HOSTNAME` | 区分不同实例 |

本工程提供了可选 Pyroscope 服务：

```bash
docker compose --profile profiling up pyroscope
```

也可以将以下服务片段加入你的 new-api compose：

```yaml
pyroscope:
  image: grafana/pyroscope:latest
  ports:
    - "4040:4040"
```

pprof 常用命令示例：

```bash
go tool pprof http://localhost:3000/debug/pprof/profile?seconds=30
go tool pprof http://localhost:3000/debug/pprof/heap
curl -s http://localhost:3000/debug/pprof/goroutine?debug=2
```

## 验收清单

- `docker compose up` 可以启动 mock-provider
- `GET /healthz` 返回 200
- `GET /v1/models` 返回 `mock-gpt`
- `POST /v1/chat/completions stream=false` 可用
- `POST /v1/chat/completions stream=true` 可用
- `delay_ms` 生效
- `ttft_ms` 生效
- `chunk_delay_ms` 生效
- `error_rate` 生效
- `timeout_rate` 生效
- `k6/smoke.js` 可运行
- `k6/chat-non-stream.js` 可运行
- `k6/chat-stream.js` 可运行
- README 说明 new-api 接入方式
- README 说明 pprof / Pyroscope 配置方式
- README 说明完整压测流程
