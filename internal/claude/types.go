// Package claude provides structured parsing and rendering of Claude Code sessions.
// Instead of terminal emulation, this package reads hook events and transcript files
// to present a structured chat view.
package claude

import (
	"encoding/json"
	"time"
)

// SessionStatus represents the current state of a Claude session.
type SessionStatus string

const (
	StatusIdle       SessionStatus = "idle"        // Stop received, waiting for input
	StatusThinking   SessionStatus = "thinking"    // After user prompt, before tools
	StatusTool       SessionStatus = "tool"        // Currently executing a tool
	StatusNeedsInput SessionStatus = "needs_input" // Permission request pending
	StatusActive     SessionStatus = "active"      // Processing (between tools)
)

// Session represents the state of a Claude Code session.
type Session struct {
	ID             string        `json:"session_id"`
	TranscriptPath string        `json:"transcript_path"`
	TmuxSession    string        `json:"tmux_session"`
	Cwd            string        `json:"cwd"`
	PermissionMode string        `json:"permission_mode"`
	Status         SessionStatus `json:"status"`
	LastUpdate     time.Time     `json:"last_update"`

	// Current activity (when Status == StatusTool)
	CurrentTool *ToolCall `json:"current_tool,omitempty"`

	// Permission request details (when Status == StatusNeedsInput)
	PendingPermission *PermissionRequest `json:"pending_permission,omitempty"`

	// Conversation history (lightweight summaries)
	Messages []Message `json:"messages"`
}

// Message represents a single turn in the conversation.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Timestamp time.Time `json:"timestamp"`

	// For user messages
	Content string `json:"content,omitempty"`

	// For assistant messages
	TextPreview string     `json:"text_preview,omitempty"` // First ~500 chars
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	IsComplete  bool       `json:"is_complete"` // stop_reason == "end_turn"

	// Token usage (assistant only)
	Usage *Usage `json:"usage,omitempty"`
}

// ToolCall represents a tool invocation.
type ToolCall struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Status       ToolStatus      `json:"status"`
	InputSummary string          `json:"input_summary"` // e.g., "Edit: /path/to/file.go"
	Error        string          `json:"error,omitempty"`
	StartTime    time.Time       `json:"start_time,omitempty"`
	EndTime      time.Time       `json:"end_time,omitempty"`
	Input        json.RawMessage `json:"-"` // Full input, not serialized by default
	Response     json.RawMessage `json:"-"` // Full response, not serialized by default
}

// ToolStatus represents the state of a tool call.
type ToolStatus string

const (
	ToolPending  ToolStatus = "pending"
	ToolRunning  ToolStatus = "running"
	ToolComplete ToolStatus = "complete"
	ToolFailed   ToolStatus = "failed"
)

// PermissionRequest represents a pending permission prompt.
type PermissionRequest struct {
	ToolName    string            `json:"tool_name"`
	ToolInput   json.RawMessage   `json:"tool_input"`
	Message     string            `json:"message"`
	Suggestions []json.RawMessage `json:"suggestions,omitempty"`
}

// Usage contains token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// HookEvent represents an incoming event from the hook system.
type HookEvent struct {
	// Added by hook script
	TmuxSession string    `json:"tmux_session,omitempty"`
	TS          string    `json:"ts,omitempty"` // ISO timestamp string from hook
	Timestamp   time.Time `json:"-"`            // Parsed timestamp (not from JSON)

	// From Claude Code
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	EventName      string `json:"hook_event_name"`

	// Tool events
	ToolName     string          `json:"tool_name,omitempty"`
	ToolInput    json.RawMessage `json:"tool_input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`

	// User prompt
	Prompt string `json:"prompt,omitempty"`

	// Stop events
	StopHookActive bool `json:"stop_hook_active,omitempty"`

	// Subagent events
	AgentID             string `json:"agent_id,omitempty"`
	AgentTranscriptPath string `json:"agent_transcript_path,omitempty"`

	// Notification
	Message          string `json:"message,omitempty"`
	NotificationType string `json:"notification_type,omitempty"`

	// Permission suggestions
	PermissionSuggestions []json.RawMessage `json:"permission_suggestions,omitempty"`
}

// ParseTimestamp parses the TS field into Timestamp.
func (e *HookEvent) ParseTimestamp() {
	if e.TS != "" && e.Timestamp.IsZero() {
		if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
			e.Timestamp = t
		}
	}
}
