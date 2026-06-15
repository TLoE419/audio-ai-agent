# Decision 0001: Tech Stack

## Decision

The Audio AI Agent will rely on external APIs for ASR, LLM, and TTS instead of self-hosting a model service.

The backend will use Go because the product is latency-sensitive and mostly needs low-overhead API orchestration, streaming, connection management, timeout control, and provider abstraction.

The frontend will use Next.js, React, and TypeScript.

## Chosen Stack

| Layer | Technology | Purpose |
| --- | --- | --- |
| Frontend | Next.js + React + TypeScript | User interface, text input, audio input, playback, conversation state |
| Backend | Go | API orchestration, provider abstraction, low-latency request handling |
| Database | PostgreSQL | Sessions, conversation history, request metadata |
| Cache / Queue | Redis, later if needed | Short-lived state, rate limiting, background work |
| ASR | External ASR API | Audio to text |
| LLM | External LLM API | Agent reasoning and response generation |
| TTS | External TTS API | Text to speech |
| Realtime | WebSocket first, WebRTC later if needed | Low-latency interaction path |

## Why This Structure

Using Go for the backend reduces the overhead we control. It does not remove the latency of external model APIs, so the architecture must optimize around streaming, connection reuse, payload size, timeout budgets, retry budgets, and TTFA measurement.

Using external APIs keeps the MVP focused on product flow instead of model deployment.

## First Implementation Path

1. Text input to LLM API.
2. LLM response to TTS API.
3. Audio playback in the browser.
4. Audio input to ASR API.
5. End-to-end latency measurement.
6. Streaming improvements.
