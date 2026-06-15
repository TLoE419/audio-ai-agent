# Audio AI Agent

This folder contains the product implementation for the low-latency Audio AI Agent.

The project is intentionally created step by step. We only add folders when the next feature needs them, so the structure stays connected to real product work instead of becoming a large empty scaffold.

## Current Scope

- External APIs for ASR, LLM, and TTS.
- Go backend for low-overhead API orchestration.
- Next.js frontend for text input, audio input, and audio playback.
- Latency-first design with TTFA, P50, and P95 measurements.

## Current Structure

```text
audio-ai-agent/
  backend/
    go.mod
    main.go
    main_test.go
  docs/
    decisions/
      0001-tech-stack.md
  README.md
```

## Next Step

Connect `POST /v1/chat` to an external LLM API with a timeout budget.
