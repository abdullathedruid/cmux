// Package ui provides gocui view management and rendering utilities.
package ui

import (
	"fmt"
	"strings"

	"github.com/abdullathedruid/cmux/internal/input"
	"github.com/abdullathedruid/cmux/internal/pane"
	"github.com/jesseduffield/gocui"
)

// RenderTerminal renders a SafeTerminal's content to a gocui view.
// Recovers from panics that can occur during resize race conditions.
func RenderTerminal(v *gocui.View, term *pane.SafeTerminal) {
	// Recover from panics during resize race conditions
	defer func() {
		if r := recover(); r != nil {
			// Silently ignore - will redraw on next update
		}
	}()

	var sb strings.Builder
	if err := term.Render(&sb); err != nil {
		return
	}
	fmt.Fprint(v, sb.String())
}

// ConfigurePaneView sets up a gocui view for a pane with proper styling.
func ConfigurePaneView(v *gocui.View, p *pane.Pane, isActive bool, mode input.Mode) {
	if isActive {
		v.Title = fmt.Sprintf(" [%s] %d: %s ", mode.String(), p.Index, p.Name)
		// Bold frame for active pane using heavy box-drawing characters
		v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
		// Color based on mode: blue for normal, green for terminal
		if mode.IsTerminal() {
			v.FrameColor = gocui.ColorGreen
		} else {
			v.FrameColor = gocui.ColorBlue
		}
	} else {
		v.Title = fmt.Sprintf(" %d: %s ", p.Index, p.Name)
		// Regular frame for inactive panes
		v.FrameRunes = []rune{'─', '│', '┌', '┐', '└', '┘'}
		v.FrameColor = gocui.ColorDefault
	}
	v.Frame = true
	v.Wrap = false
	v.Editable = mode.IsTerminal() && isActive
}

// ConfigureInputModal sets up the input modal view.
func ConfigureInputModal(v *gocui.View, inputBuffer string) {
	v.Title = " New Worktree (Enter=confirm, Esc=cancel) "
	v.Frame = true
	v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
	v.FrameColor = gocui.ColorYellow
	v.Editable = true
	v.Clear()
	fmt.Fprintf(v, " %s", inputBuffer)
}

// ModalDimensions calculates centered modal dimensions.
func ModalDimensions(maxX, maxY, width, height int) (x0, y0, x1, y1 int) {
	x0 = (maxX - width) / 2
	y0 = (maxY - height) / 2
	x1 = x0 + width
	y1 = y0 + height
	return
}
