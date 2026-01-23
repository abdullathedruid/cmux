package input

import (
	"sync"
)

// Handler manages mode state and input buffer for text input.
type Handler struct {
	mode        Mode
	inputBuffer string
	mu          sync.RWMutex
}

// NewHandler creates a new input handler in normal mode.
func NewHandler() *Handler {
	return &Handler{
		mode: ModeNormal,
	}
}

// Mode returns the current input mode.
func (h *Handler) Mode() Mode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mode
}

// SetMode changes the current input mode.
func (h *Handler) SetMode(mode Mode) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode = mode
}

// EnterTerminalMode switches to terminal mode.
func (h *Handler) EnterTerminalMode() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode = ModeTerminal
}

// EnterNormalMode switches to normal mode.
func (h *Handler) EnterNormalMode() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode = ModeNormal
}

// EnterInputMode switches to input mode and clears the buffer.
func (h *Handler) EnterInputMode() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode = ModeInput
	h.inputBuffer = ""
}

// ExitInputMode exits input mode, returns to normal, and clears buffer.
func (h *Handler) ExitInputMode() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mode = ModeNormal
	h.inputBuffer = ""
}

// InputBuffer returns the current input buffer contents.
func (h *Handler) InputBuffer() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.inputBuffer
}

// SetInputBuffer sets the input buffer contents.
func (h *Handler) SetInputBuffer(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inputBuffer = s
}

// AppendToInputBuffer adds a character to the input buffer.
func (h *Handler) AppendToInputBuffer(ch rune) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inputBuffer += string(ch)
}

// BackspaceInputBuffer removes the last character from the buffer.
func (h *Handler) BackspaceInputBuffer() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.inputBuffer) > 0 {
		h.inputBuffer = h.inputBuffer[:len(h.inputBuffer)-1]
	}
}

// ConsumeInputBuffer returns and clears the input buffer.
func (h *Handler) ConsumeInputBuffer() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := h.inputBuffer
	h.inputBuffer = ""
	h.mode = ModeNormal
	return result
}
