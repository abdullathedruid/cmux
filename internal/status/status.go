// Package status reads Claude session status from hook-written files.
package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StatusDir returns the directory where status files are stored.
func StatusDir() string {
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "cmux", "sessions")
}

// FileStatus represents the status written by the hook script.
type FileStatus struct {
	Status  string `json:"status"`
	Tool    string `json:"tool,omitempty"`
	Summary string `json:"summary,omitempty"`
	TS      int64  `json:"ts"`
}

// ReadStatus reads the status for a given tmux session name.
// Returns the status string, current tool (if any), tool summary, last active time, and whether the status was found.
func ReadStatus(sessionName string) (status string, tool string, summary string, lastActive time.Time, found bool) {
	statusFile := filepath.Join(StatusDir(), sessionName+".status")

	data, err := os.ReadFile(statusFile)
	if err != nil {
		return "idle", "", "", time.Time{}, false
	}

	var fs FileStatus
	if err := json.Unmarshal(data, &fs); err != nil {
		return "idle", "", "", time.Time{}, false
	}

	// Convert timestamp to time.Time
	if fs.TS > 0 {
		lastActive = time.Unix(fs.TS, 0)
	}

	// Check if status is stale (older than 30 seconds = probably not running)
	if fs.TS > 0 {
		age := time.Now().Unix() - fs.TS
		if age > 30 {
			return "idle", "", "", lastActive, true
		}
	}

	return fs.Status, fs.Tool, fs.Summary, lastActive, true
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
