// Package app provides application lifecycle and orchestration.
package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/abdullathedruid/cmux/internal/claude"
	"github.com/abdullathedruid/cmux/internal/config"
	"github.com/abdullathedruid/cmux/internal/input"
	"github.com/abdullathedruid/cmux/internal/pane"
	"github.com/abdullathedruid/cmux/internal/terminal"
	"github.com/abdullathedruid/cmux/internal/ui"
	"github.com/jesseduffield/gocui"
)

// StructuredApp is an application that renders Claude sessions using
// structured data from hooks and transcripts instead of terminal emulation.
type StructuredApp struct {
	gui    *gocui.Gui
	config *config.Config
	input  *input.Handler

	// Session views keyed by tmux session name
	views map[string]*claude.View

	// Session order for layout
	sessions []string

	// Event watcher for hook events
	eventWatcher *claude.EventWatcher

	// Active session index
	activeIdx int

	// Layout state for resize detection
	lastMaxX, lastMaxY int
	lastSessionCount   int
	lastLayouts        []pane.Layout

	// Terminal modal state
	terminalCtrl *terminal.ControlMode
	terminalTerm *pane.SafeTerminal
}

// NewStructuredApp creates a new structured view application.
func NewStructuredApp() (*StructuredApp, error) {
	return NewStructuredAppWithConfig(nil)
}

// NewStructuredAppWithConfig creates a new structured view application with the given config.
func NewStructuredAppWithConfig(cfg *config.Config) (*StructuredApp, error) {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
	}

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode: gocui.OutputTrue,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing GUI: %w", err)
	}

	// Create event watcher for hooks
	eventsDir := claude.EventsDir()
	watcher, err := claude.NewEventWatcher(eventsDir)
	if err != nil {
		g.Close()
		return nil, fmt.Errorf("creating event watcher: %w", err)
	}

	return &StructuredApp{
		gui:          g,
		config:       cfg,
		input:        input.NewHandler(),
		views:        make(map[string]*claude.View),
		sessions:     make([]string, 0),
		eventWatcher: watcher,
	}, nil
}

// InitSessions initializes views for the given tmux session names.
func (a *StructuredApp) InitSessions(sessions []string) error {
	maxX, maxY := a.gui.Size()
	paneMaxY := maxY - pane.StatusBarHeight
	layouts := pane.CalculateLayouts(len(sessions), maxX, paneMaxY)

	for i, session := range sessions {
		layout := layouts[i]
		width := layout.Width()
		height := layout.Height()

		view := claude.NewView(session, width, height)

		// TODO: cwd filtering disabled for now - need better solution
		// The issue is multiple Claude sessions in same tmux pane write to same events file
		// if cwd := getTmuxSessionCwd(session); cwd != "" {
		// 	view.SetCwdFilter(cwd)
		// }

		a.views[session] = view
		a.sessions = append(a.sessions, session)
	}

	// Wire up event callbacks
	a.eventWatcher.OnEvent(func(tmuxSession string, event claude.HookEvent) {
		if view, ok := a.views[tmuxSession]; ok {
			view.UpdateFromHookEvent(event)
			// Trigger redraw
			a.gui.Update(func(g *gocui.Gui) error { return nil })
		}
	})

	return nil
}

// Run starts the main event loop.
func (a *StructuredApp) Run() error {
	defer a.Close()

	// Start event watcher
	if err := a.eventWatcher.Start(); err != nil {
		return fmt.Errorf("starting event watcher: %w", err)
	}

	a.gui.SetManagerFunc(a.layout)

	if err := a.setupKeybindings(); err != nil {
		return fmt.Errorf("setting up keybindings: %w", err)
	}

	// Start transcript polling goroutine
	go a.pollTranscripts()

	// Handle SIGINT/SIGTERM for clean exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		a.gui.Update(func(g *gocui.Gui) error {
			return gocui.ErrQuit
		})
	}()

	if err := a.gui.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) && err.Error() != "quit" {
		return fmt.Errorf("main loop: %w", err)
	}

	return nil
}

