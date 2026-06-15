package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleChatRejectsNonPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat", nil)
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleChatRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleChatRejectsMissingMessage(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":""}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleChatReturnsPlaceholderTextAndLatency(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"hello"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var res chatResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("expected valid json response, got %v", err)
	}

	if res.Text == "" {
		t.Fatal("expected placeholder text")
	}

	if res.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", res.LatencyMS)
	}
}
