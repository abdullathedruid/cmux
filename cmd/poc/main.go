// Package main provides a proof of concept for tmux control mode with a terminal emulator.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/jesseduffield/gocui"
	"github.com/vito/midterm"
)

// Pane represents a single pane with its control mode connection and terminal emulator.
type Pane struct {
	name     string
	ctrl     *ControlMode
	term     *SafeTerminal
	viewName string
}

// ControlMode manages a tmux -CC connection.
type ControlMode struct {
	session  string
	cmd      *exec.Cmd
	pty      *os.File
	outputCh chan []byte
	doneCh   chan struct{}
	mu       sync.Mutex
}

// SafeTerminal wraps midterm.Terminal with a mutex for thread safety.
type SafeTerminal struct {
	*midterm.Terminal
	mu sync.Mutex
}

// outputPattern matches "%output %<pane-id> <data>" lines.
// The line may be prefixed with DCS escape sequences like \033P1000p
var outputPattern = regexp.MustCompile(`%output %(\d+) (.*)$`)

// NewControlMode creates a new control mode connection.
func NewControlMode(session string) *ControlMode {
	return &ControlMode{
		session:  session,
		outputCh: make(chan []byte, 100),
		doneCh:   make(chan struct{}),
	}
}

// Start begins the tmux control mode connection.
func (c *ControlMode) Start(width, height int) error {
	c.cmd = exec.Command("tmux", "-CC", "attach-session", "-t", c.session)

	// Start with a PTY (tmux needs a real terminal)
	var err error
	c.pty, err = pty.Start(c.cmd)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	// Set the PTY size
	pty.Setsize(c.pty, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})

	// Monitor process exit
	go func() {
		c.cmd.Wait()
	}()

	// Start reading output
	go c.readOutput()

	// Force a full redraw by resizing to a different size first, then back
	// This tricks tmux into thinking the terminal changed and needs a full redraw
	c.Resize(width-1, height-1)
	c.Resize(width, height)

	return nil
}


// Resize tells tmux about our new window size.
func (c *ControlMode) Resize(width, height int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Resize the PTY
	pty.Setsize(c.pty, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})

	// Tell tmux to refresh with the new size
	cmd := fmt.Sprintf("refresh-client -C %d,%d\n", width, height)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// readOutput reads lines from tmux control mode and parses them.
func (c *ControlMode) readOutput() {
	defer close(c.outputCh)

	scanner := bufio.NewScanner(c.pty)
	// Increase buffer size for large outputs
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse %output lines
		if data, ok := parseOutputLine(line); ok {
			select {
			case c.outputCh <- data:
			case <-c.doneCh:
				return
			}
		}
	}
}

// parseOutputLine parses a "%output %N <data>" line and returns decoded data.
func parseOutputLine(line string) ([]byte, bool) {
	matches := outputPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}

	// matches[2] contains the octal-encoded data
	data := decodeOctal(matches[2])
	return data, true
}

// decodeOctal converts \NNN octal sequences to bytes.
func decodeOctal(s string) []byte {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+3 < len(s) {
			// Check if next 3 chars are octal digits
			if isOctalDigit(s[i+1]) && isOctalDigit(s[i+2]) && isOctalDigit(s[i+3]) {
				val, _ := strconv.ParseInt(s[i+1:i+4], 8, 32)
				result = append(result, byte(val))
				i += 4
				continue
			}
			// Handle \\ (escaped backslash)
			if i+1 < len(s) && s[i+1] == '\\' {
				result = append(result, '\\')
				i += 2
				continue
			}
		}
		result = append(result, s[i])
		i++
	}
	return result
}

func isOctalDigit(b byte) bool {
	return b >= '0' && b <= '7'
}

