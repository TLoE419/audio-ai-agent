package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLiteAvatarVideoCallsBundledPython(t *testing.T) {
	root := t.TempDir()
	python := filepath.Join(root, ".venv", "bin", "python")
	algoDir := filepath.Join(root, "src", "handlers", "avatar", "liteavatar", "algo", "liteavatar")
	dataDir := filepath.Join(root, "avatar-data")

	for _, dir := range []string{filepath.Dir(python), algoDir, dataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("make dir: %v", err)
		}
	}

	script := `#!/bin/sh
[ "$1" = "lite_avatar.py" ] || exit 3
shift
while [ "$#" -gt 0 ]; do
  case "$1" in
    --data_dir) data="$2"; shift 2 ;;
    --audio_file) audio="$2"; shift 2 ;;
    --result_dir) result="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ "$data" = "$OPENAVATARCHAT_AVATAR_DATA_DIR" ] || exit 4
[ "$(cat "$audio")" = "RIFFtest" ] || exit 5
mkdir -p "$result"
printf MP4fake > "$result/test_demo.mp4"
`
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python: %v", err)
	}

	t.Setenv("OPENAVATARCHAT_DIR", root)
	t.Setenv("OPENAVATARCHAT_AVATAR_DATA_DIR", dataDir)

	video, err := liteAvatarVideo(context.Background(), []byte("RIFFtest"))
	if err != nil {
		t.Fatalf("generate video: %v", err)
	}

	if string(video) != "MP4fake" {
		t.Fatalf("expected fake mp4 bytes, got %q", string(video))
	}
}

func TestLiteAvatarVideoUsesPreloadedWorker(t *testing.T) {
	previousWorker := currentLiteAvatarWorker()
	previousHTTPClient := liteAvatarHTTPClient
	t.Cleanup(func() {
		setLiteAvatarWorker(previousWorker)
		liteAvatarHTTPClient = previousHTTPClient
	})

	liteAvatarHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "http://liteavatar.test/render" {
				t.Fatalf("expected render url, got %s", r.URL.String())
			}
			if r.Method != http.MethodPost {
				t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != "RIFFworker" {
				t.Fatalf("expected wav bytes, got %q", string(body))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"video/mp4"},
				},
				Body: io.NopCloser(strings.NewReader("MP4worker")),
			}, nil
		}),
	}

	setLiteAvatarWorker(&liteAvatarWorkerClient{baseURL: "http://liteavatar.test"})

	video, err := liteAvatarVideo(context.Background(), []byte("RIFFworker"))
	if err != nil {
		t.Fatalf("generate video: %v", err)
	}

	if string(video) != "MP4worker" {
		t.Fatalf("expected worker mp4 bytes, got %q", string(video))
	}
}

func TestLiteAvatarVideoFallsBackWhenPreloadedWorkerFails(t *testing.T) {
	previousWorker := currentLiteAvatarWorker()
	previousHTTPClient := liteAvatarHTTPClient
	t.Cleanup(func() {
		setLiteAvatarWorker(previousWorker)
		liteAvatarHTTPClient = previousHTTPClient
	})

	root := t.TempDir()
	python := filepath.Join(root, ".venv", "bin", "python")
	algoDir := filepath.Join(root, "src", "handlers", "avatar", "liteavatar", "algo", "liteavatar")
	dataDir := filepath.Join(root, "avatar-data")

	for _, dir := range []string{filepath.Dir(python), algoDir, dataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("make dir: %v", err)
		}
	}

	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    --audio_file) audio="$2"; shift 2 ;;
    --result_dir) result="$2"; shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$result"
printf fallback- > "$result/test_demo.mp4"
cat "$audio" >> "$result/test_demo.mp4"
`
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python: %v", err)
	}

	liteAvatarHTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("worker failed")),
			}, nil
		}),
	}
	setLiteAvatarWorker(&liteAvatarWorkerClient{baseURL: "http://liteavatar.test"})
	t.Setenv("OPENAVATARCHAT_DIR", root)
	t.Setenv("OPENAVATARCHAT_AVATAR_DATA_DIR", dataDir)

	video, err := liteAvatarVideo(context.Background(), []byte("RIFFfallback"))
	if err != nil {
		t.Fatalf("generate video: %v", err)
	}

	if string(video) != "fallback-RIFFfallback" {
		t.Fatalf("expected fallback mp4 bytes, got %q", string(video))
	}
}

func TestStartLiteAvatarWorkerSurvivesPreloadContextCancel(t *testing.T) {
	root := t.TempDir()
	python := filepath.Join(root, ".venv", "bin", "python")
	algoDir := filepath.Join(root, "src", "handlers", "avatar", "liteavatar", "algo", "liteavatar")
	dataDir := filepath.Join(root, "avatar-data")

	for _, dir := range []string{filepath.Dir(python), algoDir, dataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("make dir: %v", err)
		}
	}

	script := `#!/bin/sh
printf 'READY http://liteavatar.test\n'
trap 'exit 0' TERM
while true; do sleep 1; done
`
	if err := os.WriteFile(python, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python: %v", err)
	}

	t.Setenv("OPENAVATARCHAT_DIR", root)
	t.Setenv("OPENAVATARCHAT_AVATAR_DATA_DIR", dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	worker, err := startLiteAvatarWorker(ctx)
	if err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer worker.Close()

	cancel()
	time.Sleep(10 * time.Millisecond)

	if err := worker.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("expected worker to survive preload context cancel: %v", err)
	}
}
