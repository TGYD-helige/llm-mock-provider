# llm-mock-provider

`llm-mock-provider` is an OpenAI-compatible mock provider for testing LLM gateways, API routers, billing layers, retry logic, streaming behavior, and observability pipelines without calling a real model provider.

It can be used as a lightweight upstream for any OpenAI-compatible gateway:

```text
k6 / client / gateway test runner
  -> your LLM gateway
  -> llm-mock-provider
```

You can also call `llm-mock-provider` directly when validating mock behavior.

## Security

This is a public project. Do not commit real API keys, cloud credentials, kubeconfigs, tokens, production URLs, or private deployment values.

The examples use placeholder values such as `dummy-key`, `localhost`, and `mock-gpt`. Replace them only in your private deployment environment or secret manager.

## Features

- OpenAI-compatible chat completions endpoint
- OpenAI-compatible embeddings endpoint
- Non-streaming and SSE streaming responses
- Configurable latency, TTFT, chunk delay, token counts, error rates, and timeout simulation
- k6 scripts for smoke, load, streaming, stress, and soak tests
- Docker and docker compose support
- No OpenAI SDK dependency
- Go standard library HTTP server, no Gin/Echo/Fiber

## Project Structure

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

Kubernetes manifests are intentionally not included here because deployment topology is gateway-specific. Keep cluster credentials and production values in a private infrastructure repository.

## Quick Start

```bash
docker compose up --build
```

The mock provider listens on:

```text
http://localhost:3001
```

Verify:

```bash
curl -sS http://localhost:3001/healthz
curl -sS http://localhost:3001/v1/models
```

## API

### `GET /healthz`

```json
{"status":"ok"}
```

### `GET /v1/models`

The model name comes from `DEFAULT_MODEL`; the default is `mock-gpt`.

### `POST /v1/chat/completions`

Supports both `stream=false` and `stream=true`.

Non-streaming:

```bash
curl --request POST \
  --url 'http://localhost:3001/v1/chat/completions' \
  --header 'Content-Type: application/json' \
  --data '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":false}'
```

Streaming:

```bash
curl --no-buffer --request POST \
  --url 'http://localhost:3001/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=10' \
  --header 'Content-Type: application/json' \
  --data '{"model":"mock-gpt","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

### `POST /v1/embeddings`

Returns a fixed OpenAI-compatible embedding response. It does not call a real model.

## Mock Controls

Control behavior with query parameters:

| Parameter | Description | Example |
| --- | --- | --- |
| `delay_ms` | Whole-response delay for non-streaming requests | `delay_ms=1000` |
| `ttft_ms` | Time to first token for streaming requests | `ttft_ms=300` |
| `chunk_delay_ms` | Delay between streaming chunks | `chunk_delay_ms=50` |
| `prompt_tokens` | Simulated input tokens | `prompt_tokens=100` |
| `completion_tokens` | Simulated output tokens; also controls streaming content chunk count | `completion_tokens=200` |
| `error_rate` | Random error probability, from `0` to `1` | `error_rate=0.05` |
| `error_status` | Error status code, default `500` | `error_status=429` |
| `timeout_rate` | Random timeout probability, from `0` to `1` | `timeout_rate=0.01` |
| `timeout_ms` | Timeout sleep duration, default `30000` | `timeout_ms=30000` |

Examples:

```text
/v1/chat/completions?delay_ms=1000
/v1/chat/completions?error_rate=0.05&error_status=429
/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200
/v1/chat/completions?timeout_rate=0.01&timeout_ms=30000
```

Error response:

```json
{
  "error": {
    "message": "mock upstream error",
    "type": "mock_error",
    "code": "mock_error"
  }
}
```

## Gateway Integration

For an OpenAI-compatible gateway, configure a provider/channel/upstream like this:

| Field | Value |
| --- | --- |
| Provider type | `OpenAI-compatible` or `OpenAI` |
| Base URL | `http://mock-provider:3001/v1` or `http://localhost:3001/v1` |
| API key | `dummy-key` |
| Model | `mock-gpt` |

If the gateway and mock provider run in the same docker compose network:

```text
http://mock-provider:3001/v1
```

