// Package input provides modal input handling with vim-like keybindings.
package input

// Mode represents the current input mode.
type Mode int

const (
	// ModeNormal is the default mode for navigation and commands.
	ModeNormal Mode = iota
	// ModeTerminal forwards all input to the active terminal.
	ModeTerminal
	// ModeInput is for text input (e.g., new worktree name).
	ModeInput
)

// String returns the human-readable mode name.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeTerminal:
		return "TERMINAL"
	case ModeInput:
		return "INPUT"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal returns true if the mode forwards input to the terminal.
func (m Mode) IsTerminal() bool {
	return m == ModeTerminal
}

// IsNormal returns true if the mode is normal (navigation) mode.
func (m Mode) IsNormal() bool {
	return m == ModeNormal
}

// IsInput returns true if the mode is text input mode.
func (m Mode) IsInput() bool {
	return m == ModeInput
}
