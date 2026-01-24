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

// RenderScrollback renders scrollback history lines to a gocui view.
func RenderScrollback(v *gocui.View, lines []string) {
	for i, line := range lines {
		if i > 0 {
			fmt.Fprintln(v)
		}
		fmt.Fprint(v, line)
	}
}

// ConfigurePaneView sets up a gocui view for a pane with proper styling.
func ConfigurePaneView(v *gocui.View, p *pane.Pane, isActive bool, mode input.Mode) {
	scrollPos := p.Scrollback.ScrollPos()
	scrollIndicator := ""
	if scrollPos > 0 {
		scrollIndicator = fmt.Sprintf(" [+%d]", scrollPos)
	}

	if isActive {
		v.Title = fmt.Sprintf(" [%s] %d: %s%s ", mode.String(), p.Index, p.Name, scrollIndicator)
		// Bold frame for active pane using heavy box-drawing characters
		v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
		// Color based on mode: blue for normal, green for terminal, yellow for scrolled
		if scrollPos > 0 {
			v.FrameColor = gocui.ColorYellow
		} else if mode.IsTerminal() {
			v.FrameColor = gocui.ColorGreen
		} else {
			v.FrameColor = gocui.ColorBlue
		}
	} else {
		v.Title = fmt.Sprintf(" %d: %s%s ", p.Index, p.Name, scrollIndicator)
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

// StatusBarContent returns the hotkey hints for the given mode.
func StatusBarContent(mode input.Mode, paneCount int) string {
	switch mode {
	case input.ModeTerminal:
		return " TERMINAL │ Ctrl+Q:normal mode │ All other keys sent to terminal"
	case input.ModeInput:
		return " INPUT │ Enter:confirm │ Esc:cancel │ Backspace:delete"
	default: // ModeNormal
		hints := " NORMAL │ hjkl:nav │ i/Enter:terminal │ Ctrl+U/D:scroll │ G:bottom │ N:new │ q:quit"
		if paneCount > 1 {
			hints = " NORMAL │ hjkl:nav │ 1-9:pane │ i:terminal │ Ctrl+U/D:scroll │ N:new │ q:quit"
		}
		return hints
	}
}

// ConfigureStatusBar sets up the status bar view with proper styling.
func ConfigureStatusBar(v *gocui.View, mode input.Mode, paneCount int) {
	v.Frame = false
	v.Wrap = false
	v.Editable = false

	// Transparent background with mode-colored text
	v.BgColor = gocui.ColorDefault
	switch mode {
	case input.ModeTerminal:
		v.FgColor = gocui.ColorGreen
	case input.ModeInput:
		v.FgColor = gocui.ColorYellow
	default:
		v.FgColor = gocui.ColorBlue
	}

	v.Clear()
	fmt.Fprint(v, StatusBarContent(mode, paneCount))
}