// Close cleans up all resources.
func (a *StructuredApp) Close() {
	a.eventWatcher.Stop()
	a.gui.Close()
}

// pollTranscripts periodically polls transcript files for updates.
func (a *StructuredApp) pollTranscripts() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		for _, view := range a.views {
			if err := view.PollTranscript(); err != nil {
				continue // Ignore errors
			}
			if view.IsDirty() {
				a.gui.Update(func(g *gocui.Gui) error { return nil })
			}
		}
	}
}

// layout is the gocui manager function that arranges views.
func (a *StructuredApp) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	currentMode := a.input.Mode()

	// Reserve 1 visible row for status bar
	paneMaxY := maxY - pane.StatusBarHeight

	// Recalculate layouts if size or session count changed
	var layouts []pane.Layout
	if maxX != a.lastMaxX || maxY != a.lastMaxY || len(a.sessions) != a.lastSessionCount {
		layouts = pane.CalculateLayouts(len(a.sessions), maxX, paneMaxY)
		a.lastMaxX, a.lastMaxY = maxX, maxY
		a.lastSessionCount = len(a.sessions)
	} else {
		layouts = a.lastLayouts
	}

	for i, session := range a.sessions {
		if i >= len(layouts) {
			continue
		}
		layout := layouts[i]
		view := a.views[session]

		// Handle resize
		width := layout.Width()
		height := layout.Height()
		view.Resize(width, height)

		viewName := fmt.Sprintf("pane-%d", i)
		v, err := g.SetView(viewName, layout.X0, layout.Y0, layout.X1, layout.Y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}

		// Configure view styling
		isActive := i == a.activeIdx
		a.configureStructuredView(v, session, isActive, currentMode)

		// Render structured content
		v.Clear()
		fmt.Fprint(v, view.Render())
	}

	// Render status bar at the bottom
	statusBarView, err := g.SetView("status-bar", 0, maxY-pane.StatusBarHeight, maxX-1, maxY, 0)
	if err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
			return err
		}
	}
	a.configureStatusBar(statusBarView, currentMode)

	// Handle terminal modal
	if currentMode.IsTerminal() && a.terminalCtrl != nil && a.terminalTerm != nil {
		// Calculate 80% modal dimensions
		width := maxX * 80 / 100
		height := maxY * 80 / 100
		if width < 40 {
			width = 40
		}
		if height < 10 {
			height = 10
		}

		x0, y0, x1, y1 := ui.ModalDimensions(maxX, maxY, width, height)
		v, err := g.SetView("terminal-modal", x0, y0, x1, y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}

		// Configure terminal modal styling
		session := a.ActiveSession()
		v.Title = fmt.Sprintf(" %s [Ctrl+Q to exit] ", session)
		v.Frame = true
		v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
		v.FrameColor = gocui.ColorGreen
		v.TitleColor = gocui.ColorGreen
		v.Wrap = false
		v.Editable = true
		v.Editor = gocui.EditorFunc(a.makeTerminalModalEditor())

		// Resize terminal if modal size changed
		modalWidth := width - 2
		modalHeight := height - 2
		a.terminalTerm.Resize(modalHeight, modalWidth)
		a.terminalCtrl.Resize(modalWidth, modalHeight)

		// Render terminal content
		v.Clear()
		ui.RenderTerminal(v, a.terminalTerm)

		if _, err := g.SetCurrentView("terminal-modal"); err != nil {
			return err
		}

		// Only show cursor if the terminal wants it visible
		// Apps like Claude hide the cursor while working
		if a.terminalTerm.CursorVisible() {
			cx, cy := a.terminalTerm.Cursor()
			v.SetCursor(cx, cy)
			g.Cursor = true
		} else {
			g.Cursor = false
		}
	} else {
		g.DeleteView("terminal-modal")

		// Handle input modal
		if currentMode.IsInput() {
			inputBuffer := a.input.InputBuffer()
			x0, y0, x1, y1 := ui.ModalDimensions(maxX, maxY, 50, 3)
			v, err := g.SetView("input-modal", x0, y0, x1, y1, 0)
			if err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
					return err
				}
			}
			ui.ConfigureInputModal(v, inputBuffer)
			v.Editor = gocui.EditorFunc(a.makeInputEditor())

			if _, err := g.SetCurrentView("input-modal"); err != nil {
				return err
			}
			g.Cursor = true
			v.SetCursor(len(inputBuffer)+1, 0)
		} else {
			g.DeleteView("input-modal")

			// Set focus to active view
			if len(a.sessions) > 0 && a.activeIdx < len(a.sessions) {
				viewName := fmt.Sprintf("pane-%d", a.activeIdx)
				g.SetCurrentView(viewName)
			}
			g.Cursor = false
		}
	}

	// Save layouts for next comparison
	if len(layouts) != len(a.lastLayouts) {
		a.lastLayouts = make([]pane.Layout, len(layouts))
	}
	copy(a.lastLayouts, layouts)

	return nil
}

