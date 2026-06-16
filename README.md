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
  .env.example
  .gitignore
  backend/
    anthropic.go
    env.go
    go.mod
    llm.go
    main.go
    main_test.go
    tts.go
  docs/
    decisions/
      0001-tech-stack.md
  README.md
```

## Next Step

Build a minimal frontend that calls `POST /v1/chat`, then calls `POST /v1/tts` and plays the returned audio.

## Backend Configuration

For local development, copy `.env.example` to `.env` and fill in your API keys. The backend loads `.env` on startup, `.env` values override stale shell values, and real secrets stay out of Git.

`POST /v1/chat` uses a placeholder response unless an LLM provider API key is set. Set `LLM_PROVIDER=anthropic` to use Claude, or `LLM_PROVIDER=openai` to use OpenAI. If `LLM_PROVIDER` is empty, the backend tries Anthropic first, then OpenAI.

Shared optional setting:

- `LLM_TIMEOUT_MS`: defaults to `10000`.

Claude required setting:

- `ANTHROPIC_API_KEY`: Claude API key.

Claude optional settings:

- `ANTHROPIC_MODEL`: defaults to `claude-sonnet-4-6`.
- `ANTHROPIC_BASE_URL`: defaults to `https://api.anthropic.com`.
- `ANTHROPIC_VERSION`: defaults to `2023-06-01`.
- `ANTHROPIC_MAX_TOKENS`: defaults to `1024`.

OpenAI required setting:

- `OPENAI_API_KEY`: OpenAI API key.

OpenAI optional settings:

- `OPENAI_MODEL`: defaults to `gpt-5.5`.
- `OPENAI_BASE_URL`: defaults to `https://api.openai.com/v1`.

`POST /v1/tts` uses Boson AI Higgs Audio v3 TTS unless `BOSON_API_KEY` is missing.

Required setting:

- `BOSON_API_KEY`: Boson API key. Boson keys use the `bai-xxxx` format.

Optional settings:

- `BOSON_BASE_URL`: defaults to `https://api.boson.ai`.
- `BOSON_TTS_MODEL`: defaults to `higgs-audio-v3-tts`.
- `BOSON_TTS_VOICE`: defaults to `default`.
- `BOSON_TTS_RESPONSE_FORMAT`: defaults to `mp3`.
- `BOSON_TTS_TIMEOUT_MS`: defaults to `30000`.
