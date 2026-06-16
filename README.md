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
  frontend/
    app/
      globals.css
      layout.tsx
      page.tsx
    next-env.d.ts
    next.config.mjs
    package-lock.json
    package.json
    tsconfig.json
  docs/
    decisions/
      0001-tech-stack.md
  README.md
```

## Next Step

Add microphone input after the text-to-voice loop is stable.

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

## Frontend

The frontend lives in `frontend/` so the Next.js app can evolve separately from the Go backend without mixing package files at the repo root.

Run the backend first:

```sh
cd backend
GOCACHE=/private/tmp/audio-ai-agent-go-cache go run .
```

Then run the frontend:

```sh
cd frontend
npm install
npm run dev
```

The frontend calls `/backend/v1/chat` and `/backend/v1/tts`. `frontend/next.config.mjs` rewrites those requests to the Go backend at `http://127.0.0.1:18080` by default.

Optional setting:

- `BACKEND_URL`: overrides the backend URL used by the Next.js rewrite.