If the gateway runs on the host:

```text
http://localhost:3001/v1
```

## k6 Scripts

All scripts support:

| Environment variable | Default | Description |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:3001` | Target gateway or mock provider URL |
| `API_KEY` | `dummy-key` | Placeholder bearer token |
| `MODEL` | `mock-gpt` | Model name |
| `QUERY` | Script-specific default | Query string appended to `/v1/chat/completions` |

Smoke test:

```bash
k6 run k6/smoke.js
```

Run through a gateway:

```bash
BASE_URL=http://localhost:3000 API_KEY=dummy-key MODEL=mock-gpt k6 run k6/smoke.js
```

Non-streaming load test:

```bash
k6 run k6/chat-non-stream.js
```

Simulate upstream latency:

```bash
QUERY='?delay_ms=1000' k6 run k6/chat-non-stream.js
```

Streaming test:

```bash
k6 run k6/chat-stream.js
```

The default streaming query is:

```text
?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200
```

Stress test:

```bash
k6 run k6/stress.js
```

Soak test:

```bash
k6 run k6/soak.js
```

## Suggested Test Flow

1. Smoke test

Goal: verify basic connectivity.

Suggested load: 1 VU for 30 seconds.

2. Load test

Goal: understand normal gateway capacity.

Suggested load: 50 / 100 / 200 VUs for 10-20 minutes.

3. Stress test

Goal: find gateway bottlenecks.

Suggested load: 100 / 200 / 500 / 1000 VUs.

4. Streaming test

Goal: validate SSE long-lived connections and buffering behavior.

Suggested load: 100 / 500 / 1000 / 3000 concurrent connections.

Suggested mock parameters:

```text
ttft_ms=300
chunk_delay_ms=50
completion_tokens=200
```

5. Fault injection

Goal: validate retry behavior, billing correctness, logs, error handling, and circuit breaking.

Suggested mock parameters:

```text
error_rate=0.01&error_status=429
error_rate=0.02&error_status=500
timeout_rate=0.01&timeout_ms=30000
```

6. Soak test

Goal: validate long-running stability.

Suggested load: 50%-70% of expected production concurrency for 1-6 hours.

## Metrics to Watch

Gateway:

- QPS
- P50 / P95 / P99 latency
- Error rate
- HTTP 429 / 500 / 502 / 504
- Active SSE connections
- Request log write latency
- Quota or billing deduction correctness
- Whether failed requests are billed repeatedly
- Retry amplification

System:

- CPU
- Memory
- Goroutine / thread count
- File descriptors
- Network connections
- Database connection pool
- Cache connection pool
- Slow queries
- Log table or log sink growth

Profiling:

- CPU profile
- Heap profile
- Goroutine profile
- Flame graphs
- Mutex / block profile

## Roadmap

- Long-lived connection profiles for streaming tests, including configurable idle periods and heartbeat chunks
- More SSE patterns, such as role-only chunks, empty deltas, tool-call chunks, abrupt disconnects, and malformed frames
- Richer error scenarios: 401, 403, 408, 409, 429, 500, 502, 503, 504, and provider-specific error bodies
- Retry-oriented scenarios, including partial failures, slow-first-attempt then success, and deterministic failure sequences
- Request-size controls for large prompts, long message histories, and large tool/function call payloads
- Token accounting modes for validating gateway quota and billing behavior
- Per-route controls for embeddings, chat, model listing, and future OpenAI-compatible endpoints
- Metrics endpoint for mock-provider-side request counts, latency buckets, active streams, and injected failures
- Config-file based scenarios in addition to query-string controls
- Optional response templates for custom provider shapes while keeping OpenAI-compatible defaults

## Acceptance Checklist

- `docker compose up` starts the mock provider
- `GET /healthz` returns 200
- `GET /v1/models` returns `mock-gpt`
- `POST /v1/chat/completions` works with `stream=false`
- `POST /v1/chat/completions` works with `stream=true`
- `delay_ms` works
- `ttft_ms` works
- `chunk_delay_ms` works
- `error_rate` works
- `timeout_rate` works
- k6 smoke script runs
- k6 non-streaming script runs
- k6 streaming script runs
