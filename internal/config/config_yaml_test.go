package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NoConfigFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "cmux-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override the data directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create a default config to get the default data dir path
	defaultCfg := Default()
	defaultCfg.DataDir = filepath.Join(tmpDir, ".config", "cmux")

	// Ensure data directory exists
	if err := os.MkdirAll(defaultCfg.DataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test that Load returns defaults when no config file exists
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	// Verify default keybindings
	if cfg.Keys.Quit != "q" {
		t.Errorf("cfg.Keys.Quit = %q, want %q", cfg.Keys.Quit, "q")
	}
	if cfg.Keys.ToggleView != "v" {
		t.Errorf("cfg.Keys.ToggleView = %q, want %q", cfg.Keys.ToggleView, "v")
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "cmux-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dataDir := filepath.Join(tmpDir, ".config", "cmux")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a test config file
	configContent := `keys:
  quit: "Q"
  toggle_view: "V"
theme:
  colors:
    selection_bg: "green"
`
	configPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override the data directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	// Verify custom keybindings are loaded
	if cfg.Keys.Quit != "Q" {
		t.Errorf("cfg.Keys.Quit = %q, want %q", cfg.Keys.Quit, "Q")
	}
	if cfg.Keys.ToggleView != "V" {
		t.Errorf("cfg.Keys.ToggleView = %q, want %q", cfg.Keys.ToggleView, "V")
	}

	// Verify defaults are preserved for unset values
	if cfg.Keys.Help != "?" {
		t.Errorf("cfg.Keys.Help = %q, want %q (default)", cfg.Keys.Help, "?")
	}

	// Verify theme colors
	if cfg.Theme.Colors.SelectionBg != "green" {
		t.Errorf("cfg.Theme.Colors.SelectionBg = %q, want %q", cfg.Theme.Colors.SelectionBg, "green")
	}
	// Default for unset colors
	if cfg.Theme.Colors.SelectionFg != "white" {
		t.Errorf("cfg.Theme.Colors.SelectionFg = %q, want %q (default)", cfg.Theme.Colors.SelectionFg, "white")
	}
}

func TestLoad_DuplicateKeysError(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "cmux-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dataDir := filepath.Join(tmpDir, ".config", "cmux")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a config file with duplicate keys
	configContent := `keys:
  quit: "x"
  delete: "x"
`
	configPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override the data directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err = Load()
	if err == nil {
		t.Error("Load() expected error for duplicate keys, got nil")
	}
}

func TestLoad_StatusStyle(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "cmux-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dataDir := filepath.Join(tmpDir, ".config", "cmux")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a config file with custom status style
	configContent := `theme:
  status:
    attached:
      icon: "★"
      color: "magenta"
      label: "CONNECTED"
`
	configPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override the data directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	// Verify custom status style
	attachedStyle, ok := cfg.Theme.Status["attached"]
	if !ok {
		t.Fatal("attached status style not found")
	}
	if attachedStyle.Icon != "★" {
		t.Errorf("attached.Icon = %q, want %q", attachedStyle.Icon, "★")
	}
	if attachedStyle.Color != "magenta" {
		t.Errorf("attached.Color = %q, want %q", attachedStyle.Color, "magenta")
	}
	if attachedStyle.Label != "CONNECTED" {
		t.Errorf("attached.Label = %q, want %q", attachedStyle.Label, "CONNECTED")
	}

	// Verify other status styles have defaults
	idleStyle, ok := cfg.Theme.Status["idle"]
	if !ok {
		t.Fatal("idle status style not found")
	}
	if idleStyle.Icon == "" {
		t.Error("idle.Icon should have default value")
	}
}
