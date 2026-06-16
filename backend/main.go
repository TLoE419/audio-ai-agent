package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backend stopped", "error", err)
		os.Exit(1)
	}
}

var convertAudioTo16kMonoWAV = ffmpegTo16kMonoWAV
var generateAvatarVideo = liteAvatarVideo
var liteAvatarHTTPClient = http.DefaultClient
var liteAvatarWorkerMu sync.RWMutex
var liteAvatarWorker *liteAvatarWorkerClient

func run() error {
	if err := loadDotEnvFiles(".env", "../.env"); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + port(),
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if shouldPreloadLiteAvatar() {
		preloadCtx, cancelPreload := context.WithTimeout(ctx, avatarTimeout())
		worker, err := startLiteAvatarWorker(preloadCtx)
		cancelPreload()
		if err != nil {
			slog.Warn("liteavatar preload failed, falling back to per-request python", "error", err)
		} else {
			setLiteAvatarWorker(worker)
			defer func() {
				setLiteAvatarWorker(nil)
				worker.Close()
			}()
		}
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("backend listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}
}

func newRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/v1/chat", handleChat)
	mux.HandleFunc("/v1/tts", handleTTS)
	mux.HandleFunc("/v1/avatar", handleAvatar)

	return logRequests(withCORS(mux))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		slog.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

func withCORS(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:3000": true,
		"http://127.0.0.1:3000": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func logStep(scope string, step string, start time.Time, attrs ...any) {
	args := []any{
		"step", step,
		"latency_ms", time.Since(start).Milliseconds(),
	}
	args = append(args, attrs...)

	slog.Info(scope+" step completed", args...)
}

func avatarTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("AVATAR_TIMEOUT_MS"))
	if value == "" {
		return 5 * time.Minute
	}

	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return 5 * time.Minute
	}

	return time.Duration(ms) * time.Millisecond
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}

	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.ResponseWriter.Write(body)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Text      string `json:"text"`
	LatencyMS int64  `json:"latency_ms"`
}

type ttsRequest struct {
	Text string `json:"text"`
}

type avatarRequest struct {
	Text string `json:"text"`
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	text := "chat provider not connected yet"
	if client, ok := newLLMClientFromEnv(); ok {
		ctx, cancel := context.WithTimeout(r.Context(), llmTimeout())
		defer cancel()

		stepStart := time.Now()
		generatedText, err := client.GenerateText(ctx, strings.TrimSpace(req.Message))
		if err != nil {
			slog.Error("chat provider failed",
				"error", err,
				"latency_ms", time.Since(stepStart).Milliseconds(),
			)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "chat provider failed",
			})
			return
		}
		logStep("chat", "llm_generate_text", stepStart)

		text = generatedText
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Text:      text,
		LatencyMS: time.Since(start).Milliseconds(),
	})
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ttsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	client, ok := newBosonTTSClientFromEnv()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "tts provider not connected yet",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ttsTimeout())
	defer cancel()

	stepStart := time.Now()
	speech, err := client.GenerateSpeech(ctx, text)
	if err != nil {
		slog.Error("tts provider failed",
			"error", err,
			"latency_ms", time.Since(stepStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "tts provider failed",
		})
		return
	}
	logStep("tts", "boson_generate_speech", stepStart,
		"audio_bytes", len(speech.Audio),
		"content_type", speech.ContentType,
	)

	w.Header().Set("Content-Type", speech.ContentType)
	w.Header().Set("X-Latency-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(speech.Audio)
}

