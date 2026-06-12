package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort  = "3001"
	defaultModel = "mock-gpt"
)

type contextKey string

const scenarioContextKey contextKey = "scenario"

type server struct {
	model string
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type faultConfig struct {
	ErrorRate   float64
	ErrorStatus int
	TimeoutRate float64
	TimeoutMS   int
}

type chatConfig struct {
	DelayMS          int
	TTFTMS           int
	ChunkDelayMS     int
	PromptTokens     int
	CompletionTokens int
	Fault            faultConfig
}

func main() {
	port := getenv("PORT", defaultPort)
	s := &server{model: getenv("DEFAULT_MODEL", defaultModel)}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.withCORS(s.handleHealthz))
	mux.HandleFunc("/v1/models", s.withCORS(s.handleModels))
	mux.HandleFunc("/v1/chat/completions", s.withCORS(s.handleChatCompletions))
	mux.HandleFunc("/v1/embeddings", s.withCORS(s.handleEmbeddings))
	mux.HandleFunc("/scenario/", s.withCORS(s.handleScenario))

	addr := ":" + strings.TrimPrefix(port, ":")
	log.Printf("mock provider listening on %s, default model=%s", addr, s.model)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) handleScenario(w http.ResponseWriter, r *http.Request) {
	scenario, strippedPath, ok := parseScenarioPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown scenario route")
		return
	}
	scenario, ok = normalizeScenario(scenario)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown scenario")
		return
	}

	w.Header().Set("X-Mock-Scenario", scenario)

	scenarioRequest := r.Clone(context.WithValue(r.Context(), scenarioContextKey, scenario))
	scenarioRequest.URL.Path = strippedPath

	switch strippedPath {
	case "/v1/models":
		s.handleModels(w, scenarioRequest)
	case "/v1/chat/completions":
		s.handleChatCompletions(w, scenarioRequest)
	case "/v1/embeddings":
		s.handleEmbeddings(w, scenarioRequest)
	default:
		writeError(w, http.StatusNotFound, "unknown scenario route")
	}
}

func (s *server) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]string{
			{
				"id":     s.model,
				"object": "model",
			},
		},
	})
}

func (s *server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg := parseChatConfig(r)
	if r.URL.Query().Get("prompt_tokens") == "" {
		cfg.PromptTokens = 10
	}

	if handleFault(w, r, cfg.Fault) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.1, 0.2, 0.3},
			},
		},
		"model": s.model,
		"usage": usage{
			PromptTokens: cfg.PromptTokens,
			TotalTokens:  cfg.PromptTokens,
		},
	})
}

func (s *server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	cfg := parseChatConfig(r)
	if handleFault(w, r, cfg.Fault) {
		return
	}

	model := req.Model
	if model == "" {
		model = s.model
	}

	if req.Stream {
		s.handleChatStream(w, r, model, cfg)
		return
	}

	if !sleepOrCancel(r.Context(), time.Duration(cfg.DelayMS)*time.Millisecond) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "mock response",
				},
				"finish_reason": "stop",
			},
		},
		"usage": usage{
			PromptTokens:     cfg.PromptTokens,
			CompletionTokens: cfg.CompletionTokens,
			TotalTokens:      cfg.PromptTokens + cfg.CompletionTokens,
		},
	})
}

func (s *server) handleChatStream(w http.ResponseWriter, r *http.Request, model string, cfg chatConfig) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if !sleepOrCancel(r.Context(), time.Duration(cfg.TTFTMS)*time.Millisecond) {
		return
	}

	if !writeSSE(w, flusher, streamChunk(model, map[string]string{"role": "assistant"}, nil)) {
		return
	}

	for i := 0; i < cfg.CompletionTokens; i++ {
		if cfg.ChunkDelayMS > 0 && !sleepOrCancel(r.Context(), time.Duration(cfg.ChunkDelayMS)*time.Millisecond) {
			return
		}

		token := "mock "
		if i == cfg.CompletionTokens-1 {
			token = "response"
		}

		if !writeSSE(w, flusher, streamChunk(model, map[string]string{"content": token}, nil)) {
			return
		}
	}

	stop := "stop"
	if !writeSSE(w, flusher, streamChunk(model, map[string]string{}, &stop)) {
		return
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func streamChunk(model string, delta map[string]string, finishReason *string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
			},
		},
	}
}

