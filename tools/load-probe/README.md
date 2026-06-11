# load-probe

`load-probe` is a small Go-based load probe for quick comparisons against k6 results.

It sends OpenAI-compatible non-streaming chat completion requests with a fixed number of workers. It is useful when you want to rule out tool/runtime-specific behavior, such as Docker networking or k6 runner differences.

## Usage

```bash
cd tools/load-probe

API_KEY=dummy-key go run . \
  -url=http://localhost:3001/v1/chat/completions \
  -model=mock-gpt \
  -concurrency=50 \
  -duration=2m \
  -sleep=1s \
  -timeout=30s
```

Disable HTTP keep-alive to create a new connection for each request:

```bash
cd tools/load-probe

API_KEY=dummy-key go run . \
  -url=http://localhost:3001/v1/chat/completions \
  -concurrency=50 \
  -duration=1m \
  -disable-keepalive
```

The tool prints total requests, success/failure counts, failure rate, latency percentiles, and grouped error messages.
