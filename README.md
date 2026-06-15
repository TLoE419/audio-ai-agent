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

`POST /v1/chat` uses a placeholder response unless `OPENAI_API_KEY` is set.

Optional settings:

- `OPENAI_MODEL`: defaults to `gpt-5.5`.
- `OPENAI_BASE_URL`: defaults to `https://api.openai.com/v1`.
- `LLM_TIMEOUT_MS`: defaults to `10000`.

`POST /v1/tts` uses Boson AI Higgs Audio v3 TTS unless `BOSON_API_KEY` is missing.

Required setting:

- `BOSON_API_KEY`: Boson API key. Boson keys use the `bai-xxxx` format.

Optional settings:

- `BOSON_BASE_URL`: defaults to `https://api.boson.ai`.
- `BOSON_TTS_MODEL`: defaults to `higgs-audio-v3-tts`.
- `BOSON_TTS_VOICE`: defaults to `default`.
- `BOSON_TTS_RESPONSE_FORMAT`: defaults to `mp3`.
- `BOSON_TTS_TIMEOUT_MS`: defaults to `30000`.
