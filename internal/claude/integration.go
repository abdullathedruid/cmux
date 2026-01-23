package claude

import (
	"os/exec"
	"strings"
)

// SendKeys sends input to a tmux session via tmux send-keys.
// This is used instead of terminal passthrough for the structured view.
func SendKeys(tmuxSession, keys string) error {
	args := []string{"send-keys", "-t", tmuxSession, keys}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

// SendText sends text input followed by Enter to a tmux session.
func SendText(tmuxSession, text string) error {
	args := []string{"send-keys", "-t", tmuxSession, "-l", text}
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Send Enter
	return SendKeys(tmuxSession, "Enter")
}

// SendPermissionResponse sends a permission response (y/n/a) to a tmux session.
func SendPermissionResponse(tmuxSession string, allow bool) error {
	key := "n"
	if allow {
		key = "y"
	}
	return SendKeys(tmuxSession, key)
}

// SendEscape sends Escape key to a tmux session.
func SendEscape(tmuxSession string) error {
	return SendKeys(tmuxSession, "Escape")
}

// SendInterrupt sends Ctrl-C to a tmux session.
func SendInterrupt(tmuxSession string) error {
	return SendKeys(tmuxSession, "C-c")
}

// ViewMode represents the display mode for a pane.
type ViewMode int

const (
	// ViewModeTerminal uses traditional terminal emulation
	ViewModeTerminal ViewMode = iota
	// ViewModeStructured uses the structured Claude view
	ViewModeStructured
)

// PaneAdapter adapts a Claude View to work with the existing pane system.
// This allows gradual migration - panes can be either terminal or structured.
type PaneAdapter struct {
	view        *View
	tmuxSession string
	mode        ViewMode
}

// NewPaneAdapter creates a new adapter for a Claude view.
func NewPaneAdapter(tmuxSession string, width, height int) *PaneAdapter {
	return &PaneAdapter{
		view:        NewView(tmuxSession, width, height),
		tmuxSession: tmuxSession,
		mode:        ViewModeStructured,
	}
}

// Render returns the current view content.
func (p *PaneAdapter) Render() string {
	return p.view.Render()
}

// Resize updates the view dimensions.
func (p *PaneAdapter) Resize(width, height int) {
	p.view.Resize(width, height)
}

// HandleKey processes a key press.
// Returns true if the key was handled, false if it should be passed through.
func (p *PaneAdapter) HandleKey(key string) bool {
	session := p.view.Session()
	if session == nil {
		return false
	}

	// Handle permission prompts specially
	if session.Status == StatusNeedsInput {
		switch strings.ToLower(key) {
		case "y":
			SendPermissionResponse(p.tmuxSession, true)
			return true
		case "n":
			SendPermissionResponse(p.tmuxSession, false)
			return true
		case "a": // Allow always
			SendKeys(p.tmuxSession, "a")
			return true
		}
	}

	return false
}

// IsDirty returns true if the view needs re-rendering.
func (p *PaneAdapter) IsDirty() bool {
	return p.view.IsDirty()
}

// Session returns the current Claude session state.
func (p *PaneAdapter) Session() *Session {
	return p.view.Session()
}

// View returns the underlying Claude view.
func (p *PaneAdapter) View() *View {
	return p.view
}

// TmuxSession returns the tmux session name.
func (p *PaneAdapter) TmuxSession() string {
	return p.tmuxSession
}
