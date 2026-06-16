package main

import (
	"encoding/json"
	"io"
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
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

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

func TestHandleChatUsesConfiguredAnthropicProvider(t *testing.T) {
	previousHTTPClient := anthropicHTTPClient
	t.Cleanup(func() {
		anthropicHTTPClient = previousHTTPClient
	})

	anthropicHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://anthropic.test/v1/messages" {
				t.Fatalf("expected messages url, got %s", r.URL.String())
			}

			if r.Method != http.MethodPost {
				t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
			}

			if got := r.Header.Get("x-api-key"); got != "test-anthropic-key" {
				t.Fatalf("expected x-api-key header, got %q", got)
			}

			if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
				t.Fatalf("expected anthropic-version header, got %q", got)
			}

			var req anthropicMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("expected valid provider request, got %v", err)
			}

			if req.Model != "test-claude-model" {
				t.Fatalf("expected model test-claude-model, got %q", req.Model)
			}

			if req.MaxTokens != 128 {
				t.Fatalf("expected max tokens 128, got %d", req.MaxTokens)
			}

			if len(req.Messages) != 1 {
				t.Fatalf("expected 1 message, got %d", len(req.Messages))
			}

			if req.Messages[0].Role != "user" {
				t.Fatalf("expected user role, got %q", req.Messages[0].Role)
			}

			if req.Messages[0].Content != "hello" {
				t.Fatalf("expected content hello, got %q", req.Messages[0].Content)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"content": [
						{
							"type": "text",
							"text": "hi from claude"
						}
					]
				}`)),
			}, nil
		}),
	}

	t.Setenv("LLM_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("ANTHROPIC_MODEL", "test-claude-model")
	t.Setenv("ANTHROPIC_BASE_URL", "https://anthropic.test")
	t.Setenv("ANTHROPIC_VERSION", "2023-06-01")
	t.Setenv("ANTHROPIC_MAX_TOKENS", "128")
	t.Setenv("OPENAI_API_KEY", "")

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

	if res.Text != "hi from claude" {
		t.Fatalf("expected provider text, got %q", res.Text)
	}

	if res.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", res.LatencyMS)
	}
}

func TestHandleChatUsesConfiguredLLMProvider(t *testing.T) {
	previousHTTPClient := openAIHTTPClient
	t.Cleanup(func() {
		openAIHTTPClient = previousHTTPClient
	})

	openAIHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://example.test/v1/responses" {
				t.Fatalf("expected responses url, got %s", r.URL.String())
			}

			if r.Method != http.MethodPost {
				t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
			}

			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("expected authorization header, got %q", got)
			}

			var req openAIResponseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("expected valid provider request, got %v", err)
			}

			if req.Model != "test-model" {
				t.Fatalf("expected model test-model, got %q", req.Model)
			}

			if req.Input != "hello" {
				t.Fatalf("expected input hello, got %q", req.Input)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"output": [
						{
							"type": "message",
							"content": [
								{
									"type": "output_text",
									"text": "hi from provider"
								}
							]
						}
					]
				}`)),
			}, nil
		}),
	}

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_BASE_URL", "https://example.test/v1")

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

	if res.Text != "hi from provider" {
		t.Fatalf("expected provider text, got %q", res.Text)
	}

	if res.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", res.LatencyMS)
	}
}

func TestHandleTTSRejectsNonPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/tts", nil)
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleTTSRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/tts", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleTTSRejectsMissingText(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/tts", strings.NewReader(`{"text":""}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleTTSReturnsNotImplementedUntilProviderIsConnected(t *testing.T) {
	t.Setenv("BOSON_API_KEY", "")

	req := httptest.NewRequest(http.MethodPost, "/v1/tts", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}

	if got := rec.Body.String(); !strings.Contains(got, "tts provider not connected yet") {
		t.Fatalf("expected provider placeholder error, got %q", got)
	}
}

func TestHandleTTSUsesConfiguredBosonProvider(t *testing.T) {
	previousHTTPClient := bosonTTSHTTPClient
	t.Cleanup(func() {
		bosonTTSHTTPClient = previousHTTPClient
	})

	bosonTTSHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://boson.test/v1/audio/speech" {
				t.Fatalf("expected speech url, got %s", r.URL.String())
			}

			if r.Method != http.MethodPost {
				t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
			}

			if got := r.Header.Get("Authorization"); got != "Bearer test-boson-key" {
				t.Fatalf("expected authorization header, got %q", got)
			}

			var req bosonTTSRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("expected valid provider request, got %v", err)
			}

			if req.Model != "higgs-audio-v3-tts" {
				t.Fatalf("expected model higgs-audio-v3-tts, got %q", req.Model)
			}

			if req.Input != "hello" {
				t.Fatalf("expected input hello, got %q", req.Input)
			}

			if req.Voice != "jake" {
				t.Fatalf("expected voice jake, got %q", req.Voice)
			}

			if req.ResponseFormat != "mp3" {
				t.Fatalf("expected response format mp3, got %q", req.ResponseFormat)
			}

			if req.Stream {
				t.Fatal("expected non-streaming speech request")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"audio/mpeg"},
				},
				Body: io.NopCloser(strings.NewReader("ID3fakemp3")),
			}, nil
		}),
	}

	t.Setenv("BOSON_API_KEY", "test-boson-key")
	t.Setenv("BOSON_BASE_URL", "https://boson.test")
	t.Setenv("BOSON_TTS_VOICE", "jake")

	req := httptest.NewRequest(http.MethodPost, "/v1/tts", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Fatalf("expected audio/mpeg content type, got %q", got)
	}

	if got := rec.Header().Get("X-Latency-Ms"); got == "" {
		t.Fatal("expected X-Latency-Ms header")
	}

	if got := rec.Body.String(); got != "ID3fakemp3" {
		t.Fatalf("expected audio bytes, got %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
