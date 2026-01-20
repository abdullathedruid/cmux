// Package status reads Claude session status from hook-written files
// and enriches it with data from JSONL transcripts.
package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/abdullathedruid/cmux/internal/transcript"
)

// StatusDir returns the directory where status files are stored.
func StatusDir() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "cmux", "sessions")
}

// HookStatus represents the minimal status written by the hook script.
type HookStatus struct {
	Status         string `json:"status"`
	Tool           string `json:"tool,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	TS             int64  `json:"ts"`
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
	return ReadFullStatusWithHistory(sessionName, 10)
}

// ReadFullStatusWithHistory reads complete status with configurable history depth.
func ReadFullStatusWithHistory(sessionName string, maxHistory int) SessionStatus {
	statusFile := filepath.Join(StatusDir(), sessionName+".status")

	result := SessionStatus{
		Status: "idle",
		Found:  false,
	}

	// Read hook status file
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return result
	}

	var hs HookStatus
	if err := json.Unmarshal(data, &hs); err != nil {
		return result
	}

	result.Found = true
	result.Status = hs.Status
	result.Tool = hs.Tool
	result.SessionID = hs.SessionID
	result.TranscriptPath = hs.TranscriptPath

	// Convert timestamp to time.Time
	if hs.TS > 0 {
		result.LastActive = time.Unix(hs.TS, 0)
	}

	// Check if status is stale (older than 30 seconds = probably not running)
	if hs.TS > 0 {
		age := time.Now().Unix() - hs.TS
		if age > 30 {
			result.Status = "idle"
			result.Tool = ""
		}
	}

	// Read transcript for historical data
	if hs.TranscriptPath != "" && maxHistory > 0 {
		if ts, err := transcript.ReadTranscript(hs.TranscriptPath, maxHistory); err == nil && ts != nil {
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

// CleanupStatus removes the status file for a session.
func CleanupStatus(sessionName string) error {
	statusFile := filepath.Join(StatusDir(), sessionName+".status")
	err := os.Remove(statusFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
