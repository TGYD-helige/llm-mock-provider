package main

import (
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
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	latency time.Duration
	err     string
	status  int
}

func main() {
	url := flag.String("url", "http://127.0.0.1:3001/v1/chat/completions", "request URL")
	model := flag.String("model", "mock-gpt", "model")
	concurrency := flag.Int("concurrency", 10, "worker count")
	duration := flag.Duration("duration", 2*time.Minute, "test duration")
	sleep := flag.Duration("sleep", time.Second, "sleep after each request")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
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
	client := &http.Client{Transport: transport, Timeout: *timeout}

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
				results <- doRequest(ctx, client, *url, apiKey, *model)
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
	var latencies []float64
	errs := map[string]int{}
	for r := range results {
		total++
		if r.err == "" && r.status == http.StatusOK {
			ok++
			latencies = append(latencies, float64(r.latency.Milliseconds()))
		} else {
			failed++
			key := r.err
			if key == "" {
				key = fmt.Sprintf("status_%d", r.status)
			}
			errs[key]++
		}
	}

	sort.Float64s(latencies)
	fmt.Printf("concurrency=%d duration=%s started=%d total=%d ok=%d failed=%d fail_rate=%.2f%%\n",
		*concurrency, duration.String(), started.Load(), total, ok, failed, percent(failed, total))
	fmt.Printf("latency_ms avg=%.2f p50=%.2f p90=%.2f p95=%.2f max=%.2f\n",
		avg(latencies), percentile(latencies, 50), percentile(latencies, 90), percentile(latencies, 95), max(latencies))
	if len(errs) > 0 {
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
}

func doRequest(ctx context.Context, client *http.Client, url, apiKey, model string) result {
	payload := map[string]any{
		"model":    model,
		"stream":   false,
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return result{err: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return result{latency: time.Since(start), err: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return result{latency: time.Since(start), status: resp.StatusCode}
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
