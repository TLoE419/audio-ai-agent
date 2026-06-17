package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleAvatarRejectsNonPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/avatar", nil)
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleAvatarRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/avatar", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleAvatarRejectsMissingText(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/avatar", strings.NewReader(`{"text":""}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleAvatarReturnsNotImplementedUntilTTSProviderIsConnected(t *testing.T) {
	t.Setenv("BOSON_API_KEY", "")

	req := httptest.NewRequest(http.MethodPost, "/v1/avatar", strings.NewReader(`{"text":"hello"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, rec.Code)
	}

	if got := rec.Body.String(); !strings.Contains(got, "tts provider not connected yet") {
		t.Fatalf("expected provider placeholder error, got %q", got)
	}
}

func TestHandleAvatarUsesConfiguredBosonProvider(t *testing.T) {
	previousHTTPClient := bosonTTSHTTPClient
	previousConverter := convertAudioTo16kMonoWAV
	previousGenerator := generateAvatarVideo
	t.Cleanup(func() {
		bosonTTSHTTPClient = previousHTTPClient
		convertAudioTo16kMonoWAV = previousConverter
		generateAvatarVideo = previousGenerator
	})

	bosonTTSHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://boson.test/v1/audio/speech" {
				t.Fatalf("expected speech url, got %s", r.URL.String())
			}

			var req bosonTTSRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("expected valid provider request, got %v", err)
			}

			if req.Input != "hello avatar" {
				t.Fatalf("expected input hello avatar, got %q", req.Input)
			}

			if req.ResponseFormat != "mp3" {
				t.Fatalf("expected response format mp3, got %q", req.ResponseFormat)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"audio/mpeg"},
				},
				Body: io.NopCloser(strings.NewReader("ID3avatar")),
			}, nil
		}),
	}
	convertAudioTo16kMonoWAV = func(ctx context.Context, audio []byte) ([]byte, error) {
		if string(audio) != "ID3avatar" {
			t.Fatalf("expected mp3 bytes, got %q", string(audio))
		}

		return []byte("RIFFavatar"), nil
	}
	generateAvatarVideo = func(ctx context.Context, wavAudio []byte) ([]byte, error) {
		if string(wavAudio) != "RIFFavatar" {
			t.Fatalf("expected wav bytes, got %q", string(wavAudio))
		}

		return []byte("MP4avatar"), nil
	}

	t.Setenv("BOSON_API_KEY", "test-boson-key")
	t.Setenv("BOSON_BASE_URL", "https://boson.test")

	req := httptest.NewRequest(http.MethodPost, "/v1/avatar", strings.NewReader(`{"text":"hello avatar"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("expected video/mp4 content type, got %q", got)
	}

	if got := rec.Body.String(); got != "MP4avatar" {
		t.Fatalf("expected mp4 bytes, got %q", got)
	}
}

func TestHandleAvatarUsesAvatarTimeoutForVideoGeneration(t *testing.T) {
	previousHTTPClient := bosonTTSHTTPClient
	previousConverter := convertAudioTo16kMonoWAV
	previousGenerator := generateAvatarVideo
	t.Cleanup(func() {
		bosonTTSHTTPClient = previousHTTPClient
		convertAudioTo16kMonoWAV = previousConverter
		generateAvatarVideo = previousGenerator
	})

	bosonTTSHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"audio/mpeg"},
				},
				Body: io.NopCloser(strings.NewReader("ID3avatar")),
			}, nil
		}),
	}
	convertAudioTo16kMonoWAV = func(ctx context.Context, audio []byte) ([]byte, error) {
		return []byte("RIFFavatar"), nil
	}
	generateAvatarVideo = func(ctx context.Context, wavAudio []byte) ([]byte, error) {
		select {
		case <-time.After(20 * time.Millisecond):
			return []byte("MP4avatar"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	t.Setenv("BOSON_API_KEY", "test-boson-key")
	t.Setenv("BOSON_BASE_URL", "https://boson.test")
	t.Setenv("BOSON_TTS_TIMEOUT_MS", "1")
	t.Setenv("AVATAR_TIMEOUT_MS", "1000")

	req := httptest.NewRequest(http.MethodPost, "/v1/avatar", strings.NewReader(`{"text":"hello avatar"}`))
	rec := httptest.NewRecorder()

	newRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}