func handleAvatar(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req avatarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	client, ok := newBosonTTSClientFromEnv()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "tts provider not connected yet",
		})
		return
	}

	avatarCtx, cancel := context.WithTimeout(r.Context(), avatarTimeout())
	defer cancel()

	ttsStart := time.Now()
	ttsCtx, cancelTTS := context.WithTimeout(avatarCtx, ttsTimeout())
	speech, err := client.GenerateSpeech(ttsCtx, text)
	cancelTTS()
	if err != nil {
		slog.Error("avatar tts provider failed",
			"error", err,
			"latency_ms", time.Since(ttsStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "tts provider failed",
		})
		return
	}
	logStep("avatar", "boson_generate_speech", ttsStart,
		"audio_bytes", len(speech.Audio),
		"content_type", speech.ContentType,
	)

	convertStart := time.Now()
	wavAudio, err := convertAudioTo16kMonoWAV(avatarCtx, speech.Audio)
	if err != nil {
		slog.Error("avatar audio conversion failed",
			"error", err,
			"latency_ms", time.Since(convertStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "audio conversion failed",
		})
		return
	}
	logStep("avatar", "ffmpeg_to_16k_mono_wav", convertStart,
		"input_bytes", len(speech.Audio),
		"output_bytes", len(wavAudio),
	)

	videoStart := time.Now()
	video, err := generateAvatarVideo(avatarCtx, wavAudio)
	if err != nil {
		slog.Error("avatar video generation failed",
			"error", err,
			"latency_ms", time.Since(videoStart).Milliseconds(),
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "avatar video generation failed",
		})
		return
	}
	logStep("avatar", "liteavatar_generate_video", videoStart,
		"input_bytes", len(wavAudio),
		"output_bytes", len(video),
	)

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Latency-Ms", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(video)
}

func ffmpegTo16kMonoWAV(ctx context.Context, audio []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"pipe:1",
	)

	var wavAudio bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(audio)
	cmd.Stdout = &wavAudio
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return wavAudio.Bytes(), nil
}

func liteAvatarVideo(ctx context.Context, wavAudio []byte) ([]byte, error) {
	if worker := currentLiteAvatarWorker(); worker != nil {
		video, err := worker.Render(ctx, wavAudio)
		if err == nil {
			return video, nil
		}
		slog.Warn("preloaded liteavatar worker failed, falling back to cli", "error", err)
	}

	return liteAvatarVideoCLI(ctx, wavAudio)
}

func liteAvatarVideoCLI(ctx context.Context, wavAudio []byte) ([]byte, error) {
	prepareStart := time.Now()
	tempDir, err := os.MkdirTemp("", "audio-ai-agent-avatar-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	wavPath := filepath.Join(tempDir, "input.wav")
	resultDir := filepath.Join(tempDir, "result")
	if err := os.WriteFile(wavPath, wavAudio, 0o600); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(resultDir, 0o700); err != nil {
		return nil, err
	}
	logStep("liteavatar", "prepare_files", prepareStart, "wav_bytes", len(wavAudio))

	dataStart := time.Now()
	openAvatarChatDir, err := filepath.Abs(defaultOpenAvatarChatDir())
	if err != nil {
		return nil, err
	}
	dataDir, err := liteAvatarDataDir(ctx, openAvatarChatDir, tempDir)
	if err != nil {
		return nil, err
	}
	logStep("liteavatar", "prepare_avatar_data", dataStart, "data_dir", dataDir)

	algoDir := filepath.Join(openAvatarChatDir, "src", "handlers", "avatar", "liteavatar", "algo", "liteavatar")
	cmd := exec.CommandContext(ctx,
		filepath.Join(openAvatarChatDir, ".venv", "bin", "python"),
		"lite_avatar.py",
		"--data_dir", dataDir,
		"--audio_file", wavPath,
		"--result_dir", resultDir,
	)
	cmd.Dir = algoDir

	pythonStart := time.Now()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("lite_avatar.py: %w: %s", err, strings.TrimSpace(string(output)))
	}
	logStep("liteavatar", "run_python", pythonStart)

	readStart := time.Now()
	video, err := os.ReadFile(filepath.Join(resultDir, "test_demo.mp4"))
	if err != nil {
		return nil, err
	}
	logStep("liteavatar", "read_video", readStart, "video_bytes", len(video))

	return video, nil
}

type liteAvatarWorkerClient struct {
	cmd     *exec.Cmd
	baseURL string
	tempDir string
}

