package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBosonBaseURL = "https://api.boson.ai"

var bosonTTSHTTPClient = http.DefaultClient

type bosonTTSClient struct {
	apiKey         string
	baseURL        string
	model          string
	voice          string
	responseFormat string
	httpClient     *http.Client
}

type bosonTTSRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
	Stream         bool   `json:"stream"`
}

type speechResult struct {
	Audio       []byte
	ContentType string
}

func newBosonTTSClientFromEnv() (*bosonTTSClient, bool) {
	apiKey := strings.TrimSpace(os.Getenv("BOSON_API_KEY"))
	if apiKey == "" {
		return nil, false
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BOSON_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultBosonBaseURL
	}

	model := strings.TrimSpace(os.Getenv("BOSON_TTS_MODEL"))
	if model == "" {
		model = "higgs-audio-v3-tts"
	}

	voice := strings.TrimSpace(os.Getenv("BOSON_TTS_VOICE"))
	if voice == "" {
		voice = "default"
	}

	responseFormat := strings.TrimSpace(os.Getenv("BOSON_TTS_RESPONSE_FORMAT"))
	if responseFormat == "" {
		responseFormat = "mp3"
	}

	return &bosonTTSClient{
		apiKey:         apiKey,
		baseURL:        baseURL,
		model:          model,
		voice:          voice,
		responseFormat: responseFormat,
		httpClient:     bosonTTSHTTPClient,
	}, true
}

func ttsTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("BOSON_TTS_TIMEOUT_MS"))
	if value == "" {
		return 30 * time.Second
	}

	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return 30 * time.Second
	}

	return time.Duration(ms) * time.Millisecond
}

func (c *bosonTTSClient) GenerateSpeech(ctx context.Context, text string) (speechResult, error) {
	body, err := json.Marshal(bosonTTSRequest{
		Model:          c.model,
		Input:          text,
		Voice:          c.voice,
		ResponseFormat: c.responseFormat,
		Stream:         false,
	})
	if err != nil {
		return speechResult{}, fmt.Errorf("marshal boson tts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return speechResult{}, fmt.Errorf("create boson tts request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return speechResult{}, fmt.Errorf("call boson tts api: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return speechResult{}, fmt.Errorf("boson tts api returned status %d: %s", res.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	audio, err := io.ReadAll(res.Body)
	if err != nil {
		return speechResult{}, fmt.Errorf("read boson tts audio: %w", err)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	return speechResult{
		Audio:       audio,
		ContentType: contentType,
	}, nil
}
