package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScenarioChatCompletionsHealthy(t *testing.T) {
	s := &server{model: defaultModel}
	req := httptest.NewRequest(http.MethodPost, "/scenario/healthy/v1/chat/completions?completion_tokens=3", strings.NewReader(`{
		"model":"mock-gpt",
		"messages":[{"role":"user","content":"hello"}],
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.withCORS(s.handleScenario)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Mock-Scenario"); got != "healthy" {
		t.Fatalf("expected X-Mock-Scenario=healthy, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"chat.completion"`) {
		t.Fatalf("expected chat completion body, got %s", rec.Body.String())
	}
}

func TestScenarioOverridesQueryFaults(t *testing.T) {
	s := &server{model: defaultModel}
	req := httptest.NewRequest(http.MethodPost, "/scenario/always-429/v1/chat/completions?error_rate=0", strings.NewReader(`{
		"model":"mock-gpt",
		"messages":[{"role":"user","content":"hello"}],
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.withCORS(s.handleScenario)(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"mock upstream error"`) {
		t.Fatalf("expected mock error body, got %s", rec.Body.String())
	}
}

func TestScenarioModels(t *testing.T) {
	s := &server{model: defaultModel}
	req := httptest.NewRequest(http.MethodGet, "/scenario/healthy/v1/models", nil)
	rec := httptest.NewRecorder()

	s.withCORS(s.handleScenario)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), defaultModel) {
		t.Fatalf("expected model %q, got %s", defaultModel, rec.Body.String())
	}
}

func TestScenarioRejectsUnknownRoute(t *testing.T) {
	s := &server{model: defaultModel}
	req := httptest.NewRequest(http.MethodGet, "/scenario/healthy/healthz", nil)
	rec := httptest.NewRecorder()

	s.withCORS(s.handleScenario)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestScenarioRejectsUnknownName(t *testing.T) {
	s := &server{model: defaultModel}
	req := httptest.NewRequest(http.MethodGet, "/scenario/typo/v1/models", nil)
	rec := httptest.NewRecorder()

	s.withCORS(s.handleScenario)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
