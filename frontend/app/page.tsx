"use client";

import { FormEvent, MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";

type ChatResponse = {
  text?: string;
  latency_ms?: number;
  error?: string;
};

type VoiceReply = {
  userText: string;
  aiText: string;
  chatLatencyMs: number | null;
  audioUrl: string;
};

const WAVE_BARS = Array.from({ length: 54 }, (_, index) => 18 + ((index * 37) % 58));

function formatTime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "0:00";
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.floor(seconds % 60);
  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

async function readError(response: Response) {
  const raw = await response.text();
  if (!raw) {
    return `HTTP ${response.status}`;
  }

  try {
    const parsed = JSON.parse(raw) as { error?: string };
    return parsed.error ?? raw;
  } catch {
    return raw;
  }
}

export default function Home() {
  const [draft, setDraft] = useState("");
  const [submittedText, setSubmittedText] = useState("");
  const [reply, setReply] = useState<VoiceReply | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState("");

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const currentAudioUrlRef = useRef<string | null>(null);
  const shouldAutoPlayRef = useRef(false);

  const activeUserText = reply?.userText ?? submittedText;

  const releaseCurrentAudio = useCallback(() => {
    if (currentAudioUrlRef.current) {
      URL.revokeObjectURL(currentAudioUrlRef.current);
      currentAudioUrlRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      releaseCurrentAudio();
    };
  }, [releaseCurrentAudio]);

  const resetConversation = useCallback(() => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
    }

    releaseCurrentAudio();
    shouldAutoPlayRef.current = false;
    setReply(null);
    setSubmittedText("");
    setElapsed(0);
    setDuration(0);
    setIsPlaying(false);
    setError("");
  }, [releaseCurrentAudio]);

  async function submitMessage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const text = draft.trim();
    if (!text || isLoading) {
      return;
    }

    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
    }
    releaseCurrentAudio();

    setDraft("");
    setSubmittedText(text);
    setReply(null);
    setElapsed(0);
    setDuration(0);
    setIsPlaying(false);
    setError("");
    setIsLoading(true);

    try {
      const chatResponse = await fetch("/backend/v1/chat", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ message: text }),
      });

      const chatPayload = (await chatResponse.json()) as ChatResponse;
      if (!chatResponse.ok) {
        throw new Error(chatPayload.error ?? `Chat request failed (${chatResponse.status})`);
      }

      const aiText = chatPayload.text?.trim();
      if (!aiText) {
        throw new Error("Chat response did not include text.");
      }

      const ttsResponse = await fetch("/backend/v1/tts", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ text: aiText }),
      });

      if (!ttsResponse.ok) {
        throw new Error(await readError(ttsResponse));
      }

      const audioBlob = await ttsResponse.blob();
      const audioUrl = URL.createObjectURL(audioBlob);
      currentAudioUrlRef.current = audioUrl;
      shouldAutoPlayRef.current = true;

      setReply({
        userText: text,
        aiText,
        chatLatencyMs: typeof chatPayload.latency_ms === "number" ? chatPayload.latency_ms : null,
        audioUrl,
      });
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : "Voice generation failed.");
    } finally {
      setIsLoading(false);
    }
  }

  async function togglePlayback() {
    const audio = audioRef.current;
    if (!audio || !reply) {
      return;
    }

    if (isPlaying) {
      audio.pause();
      setIsPlaying(false);
      return;
    }

    try {
      await audio.play();
      setIsPlaying(true);
    } catch {
      setIsPlaying(false);
    }
  }

  function handleLoadedMetadata() {
    const audio = audioRef.current;
    if (!audio) {
      return;
    }

    if (Number.isFinite(audio.duration)) {
      setDuration(audio.duration);
    }

    if (shouldAutoPlayRef.current) {
      shouldAutoPlayRef.current = false;
      void audio.play().then(() => setIsPlaying(true)).catch(() => setIsPlaying(false));
    }
  }

  function handleSeek(event: MouseEvent<HTMLButtonElement>) {
    const audio = audioRef.current;
    if (!audio || !duration) {
      return;
    }

    const bounds = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - bounds.left) / bounds.width, 0), 1);
    const nextTime = ratio * duration;
    audio.currentTime = nextTime;
    setElapsed(nextTime);
  }

  const progress = duration > 0 ? Math.min(elapsed / duration, 1) : 0;
  const activeBarIndex = Math.floor(progress * WAVE_BARS.length);

  const transcriptText = useMemo(() => {
    if (!reply) {
      return "";
    }

    if (!duration || (!isPlaying && elapsed === 0)) {
      return reply.aiText;
    }

    const visibleCharacters = Math.max(1, Math.round(reply.aiText.length * Math.min(elapsed / duration, 1)));
    return reply.aiText.slice(0, visibleCharacters);
  }, [duration, elapsed, isPlaying, reply]);

  return (
    <main className="voice-shell">
      <section className="voice-panel" aria-label="Audio AI Agent">
        <header className="voice-header">
          <div className="brand">
            <span className="brand-mark" aria-hidden="true" />
            <span>Voice Interface</span>
          </div>
          {(activeUserText || error) && (
            <button className="clear-button" type="button" onClick={resetConversation}>
              Clear
            </button>
          )}
        </header>

        <div className={`conversation ${!activeUserText && !isLoading && !error ? "is-empty" : ""}`}>
          {!activeUserText && !isLoading && !error && (
            <div className="empty-state" aria-label="No message submitted yet">
              <div className="idle-wave" aria-hidden="true">
                {WAVE_BARS.slice(0, 28).map((height, index) => (
                  <span key={index} style={{ height: `${Math.max(10, height * 0.55)}px` }} />
                ))}
              </div>
              <p>Type a message, and the AI will reply with playable voice audio.</p>
            </div>
          )}

          {activeUserText && (
            <div className="message-row">
              <div className="user-bubble">{activeUserText}</div>
            </div>
          )}

          {isLoading && (
            <div className="loading-card" role="status" aria-live="polite">
              <div className="loading-dots" aria-hidden="true">
                <span />
                <span />
                <span />
              </div>
              <span>Generating voice...</span>
            </div>
          )}

          {error && !isLoading && <div className="error-card">{error}</div>}

          {reply && !isLoading && (
            <article className="voice-card">
              <audio
                ref={audioRef}
                src={reply.audioUrl}
                preload="metadata"
                onLoadedMetadata={handleLoadedMetadata}
                onPlay={() => setIsPlaying(true)}
                onPause={() => setIsPlaying(false)}
                onTimeUpdate={(event) => setElapsed(event.currentTarget.currentTime)}
                onEnded={() => {
                  setIsPlaying(false);
                  setElapsed(duration);
                }}
              />

              <div className="voice-card-header">
                <span>AI Voice Reply</span>
                {reply.chatLatencyMs !== null && <span className="latency">{reply.chatLatencyMs} ms</span>}
              </div>

              <div className="player-row">
                <button
                  className="play-button"
                  type="button"
                  onClick={togglePlayback}
                  aria-label={isPlaying ? "Pause voice" : "Play voice"}
                >
                  <span className={isPlaying ? "pause-glyph" : "play-glyph"} aria-hidden="true" />
                </button>

                <div className="waveform" aria-hidden="true">
                  {WAVE_BARS.map((height, index) => (
                    <span
                      className={index <= activeBarIndex && (isPlaying || elapsed > 0) ? "is-active" : ""}
                      key={index}
                      style={{ height: `${height}px` }}
                    />
                  ))}
                </div>
              </div>

              <button className="progress-track" type="button" onClick={handleSeek} aria-label="Seek playback">
                <span style={{ width: `${progress * 100}%` }} />
              </button>

              <div className="time-row">
                <span>{formatTime(elapsed)}</span>
                <span>{duration ? formatTime(duration) : "--:--"}</span>
              </div>

              <div className="transcript">
                <div className="transcript-label">Transcript</div>
                <p>
                  {transcriptText}
                  <span className={isPlaying ? "caret is-visible" : "caret"} aria-hidden="true" />
                </p>
              </div>
            </article>
          )}
        </div>

        <form className="input-bar" onSubmit={submitMessage}>
          <input
            value={draft}
            disabled={isLoading}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="Type a message, and voice will reply..."
            aria-label="Message input"
          />
          <button type="submit" disabled={!draft.trim() || isLoading} aria-label="Send message">
            <span className="send-glyph" aria-hidden="true" />
          </button>
        </form>
      </section>
    </main>
  );
}
