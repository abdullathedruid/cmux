// Package tmux provides a wrapper for tmux operations.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Session represents a tmux session.
type Session struct {
	Name      string
	Path      string
	Created   time.Time
	Attached  bool
	WindowCount int
}

// Client provides tmux operations.
type Client interface {
	// ListSessions returns all cmux-managed tmux sessions.
	ListSessions() ([]Session, error)
	// CreateSession creates a new tmux session with the given name in the specified directory.
	CreateSession(name, dir string, runClaude bool) error
	// AttachSession attaches to the specified session.
	AttachSession(name string) error
	// SwitchSession switches the current client to the specified session.
	SwitchSession(name string) error
	// KillSession kills the specified session.
	KillSession(name string) error
	// HasSession checks if a session exists.
	HasSession(name string) bool
	// IsInsideTmux returns true if we're running inside a tmux session.
	IsInsideTmux() bool
	// CapturePane captures the current pane output.
	CapturePane(name string, lines int) (string, error)
	// SendKeys sends keys to a session.
	SendKeys(name string, keys string) error
	// SupportsPopup returns true if tmux version supports display-popup (3.2+).
	SupportsPopup() bool
	// DisplayPopup opens a session in a tmux popup window.
	DisplayPopup(name string) error
	// GetCurrentSession returns the name of the current tmux session, or empty if not in tmux.
	GetCurrentSession() string
}

// RealClient implements Client using actual tmux commands.
type RealClient struct {
	claudeCommand string
}

// NewClient creates a new tmux client.
func NewClient(claudeCommand string) *RealClient {
	return &RealClient{
		claudeCommand: claudeCommand,
	}
}

// ListSessions returns all tmux sessions.
func (c *RealClient) ListSessions() ([]Session, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_path}\t#{session_created}\t#{session_attached}\t#{session_windows}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// No sessions is not an error
		if strings.Contains(stderr.String(), "no server running") ||
			strings.Contains(stderr.String(), "no sessions") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w: %s", err, stderr.String())
	}

	return parseSessions(stdout.String()), nil
}

// parseSessions parses tmux list-sessions output.
func parseSessions(output string) []Session {
	var sessions []Session
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		created := time.Now()
		if ts, err := time.Parse("2006-01-02T15:04:05", parts[2]); err == nil {
			created = ts
		} else if epoch, err := parseUnixTimestamp(parts[2]); err == nil {
			created = epoch
		}

		windowCount := 1
		if _, err := fmt.Sscanf(parts[4], "%d", &windowCount); err != nil {
			windowCount = 1
		}

		sessions = append(sessions, Session{
			Name:        parts[0],
			Path:        parts[1],
			Created:     created,
			Attached:    parts[3] == "1",
			WindowCount: windowCount,
		})
	}

	return sessions
}

func parseUnixTimestamp(s string) (time.Time, error) {
	var ts int64
	if _, err := fmt.Sscanf(s, "%d", &ts); err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

// CreateSession creates a new tmux session.
func (c *RealClient) CreateSession(name, dir string, runClaude bool) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	if runClaude {
		args = append(args, c.claudeCommand)
	}

	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, stderr.String())
	}
	return nil
}

// AttachSession attaches to a tmux session (from outside tmux).
func (c *RealClient) AttachSession(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SwitchSession switches the current tmux client to another session.
func (c *RealClient) SwitchSession(name string) error {
	cmd := exec.Command("tmux", "switch-client", "-t", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux switch-client: %w: %s", err, stderr.String())
	}
	return nil
}

// KillSession kills a tmux session.
func (c *RealClient) KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, stderr.String())
	}
	return nil
}

// HasSession checks if a session exists.
func (c *RealClient) HasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// IsInsideTmux returns true if we're running inside tmux.
func (c *RealClient) IsInsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// GetCurrentSession returns the name of the current tmux session, or empty if not in tmux.
func (c *RealClient) GetCurrentSession() string {
	if !c.IsInsideTmux() {
		return ""
	}
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// CapturePane captures the pane output from a session.
func (c *RealClient) CapturePane(name string, lines int) (string, error) {
	args := []string{"capture-pane", "-t", name, "-p"}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// SendKeys sends keys to a session.
func (c *RealClient) SendKeys(name string, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", name, keys, "Enter")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux send-keys: %w: %s", err, stderr.String())
	}
	return nil
}

// SupportsPopup returns true if tmux version supports display-popup (3.2+).
func (c *RealClient) SupportsPopup() bool {
	cmd := exec.Command("tmux", "-V")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false
	}

	return parseVersionSupportsPopup(stdout.String())
}

// parseVersionSupportsPopup parses tmux version output and returns true if >= 3.2.
func parseVersionSupportsPopup(versionOutput string) bool {
	// Parse version from "tmux X.Y" output
	version := strings.TrimSpace(versionOutput)
	version = strings.TrimPrefix(version, "tmux ")

	// Handle versions like "3.2a", "3.3", "next-3.4"
	version = strings.TrimPrefix(version, "next-")

	// Extract major.minor
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	var major, minor int
	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return false
	}
	// Minor might have suffix like "2a"
	minorStr := strings.TrimRight(parts[1], "abcdefghijklmnopqrstuvwxyz")
	if _, err := fmt.Sscanf(minorStr, "%d", &minor); err != nil {
		return false
	}

	// display-popup requires tmux 3.2+
	return major > 3 || (major == 3 && minor >= 2)
}

// DisplayPopup opens a session in a tmux popup window.
func (c *RealClient) DisplayPopup(name string) error {
	// Use display-popup with -E to close on command exit
	// -w and -h set width/height as percentage
	// The command attaches to the target session
	cmd := exec.Command("tmux", "display-popup",
		"-E",       // Close popup when command exits
		"-w", "80%", // Width
		"-h", "80%", // Height
		"tmux", "attach-session", "-t", name,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux display-popup: %w: %s", err, stderr.String())
	}
	return nil
}