// SendKeys sends keys to the tmux session.
func (c *ControlMode) SendKeys(keys string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := fmt.Sprintf("send-keys -t %s %s\n", c.session, keys)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// SendLiteralKeys sends literal key input to tmux.
func (c *ControlMode) SendLiteralKeys(keys string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := fmt.Sprintf("send-keys -t %s -l %q\n", c.session, keys)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// Close terminates the control mode connection.
func (c *ControlMode) Close() error {
	close(c.doneCh)
	if c.pty != nil {
		c.pty.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}

// Layout represents the position and size of a pane.
type Layout struct {
	x0, y0, x1, y1 int
}

// calculateLayouts returns the layout for each pane based on the total count.
// Layout patterns:
// 1 pane:  [    1    ]
// 2 panes: [  1  ][  2  ]
// 3 panes: [  1  ][  2  ]
//
//	[      3      ]
//
// 4 panes: [  1  ][  2  ]
//
//	[  3  ][  4  ]
//
// 5 panes: [  1  ][  2  ][  3  ]
//
//	[    4    ][    5    ]
//
// etc.
func calculateLayouts(count, maxX, maxY int) []Layout {
	if count == 0 {
		return nil
	}

	layouts := make([]Layout, count)

	switch count {
	case 1:
		layouts[0] = Layout{0, 0, maxX - 1, maxY - 1}
	case 2:
		halfX := maxX / 2
		layouts[0] = Layout{0, 0, halfX - 1, maxY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, maxY - 1}
	case 3:
		halfX := maxX / 2
		halfY := maxY / 2
		layouts[0] = Layout{0, 0, halfX - 1, halfY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, halfY - 1}
		layouts[2] = Layout{0, halfY, maxX - 1, maxY - 1}
	case 4:
		halfX := maxX / 2
		halfY := maxY / 2
		layouts[0] = Layout{0, 0, halfX - 1, halfY - 1}
		layouts[1] = Layout{halfX, 0, maxX - 1, halfY - 1}
		layouts[2] = Layout{0, halfY, halfX - 1, maxY - 1}
		layouts[3] = Layout{halfX, halfY, maxX - 1, maxY - 1}
	default:
		// For 5+ panes: calculate rows and columns
		// Top row has ceil(n/2) panes, bottom row has floor(n/2) panes
		topCount := (count + 1) / 2
		bottomCount := count - topCount
		halfY := maxY / 2

		// Top row
		for i := range topCount {
			x0 := (maxX * i) / topCount
			x1 := (maxX * (i + 1)) / topCount
			layouts[i] = Layout{x0, 0, x1 - 1, halfY - 1}
		}

		// Bottom row
		for i := range bottomCount {
			x0 := (maxX * i) / bottomCount
			x1 := (maxX * (i + 1)) / bottomCount
			layouts[topCount+i] = Layout{x0, halfY, x1 - 1, maxY - 1}
		}
	}

	return layouts
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: poc <session-name> [session-name...]")
		os.Exit(1)
	}

	sessions := flag.Args()

	// Initialize gocui
	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode: gocui.OutputTrue,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing GUI: %v\n", err)
		os.Exit(1)
	}
	defer g.Close()

	maxX, maxY := g.Size()

	// Calculate layouts for all panes
	layouts := calculateLayouts(len(sessions), maxX, maxY)

	// Create panes
	panes := make([]*Pane, len(sessions))
	for i, session := range sessions {
		layout := layouts[i]
		// Account for border (2 chars each dimension)
		termWidth := layout.x1 - layout.x0 - 1
		termHeight := layout.y1 - layout.y0 - 1
		if termWidth < 1 {
			termWidth = 80
		}
		if termHeight < 1 {
			termHeight = 24
		}

		term := &SafeTerminal{Terminal: midterm.NewTerminal(termHeight, termWidth)}
		ctrl := NewControlMode(session)

		if err := ctrl.Start(termWidth, termHeight); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting control mode for %s: %v\n", session, err)
			// Clean up already created panes
			for j := range i {
				panes[j].ctrl.Close()
			}
			os.Exit(1)
		}

		panes[i] = &Pane{
			name:     session,
			ctrl:     ctrl,
			term:     term,
			viewName: fmt.Sprintf("pane-%d", i),
		}
	}

	// Ensure all panes are closed on exit
	defer func() {
		for _, p := range panes {
			p.ctrl.Close()
		}
	}()

	// Process output goroutines for each pane
	for _, p := range panes {
		pane := p // capture for goroutine
		go func() {
			for data := range pane.ctrl.outputCh {
				pane.term.mu.Lock()
				pane.term.Write(data)
				pane.term.mu.Unlock()
				g.Update(func(g *gocui.Gui) error { return nil })
			}
		}()
	}

	// Set up layout
	g.SetManagerFunc(multiPaneLayoutFunc(panes))

	// Set up keybindings
	if err := setupMultiPaneKeybindings(g, panes); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up keybindings: %v\n", err)
		os.Exit(1)
	}

	// Handle SIGINT/SIGTERM to ensure clean exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		g.Update(func(g *gocui.Gui) error {
			return gocui.ErrQuit
		})
	}()

	// Run main loop
	if err := g.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) && err.Error() != "quit" {
		fmt.Fprintf(os.Stderr, "Error in main loop: %v\n", err)
		os.Exit(1)
	}
}

