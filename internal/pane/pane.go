package pane

import (
	"fmt"

	"github.com/abdullathedruid/cmux/internal/terminal"
)

// Pane represents a single pane with its tmux control mode connection and terminal emulator.
type Pane struct {
	Index    int
	Name     string
	Ctrl     *terminal.ControlMode
	Term     *SafeTerminal
	ViewName string
}

// New creates a new Pane with the given session name and dimensions.
// The caller is responsible for starting the control mode connection.
func New(index int, sessionName string, width, height int) *Pane {
	// Ensure minimum dimensions
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	return &Pane{
		Index:    index,
		Name:     sessionName,
		Ctrl:     terminal.NewControlMode(sessionName),
		Term:     NewSafeTerminal(height, width),
		ViewName: fmt.Sprintf("pane-%d", index-1), // 0-indexed view name
	}
}

// Start starts the control mode connection for this pane.
func (p *Pane) Start(width, height int) error {
	return p.Ctrl.Start(width, height)
}

// Close closes the control mode connection.
func (p *Pane) Close() error {
	return p.Ctrl.Close()
}

// Resize resizes the terminal and notifies tmux.
func (p *Pane) Resize(width, height int) {
	p.Term.Resize(height, width) // midterm uses (rows, cols)
	p.Ctrl.Resize(width, height)
}

// OutputChan returns the channel that receives terminal output data.
func (p *Pane) OutputChan() <-chan []byte {
	return p.Ctrl.OutputChan()
}

// WriteToTerminal writes data to the terminal buffer.
func (p *Pane) WriteToTerminal(data []byte) {
	p.Term.Write(data)
}
