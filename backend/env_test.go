package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFilesSetsMissingVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("AUDIO_AGENT_TEST_KEY=from-file\n"), 0o600); err != nil {
		t.Fatalf("write test env file: %v", err)
	}

	t.Setenv("AUDIO_AGENT_TEST_KEY", "")
	if err := os.Unsetenv("AUDIO_AGENT_TEST_KEY"); err != nil {
		t.Fatalf("unset test env: %v", err)
	}

	if err := loadDotEnvFiles(path); err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := os.Getenv("AUDIO_AGENT_TEST_KEY"); got != "from-file" {
		t.Fatalf("expected env value from file, got %q", got)
	}
}

func TestLoadDotEnvFilesOverridesExistingVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("AUDIO_AGENT_TEST_KEY=from-file\n"), 0o600); err != nil {
		t.Fatalf("write test env file: %v", err)
	}

	t.Setenv("AUDIO_AGENT_TEST_KEY", "from-process")

	if err := loadDotEnvFiles(path); err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := os.Getenv("AUDIO_AGENT_TEST_KEY"); got != "from-file" {
		t.Fatalf("expected env file value to win, got %q", got)
	}
}
