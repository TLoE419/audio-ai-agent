package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

type textGenerator interface {
	GenerateText(ctx context.Context, message string) (string, error)
}

func newLLMClientFromEnv() (textGenerator, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER"))) {
	case "anthropic", "claude":
		return newAnthropicClientFromEnv()
	case "openai":
		return newOpenAIClientFromEnv()
	case "":
		if client, ok := newAnthropicClientFromEnv(); ok {
			return client, true
		}

		return newOpenAIClientFromEnv()
	default:
		return nil, false
	}
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
