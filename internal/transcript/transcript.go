// Package transcript reads Claude Code JSONL transcript files.
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// ToolCall represents a tool invocation extracted from the transcript.
type ToolCall struct {
	ID        string    // tool_use_id
	Name      string    // tool name (Bash, Read, Edit, etc.)
	Input     ToolInput // parsed tool input
	Result    string    // summarized result
	Success   bool      // whether tool succeeded
	Timestamp time.Time
}

// ToolInput contains common tool input fields.
type ToolInput struct {
	// Bash
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`

	// Read/Edit/Write
	FilePath string `json:"file_path,omitempty"`

	// Grep/Glob
	Pattern string `json:"pattern,omitempty"`

	// Task
	TaskDescription string `json:"description,omitempty"`

	// WebFetch
	URL string `json:"url,omitempty"`

	// WebSearch
	Query string `json:"query,omitempty"`
}

// UserPrompt represents a user message from the transcript.
type UserPrompt struct {
	Content   string
	Timestamp time.Time
}

// TranscriptSummary contains extracted information from a transcript.
type TranscriptSummary struct {
	ToolHistory []ToolCall   // recent tool calls (newest first)
	LastPrompt  *UserPrompt  // most recent user prompt
	LastActive  time.Time    // timestamp of most recent activity
	MessageCount int         // total messages in transcript
}

// message represents a line in the JSONL transcript.
type message struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
	Prompt    string          `json:"prompt,omitempty"` // for UserPromptSubmit in progress messages
}

// assistantMessage represents the message field for assistant messages.
type assistantMessage struct {
	Role    string        `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock represents a content block in assistant messages.
type contentBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// userMessage represents the message field for user messages.
type userMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// toolResultBlock represents a tool result in user messages.
type toolResultMessage struct {
	Role    string             `json:"role"`
	Content []toolResultContent `json:"content"`
}

type toolResultContent struct {
	Type       string `json:"type"`
	ToolUseID  string `json:"tool_use_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ReadTranscript reads a JSONL transcript and extracts summary information.
// It reads from the end of the file for efficiency, returning up to maxTools recent tool calls.
func ReadTranscript(path string, maxTools int) (*TranscriptSummary, error) {
	if path == "" {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	summary := &TranscriptSummary{
		ToolHistory: make([]ToolCall, 0, maxTools),
	}

	// Track tool calls by ID so we can match results
	pendingTools := make(map[string]*ToolCall)

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip malformed lines
		}

		summary.MessageCount++

		ts := parseTimestamp(msg.Timestamp)
		if ts.After(summary.LastActive) {
			summary.LastActive = ts
		}

		switch msg.Type {
		case "user":
			// Extract user prompt
			var um userMessage
			if err := json.Unmarshal(msg.Message, &um); err == nil && um.Content != "" {
				summary.LastPrompt = &UserPrompt{
					Content:   truncate(um.Content, 500),
					Timestamp: ts,
				}
			}

			// Also check for tool results
			var trm toolResultMessage
			if err := json.Unmarshal(msg.Message, &trm); err == nil {
				for _, tr := range trm.Content {
					if tr.Type == "tool_result" {
						if tc, ok := pendingTools[tr.ToolUseID]; ok {
							tc.Result = summarizeResult(tc.Name, tr.Content, tr.IsError)
							tc.Success = !tr.IsError
							delete(pendingTools, tr.ToolUseID)
						}
					}
				}
			}

		case "assistant":
			// Extract tool calls
			var am assistantMessage
			if err := json.Unmarshal(msg.Message, &am); err != nil {
				continue
			}

			for _, block := range am.Content {
				if block.Type == "tool_use" {
					var input ToolInput
					json.Unmarshal(block.Input, &input)

					tc := &ToolCall{
						ID:        block.ID,
						Name:      block.Name,
						Input:     input,
						Timestamp: ts,
						Success:   true, // assume success until we see error
					}

					// Add to history (will be reversed later)
					summary.ToolHistory = append(summary.ToolHistory, *tc)
					pendingTools[block.ID] = &summary.ToolHistory[len(summary.ToolHistory)-1]
				}
			}
		}
	}

	// Reverse to get newest first
	for i, j := 0, len(summary.ToolHistory)-1; i < j; i, j = i+1, j-1 {
		summary.ToolHistory[i], summary.ToolHistory[j] = summary.ToolHistory[j], summary.ToolHistory[i]
	}

	// Trim to maxTools
	if len(summary.ToolHistory) > maxTools {
		summary.ToolHistory = summary.ToolHistory[:maxTools]
	}

	return summary, scanner.Err()
}

// GetToolSummary returns a human-readable summary of a tool call.
func GetToolSummary(tc *ToolCall) string {
	switch tc.Name {
	case "Bash":
		if tc.Input.Command != "" {
			return "Running: " + truncate(tc.Input.Command, 100)
		}
		return "Bash command"
	case "Read":
		if tc.Input.FilePath != "" {
			return "Reading " + baseName(tc.Input.FilePath)
		}
		return "Reading file"
	case "Edit":
		if tc.Input.FilePath != "" {
			return "Editing " + baseName(tc.Input.FilePath)
		}
		return "Editing file"
	case "Write":
		if tc.Input.FilePath != "" {
			return "Writing " + baseName(tc.Input.FilePath)
		}
		return "Writing file"
	case "Grep":
		if tc.Input.Pattern != "" {
			return "Searching: " + truncate(tc.Input.Pattern, 50)
		}
		return "Searching"
	case "Glob":
		if tc.Input.Pattern != "" {
			return "Finding: " + truncate(tc.Input.Pattern, 50)
		}
		return "Finding files"
	case "Task":
		if tc.Input.TaskDescription != "" {
			return "Agent: " + truncate(tc.Input.TaskDescription, 50)
		}
		return "Running agent"
	case "WebFetch":
		if tc.Input.URL != "" {
			return "Fetching: " + truncate(tc.Input.URL, 50)
		}
		return "Fetching URL"
	case "WebSearch":
		if tc.Input.Query != "" {
			return "Searching: " + truncate(tc.Input.Query, 50)
		}
		return "Web search"
	case "TodoWrite":
		return "Updating todos"
	case "LSP":
		return "LSP operation"
	default:
		return tc.Name
	}
}

func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func summarizeResult(toolName, content string, isError bool) string {
	if isError {
		return "error"
	}
	if content == "" {
		return "done"
	}

	// Tool-specific summaries
	switch toolName {
	case "Bash":
		lines := countLines(content)
		if lines > 1 {
			return "done"
		}
		return truncate(content, 50)
	case "Grep":
		lines := countLines(content)
		if lines > 0 {
			return string(rune(lines)) + " matches"
		}
		return "no matches"
	case "Read":
		lines := countLines(content)
		return string(rune(lines)) + " lines"
	default:
		return "done"
	}
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
