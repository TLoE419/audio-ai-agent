package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backend stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadDotEnvFiles(".env", "../.env"); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + port(),
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("backend listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}
}

func newRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/v1/chat", handleChat)
	mux.HandleFunc("/v1/tts", handleTTS)

	return mux
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Text      string `json:"text"`
	LatencyMS int64  `json:"latency_ms"`
}

type ttsRequest struct {
	Text string `json:"text"`
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	text := "chat provider not connected yet"
	if client, ok := newLLMClientFromEnv(); ok {
		ctx, cancel := context.WithTimeout(r.Context(), llmTimeout())
		defer cancel()

		generatedText, err := client.GenerateText(ctx, strings.TrimSpace(req.Message))
		if err != nil {
			slog.Error("chat provider failed", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "chat provider failed",
			})
			return
		}

		text = generatedText
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Text:      text,
		LatencyMS: time.Since(start).Milliseconds(),
	})
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

	speech, err := client.GenerateSpeech(ctx, text)
	if err != nil {
		slog.Error("tts provider failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "tts provider failed",
		})
		return
	}

	w.Header().Set("Content-Type", speech.ContentType)
	w.Header().Set("X-Latency-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(speech.Audio)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}

func port() string {
	if value := os.Getenv("PORT"); value != "" {
		return value
	}

	return "8080"
}