func parseChatConfig(r *http.Request) chatConfig {
	q := r.URL.Query()
	promptTokens := intParam(q.Get("prompt_tokens"), 100)
	completionTokens := intParam(q.Get("completion_tokens"), 50)
	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}

	cfg := chatConfig{
		DelayMS:          nonNegativeInt(q.Get("delay_ms"), 0),
		TTFTMS:           nonNegativeInt(q.Get("ttft_ms"), 0),
		ChunkDelayMS:     nonNegativeInt(q.Get("chunk_delay_ms"), 0),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Fault: faultConfig{
			ErrorRate:   rateParam(q.Get("error_rate"), 0),
			ErrorStatus: statusParam(q.Get("error_status"), http.StatusInternalServerError),
			TimeoutRate: rateParam(q.Get("timeout_rate"), 0),
			TimeoutMS:   nonNegativeInt(q.Get("timeout_ms"), 30000),
		},
	}

	applyScenario(&cfg, scenarioFromContext(r.Context()))
	return cfg
}

func parseScenarioPath(path string) (string, string, bool) {
	const prefix = "/scenario/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(path, prefix)
	scenario, strippedPath, ok := strings.Cut(rest, "/")
	if !ok || scenario == "" || strippedPath == "" {
		return "", "", false
	}

	strippedPath = "/" + strippedPath
	if !strings.HasPrefix(strippedPath, "/v1/") {
		return "", "", false
	}

	return scenario, strippedPath, true
}

func scenarioFromContext(ctx context.Context) string {
	scenario, _ := ctx.Value(scenarioContextKey).(string)
	return scenario
}

func normalizeScenario(scenario string) (string, bool) {
	scenario = strings.ToLower(strings.TrimSpace(scenario))
	switch scenario {
	case "healthy", "flaky-500", "flaky-429", "timeout", "slow-ttft", "always-500", "always-429":
		return scenario, true
	default:
		return "", false
	}
}

func applyScenario(cfg *chatConfig, scenario string) {
	switch scenario {
	case "", "healthy":
		return
	case "flaky-500":
		cfg.Fault.ErrorRate = 0.20
		cfg.Fault.ErrorStatus = http.StatusInternalServerError
	case "flaky-429":
		cfg.Fault.ErrorRate = 0.20
		cfg.Fault.ErrorStatus = http.StatusTooManyRequests
	case "timeout":
		cfg.Fault.TimeoutRate = 0.05
		cfg.Fault.TimeoutMS = 30000
	case "slow-ttft":
		cfg.TTFTMS = 2000
		cfg.DelayMS = 2000
	case "always-500":
		cfg.Fault.ErrorRate = 1
		cfg.Fault.ErrorStatus = http.StatusInternalServerError
	case "always-429":
		cfg.Fault.ErrorRate = 1
		cfg.Fault.ErrorStatus = http.StatusTooManyRequests
	}
}

func handleFault(w http.ResponseWriter, r *http.Request, cfg faultConfig) bool {
	if shouldHit(cfg.TimeoutRate) {
		sleepOrCancel(r.Context(), time.Duration(cfg.TimeoutMS)*time.Millisecond)
		writeError(w, http.StatusGatewayTimeout, "mock upstream timeout")
		return true
	}

	if shouldHit(cfg.ErrorRate) {
		writeError(w, cfg.ErrorStatus, "mock upstream error")
		return true
	}

	return false
}

func shouldHit(rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return rand.Float64() < rate
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, payload any) bool {
	b, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func sleepOrCancel(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}

	payload := errorResponse{}
	payload.Error.Message = message
	payload.Error.Type = "mock_error"
	payload.Error.Code = "mock_error"
	writeJSON(w, status, payload)
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intParam(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func nonNegativeInt(raw string, fallback int) int {
	v := intParam(raw, fallback)
	if v < 0 {
		return 0
	}
	return v
}

func rateParam(raw string, fallback float64) float64 {
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func statusParam(raw string, fallback int) int {
	status := intParam(raw, fallback)
	if status < 400 || status > 599 {
		return fallback
	}
	return status
}
