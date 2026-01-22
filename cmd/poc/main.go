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
	"github.com/hinshun/vt10x"
	"github.com/jesseduffield/gocui"
)

// ControlMode manages a tmux -CC connection.
type ControlMode struct {
	session  string
	cmd      *exec.Cmd
	pty      *os.File
	outputCh chan []byte
	doneCh   chan struct{}
	mu       sync.Mutex
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

	// Tell tmux our window size and request initial content
	c.Resize(width, height)
	c.RequestRefresh()

	return nil
}

// RequestRefresh asks tmux to redraw the pane content.
func (c *ControlMode) RequestRefresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Send Ctrl+L to trigger a screen redraw in most applications
	// This is a common convention for terminal apps to redraw
	cmd := fmt.Sprintf("send-keys -t %s C-l\n", c.session)
	_, err := c.pty.Write([]byte(cmd))
	return err
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

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: poc <session-name>")
		os.Exit(1)
	}

	session := flag.Arg(0)

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

	// Create terminal emulator (account for border)
	termWidth := maxX - 2
	termHeight := maxY - 2
	if termWidth < 1 {
		termWidth = 80
	}
	if termHeight < 1 {
		termHeight = 24
	}
	term := vt10x.New(vt10x.WithSize(termWidth, termHeight))

	// Create control mode connection
	ctrl := NewControlMode(session)
	if err := ctrl.Start(termWidth, termHeight); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting control mode: %v\n", err)
		os.Exit(1)
	}
	defer ctrl.Close()

	// Process output goroutine
	go func() {
		for data := range ctrl.outputCh {
			term.Write(data)
			g.Update(func(g *gocui.Gui) error { return nil })
		}
	}()

	// Set up layout
	g.SetManagerFunc(layoutFunc(term, session, ctrl))

	// Set up keybindings
	if err := setupKeybindings(g, ctrl); err != nil {
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

func layoutFunc(term vt10x.Terminal, title string, ctrl *ControlMode) func(*gocui.Gui) error {
	firstCall := true
	lastWidth, lastHeight := 0, 0
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()
		termWidth := maxX - 2
		termHeight := maxY - 2

		// Handle resize
		if termWidth != lastWidth || termHeight != lastHeight {
			if termWidth > 0 && termHeight > 0 {
				term.Resize(termWidth, termHeight)
				ctrl.Resize(termWidth, termHeight)
				lastWidth, lastHeight = termWidth, termHeight
			}
		}

		v, err := g.SetView("terminal", 0, 0, maxX-1, maxY-1, 0)
		if err != nil {
			// Check both errors.Is and string match (gocui may return wrapped errors)
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}

		// Set up view properties
		v.Title = fmt.Sprintf(" %s (Ctrl+C to quit) ", title)
		v.Frame = true
		v.Wrap = false
		v.Editable = true
		v.Editor = gocui.EditorFunc(makeTerminalEditor(ctrl))

		// Set current view only on first call
		if firstCall {
			if _, err := g.SetCurrentView("terminal"); err != nil {
				return err
			}
			firstCall = false
		}

		// Render terminal buffer to view
		v.Clear()
		renderTerminal(v, term)

		// Set cursor position
		term.Lock()
		cursor := term.Cursor()
		cursorVisible := term.CursorVisible()
		term.Unlock()

		if cursorVisible {
			v.SetCursor(cursor.X, cursor.Y)
			g.Cursor = true
		} else {
			g.Cursor = false
		}

		return nil
	}
}

// Attribute flags from vt10x (internal constants)
const (
	attrReverse   = 1 << 0
	attrUnderline = 1 << 1
	attrBold      = 1 << 2
	attrItalic    = 1 << 3
	attrBlink     = 1 << 4
)

