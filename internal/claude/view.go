package claude

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"
)

// View represents a structured view of a Claude session.
// It replaces terminal emulation with parsed hook/transcript data.
type View struct {
	tmuxSession string
	session     *Session
	transcript  *TranscriptReader
	renderer    *Renderer
	mu          sync.RWMutex

	// Cached render output
	lastRender string
	dirty      bool

	// Dimensions
	width  int
	height int

	// Optional filter: only accept events from this cwd
	cwdFilter string
}

// NewView creates a new Claude view for a tmux session.
func NewView(tmuxSession string, width, height int) *View {
	return &View{
		tmuxSession: tmuxSession,
		session: &Session{
			TmuxSession: tmuxSession,
			Status:      StatusIdle,
			Messages:    make([]Message, 0),
		},
		renderer: NewRenderer(width, height),
		width:    width,
		height:   height,
		dirty:    true,
	}
}

// Resize updates the view dimensions.
func (v *View) Resize(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.width != width || v.height != height {
		v.width = width
		v.height = height
		v.renderer.Resize(width, height)
		v.dirty = true
	}
}

// Dimensions returns the current view dimensions.
func (v *View) Dimensions() (width, height int) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.width, v.height
}

// Session returns the current session state.
func (v *View) Session() *Session {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.session
}

// UpdateFromStatus updates the view from a status file update.
func (v *View) UpdateFromStatus(status SessionStatus, tool string, sessionID, transcriptPath string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.session.ID = sessionID
	v.session.Status = status
	v.session.LastUpdate = time.Now()

	if transcriptPath != "" && v.session.TranscriptPath != transcriptPath {
		v.session.TranscriptPath = transcriptPath
		v.transcript = NewTranscriptReader(transcriptPath)
	}

	if tool != "" && status == StatusTool {
		v.session.CurrentTool = &ToolCall{
			Name:      tool,
			Status:    ToolRunning,
			StartTime: time.Now(),
		}
	} else if status != StatusTool {
		v.session.CurrentTool = nil
	}

	v.dirty = true
}

// SetCwdFilter sets a working directory filter.
// Only events from Claude sessions with a matching cwd will be accepted.
func (v *View) SetCwdFilter(cwd string) {
	v.mu.Lock()
	v.cwdFilter = cwd
	v.mu.Unlock()
}

// UpdateFromHookEvent updates the view from a hook event.
func (v *View) UpdateFromHookEvent(event HookEvent) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Filter by cwd if set
	if v.cwdFilter != "" && event.Cwd != "" && event.Cwd != v.cwdFilter {
		return // Ignore events from different projects
	}

	// Once we have a session ID, only accept events from that same Claude session
	// This prevents mixing events from different Claude instances in the same tmux pane
	if v.session.ID != "" && event.SessionID != "" && v.session.ID != event.SessionID {
		return // Ignore events from different Claude sessions
	}

	v.session.ID = event.SessionID
	v.session.Cwd = event.Cwd
	v.session.PermissionMode = event.PermissionMode
	v.session.LastUpdate = time.Now()

	if event.TranscriptPath != "" && v.session.TranscriptPath != event.TranscriptPath {
		v.session.TranscriptPath = event.TranscriptPath
		v.transcript = NewTranscriptReader(event.TranscriptPath)
	}

	switch event.EventName {
	case "UserPromptSubmit":
		v.session.Status = StatusThinking
		// Add user message immediately
		v.session.Messages = append(v.session.Messages, Message{
			Role:      "user",
			Content:   event.Prompt,
			Timestamp: time.Now(),
		})

	case "PreToolUse":
		v.session.Status = StatusTool
		v.session.CurrentTool = &ToolCall{
			ID:        event.ToolUseID,
			Name:      event.ToolName,
			Status:    ToolRunning,
			StartTime: time.Now(),
			Input:     event.ToolInput,
		}
		v.session.CurrentTool.InputSummary = summarizeToolInput(event.ToolName, event.ToolInput)

	case "PostToolUse":
		v.session.Status = StatusActive
		if v.session.CurrentTool != nil && v.session.CurrentTool.ID == event.ToolUseID {
			v.session.CurrentTool.Status = ToolComplete
			v.session.CurrentTool.EndTime = time.Now()
			v.session.CurrentTool.Response = event.ToolResponse
		}
		v.session.CurrentTool = nil

	case "PermissionRequest":
		v.session.Status = StatusNeedsInput
		v.session.PendingPermission = &PermissionRequest{
			ToolName:    event.ToolName,
			ToolInput:   event.ToolInput,
			Suggestions: event.PermissionSuggestions,
		}

	case "Notification":
		if event.NotificationType == "permission_prompt" {
			v.session.Status = StatusNeedsInput
			if v.session.PendingPermission != nil {
				v.session.PendingPermission.Message = event.Message
			} else {
				// Notification arrived before PermissionRequest, create placeholder
				v.session.PendingPermission = &PermissionRequest{
					Message: event.Message,
				}
			}
		}

	case "Stop", "SubagentStop":
		v.session.Status = StatusIdle
		v.session.CurrentTool = nil
		v.session.PendingPermission = nil
	}

	v.dirty = true
}

// PollTranscript reads new entries from the transcript file.
func (v *View) PollTranscript() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.transcript == nil {
		return nil
	}

	newMessages, err := v.transcript.Poll()
	if err != nil {
		return err
	}

	if len(newMessages) > 0 {
		// Merge with existing messages (transcript reader handles dedup)
		v.session.Messages = v.transcript.Messages()
		v.dirty = true
	}

	return nil
}

// Render returns the rendered view content.
func (v *View) Render() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.dirty {
		v.lastRender = v.renderer.Render(v.session)
		v.dirty = false
	}

	return v.lastRender
}

// IsDirty returns true if the view needs re-rendering.
func (v *View) IsDirty() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.dirty
}

// MarkDirty marks the view as needing re-render.
func (v *View) MarkDirty() {
	v.mu.Lock()
	v.dirty = true
	v.mu.Unlock()
}

// summarizeToolInput creates a short description of a tool call.
func summarizeToolInput(toolName string, input []byte) string {
	if len(input) == 0 {
		return toolName
	}

	// Parse just what we need for summary
	var data map[string]any
	if err := jsonUnmarshal(input, &data); err != nil {
		return toolName
	}

	switch toolName {
	case "Bash":
		if cmd, ok := data["command"].(string); ok {
			if len(cmd) > 60 {
				return "$ " + cmd[:57] + "..."
			}
			return "$ " + cmd
		}

	case "Read":
		if path, ok := data["file_path"].(string); ok {
			return "Read: " + filepath.Base(path)
		}

	case "Edit":
		if path, ok := data["file_path"].(string); ok {
			return "Edit: " + filepath.Base(path)
		}

	case "Write":
		if path, ok := data["file_path"].(string); ok {
			return "Write: " + filepath.Base(path)
		}

	case "Glob":
		if pattern, ok := data["pattern"].(string); ok {
			return "Glob: " + pattern
		}

	case "Grep":
		if pattern, ok := data["pattern"].(string); ok {
			return "Grep: " + pattern
		}

	case "Task":
		if desc, ok := data["description"].(string); ok {
			return "Task: " + desc
		}
	}

	return toolName
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