func startLiteAvatarWorker(ctx context.Context) (*liteAvatarWorkerClient, error) {
	start := time.Now()
	tempDir, err := os.MkdirTemp("", "audio-ai-agent-liteavatar-worker-*")
	if err != nil {
		return nil, err
	}

	openAvatarChatDir, err := filepath.Abs(defaultOpenAvatarChatDir())
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	dataDir, err := liteAvatarDataDir(ctx, openAvatarChatDir, tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	scriptPath, err := filepath.Abs(liteAvatarWorkerScript())
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	pythonPath := filepath.Join(openAvatarChatDir, ".venv", "bin", "python")
	cmd := exec.CommandContext(ctx,
		pythonPath,
		scriptPath,
		"--open-avatar-chat-dir", openAvatarChatDir,
		"--data-dir", dataDir,
	)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	readyCh := make(chan string, 1)
	scanDone := make(chan error, 1)
	go scanLiteAvatarWorkerStdout(stdout, readyCh, scanDone)

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case baseURL := <-readyCh:
		logStep("liteavatar", "preload_worker", start, "url", baseURL)
		return &liteAvatarWorkerClient{cmd: cmd, baseURL: baseURL, tempDir: tempDir}, nil
	case err := <-scanDone:
		if err == nil {
			err = io.EOF
		}
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("liteavatar worker stdout closed before ready: %w", err)
	case err := <-waitCh:
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("liteavatar worker exited before ready: %w", err)
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		os.RemoveAll(tempDir)
		return nil, ctx.Err()
	}
}

func scanLiteAvatarWorkerStdout(stdout io.Reader, readyCh chan<- string, doneCh chan<- error) {
	scanner := bufio.NewScanner(stdout)
	readySent := false
	for scanner.Scan() {
		line := scanner.Text()
		if !readySent && strings.HasPrefix(line, "READY ") {
			readyCh <- strings.TrimSpace(strings.TrimPrefix(line, "READY "))
			readySent = true
			continue
		}
		slog.Info("liteavatar worker", "line", line)
	}
	doneCh <- scanner.Err()
}

func (c *liteAvatarWorkerClient) Render(ctx context.Context, wavAudio []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/render", bytes.NewReader(wavAudio))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "audio/wav")

	resp, err := liteAvatarHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("liteavatar worker returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func (c *liteAvatarWorkerClient) Close() {
	if c == nil {
		return
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.tempDir != "" {
		_ = os.RemoveAll(c.tempDir)
	}
}

func setLiteAvatarWorker(worker *liteAvatarWorkerClient) {
	liteAvatarWorkerMu.Lock()
	defer liteAvatarWorkerMu.Unlock()
	liteAvatarWorker = worker
}

func currentLiteAvatarWorker() *liteAvatarWorkerClient {
	liteAvatarWorkerMu.RLock()
	defer liteAvatarWorkerMu.RUnlock()
	return liteAvatarWorker
}

func shouldPreloadLiteAvatar() bool {
	value := strings.TrimSpace(os.Getenv("OPENAVATARCHAT_PRELOAD"))
	if value == "0" || strings.EqualFold(value, "false") {
		return false
	}
	if value == "1" || strings.EqualFold(value, "true") {
		return true
	}

	_, err := os.Stat(defaultOpenAvatarChatDir())
	return err == nil
}

func liteAvatarWorkerScript() string {
	if _, err := os.Stat("liteavatar_worker.py"); err == nil {
		return "liteavatar_worker.py"
	}
	return filepath.Join("backend", "liteavatar_worker.py")
}

func defaultOpenAvatarChatDir() string {
	if value := strings.TrimSpace(os.Getenv("OPENAVATARCHAT_DIR")); value != "" {
		return value
	}
	if _, err := os.Stat("../external/OpenAvatarChat"); err == nil {
		return "../external/OpenAvatarChat"
	}
	return "external/OpenAvatarChat"
}

func liteAvatarDataDir(ctx context.Context, openAvatarChatDir string, tempDir string) (string, error) {
	if value := strings.TrimSpace(os.Getenv("OPENAVATARCHAT_AVATAR_DATA_DIR")); value != "" {
		return value, nil
	}

	extractDir := filepath.Join(tempDir, "sample_data")
	zipPath := filepath.Join(openAvatarChatDir, "src", "handlers", "avatar", "liteavatar", "algo", "liteavatar", "data", "sample_data.zip")
	cmd := exec.CommandContext(ctx, "unzip", "-q", "-o", zipPath, "-d", extractDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("unzip sample avatar data: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return filepath.Join(extractDir, "preload"), nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}

func port() string {
	if value := os.Getenv("PORT"); value != "" {
		return value
	}

	return "8080"
}