func multiPaneLayoutFunc(panes []*Pane) func(*gocui.Gui) error {
	firstCall := true
	lastMaxX, lastMaxY := 0, 0
	lastLayouts := make([]Layout, len(panes))

	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()

		// Recalculate layouts if size changed
		var layouts []Layout
		if maxX != lastMaxX || maxY != lastMaxY {
			layouts = calculateLayouts(len(panes), maxX, maxY)
			lastMaxX, lastMaxY = maxX, maxY
		} else {
			layouts = lastLayouts
		}

		for i, p := range panes {
			layout := layouts[i]
			termWidth := layout.x1 - layout.x0 - 1
			termHeight := layout.y1 - layout.y0 - 1

			// Handle resize for this pane
			if layouts[i] != lastLayouts[i] {
				if termWidth > 0 && termHeight > 0 {
					p.term.mu.Lock()
					p.term.Resize(termHeight, termWidth) // midterm uses (rows, cols)
					p.term.mu.Unlock()
					p.ctrl.Resize(termWidth, termHeight)
				}
			}

			v, err := g.SetView(p.viewName, layout.x0, layout.y0, layout.x1, layout.y1, 0)
			if err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
					return err
				}
			}

			// Set up view properties
			v.Title = fmt.Sprintf(" %s ", p.name)
			v.Frame = true
			v.Wrap = false
			v.Editable = true
			v.Editor = gocui.EditorFunc(makeTerminalEditor(p.ctrl))

			// Render terminal buffer to view
			v.Clear()
			renderTerminal(v, p.term)

			// Set cursor position for current view
			currentView := g.CurrentView()
			if currentView != nil && currentView.Name() == p.viewName {
				p.term.mu.Lock()
				cursor := p.term.Cursor
				cursorVisible := p.term.CursorVisible
				p.term.mu.Unlock()

				if cursorVisible {
					v.SetCursor(cursor.X, cursor.Y)
					g.Cursor = true
				} else {
					g.Cursor = false
				}
			}
		}

		// Set focus to first pane on first call
		if firstCall && len(panes) > 0 {
			if _, err := g.SetCurrentView(panes[0].viewName); err != nil {
				return err
			}
			firstCall = false
		}

		// Save layouts for next comparison
		copy(lastLayouts, layouts)

		return nil
	}
}

func renderTerminal(v *gocui.View, term *SafeTerminal) {
	// Recover from panics during resize race conditions
	defer func() {
		if r := recover(); r != nil {
			// Silently ignore - will redraw on next update
		}
	}()

	term.mu.Lock()
	defer term.mu.Unlock()

	if term.Height <= 0 || term.Width <= 0 {
		return
	}

	var sb strings.Builder
	if err := term.Render(&sb); err != nil {
		return
	}
	fmt.Fprint(v, sb.String())
}

