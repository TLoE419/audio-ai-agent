package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
