// Package tmux provides a wrapper for tmux operations.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// DiscoverClaudeSessions returns tmux sessions that have Claude Code running.
	DiscoverClaudeSessions() ([]Session, error)
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
	// DisplayDiffPopup opens a git diff in a tmux popup window.
	DisplayDiffPopup(workdir string) error
	// GetCurrentSession returns the name of the current tmux session, or empty if not in tmux.
	GetCurrentSession() string
	// GetPanePID returns the PID of the shell process running in the pane.
	GetPanePID(name string) (int, error)
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

// DiscoverClaudeSessions returns tmux sessions that have Claude Code running.
// Detection strategy:
// 1. Check if session has event file in $TMPDIR/cmux/events/{session}.jsonl
// 2. Fallback: Check if session is running a process containing "claude"
func (c *RealClient) DiscoverClaudeSessions() ([]Session, error) {
	allSessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}

	// Get events directory
	tmpdir := os.Getenv("TMPDIR")
	if tmpdir == "" {
		tmpdir = "/tmp"
	}
	eventsDir := filepath.Join(tmpdir, "cmux", "events")

	var claudeSessions []Session
	for _, session := range allSessions {
		// Check if event file exists for this session (fast path)
		eventFile := filepath.Join(eventsDir, session.Name+".jsonl")
		if _, err := os.Stat(eventFile); err == nil {
			claudeSessions = append(claudeSessions, session)
			continue
		}

		// Fallback: check if session is running claude
		if c.isRunningClaude(session.Name) {
			claudeSessions = append(claudeSessions, session)
		}
	}

	return claudeSessions, nil
}

// isRunningClaude checks if a tmux session is running a claude process.
func (c *RealClient) isRunningClaude(sessionName string) bool {
	pid, err := c.GetPanePID(sessionName)
	if err != nil {
		return false
	}

	// First check the pane's own command (claude might be the direct process)
	paneCmd, err := getProcessCommand(pid)
	if err == nil && strings.Contains(strings.ToLower(paneCmd), "claude") {
		return true
	}

	// Then check child processes (claude running under a shell)
	children, err := getChildProcesses(pid)
	if err != nil {
		return false
	}

	for _, child := range children {
		if strings.Contains(strings.ToLower(child), "claude") {
			return true
		}
	}

	return false
}

// getProcessCommand returns the command line for a PID.
func getProcessCommand(pid int) (string, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "args=")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getChildProcesses returns command lines of child processes for a PID.
func getChildProcesses(pid int) ([]string, error) {
	cmd := exec.Command("ps", "-eo", "pid=,ppid=,args=")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	parentPID := fmt.Sprintf("%d", pid)
	var children []string

	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		if fields[1] == parentPID {
			children = append(children, strings.Join(fields[2:], " "))
		}
	}

	return children, nil
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
	// Don't pass stderr - tmux prints "[detached (from session ...)]" messages
	// that clutter the terminal. Actual errors will still cause cmd.Run() to fail.
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

// DisplayDiffPopup opens a git diff in a tmux popup window.
// The user's configured git pager (e.g., delta) will be used automatically.
func (c *RealClient) DisplayDiffPopup(workdir string) error {
	cmd := exec.Command("tmux", "display-popup",
		"-E",        // Close popup when command exits
		"-w", "90%", // Width
		"-h", "90%", // Height
		"-d", workdir, // Set working directory
		"git", "diff",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux display-popup (diff): %w: %s", err, stderr.String())
	}
	return nil
}

// GetPanePID returns the PID of the shell process running in the pane.
func (c *RealClient) GetPanePID(name string) (int, error) {
	cmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_pid}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("tmux display-message: %w: %s", err, stderr.String())
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &pid); err != nil {
		return 0, fmt.Errorf("parse pane pid: %w", err)
	}
	return pid, nil
}
