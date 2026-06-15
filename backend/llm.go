package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

var openAIHTTPClient = http.DefaultClient

type openAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type openAIResponseRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func newOpenAIClientFromEnv() (*openAIClient, bool) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, false
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = "gpt-5.5"
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}

	return &openAIClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: openAIHTTPClient,
	}, true
}

func llmTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("LLM_TIMEOUT_MS"))
	if value == "" {
		return 10 * time.Second
	}

	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return 10 * time.Second
	}

	return time.Duration(ms) * time.Millisecond
}

func (c *openAIClient) GenerateText(ctx context.Context, message string) (string, error) {
	body, err := json.Marshal(openAIResponseRequest{
		Model: c.model,
		Input: message,
	})
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create openai request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call openai responses api: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return "", fmt.Errorf("openai responses api returned status %d: %s", res.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var decoded openAIResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}

	text := decoded.Text()
	if strings.TrimSpace(text) == "" {
		return "", errors.New("openai response did not include text")
	}

	return text, nil
}

func (r openAIResponse) Text() string {
	if strings.TrimSpace(r.OutputText) != "" {
		return r.OutputText
	}

	for _, output := range r.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text
			}
		}
	}

	return ""
}
