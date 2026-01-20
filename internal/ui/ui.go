// Package ui provides shared UI components for cmux.
package ui

import (
	"fmt"
	"strings"

	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/mattn/go-runewidth"
)

// Colors and styles for the TUI
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// StatusIcon returns the icon for a session status.
func StatusIcon(attached bool, status state.SessionStatus) string {
	if attached {
		return "●" // Filled circle for attached
	}
	switch status {
	case state.StatusActive:
		return "◐" // Half circle for active
	case state.StatusTool:
		return "⚙" // Gear for tool use
	case state.StatusThinking:
		return "◑" // Other half for thinking
	default:
		return "○" // Empty circle for idle
	}
}

// StatusColor returns the color for a session status.
func StatusColor(attached bool, status state.SessionStatus) string {
	if attached {
		return ColorGreen
	}
	switch status {
	case state.StatusActive, state.StatusThinking:
		return ColorYellow
	case state.StatusTool:
		return ColorCyan
	default:
		return ColorWhite
	}
}

// StatusText returns the text for a session status.
func StatusText(attached bool, status state.SessionStatus) string {
	if attached {
		return "ATTACHED"
	}
	switch status {
	case state.StatusActive:
		return "ACTIVE"
	case state.StatusTool:
		return "TOOL"
	case state.StatusThinking:
		return "THINKING"
	default:
		return "IDLE"
	}
}

// Card renders a session card for the dashboard view.
type Card struct {
	Title       string
	Status      string
	Icon        string
	LastActive  string
	CurrentTool string
	Note        string
	Width       int
	Selected    bool
}

// Render renders the card as a string slice (one per line).
func (c *Card) Render() []string {
	width := c.Width
	if width < 20 {
		width = 30
	}

	// Calculate inner width (accounting for borders and padding)
	innerWidth := width - 4 // 2 for borders, 2 for padding

	lines := make([]string, 0, 5)

	// Top border
	borderChar := "─"
	corner := "┌"
	endCorner := "┐"
	if c.Selected {
		corner = "┏"
		endCorner = "┓"
		borderChar = "━"
	}
	lines = append(lines, corner+c.Title+" "+strings.Repeat(borderChar, max(0, width-runewidth.StringWidth(c.Title)-3))+endCorner)

	// Status line with current tool if present
	statusLine := fmt.Sprintf("%s %s", c.Icon, c.Status)
	if c.CurrentTool != "" {
		statusLine = fmt.Sprintf("%s %s: %s", c.Icon, c.Status, c.CurrentTool)
	}
	lines = append(lines, c.borderLine(truncate(statusLine, innerWidth), innerWidth))

	// Last active line
	if c.LastActive != "" {
		lines = append(lines, c.borderLine(c.LastActive, innerWidth))
	} else {
		lines = append(lines, c.borderLine("", innerWidth))
	}

	// Note line (if present)
	if c.Note != "" {
		lines = append(lines, c.borderLine(Truncate(c.Note, innerWidth), innerWidth))
	} else {
		lines = append(lines, c.borderLine("", innerWidth))
	}

	// Bottom border
	bottomCorner := "└"
	bottomEndCorner := "┘"
	if c.Selected {
		bottomCorner = "┗"
		bottomEndCorner = "┛"
		borderChar = "━"
	}
	lines = append(lines, bottomCorner+strings.Repeat(borderChar, width-2)+bottomEndCorner)

	return lines
}

// borderLine creates a line with borders.
func (c *Card) borderLine(content string, innerWidth int) string {
	border := "│"
	if c.Selected {
		border = "┃"
	}
	contentWidth := runewidth.StringWidth(content)
	padding := innerWidth - contentWidth
	if padding < 0 {
		padding = 0
		content = runewidth.Truncate(content, innerWidth, "")
	}
	return border + " " + content + strings.Repeat(" ", padding) + " " + border
}

// Truncate shortens a string to fit in the given width.
func Truncate(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return runewidth.Truncate(s, width, "")
	}
	return runewidth.Truncate(s, width, "...")
}

// PadRight pads a string to the right.
func PadRight(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-sw)
}

// PadLeft pads a string to the left.
func PadLeft(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "")
	}
	return strings.Repeat(" ", width-sw) + s
}

// Center centers a string in the given width.
func Center(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return runewidth.Truncate(s, width, "")
	}
	padding := (width - sw) / 2
	return strings.Repeat(" ", padding) + s + strings.Repeat(" ", width-sw-padding)
}

// RenderStatusBar creates the bottom status bar content.
func RenderStatusBar(sessionCount, attachedCount, activeCount int, isDashboard bool) string {
	viewName := "dashboard"
	if !isDashboard {
		viewName = "list"
	}

	idleCount := sessionCount - attachedCount - activeCount
	stats := fmt.Sprintf("%d sessions │ %d attached │ %d active │ %d idle", sessionCount, attachedCount, activeCount, idleCount)
	help := "hjkl:nav enter:attach n:new ?:help v:" + viewName

	return stats + "        " + help
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// HelpText returns the help screen content.
func HelpText() string {
	return `cmux - Claude Session Manager

Navigation
  h/j/k/l or arrows  Navigate between sessions
  Enter              Attach to selected session
  Tab                Cycle panels (list view)
  1-9                Jump to session by number

Session Management
  n                  New session in current directory
  N                  New session wizard
  w                  Worktree picker
  d                  Delete selected session
  e                  Edit session note (Ctrl+S to save)
  r                  Refresh session list

Views
  v                  Toggle dashboard/list view
  /                  Search/filter sessions

Other
  ?                  Show this help
  q                  Quit cmux

Press any key to close this help...`
}

// WrapText wraps text to fit within the given width.
func WrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if runewidth.StringWidth(line) <= width {
			lines = append(lines, line)
			continue
		}

		// Wrap long lines
		for runewidth.StringWidth(line) > width {
			// Find a break point that fits within width
			breakIdx := 0
			currentWidth := 0
			lastSpace := -1
			for i, r := range line {
				rw := runewidth.RuneWidth(r)
				if currentWidth+rw > width {
					break
				}
				currentWidth += rw
				breakIdx = i + len(string(r))
				if r == ' ' {
					lastSpace = breakIdx
				}
			}
			if lastSpace > 0 {
				breakIdx = lastSpace
			}
			lines = append(lines, line[:breakIdx])
			line = strings.TrimSpace(line[breakIdx:])
		}
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// FormatDuration formats a duration for display.
func FormatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds ago", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm ago", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%dh ago", seconds/3600)
	}
	return fmt.Sprintf("%dd ago", seconds/86400)
}
