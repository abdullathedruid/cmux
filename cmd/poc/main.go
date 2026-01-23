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

// Mode represents the current input mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeTerminal
	ModeInput
)

// AppState holds the global application state.
type AppState struct {
	mode          Mode
	activePaneIdx int
	panes         []*Pane
	gui           *gocui.Gui
	inputBuffer   string
	repoRoot      string // root directory of the git repo for worktrees
	mu            sync.Mutex
}

// Pane represents a single pane with its control mode connection and terminal emulator.
type Pane struct {
	index    int
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

// getGitRepoRoot returns the root directory of the current git repository.
func getGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// createWorktreeAndSession creates a git worktree and a tmux session in it.
func createWorktreeAndSession(repoRoot, name string) error {
	worktreePath := fmt.Sprintf("%s/.worktrees/%s", repoRoot, name)

	// Create the worktree with a new branch
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", name, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, out)
	}

	// Create a new tmux session in the worktree directory
	cmd = exec.Command("tmux", "new-session", "-d", "-s", name, "-c", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, out)
	}

	return nil
}

// addNewPane creates and adds a new pane for the given tmux session.
func addNewPane(state *AppState, sessionName string) error {
	state.mu.Lock()
	g := state.gui
	paneCount := len(state.panes)
	state.mu.Unlock()

	maxX, maxY := g.Size()
	layouts := calculateLayouts(paneCount+1, maxX, maxY)
	layout := layouts[paneCount]

	termWidth := layout.x1 - layout.x0 - 1
	termHeight := layout.y1 - layout.y0 - 1
	if termWidth < 1 {
		termWidth = 80
	}
	if termHeight < 1 {
		termHeight = 24
	}

	term := &SafeTerminal{Terminal: midterm.NewTerminal(termHeight, termWidth)}
	ctrl := NewControlMode(sessionName)

	if err := ctrl.Start(termWidth, termHeight); err != nil {
		return fmt.Errorf("start control mode: %w", err)
	}

	pane := &Pane{
		index:    paneCount + 1,
		name:     sessionName,
		ctrl:     ctrl,
		term:     term,
		viewName: fmt.Sprintf("pane-%d", paneCount),
	}

	// Start output processing goroutine
	go func() {
		for data := range pane.ctrl.outputCh {
			pane.term.mu.Lock()
			pane.term.Write(data)
			pane.term.mu.Unlock()
			g.Update(func(g *gocui.Gui) error { return nil })
		}
	}()

	// Add to state
	state.mu.Lock()
	state.panes = append(state.panes, pane)
	state.activePaneIdx = len(state.panes) - 1 // Focus new pane
	state.mu.Unlock()

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
			index:    i + 1, // 1-indexed for display
			name:     session,
			ctrl:     ctrl,
			term:     term,
			viewName: fmt.Sprintf("pane-%d", i),
		}
	}

	// Get git repo root for worktree creation
	repoRoot, err := getGitRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine git repo root: %v\n", err)
		repoRoot = "."
	}

	// Initialize app state
	state := &AppState{
		mode:          ModeNormal,
		activePaneIdx: 0,
		panes:         panes,
		gui:           g,
		repoRoot:      repoRoot,
	}

	// Ensure all panes are closed on exit
	defer func() {
		state.mu.Lock()
		for _, p := range state.panes {
			p.ctrl.Close()
		}
		state.mu.Unlock()
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
	g.SetManagerFunc(multiPaneLayoutFunc(state))

	// Set up keybindings
	if err := setupMultiPaneKeybindings(g, state); err != nil {
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

func multiPaneLayoutFunc(state *AppState) func(*gocui.Gui) error {
	firstCall := true
	lastMaxX, lastMaxY := 0, 0
	lastPaneCount := 0
	var lastLayouts []Layout

	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()

		state.mu.Lock()
		panes := state.panes
		currentMode := state.mode
		activePaneIdx := state.activePaneIdx
		inputBuffer := state.inputBuffer
		state.mu.Unlock()

		// Recalculate layouts if size or pane count changed
		var layouts []Layout
		if maxX != lastMaxX || maxY != lastMaxY || len(panes) != lastPaneCount {
			layouts = calculateLayouts(len(panes), maxX, maxY)
			lastMaxX, lastMaxY = maxX, maxY
			lastPaneCount = len(panes)
			lastLayouts = make([]Layout, len(panes))
		} else {
			layouts = lastLayouts
		}

		// Mode indicator for title
		modeStr := "NORMAL"
		if currentMode == ModeTerminal {
			modeStr = "TERMINAL"
		} else if currentMode == ModeInput {
			modeStr = "INPUT"
		}

		for i, p := range panes {
			if i >= len(layouts) {
				continue
			}
			layout := layouts[i]
			termWidth := layout.x1 - layout.x0 - 1
			termHeight := layout.y1 - layout.y0 - 1

			// Handle resize for this pane
			if i < len(lastLayouts) && layouts[i] != lastLayouts[i] {
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

			// Set up view properties with numbered title
			isActive := i == activePaneIdx
			if isActive {
				v.Title = fmt.Sprintf(" [%s] %d: %s ", modeStr, p.index, p.name)
				// Bold frame for active pane using heavy box-drawing characters
				v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
				// Color based on mode: blue for normal, green for terminal
				if currentMode == ModeTerminal {
					v.FrameColor = gocui.ColorGreen
				} else {
					v.FrameColor = gocui.ColorBlue
				}
			} else {
				v.Title = fmt.Sprintf(" %d: %s ", p.index, p.name)
				// Regular frame for inactive panes
				v.FrameRunes = []rune{'─', '│', '┌', '┐', '└', '┘'}
				v.FrameColor = gocui.ColorDefault
			}
			v.Frame = true
			v.Wrap = false
			v.Editable = currentMode == ModeTerminal && isActive
			v.Editor = gocui.EditorFunc(makeTerminalEditor(p.ctrl, state))

			// Render terminal buffer to view
			v.Clear()
			renderTerminal(v, p.term)
		}

		// Handle input modal
		if currentMode == ModeInput {
			modalWidth := 50
			modalHeight := 3
			modalX0 := (maxX - modalWidth) / 2
			modalY0 := (maxY - modalHeight) / 2
			modalX1 := modalX0 + modalWidth
			modalY1 := modalY0 + modalHeight

			v, err := g.SetView("input-modal", modalX0, modalY0, modalX1, modalY1, 0)
			if err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
					return err
				}
			}
			v.Title = " New Worktree (Enter=confirm, Esc=cancel) "
			v.Frame = true
			v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
			v.FrameColor = gocui.ColorYellow
			v.Editable = true
			v.Editor = gocui.EditorFunc(makeInputEditor(state))
			v.Clear()
			fmt.Fprintf(v, " %s", inputBuffer)

			if _, err := g.SetCurrentView("input-modal"); err != nil {
				return err
			}
			g.Cursor = true
			v.SetCursor(len(inputBuffer)+1, 0)
		} else {
			// Delete modal if not in input mode
			g.DeleteView("input-modal")

			// Set focus to active pane and handle cursor
			if len(panes) > 0 && activePaneIdx < len(panes) {
				activePane := panes[activePaneIdx]
				if _, err := g.SetCurrentView(activePane.viewName); err != nil && firstCall {
					return err
				}
				firstCall = false

				// Set cursor after view is focused
				if currentMode == ModeTerminal {
					activePane.term.mu.Lock()
					cursor := activePane.term.Cursor
					activePane.term.mu.Unlock()

					if v, err := g.View(activePane.viewName); err == nil {
						v.SetCursor(cursor.X, cursor.Y)
					}
					g.Cursor = true
				} else {
					g.Cursor = false
				}
			}
		}

		// Save layouts for next comparison
		if len(layouts) == len(lastLayouts) {
			copy(lastLayouts, layouts)
		} else {
			lastLayouts = make([]Layout, len(layouts))
			copy(lastLayouts, layouts)
		}

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

func setupMultiPaneKeybindings(g *gocui.Gui, state *AppState) error {
	// Helper to get control mode for active pane
	getActiveCtrl := func() *ControlMode {
		state.mu.Lock()
		idx := state.activePaneIdx
		panes := state.panes
		state.mu.Unlock()
		if idx >= 0 && idx < len(panes) {
			return panes[idx].ctrl
		}
		return nil
	}

	// Helper to check current mode
	getMode := func() Mode {
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.mode
	}

	// Helper to set active pane
	setActivePane := func(idx int) {
		state.mu.Lock()
		if idx >= 0 && idx < len(state.panes) {
			state.activePaneIdx = idx
		}
		state.mu.Unlock()
	}

	// Helper to move to adjacent pane
	movePaneDirection := func(g *gocui.Gui, dx, dy int) error {
		state.mu.Lock()
		currentIdx := state.activePaneIdx
		paneCount := len(state.panes)
		state.mu.Unlock()

		if paneCount == 0 {
			return nil
		}

		// Simple navigation: for now just cycle through panes
		// TODO: implement spatial navigation based on layout
		newIdx := currentIdx
		if dx > 0 || dy > 0 {
			newIdx = (currentIdx + 1) % paneCount
		} else if dx < 0 || dy < 0 {
			newIdx = (currentIdx - 1 + paneCount) % paneCount
		}

		setActivePane(newIdx)
		return nil
	}

	// === GLOBAL KEYBINDINGS ===

	// Ctrl+Q: Exit terminal mode (works in both modes, but only matters in terminal mode)
	if err := g.SetKeybinding("", gocui.KeyCtrlQ, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		state.mu.Lock()
		state.mode = ModeNormal
		state.mu.Unlock()
		return nil
	}); err != nil {
		return err
	}

	// === NORMAL MODE KEYBINDINGS ===

	// q: Quit application (normal mode only)
	if err := g.SetKeybinding("", 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode() == ModeTerminal {
			// Forward 'q' to terminal
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("q")
			}
			return nil
		}
		return gocui.ErrQuit
	}); err != nil {
		return err
	}

	// N: Open new worktree input modal (normal mode only)
	if err := g.SetKeybinding("", 'N', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode == ModeTerminal {
			// Forward 'N' to terminal
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("N")
			}
			return nil
		}
		if mode == ModeInput {
			return nil // Already in input mode
		}
		// Enter input mode
		state.mu.Lock()
		state.mode = ModeInput
		state.inputBuffer = ""
		state.mu.Unlock()
		return nil
	}); err != nil {
		return err
	}

	// i or Enter: Enter terminal mode
	for _, key := range []rune{'i'} {
		k := key
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() == ModeTerminal {
				// Forward to terminal
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendLiteralKeys(string(k))
				}
				return nil
			}
			state.mu.Lock()
			state.mode = ModeTerminal
			state.mu.Unlock()
			return nil
		}); err != nil {
			return err
		}
	}

	if err := g.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode == ModeTerminal {
			// Forward Enter to terminal
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("Enter")
			}
			return nil
		}
		if mode == ModeInput {
			// Confirm input and create worktree + session
			state.mu.Lock()
			name := strings.TrimSpace(state.inputBuffer)
			repoRoot := state.repoRoot
			state.mode = ModeNormal
			state.inputBuffer = ""
			state.mu.Unlock()

			if name == "" {
				return nil
			}

			// Create worktree and tmux session
			if err := createWorktreeAndSession(repoRoot, name); err != nil {
				// TODO: show error to user
				return nil
			}

			// Add new pane
			return addNewPane(state, name)
		}
		state.mu.Lock()
		state.mode = ModeTerminal
		state.mu.Unlock()
		return nil
	}); err != nil {
		return err
	}

	// h/j/k/l: Navigate panes (normal mode) or forward to terminal (terminal mode)
	navKeys := map[rune]struct{ dx, dy int }{
		'h': {-1, 0},
		'l': {1, 0},
		'k': {0, -1},
		'j': {0, 1},
	}

	for key, dir := range navKeys {
		k := key
		d := dir
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() == ModeTerminal {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendLiteralKeys(string(k))
				}
				return nil
			}
			return movePaneDirection(g, d.dx, d.dy)
		}); err != nil {
			return err
		}
	}

	// 1-9: Jump to pane N
	for i := 1; i <= 9; i++ {
		paneIdx := i - 1 // 0-indexed
		key := rune('0' + i)
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() == ModeTerminal {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendLiteralKeys(string(key))
				}
				return nil
			}
			state.mu.Lock()
			paneCount := len(state.panes)
			state.mu.Unlock()
			if paneIdx < paneCount {
				setActivePane(paneIdx)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Arrow keys: Navigate in normal mode, forward in terminal mode
	arrowMappings := map[gocui.Key]struct {
		dx, dy  int
		tmuxKey string
	}{
		gocui.KeyArrowLeft:  {-1, 0, "Left"},
		gocui.KeyArrowRight: {1, 0, "Right"},
		gocui.KeyArrowUp:    {0, -1, "Up"},
		gocui.KeyArrowDown:  {0, 1, "Down"},
	}

	for key, mapping := range arrowMappings {
		k := key
		m := mapping
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() == ModeTerminal {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendKeys(m.tmuxKey)
				}
				return nil
			}
			return movePaneDirection(g, m.dx, m.dy)
		}); err != nil {
			return err
		}
	}

	// === TERMINAL AND INPUT MODE KEYBINDINGS ===

	// Esc: Cancel input mode or forward to terminal
	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode == ModeInput {
			state.mu.Lock()
			state.mode = ModeNormal
			state.inputBuffer = ""
			state.mu.Unlock()
			return nil
		}
		if mode == ModeTerminal {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("Escape")
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Backspace: Delete char in input mode or forward to terminal
	for _, key := range []gocui.Key{gocui.KeyBackspace, gocui.KeyBackspace2} {
		k := key
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			mode := getMode()
			if mode == ModeInput {
				state.mu.Lock()
				if len(state.inputBuffer) > 0 {
					state.inputBuffer = state.inputBuffer[:len(state.inputBuffer)-1]
				}
				state.mu.Unlock()
				return nil
			}
			if mode == ModeTerminal {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendKeys("BSpace")
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Special keys that only make sense in terminal mode
	terminalOnlyKeys := map[gocui.Key]string{
		gocui.KeyDelete: "DC",
		gocui.KeyHome:   "Home",
		gocui.KeyEnd:    "End",
		gocui.KeyPgup:   "PPage",
		gocui.KeyPgdn:   "NPage",
		gocui.KeySpace:  "Space",
		gocui.KeyTab:    "Tab",
	}

	for key, tmuxKey := range terminalOnlyKeys {
		tk := tmuxKey
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() != ModeTerminal {
				return nil
			}
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys(tk)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Ctrl key combinations (except Ctrl+Q which is mode switch)
	ctrlMappings := map[gocui.Key]string{
		gocui.KeyCtrlA:         "C-a",
		gocui.KeyCtrlB:         "C-b",
		gocui.KeyCtrlC:         "C-c",
		gocui.KeyCtrlD:         "C-d",
		gocui.KeyCtrlE:         "C-e",
		gocui.KeyCtrlF:         "C-f",
		gocui.KeyCtrlG:         "C-g",
		gocui.KeyCtrlH:         "C-h",
		gocui.KeyCtrlJ:         "C-j",
		gocui.KeyCtrlK:         "C-k",
		gocui.KeyCtrlL:         "C-l",
		gocui.KeyCtrlN:         "C-n",
		gocui.KeyCtrlO:         "C-o",
		gocui.KeyCtrlP:         "C-p",
		gocui.KeyCtrlR:         "C-r",
		gocui.KeyCtrlS:         "C-s",
		gocui.KeyCtrlT:         "C-t",
		gocui.KeyCtrlU:         "C-u",
		gocui.KeyCtrlV:         "C-v",
		gocui.KeyCtrlW:         "C-w",
		gocui.KeyCtrlX:         "C-x",
		gocui.KeyCtrlY:         "C-y",
		gocui.KeyCtrlZ:         "C-z",
		gocui.KeyCtrlBackslash: "C-\\",
	}

	for key, tmuxKey := range ctrlMappings {
		tk := tmuxKey
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode() != ModeTerminal {
				return nil
			}
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys(tk)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// makeInputEditor creates an editor function for the input modal.
func makeInputEditor(state *AppState) func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		state.mu.Lock()
		defer state.mu.Unlock()

		if state.mode != ModeInput {
			return false
		}

		// Handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			state.inputBuffer += string(ch)
			return true
		}
		return false
	}
}

// makeTerminalEditor creates an editor function that forwards character input to tmux.
func makeTerminalEditor(ctrl *ControlMode, state *AppState) func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		state.mu.Lock()
		mode := state.mode
		state.mu.Unlock()

		// Only forward input in terminal mode
		if mode != ModeTerminal {
			return false
		}

		// Only handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			ctrl.SendLiteralKeys(string(ch))
			return true
		}
		return false
	}
}
