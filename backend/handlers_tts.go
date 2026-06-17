package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ttsRequest struct {
	Text string `json:"text"`
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ttsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	client, ok := newBosonTTSClientFromEnv()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "tts provider not connected yet",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ttsTimeout())
	defer cancel()

	stepStart := time.Now()
	speech, err := client.GenerateSpeech(ctx, text)
	if err != nil {
		slog.Error("tts provider failed",
			"error", err,
			"latency_ms", time.Since(stepStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "tts provider failed",
		})
		return
	}
	logStep("tts", "boson_generate_speech", stepStart,
		"audio_bytes", len(speech.Audio),
		"content_type", speech.ContentType,
	)

	w.Header().Set("Content-Type", speech.ContentType)
	w.Header().Set("X-Latency-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(speech.Audio)
}
