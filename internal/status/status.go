// Package status reads Claude session status from hook-written event files
// and enriches it with data from JSONL transcripts.
package status

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/abdullathedruid/cmux/internal/transcript"
)

// EventsDir returns the directory where hook event files are stored.
func EventsDir() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "cmux", "events")
}

// HookEvent represents an event written by the hook script to the JSONL file.
type HookEvent struct {
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	TS             string `json:"ts"` // ISO8601 timestamp
}

// ToolHistoryEntry represents a single tool execution in the history.
type ToolHistoryEntry struct {
	Tool      string
	Summary   string
	Result    string
	Timestamp time.Time
}

// SessionStatus contains all status information for a session.
type SessionStatus struct {
	// From hook (real-time)
	Status    string
	Tool      string // current tool (only set during PreToolUse)
	SessionID string
	LastActive time.Time
	Found     bool

	// From transcript (historical)
	TranscriptPath string
	LastPrompt     string
	ToolHistory    []ToolHistoryEntry
	Summary        string // human-readable summary of current activity
}

// ReadStatus reads the status for a given tmux session name.
// Returns the status string, current tool (if any), tool summary, last active time, and whether the status was found.
// Deprecated: Use ReadFullStatus for access to all fields including tool history.
func ReadStatus(sessionName string) (status string, tool string, summary string, lastActive time.Time, found bool) {
	full := ReadFullStatus(sessionName)
	return full.Status, full.Tool, full.Summary, full.LastActive, full.Found
}

// ReadFullStatus reads the complete status for a given tmux session name.
// It combines real-time status from hooks with historical data from the JSONL transcript.
func ReadFullStatus(sessionName string) SessionStatus {
	return ReadFullStatusWithHistory(sessionName, 100)
}

// ReadFullStatusWithHistory reads complete status with configurable history depth.
func ReadFullStatusWithHistory(sessionName string, maxHistory int) SessionStatus {
	eventsFile := filepath.Join(EventsDir(), sessionName+".jsonl")

	result := SessionStatus{
		Status: "idle",
		Found:  false,
	}

	// Read the last event from the JSONL file
	event, err := readLastEvent(eventsFile)
	if err != nil {
		return result
	}

	result.Found = true
	result.SessionID = event.SessionID
	result.TranscriptPath = event.TranscriptPath

	// Parse timestamp
	if event.TS != "" {
		if t, err := time.Parse(time.RFC3339, event.TS); err == nil {
			result.LastActive = t
		}
	}

	// Map hook event to status
	switch event.HookEventName {
	case "PreToolUse":
		result.Status = "tool"
		result.Tool = event.ToolName
	case "PostToolUse", "UserPromptSubmit":
		result.Status = "active"
	case "Stop", "SubagentStop":
		result.Status = "stopped"
	case "Notification", "PermissionRequest":
		result.Status = "needs_input"
	default:
		result.Status = "active"
	}

	// Check if status is stale (older than 30 seconds = probably not running)
	// Exception: "needs_input" and "stopped" should persist until user actually intervenes
	if !result.LastActive.IsZero() && result.Status != "needs_input" && result.Status != "stopped" {
		age := time.Since(result.LastActive)
		if age > 30*time.Second {
			result.Status = "idle"
			result.Tool = ""
		}
	}

	// Read transcript for historical data
	if result.TranscriptPath != "" && maxHistory > 0 {
		if ts, err := transcript.ReadTranscript(result.TranscriptPath, maxHistory); err == nil && ts != nil {
			// Convert tool history
			result.ToolHistory = make([]ToolHistoryEntry, len(ts.ToolHistory))
			for i, tc := range ts.ToolHistory {
				result.ToolHistory[i] = ToolHistoryEntry{
					Tool:      tc.Name,
					Summary:   transcript.GetToolSummary(&tc),
					Result:    tc.Result,
					Timestamp: tc.Timestamp,
				}
			}

			// Get last prompt
			if ts.LastPrompt != nil {
				result.LastPrompt = ts.LastPrompt.Content
			}

			// Build summary from current state
			if result.Tool != "" {
				// Currently running a tool - build summary from tool name
				result.Summary = result.Tool
			} else if len(result.ToolHistory) > 0 {
				// Use most recent tool as context
				result.Summary = result.ToolHistory[0].Summary
			}

			// Use transcript's last active if more recent
			if ts.LastActive.After(result.LastActive) {
				result.LastActive = ts.LastActive
			}
		}
	}

	return result
}

// CleanupStatus removes the events file for a session.
func CleanupStatus(sessionName string) error {
	eventsFile := filepath.Join(EventsDir(), sessionName+".jsonl")
	err := os.Remove(eventsFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// readLastEvent reads the last line from a JSONL events file and parses it.
func readLastEvent(path string) (*HookEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lastLine string
	scanner := bufio.NewScanner(file)
	// Increase buffer for potentially large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if lastLine == "" {
		return nil, os.ErrNotExist
	}

	var event HookEvent
	if err := json.Unmarshal([]byte(lastLine), &event); err != nil {
		return nil, err
	}

	return &event, nil
}
