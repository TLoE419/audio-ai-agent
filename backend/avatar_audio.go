package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var convertAudioTo16kMonoWAV = ffmpegTo16kMonoWAV

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