func renderTerminal(v *gocui.View, term vt10x.Terminal) {
	term.Lock()
	defer term.Unlock()

	width, height := term.Size()

	var sb strings.Builder
	var lastFG, lastBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	var lastMode int16 = 0

	for y := range height {
		for x := range width {
			cell := term.Cell(x, y)

			// Check if style changed
			if cell.FG != lastFG || cell.BG != lastBG || cell.Mode != lastMode {
				// Reset and apply new style
				sb.WriteString("\033[0m")
				writeStyle(&sb, cell.FG, cell.BG, cell.Mode)
				lastFG, lastBG, lastMode = cell.FG, cell.BG, cell.Mode
			}

			if cell.Char == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(cell.Char)
			}
		}
		// Reset at end of line and add newline
		sb.WriteString("\033[0m")
		lastFG, lastBG, lastMode = vt10x.DefaultFG, vt10x.DefaultBG, 0
		if y < height-1 {
			sb.WriteRune('\n')
		}
	}
	fmt.Fprint(v, sb.String())
}

func writeStyle(sb *strings.Builder, fg, bg vt10x.Color, mode int16) {
	// Write attributes
	if mode&attrBold != 0 {
		sb.WriteString("\033[1m")
	}
	if mode&attrItalic != 0 {
		sb.WriteString("\033[3m")
	}
	if mode&attrUnderline != 0 {
		sb.WriteString("\033[4m")
	}
	if mode&attrBlink != 0 {
		sb.WriteString("\033[5m")
	}
	if mode&attrReverse != 0 {
		sb.WriteString("\033[7m")
	}

	// Write foreground color
	writeFGColor(sb, fg)

	// Write background color
	writeBGColor(sb, bg)
}

func writeFGColor(sb *strings.Builder, c vt10x.Color) {
	// Skip default colors
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG {
		return
	}
	if c < 8 {
		// Standard ANSI colors 0-7
		fmt.Fprintf(sb, "\033[%dm", 30+c)
	} else if c < 16 {
		// Bright ANSI colors 8-15
		fmt.Fprintf(sb, "\033[%dm", 90+(c-8))
	} else if c < 256 {
		// 256-color mode
		fmt.Fprintf(sb, "\033[38;5;%dm", c)
	} else {
		// True color (RGB encoded in the color value)
		// vt10x stores RGB as: color = r<<16 | g<<8 | b
		r := (c >> 16) & 0xFF
		g := (c >> 8) & 0xFF
		b := c & 0xFF
		fmt.Fprintf(sb, "\033[38;2;%d;%d;%dm", r, g, b)
	}
}

func writeBGColor(sb *strings.Builder, c vt10x.Color) {
	// Skip default colors
	if c == vt10x.DefaultBG || c == vt10x.DefaultFG {
		return
	}
	if c < 8 {
		// Standard ANSI colors 0-7
		fmt.Fprintf(sb, "\033[%dm", 40+c)
	} else if c < 16 {
		// Bright ANSI colors 8-15
		fmt.Fprintf(sb, "\033[%dm", 100+(c-8))
	} else if c < 256 {
		// 256-color mode
		fmt.Fprintf(sb, "\033[48;5;%dm", c)
	} else {
		// True color (RGB encoded in the color value)
		r := (c >> 16) & 0xFF
		g := (c >> 8) & 0xFF
		b := c & 0xFF
		fmt.Fprintf(sb, "\033[48;2;%d;%d;%dm", r, g, b)
	}
}

func setupKeybindings(g *gocui.Gui, ctrl *ControlMode) error {
	// Quit on Ctrl+C
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("terminal", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	// Also quit on Ctrl+\ as backup
	if err := g.SetKeybinding("", gocui.KeyCtrlBackslash, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("terminal", gocui.KeyCtrlBackslash, gocui.ModNone, quit); err != nil {
		return err
	}

	// Forward special keys to terminal view
	keyMappings := map[gocui.Key]string{
		gocui.KeyEnter:      "Enter",
		gocui.KeyEsc:        "Escape",
		gocui.KeyBackspace:  "BSpace",
		gocui.KeyBackspace2: "BSpace",
		gocui.KeyDelete:     "DC",
		gocui.KeyTab:        "Tab",
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
		if err := g.SetKeybinding("terminal", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return ctrl.SendKeys(tmuxKey)
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
		if err := g.SetKeybinding("terminal", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return ctrl.SendKeys(tmuxKey)
		}); err != nil {
			return err
		}
	}

	return nil
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
