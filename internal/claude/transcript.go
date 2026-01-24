package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// TranscriptReader reads and parses a Claude Code transcript file.
// It maintains an offset for incremental reading and deduplicates streaming messages.
type TranscriptReader struct {
	path     string
	offset   int64
	messages []Message
	mu       sync.Mutex

	// Deduplication: track seen message IDs
	seenMsgIDs map[string]int // msgID -> index in messages
}

// NewTranscriptReader creates a reader for the given transcript path.
func NewTranscriptReader(path string) *TranscriptReader {
	return &TranscriptReader{
		path:       path,
		messages:   make([]Message, 0),
		seenMsgIDs: make(map[string]int),
	}
}

// Poll reads new entries from the transcript.
// Returns newly added messages and whether any messages were modified (new or updated).
func (r *TranscriptReader) Poll() (newMessages []Message, hasChanges bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	file, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer file.Close()

	if _, err := file.Seek(r.offset, 0); err != nil {
		return nil, false, err
	}

	scanner := bufio.NewScanner(file)
	// Allow large lines (up to 16MB for tool results)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		r.offset += int64(len(line)) + 1 // +1 for newline

		// Quick type check to skip progress entries (they're huge and redundant)
		if bytes.Contains(line, []byte(`"type":"progress"`)) {
			continue
		}

		msg, isNew := r.parseLine(line)
		if msg.ID != "" {
			// Any parsed message (new or updated) counts as a change
			hasChanges = true
		}
		if isNew {
			newMessages = append(newMessages, msg)
		}
	}

	return newMessages, hasChanges, scanner.Err()
}

// Messages returns all parsed messages.
func (r *TranscriptReader) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return a copy
	result := make([]Message, len(r.messages))
	copy(result, r.messages)
	return result
}

// Reset clears all state and re-reads from the beginning.
func (r *TranscriptReader) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.offset = 0
	r.messages = make([]Message, 0)
	r.seenMsgIDs = make(map[string]int)
}

func (r *TranscriptReader) parseLine(line []byte) (Message, bool) {
	var entry struct {
		Type      string          `json:"type"`
		UUID      string          `json:"uuid"`
		Timestamp string          `json:"timestamp,omitempty"`
		Message   json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(line, &entry); err != nil {
		return Message{}, false
	}

	switch entry.Type {
	case "user":
		return r.parseUserMessage(entry.UUID, entry.Timestamp, entry.Message)
	case "assistant":
		return r.parseAssistantMessage(entry.UUID, entry.Timestamp, entry.Message)
	default:
		return Message{}, false
	}
}

func (r *TranscriptReader) parseUserMessage(uuid, timestamp string, raw json.RawMessage) (Message, bool) {
	var msg struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Message{}, false
	}

	ts, _ := time.Parse(time.RFC3339, timestamp)
	m := Message{
		ID:        uuid,
		Role:      "user",
		Timestamp: ts,
		Content:   msg.Content,
	}

	// Check for duplicate
	if idx, exists := r.seenMsgIDs[uuid]; exists {
		r.messages[idx] = m
		return m, false // Update, not new
	}

	r.seenMsgIDs[uuid] = len(r.messages)
	r.messages = append(r.messages, m)
	return m, true
}

func (r *TranscriptReader) parseAssistantMessage(uuid, timestamp string, raw json.RawMessage) (Message, bool) {
	var msg struct {
		ID         string            `json:"id"`
		StopReason *string           `json:"stop_reason"`
		Content    []json.RawMessage `json:"content"`
		Usage      *Usage            `json:"usage"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Message{}, false
	}

	// Use Anthropic message ID for deduplication (handles streaming)
	msgID := msg.ID
	if msgID == "" {
		msgID = uuid
	}

	ts, _ := time.Parse(time.RFC3339, timestamp)
	m := Message{
		ID:         msgID,
		Role:       "assistant",
		Timestamp:  ts,
		IsComplete: msg.StopReason != nil && *msg.StopReason == "end_turn",
		Usage:      msg.Usage,
	}

	// Parse content blocks
	for _, block := range msg.Content {
		var blockType struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(block, &blockType); err != nil {
			continue
		}

		switch blockType.Type {
		case "text":
			var t struct {
				Text string `json:"text"`
			}
			json.Unmarshal(block, &t)
			m.TextPreview += t.Text

		case "tool_use":
			var t struct {
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			json.Unmarshal(block, &t)
			m.ToolCalls = append(m.ToolCalls, ToolCall{
				ID:           t.ID,
				Name:         t.Name,
				Status:       ToolComplete,
				InputSummary: SummarizeToolInput(t.Name, t.Input),
			})
		}
	}

	// Check for duplicate/update (streaming sends multiple entries with same ID)
	if idx, exists := r.seenMsgIDs[msgID]; exists {
		r.messages[idx] = m
		return m, false // Update, not new
	}

	r.seenMsgIDs[msgID] = len(r.messages)
	r.messages = append(r.messages, m)
	return m, true
}
