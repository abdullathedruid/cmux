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
		return nil
	}); err != nil {
		return err
	}

	// Navigation in normal mode
	for _, key := range []rune{'h', 'j', 'k', 'l'} {
		k := key
		if err := a.gui.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if !a.input.Mode().IsNormal() {
				return nil
			}
			switch k {
			case 'h', 'k':
				a.prevSession()
			case 'j', 'l':
				a.nextSession()
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

	// Enter terminal mode with 'i' or Enter
	for _, key := range []interface{}{'i', gocui.KeyEnter} {
		k := key
		if err := a.gui.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if a.input.Mode().IsNormal() {
				a.input.SetMode(input.ModeTerminal)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Exit terminal mode with Ctrl+Q
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlQ, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() {
			a.input.SetMode(input.ModeNormal)
		}
		return nil
	}); err != nil {
		return err
	}

	// Terminal mode key passthrough
	if err := a.setupTerminalPassthrough(); err != nil {
		return err
	}

	// Escape key
	if err := a.gui.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() {
			// Send escape to tmux
			if a.activeIdx < len(a.sessions) {
				claude.SendEscape(a.sessions[a.activeIdx])
			}
		} else if a.input.Mode().IsInput() {
			a.input.ExitInputMode()
		}
		return nil
	}); err != nil {
		return err
	}


	// Ctrl+C in terminal mode
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
			claude.SendInterrupt(a.sessions[a.activeIdx])
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// setupTerminalPassthrough sets up keybindings to pass printable characters to tmux.
func (a *StructuredApp) setupTerminalPassthrough() error {
	// Printable ASCII characters
	for ch := rune(32); ch < 127; ch++ {
		// Skip characters we handle elsewhere
		if ch == 'q' || ch == 'h' || ch == 'j' || ch == 'k' || ch == 'l' ||
			ch == 'i' || ch == 'y' || ch == 'n' || ch == 'a' ||
			(ch >= '1' && ch <= '9') {
			continue
		}

		c := ch
		if err := a.gui.SetKeybinding("", c, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
				claude.SendKeys(a.sessions[a.activeIdx], string(c))
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Enter key in terminal mode
	if err := a.gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
			claude.SendKeys(a.sessions[a.activeIdx], "Enter")
		} else if a.input.Mode().IsNormal() {
			a.input.SetMode(input.ModeTerminal)
		}
		return nil
	}); err != nil {
		return err
	}

	// Backspace
	if err := a.gui.SetKeybinding("", gocui.KeyBackspace2, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
			claude.SendKeys(a.sessions[a.activeIdx], "BSpace")
		}
		return nil
	}); err != nil {
		return err
	}

	// Space
	if err := a.gui.SetKeybinding("", gocui.KeySpace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
			claude.SendKeys(a.sessions[a.activeIdx], "Space")
		}
		return nil
	}); err != nil {
		return err
	}

	// Tab
	if err := a.gui.SetKeybinding("", gocui.KeyTab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
			claude.SendKeys(a.sessions[a.activeIdx], "Tab")
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
			if a.input.Mode().IsTerminal() && a.activeIdx < len(a.sessions) {
				claude.SendKeys(a.sessions[a.activeIdx], n)
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

// getTmuxSessionCwd gets the working directory of a tmux session's active pane.
func getTmuxSessionCwd(session string) string {
	cmd := exec.Command("tmux", "display-message", "-t", session, "-p", "#{pane_current_path}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