func setupMultiPaneKeybindings(g *gocui.Gui, panes []*Pane) error {
	// Build a map from view name to pane
	paneMap := make(map[string]*Pane)
	for _, p := range panes {
		paneMap[p.viewName] = p
	}

	// Helper to get control mode for current view
	getCtrl := func(g *gocui.Gui) *ControlMode {
		v := g.CurrentView()
		if v == nil {
			return nil
		}
		if p, ok := paneMap[v.Name()]; ok {
			return p.ctrl
		}
		return nil
	}

	// Quit on Ctrl+C
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	// Also quit on Ctrl+\ as backup
	if err := g.SetKeybinding("", gocui.KeyCtrlBackslash, gocui.ModNone, quit); err != nil {
		return err
	}

	// Tab to cycle through panes
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return cyclePanes(g, panes, 1)
	}); err != nil {
		return err
	}

	// Shift+Tab to cycle backwards (using Ctrl+[ as alternative)
	if err := g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return cyclePanes(g, panes, -1)
	}); err != nil {
		return err
	}

	// Set up keybindings for each pane view
	for _, p := range panes {
		pane := p // capture for closure

		// Quit bindings for each pane
		if err := g.SetKeybinding(pane.viewName, gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
			return err
		}
		if err := g.SetKeybinding(pane.viewName, gocui.KeyCtrlBackslash, gocui.ModNone, quit); err != nil {
			return err
		}

		// Tab to cycle panes (override the terminal's tab)
		if err := g.SetKeybinding(pane.viewName, gocui.KeyTab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return cyclePanes(g, panes, 1)
		}); err != nil {
			return err
		}

		// Forward special keys to the pane's terminal
		keyMappings := map[gocui.Key]string{
			gocui.KeyEnter:      "Enter",
			gocui.KeyEsc:        "Escape",
			gocui.KeyBackspace:  "BSpace",
			gocui.KeyBackspace2: "BSpace",
			gocui.KeyDelete:     "DC",
			gocui.KeyArrowUp:    "Up",
			gocui.KeyArrowDown:  "Down",
			gocui.KeyArrowLeft:  "Left",
			gocui.KeyArrowRight: "Right",
			gocui.KeyHome:       "Home",
			gocui.KeyEnd:        "End",
			gocui.KeyPgup:       "PPage",
			gocui.KeyPgdn:       "NPage",
			gocui.KeySpace:      "Space",
		}

		for key, tmuxKey := range keyMappings {
			tk := tmuxKey // capture for closure
			if err := g.SetKeybinding(pane.viewName, key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
				ctrl := getCtrl(g)
				if ctrl == nil {
					return nil
				}
				return ctrl.SendKeys(tk)
			}); err != nil {
				return err
			}
		}

		// Forward Ctrl key combinations (except Ctrl+C and Ctrl+\ which are quit)
		ctrlMappings := map[gocui.Key]string{
			gocui.KeyCtrlA: "C-a",
			gocui.KeyCtrlB: "C-b",
			gocui.KeyCtrlD: "C-d",
			gocui.KeyCtrlE: "C-e",
			gocui.KeyCtrlF: "C-f",
			gocui.KeyCtrlG: "C-g",
			gocui.KeyCtrlH: "C-h",
			gocui.KeyCtrlJ: "C-j",
			gocui.KeyCtrlK: "C-k",
			gocui.KeyCtrlL: "C-l",
			gocui.KeyCtrlN: "C-n",
			gocui.KeyCtrlO: "C-o",
			gocui.KeyCtrlP: "C-p",
			gocui.KeyCtrlQ: "C-q",
			gocui.KeyCtrlR: "C-r",
			gocui.KeyCtrlS: "C-s",
			gocui.KeyCtrlT: "C-t",
			gocui.KeyCtrlU: "C-u",
			gocui.KeyCtrlV: "C-v",
			gocui.KeyCtrlW: "C-w",
			gocui.KeyCtrlX: "C-x",
			gocui.KeyCtrlY: "C-y",
			gocui.KeyCtrlZ: "C-z",
		}

		for key, tmuxKey := range ctrlMappings {
			tk := tmuxKey // capture for closure
			if err := g.SetKeybinding(pane.viewName, key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
				ctrl := getCtrl(g)
				if ctrl == nil {
					return nil
				}
				return ctrl.SendKeys(tk)
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

// cyclePanes switches focus to the next or previous pane.
func cyclePanes(g *gocui.Gui, panes []*Pane, direction int) error {
	if len(panes) <= 1 {
		return nil
	}

	currentView := g.CurrentView()
	if currentView == nil {
		_, err := g.SetCurrentView(panes[0].viewName)
		return err
	}

	// Find current pane index
	currentIdx := -1
	for i, p := range panes {
		if p.viewName == currentView.Name() {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		_, err := g.SetCurrentView(panes[0].viewName)
		return err
	}

	// Calculate next index with wrapping
	nextIdx := (currentIdx + direction + len(panes)) % len(panes)
	_, err := g.SetCurrentView(panes[nextIdx].viewName)
	return err
}

// makeTerminalEditor creates an editor function that forwards character input to tmux.
func makeTerminalEditor(ctrl *ControlMode) func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		// Only handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			ctrl.SendLiteralKeys(string(ch))
			return true
		}
		return false
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
