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
)

const defaultAnthropicBaseURL = "https://api.anthropic.com"
const defaultAnthropicVersion = "2023-06-01"

var anthropicHTTPClient = http.DefaultClient

type anthropicClient struct {
	apiKey     string
	baseURL    string
	model      string
	version    string
	maxTokens  int
	httpClient *http.Client
}

type anthropicMessageRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessageResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func newAnthropicClientFromEnv() (*anthropicClient, bool) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return nil, false
	}

	model := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}

	version := strings.TrimSpace(os.Getenv("ANTHROPIC_VERSION"))
	if version == "" {
		version = defaultAnthropicVersion
	}

	maxTokens := anthropicMaxTokens()

	return &anthropicClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		version:    version,
		maxTokens:  maxTokens,
		httpClient: anthropicHTTPClient,
	}, true
}

func anthropicMaxTokens() int {
	value := strings.TrimSpace(os.Getenv("ANTHROPIC_MAX_TOKENS"))
	if value == "" {
		return 1024
	}

	maxTokens, err := strconv.Atoi(value)
	if err != nil || maxTokens <= 0 {
		return 1024
	}

	return maxTokens
}

func (c *anthropicClient) GenerateText(ctx context.Context, message string) (string, error) {
	body, err := json.Marshal(anthropicMessageRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: message,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create anthropic request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.version)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call anthropic messages api: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return "", fmt.Errorf("anthropic messages api returned status %d: %s", res.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var decoded anthropicMessageResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}

	text := decoded.Text()
	if strings.TrimSpace(text) == "" {
		return "", errors.New("anthropic response did not include text")
	}

	return text, nil
}

func (r anthropicMessageResponse) Text() string {
	var parts []string
	for _, content := range r.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			parts = append(parts, content.Text)
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