// configureStructuredView configures styling for a structured view pane.
func (a *StructuredApp) configureStructuredView(v *gocui.View, session string, isActive bool, mode input.Mode) {
	v.Title = fmt.Sprintf(" %s ", session)
	v.Wrap = false
	v.Autoscroll = false

	if isActive {
		if mode.IsTerminal() {
			v.FrameColor = gocui.ColorGreen
			v.TitleColor = gocui.ColorGreen
		} else {
			v.FrameColor = gocui.ColorCyan
			v.TitleColor = gocui.ColorCyan
		}
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}
}

// configureStatusBar configures the status bar view.
func (a *StructuredApp) configureStatusBar(v *gocui.View, mode input.Mode) {
	v.Frame = false
	v.FgColor = gocui.ColorBlack
	v.BgColor = gocui.ColorWhite
	v.Clear()

	// Build status bar content
	modeStr := "NORMAL"
	if mode.IsTerminal() {
		modeStr = "TERMINAL"
	} else if mode.IsInput() {
		modeStr = "INPUT"
	}

	// Get active session status
	statusStr := ""
	if a.activeIdx < len(a.sessions) {
		session := a.sessions[a.activeIdx]
		if view, ok := a.views[session]; ok {
			sess := view.Session()
			if sess != nil {
				statusStr = string(sess.Status)
			}
		}
	}

	left := fmt.Sprintf(" [%s] ", modeStr)
	middle := fmt.Sprintf(" %d sessions ", len(a.sessions))
	right := fmt.Sprintf(" %s ", statusStr)

	maxX, _ := a.gui.Size()
	padding := maxX - len(left) - len(middle) - len(right)
	if padding < 0 {
		padding = 0
	}

	fmt.Fprintf(v, "%s%s%*s%s", left, middle, padding, "", right)
}

