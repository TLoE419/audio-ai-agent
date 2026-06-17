package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegTo16kMonoWAVUsesExpectedCommand(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := `#!/bin/sh
case "$*" in
  *"-ac 1 -ar 16000 -f wav pipe:1"*) ;;
  *) exit 3 ;;
esac
cat >/dev/null
printf RIFFfakewav
`
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	wav, err := ffmpegTo16kMonoWAV(context.Background(), []byte("ID3fake"))
	if err != nil {
		t.Fatalf("convert audio: %v", err)
	}

	if string(wav) != "RIFFfakewav" {
		t.Fatalf("expected fake wav bytes, got %q", string(wav))
	}
}
