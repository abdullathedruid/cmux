// Package controller provides view controllers for the cmux TUI.
package controller

import (
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/abdullathedruid/cmux/internal/tmux"
)

// Controller is the interface for view controllers.
type Controller interface {
	// Name returns the view name for this controller.
	Name() string
	// Layout sets up the view dimensions.
	Layout(g *gocui.Gui) error
	// Keybindings sets up view-specific keybindings.
	Keybindings(g *gocui.Gui) error
	// Render renders the view content.
	Render(g *gocui.Gui) error
}

// Context provides shared context for all controllers.
type Context struct {
	State         *state.State
	TmuxClient    tmux.Client
	OnAttach      func(sessionName string) error
	OnPopupAttach func(sessionName string) error
	OnShowDiff    func(sessionName string) error
	OnNew         func() error
	OnDelete      func(sessionName string) error
	OnRefresh     func() error
	OnQuit        func() error
	OnToggleView  func()
	OnShowHelp    func()
}

// NewContext creates a new controller context.
func NewContext(s *state.State, t tmux.Client) *Context {
	return &Context{
		State:      s,
		TmuxClient: t,
	}
}
