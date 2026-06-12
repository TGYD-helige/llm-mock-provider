package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	responseHeader time.Duration
	ttft           time.Duration
	total          time.Duration
	err            string
	status         int
}

func main() {
	url := flag.String("url", "http://127.0.0.1:3001/v1/chat/completions?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200", "request URL")
	model := flag.String("model", "mock-gpt", "model")
	concurrency := flag.Int("concurrency", 10, "worker count")
	duration := flag.Duration("duration", 2*time.Minute, "test duration")
	sleep := flag.Duration("sleep", 100*time.Millisecond, "sleep after each request")
	timeout := flag.Duration("timeout", 2*time.Minute, "per-request timeout")
	disableKeepAlives := flag.Bool("disable-keepalive", false, "disable HTTP keep-alive")
	flag.Parse()

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "dummy-key"
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          *concurrency * 2,
		MaxIdleConnsPerHost:   *concurrency * 2,
		MaxConnsPerHost:       *concurrency * 2,
		DisableKeepAlives:     *disableKeepAlives,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	results := make(chan result, *concurrency*128)
	var wg sync.WaitGroup
	var started atomic.Int64

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				started.Add(1)
				results <- doRequest(context.Background(), client, *url, apiKey, *model, *timeout)
				select {
				case <-ctx.Done():
					return
				case <-time.After(*sleep):
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var total, ok, failed int
	var responseHeaders, ttfts, totals []float64
	errs := map[string]int{}
	for r := range results {
		total++
		if r.err == "" && r.status == http.StatusOK {
			ok++
			responseHeaders = append(responseHeaders, milliseconds(r.responseHeader))
			ttfts = append(ttfts, milliseconds(r.ttft))
			totals = append(totals, milliseconds(r.total))
		} else {
			failed++
			key := r.err
			if key == "" {
				key = fmt.Sprintf("status_%d", r.status)
			}
			errs[key]++
		}
	}

	fmt.Printf("concurrency=%d duration=%s started=%d total=%d ok=%d failed=%d fail_rate=%.2f%%\n",
		*concurrency, duration.String(), started.Load(), total, ok, failed, percent(failed, total))
	printTrend("response_header_ms", responseHeaders)
	printTrend("ttft_ms", ttfts)
	printTrend("stream_total_ms", totals)
	printErrors(errs)
}

func doRequest(parent context.Context, client *http.Client, url, apiKey, model string, timeout time.Duration) result {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	payload := map[string]any{
		"model":    model,
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return result{err: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return result{total: time.Since(start), err: err.Error()}
	}
	defer resp.Body.Close()

	r := result{
		responseHeader: time.Since(start),
		status:         resp.StatusCode,
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		r.total = time.Since(start)
		return r
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	foundData := false
	foundDone := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		if !foundData {
			r.ttft = time.Since(start)
			foundData = true
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "data:")) == "[DONE]" {
			foundDone = true
			break
		}
	}
	r.total = time.Since(start)
	if err := scanner.Err(); err != nil {
		r.err = err.Error()
	} else if !foundData {
		r.err = "missing_sse_data"
	} else if !foundDone {
		r.err = "missing_sse_done"
	}
	return r
}

func printTrend(name string, xs []float64) {
	sort.Float64s(xs)
	fmt.Printf("%s avg=%.2f p50=%.2f p90=%.2f p95=%.2f max=%.2f\n",
		name, avg(xs), percentile(xs, 50), percentile(xs, 90), percentile(xs, 95), max(xs))
}

func printErrors(errs map[string]int) {
	if len(errs) == 0 {
		return
	}
	fmt.Println("errors:")
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, errs[k])
	}
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func percent(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

func avg(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	idx := int((p / 100) * float64(len(xs)-1))
	return xs[idx]
}

func max(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	return xs[len(xs)-1]
}
