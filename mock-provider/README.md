# mock-provider

这是一个只使用 Go 标准库 `net/http` 实现的 OpenAI-compatible mock provider，用于在不调用真实大模型的情况下给 new-api 做网关压测。

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
