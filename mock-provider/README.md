# mock-provider

这是一个只使用 Go 标准库 `net/http` 实现的 OpenAI-compatible mock provider，用于在不调用真实大模型的情况下测试任意大模型网关、API 路由、计费层、重试逻辑和流式连接能力。

默认监听端口：`3001`

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORT` | `3001` | 服务监听端口 |
| `DEFAULT_MODEL` | `mock-gpt` | `/v1/models` 返回的模型名 |

## 接口

- `GET /healthz`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/embeddings`
- `GET /scenario/{scenario}/v1/models`
- `POST /scenario/{scenario}/v1/chat/completions`
- `POST /scenario/{scenario}/v1/embeddings`

## Mock 控制参数

`/v1/chat/completions` 和 `/v1/embeddings` 支持以下 query 参数：

| 参数 | 说明 |
| --- | --- |
| `delay_ms` | 非流式整体延迟 |
| `ttft_ms` | 流式首 token 延迟 |
| `chunk_delay_ms` | 流式 chunk 间隔 |
| `prompt_tokens` | 模拟输入 token 数 |
| `completion_tokens` | 模拟输出 token 数；流式模式下会控制 content chunk 数 |
| `error_rate` | 随机错误概率，例如 `0.05` 表示 5% |
| `error_status` | 错误状态码，默认 `500` |
| `timeout_rate` | 随机超时概率 |
| `timeout_ms` | 超时等待时间，默认 `30000` |

## Scenario 路径

同一个 mock-provider 实例可以通过 path 区分不同渠道行为，适合测试网关的渠道故障转移能力。比如在网关里将不同渠道配置为：

| 场景 | Base URL |
| --- | --- |
| 健康渠道 | `http://mock-provider:3001/scenario/healthy/v1` |
| 随机 500 | `http://mock-provider:3001/scenario/flaky-500/v1` |
| 随机 429 | `http://mock-provider:3001/scenario/flaky-429/v1` |
| 随机超时 | `http://mock-provider:3001/scenario/timeout/v1` |
| 慢首 token | `http://mock-provider:3001/scenario/slow-ttft/v1` |

支持的内置场景：

| 场景 | 行为 |
| --- | --- |
| `healthy` | 不注入额外故障 |
| `flaky-500` | 20% 请求返回 500 |
| `flaky-429` | 20% 请求返回 429 |
| `timeout` | 5% 请求等待 30s 后返回 504 |
| `slow-ttft` | 流式 TTFT 固定为 2000ms；非流式整体延迟 2000ms |
| `always-500` | 100% 请求返回 500，适合 smoke/debug |
| `always-429` | 100% 请求返回 429，适合 smoke/debug |

场景会在 query 参数解析后应用，因此它可以稳定表达“渠道级行为”。例如，即使请求里带了 `error_rate=0`，`/scenario/always-429/v1/chat/completions` 仍会返回 429。

示例：

```bash
curl -s http://localhost:3001/healthz
curl -s http://localhost:3001/v1/models

curl -s http://localhost:3001/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":false}'

curl -N http://localhost:3001/v1/chat/completions?ttft_ms=300\&chunk_delay_ms=50\&completion_tokens=10 \
  -H 'Content-Type: application/json' \
  -d '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":true}'

curl -s http://localhost:3001/scenario/always-500/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":false}'
```

## 本地运行

```bash
go run .
```

## Docker 运行

```bash
docker build -t mock-provider .
docker run --rm -p 3001:3001 mock-provider
```