// setupKeybindings configures all keybindings for the structured app.
func (a *StructuredApp) setupKeybindings() error {
	// Global quit
	if err := a.gui.SetKeybinding("", 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			return gocui.ErrQuit
		}
		// Forward to terminal modal
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("q")
		}
		return nil
	}); err != nil {
		return err
	}

	// Navigation in normal mode
	for _, key := range []rune{'h', 'j', 'k', 'l'} {
		k := key
		if err := a.gui.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if a.input.Mode().IsNormal() {
				switch k {
				case 'h', 'k':
					a.prevSession()
				case 'j', 'l':
					a.nextSession()
				}
			} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
				// Forward to terminal modal
				a.terminalCtrl.SendLiteralKeys(string(k))
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Number keys: 1-3 for permission responses when pending, otherwise pane navigation
	for i := 1; i <= 9; i++ {
		num := i
		if err := a.gui.SetKeybinding("", rune('0'+i), gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			// In terminal mode, forward to terminal
			if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
				a.terminalCtrl.SendLiteralKeys(fmt.Sprintf("%d", num))
				return nil
			}

			// Check if active session has pending permission
			if a.activeIdx < len(a.sessions) {
				session := a.sessions[a.activeIdx]
				if view, ok := a.views[session]; ok {
					sess := view.Session()
					if sess != nil && sess.Status == claude.StatusNeedsInput {
						// Send permission response (1=yes, 2=always, 3=no)
						if num >= 1 && num <= 3 {
							claude.SendKeys(session, fmt.Sprintf("%d", num))
							return nil
						}
					}
				}
			}

			// Otherwise, use as pane navigation in normal mode
			if !a.input.Mode().IsNormal() {
				return nil
			}
			if num <= len(a.sessions) {
				a.activeIdx = num - 1
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Enter terminal mode with 'i'
	if err := a.gui.SetKeybinding("", 'i', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			return a.enterTerminalModal()
		}
		// In terminal mode, forward 'i' to tmux
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("i")
		}
		return nil
	}); err != nil {
		return err
	}

	// Enter terminal mode with Enter
	if err := a.gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			return a.enterTerminalModal()
		}
		// In terminal mode, forward Enter to tmux
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("Enter")
		}
		return nil
	}); err != nil {
		return err
	}

	// Exit terminal mode with Ctrl+Q
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlQ, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() {
			a.exitTerminalModal()
		}
		return nil
	}); err != nil {
		return err
	}

	// Scroll up with Ctrl+U (half page)
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlU, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			a.scrollActiveView(true)
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("C-u")
		}
		return nil
	}); err != nil {
		return err
	}

	// Scroll down with Ctrl+D (half page)
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlD, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			a.scrollActiveView(false)
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("C-d")
		}
		return nil
	}); err != nil {
		return err
	}

	// Scroll to bottom with 'G' in normal mode
	if err := a.gui.SetKeybinding("", 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() {
			a.scrollToBottom()
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("G")
		}
		return nil
	}); err != nil {
		return err
	}

	// Terminal modal key passthrough
	if err := a.setupTerminalModalPassthrough(); err != nil {
		return err
	}

	// Escape key
	if err := a.gui.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			// Send escape to tmux
			a.terminalCtrl.SendKeys("Escape")
		} else if a.input.Mode().IsInput() {
			a.input.ExitInputMode()
		}
		return nil
	}); err != nil {
		return err
	}

	// Ctrl+C in terminal mode
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("C-c")
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// setupTerminalModalPassthrough sets up keybindings to pass keys to the terminal modal.
func (a *StructuredApp) setupTerminalModalPassthrough() error {
	// Backspace handler - shared by both key codes
	handleBackspace := func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("BSpace")
		} else if a.input.Mode().IsInput() {
			a.input.BackspaceInputBuffer()
		}
		return nil
	}

	// KeyBackspace2 is Ctrl+H (0x08)
	if err := a.gui.SetKeybinding("", gocui.KeyBackspace2, gocui.ModNone, handleBackspace); err != nil {
		return err
	}

	// KeyBackspace is DEL (0x7f) - what most terminals send for backspace
	if err := a.gui.SetKeybinding("", gocui.KeyBackspace, gocui.ModNone, handleBackspace); err != nil {
		return err
	}

	// Space
	if err := a.gui.SetKeybinding("", gocui.KeySpace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("Space")
		}
		return nil
	}); err != nil {
		return err
	}

	// Tab
	if err := a.gui.SetKeybinding("", gocui.KeyTab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendKeys("Tab")
		}
		return nil
	}); err != nil {
		return err
	}

	// Arrow keys
	arrowKeys := map[gocui.Key]string{
		gocui.KeyArrowUp:    "Up",
		gocui.KeyArrowDown:  "Down",
		gocui.KeyArrowLeft:  "Left",
		gocui.KeyArrowRight: "Right",
	}
	for key, name := range arrowKeys {
		k := key
		n := name
		if err := a.gui.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
				a.terminalCtrl.SendKeys(n)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// nextSession moves to the next session.
func (a *StructuredApp) nextSession() {
	if len(a.sessions) == 0 {
		return
	}
	a.activeIdx = (a.activeIdx + 1) % len(a.sessions)
}

// prevSession moves to the previous session.
func (a *StructuredApp) prevSession() {
	if len(a.sessions) == 0 {
		return
	}
	a.activeIdx = (a.activeIdx - 1 + len(a.sessions)) % len(a.sessions)
}

// makeInputEditor creates an editor function for the input modal.
func (a *StructuredApp) makeInputEditor() func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		if !a.input.Mode().IsInput() {
			return false
		}

		if ch != 0 && mod == gocui.ModNone {
			a.input.AppendToInputBuffer(ch)
			return true
		}
		return false
	}
}

