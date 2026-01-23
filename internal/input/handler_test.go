package input

import "testing"

func TestHandler_ModeTransitions(t *testing.T) {
	h := NewHandler()

	// Should start in normal mode
	if h.Mode() != ModeNormal {
		t.Error("NewHandler should start in ModeNormal")
	}

	// Enter terminal mode
	h.EnterTerminalMode()
	if h.Mode() != ModeTerminal {
		t.Error("EnterTerminalMode should set ModeTerminal")
	}

	// Enter normal mode
	h.EnterNormalMode()
	if h.Mode() != ModeNormal {
		t.Error("EnterNormalMode should set ModeNormal")
	}

	// Enter input mode
	h.EnterInputMode()
	if h.Mode() != ModeInput {
		t.Error("EnterInputMode should set ModeInput")
	}

	// Exit input mode
	h.ExitInputMode()
	if h.Mode() != ModeNormal {
		t.Error("ExitInputMode should return to ModeNormal")
	}
}

func TestHandler_InputBuffer(t *testing.T) {
	h := NewHandler()
	h.EnterInputMode()

	// Initially empty
	if h.InputBuffer() != "" {
		t.Error("InputBuffer should be empty initially")
	}

	// Append characters
	h.AppendToInputBuffer('a')
	h.AppendToInputBuffer('b')
	h.AppendToInputBuffer('c')
	if h.InputBuffer() != "abc" {
		t.Errorf("InputBuffer = %q, want %q", h.InputBuffer(), "abc")
	}

	// Backspace
	h.BackspaceInputBuffer()
	if h.InputBuffer() != "ab" {
		t.Errorf("InputBuffer = %q, want %q", h.InputBuffer(), "ab")
	}

	// Consume should return and clear
	result := h.ConsumeInputBuffer()
	if result != "ab" {
		t.Errorf("ConsumeInputBuffer = %q, want %q", result, "ab")
	}
	if h.InputBuffer() != "" {
		t.Error("InputBuffer should be empty after consume")
	}
	if h.Mode() != ModeNormal {
		t.Error("Mode should be normal after consume")
	}
}

func TestHandler_BackspaceEmpty(t *testing.T) {
	h := NewHandler()
	h.EnterInputMode()

	// Backspace on empty buffer should be safe
	h.BackspaceInputBuffer()
	if h.InputBuffer() != "" {
		t.Error("Backspace on empty buffer should keep it empty")
	}
}
