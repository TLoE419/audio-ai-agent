package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterLogsRequestLatency(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	got := logs.String()
	if !strings.Contains(got, `"msg":"request completed"`) {
		t.Fatalf("expected request completed log, got %q", got)
	}

	if !strings.Contains(got, `"method":"GET"`) {
		t.Fatalf("expected method in log, got %q", got)
	}

	if !strings.Contains(got, `"path":"/healthz"`) {
		t.Fatalf("expected path in log, got %q", got)
	}

	if !strings.Contains(got, `"status":200`) {
		t.Fatalf("expected status in log, got %q", got)
	}

	if !strings.Contains(got, `"latency_ms":`) {
		t.Fatalf("expected latency_ms in log, got %q", got)
	}
}

func TestRouterAllowsLocalFrontendCORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/v1/avatar", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected CORS origin header, got %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
