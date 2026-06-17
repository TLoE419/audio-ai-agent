package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type avatarRequest struct {
	Text string `json:"text"`
}

func avatarTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("AVATAR_TIMEOUT_MS"))
	if value == "" {
		return 5 * time.Minute
	}

	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return 5 * time.Minute
	}

	return time.Duration(ms) * time.Millisecond
}

func handleAvatar(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req avatarRequest
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

	avatarCtx, cancel := context.WithTimeout(r.Context(), avatarTimeout())
	defer cancel()

	ttsStart := time.Now()
	ttsCtx, cancelTTS := context.WithTimeout(avatarCtx, ttsTimeout())
	speech, err := client.GenerateSpeech(ttsCtx, text)
	cancelTTS()
	if err != nil {
		slog.Error("avatar tts provider failed",
			"error", err,
			"latency_ms", time.Since(ttsStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "tts provider failed",
		})
		return
	}
	logStep("avatar", "boson_generate_speech", ttsStart,
		"audio_bytes", len(speech.Audio),
		"content_type", speech.ContentType,
	)

	convertStart := time.Now()
	wavAudio, err := convertAudioTo16kMonoWAV(avatarCtx, speech.Audio)
	if err != nil {
		slog.Error("avatar audio conversion failed",
			"error", err,
			"latency_ms", time.Since(convertStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "audio conversion failed",
		})
		return
	}
	logStep("avatar", "ffmpeg_to_16k_mono_wav", convertStart,
		"input_bytes", len(speech.Audio),
		"output_bytes", len(wavAudio),
	)

	videoStart := time.Now()
	video, err := generateAvatarVideo(avatarCtx, wavAudio)
	if err != nil {
		slog.Error("avatar video generation failed",
			"error", err,
			"latency_ms", time.Since(videoStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "avatar video generation failed",
		})
		return
	}
	logStep("avatar", "liteavatar_generate_video", videoStart,
		"input_bytes", len(wavAudio),
		"output_bytes", len(video),
	)

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Latency-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(video)
}
