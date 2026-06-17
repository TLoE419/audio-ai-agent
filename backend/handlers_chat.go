package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Text      string `json:"text"`
	LatencyMS int64  `json:"latency_ms"`
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

		stepStart := time.Now()
		generatedText, err := client.GenerateText(ctx, strings.TrimSpace(req.Message))
		if err != nil {
			slog.Error("chat provider failed",
				"error", err,
				"latency_ms", time.Since(stepStart).Milliseconds(),
			)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "chat provider failed",
			})
			return
		}
		logStep("chat", "llm_generate_text", stepStart)

		text = generatedText
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Text:      text,
		LatencyMS: time.Since(start).Milliseconds(),
	})
}
