# Stream Probe

`stream-probe` measures streaming response headers, true time to first SSE data chunk, and full stream duration separately.

It reads the response body incrementally instead of waiting for the complete body, making it useful for detecting gateway queueing and response buffering.

```bash
API_KEY=your-key go run ./tools/stream-probe/main.go \
  -url 'http://localhost:3001/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200' \
  -concurrency 20 \
  -duration 2m
```

Output metrics:

- `response_header_ms`: request start to HTTP response headers.
- `ttft_ms`: request start to the first SSE `data:` line.
- `stream_total_ms`: request start to the SSE `[DONE]` event.

When the configured upstream `ttft_ms` is known, a rising `ttft_ms` p95 or an increasing gap between observed and configured TTFT indicates queueing or buffering in the gateway path.