// makeTerminalModalEditor creates an editor function for the terminal modal.
func (a *StructuredApp) makeTerminalModalEditor() func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		if !a.input.Mode().IsTerminal() || a.terminalCtrl == nil {
			return false
		}

		// Handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			a.terminalCtrl.SendLiteralKeys(string(ch))
			return true
		}
		return false
	}
}

// ActiveSession returns the currently active session name.
func (a *StructuredApp) ActiveSession() string {
	if a.activeIdx < len(a.sessions) {
		return a.sessions[a.activeIdx]
	}
	return ""
}

// ActiveView returns the Claude view for the active session.
func (a *StructuredApp) ActiveView() *claude.View {
	session := a.ActiveSession()
	if session == "" {
		return nil
	}
	return a.views[session]
}

// scrollActiveView scrolls the active view up or down by half a page.
func (a *StructuredApp) scrollActiveView(up bool) {
	view := a.ActiveView()
	if view == nil {
		return
	}

	_, height := view.Dimensions()
	scrollAmount := height / 2
	if scrollAmount < 1 {
		scrollAmount = 1
	}

	if up {
		view.ScrollUp(scrollAmount)
	} else {
		view.ScrollDown(scrollAmount)
	}
}

// scrollToBottom scrolls the active view to show the latest content.
func (a *StructuredApp) scrollToBottom() {
	view := a.ActiveView()
	if view == nil {
		return
	}
	view.ScrollToBottom()
}

// enterTerminalModal starts the terminal modal for the active session.
func (a *StructuredApp) enterTerminalModal() error {
	session := a.ActiveSession()
	if session == "" {
		return nil
	}

	// Calculate modal dimensions (80% of screen)
	maxX, maxY := a.gui.Size()
	width := maxX * 80 / 100
	height := maxY * 80 / 100
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	// Create control mode connection
	a.terminalCtrl = terminal.NewControlMode(session)
	a.terminalTerm = pane.NewSafeTerminal(height-2, width-2) // Account for borders

	if err := a.terminalCtrl.Start(width-2, height-2); err != nil {
		a.terminalCtrl = nil
		a.terminalTerm = nil
		return fmt.Errorf("starting control mode: %w", err)
	}

	// Start output processing goroutine
	go a.processTerminalOutput()

	a.input.SetMode(input.ModeTerminal)
	return nil
}

// exitTerminalModal closes the terminal modal and cleans up.
func (a *StructuredApp) exitTerminalModal() {
	if a.terminalCtrl != nil {
		a.terminalCtrl.Close()
		a.terminalCtrl = nil
	}
	a.terminalTerm = nil
	a.input.SetMode(input.ModeNormal)
}

// processTerminalOutput reads from the terminal control mode and writes to the emulator.
func (a *StructuredApp) processTerminalOutput() {
	if a.terminalCtrl == nil {
		return
	}

	for data := range a.terminalCtrl.OutputChan() {
		if a.terminalTerm != nil {
			a.terminalTerm.Write(data)
			a.gui.Update(func(g *gocui.Gui) error { return nil })
		}
	}
}

// getTmuxSessionCwd gets the working directory of a tmux session's active pane.
func getTmuxSessionCwd(session string) string {
	cmd := exec.Command("tmux", "display-message", "-t", session, "-p", "#{pane_current_path}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
