package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var generateAvatarVideo = liteAvatarVideo
var liteAvatarHTTPClient = http.DefaultClient
var liteAvatarWorkerMu sync.RWMutex
var liteAvatarWorker *liteAvatarWorkerClient

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
	cmd := exec.Command(
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
