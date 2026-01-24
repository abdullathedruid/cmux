// Package app provides application lifecycle and orchestration.
package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/abdullathedruid/cmux/internal/claude"
	"github.com/abdullathedruid/cmux/internal/config"
	"github.com/abdullathedruid/cmux/internal/discovery"
	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/input"
	"github.com/abdullathedruid/cmux/internal/pane"
	"github.com/abdullathedruid/cmux/internal/session"
	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/abdullathedruid/cmux/internal/terminal"
	"github.com/abdullathedruid/cmux/internal/tmux"
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

	// Sidebar state
	sidebarEnabled     bool
	availableSessions  []string // All discovered Claude sessions
	sidebarSelectedIdx int      // Cursor in sidebar
	tmuxClient         *tmux.RealClient
	inputPurpose       string // "new_session" when creating a new session

	// Advanced session management
	discoveryService *discovery.Service
	sessionManager   *session.Manager

	// Unified sidebar state
	focusedPane      string                     // "repos", "sessions", or "main"
	repositories     []discovery.RepositoryInfo // All configured repositories
	repoSelectedIdx  int                        // Currently selected repo index
	sessionsForRepo  []*state.Session           // Sessions filtered for selected repo
	sessionSelectedIdx int                      // Selected session index in the list
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

	tmuxClient := tmux.NewClient(cfg.ClaudeCommand)
	discoverySvc := discovery.NewService(tmuxClient, cfg)
	sessionMgr := session.NewManager(tmuxClient, cfg)

	app := &StructuredApp{
		gui:              g,
		config:           cfg,
		input:            input.NewHandler(),
		views:            make(map[string]*claude.View),
		sessions:         make([]string, 0),
		eventWatcher:     watcher,
		tmuxClient:       tmuxClient,
		discoveryService: discoverySvc,
		sessionManager:   sessionMgr,
		focusedPane:      "sessions", // Default focus on sessions pane
	}

	return app, nil
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

// InitWithDiscovery initializes the app in discovery mode with sidebar.
func (a *StructuredApp) InitWithDiscovery() error {
	a.sidebarEnabled = true
	a.focusedPane = "sessions"

	// Load repositories and sessions
	a.refreshRepositories()

	// Also refresh available sessions for backward compatibility
	if err := a.refreshAvailableSessions(); err != nil {
		return err
	}

	// Load first session from selected repo if any exist
	if len(a.sessionsForRepo) > 0 {
		a.loadSession(a.sessionsForRepo[0].Name)
	} else if len(a.availableSessions) > 0 {
		// Fallback to old behavior if no repo sessions
		a.loadSession(a.availableSessions[0])
	}

	// Wire up event callbacks for dynamic session discovery
	a.eventWatcher.OnEvent(func(tmuxSession string, event claude.HookEvent) {
		if view, ok := a.views[tmuxSession]; ok {
			view.UpdateFromHookEvent(event)
			a.gui.Update(func(g *gocui.Gui) error { return nil })
		}
	})

	return nil
}

// refreshAvailableSessions rescans for Claude sessions.
func (a *StructuredApp) refreshAvailableSessions() error {
	sessions, err := a.tmuxClient.DiscoverClaudeSessions()
	if err != nil {
		return err
	}

	// Extract session names and sort them
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	sort.Strings(names)

	a.availableSessions = names

	// Adjust selection if needed
	if a.sidebarSelectedIdx >= len(a.availableSessions) {
		if len(a.availableSessions) > 0 {
			a.sidebarSelectedIdx = len(a.availableSessions) - 1
		} else {
			a.sidebarSelectedIdx = 0
		}
	}

	return nil
}

// loadSession loads a session into the main view area.
func (a *StructuredApp) loadSession(name string) {
	// Check if already loaded
	if _, ok := a.views[name]; ok {
		// Already loaded, just make it active
		for i, s := range a.sessions {
			if s == name {
				a.activeIdx = i
				return
			}
		}
	}

	// Calculate view dimensions
	maxX, maxY := a.gui.Size()
	paneMaxY := maxY - pane.StatusBarHeight

	var width, height int
	if a.sidebarEnabled {
		sidebarLayout := pane.CalculateSidebarLayout(maxX, paneMaxY)
		width = sidebarLayout.Main.Width()
		height = sidebarLayout.Main.Height()
	} else {
		width = maxX - 2
		height = paneMaxY - 2
	}

	// Create the view
	view := claude.NewView(name, width, height)

	// Initialize transcript from event file to load chat history
	if transcriptPath := claude.GetLatestTranscriptPath(name); transcriptPath != "" {
		view.InitTranscript(transcriptPath)
		view.PollTranscript() // Load existing messages
	}

	a.views[name] = view

	// Clear existing sessions and add just this one (single session mode for sidebar)
	a.sessions = []string{name}
	a.activeIdx = 0
}

// refreshRepositories reloads the configured repositories.
func (a *StructuredApp) refreshRepositories() {
	a.repositories = a.discoveryService.GetConfiguredRepositories()

	// Adjust selection if needed
	if a.repoSelectedIdx >= len(a.repositories) {
		if len(a.repositories) > 0 {
			a.repoSelectedIdx = len(a.repositories) - 1
		} else {
			a.repoSelectedIdx = 0
		}
	}

	// Refresh sessions for the selected repo
	a.refreshSessionsForSelectedRepo()
}

// refreshSessionsForSelectedRepo loads sessions for the currently selected repository.
func (a *StructuredApp) refreshSessionsForSelectedRepo() {
	if len(a.repositories) == 0 || a.repoSelectedIdx >= len(a.repositories) {
		a.sessionsForRepo = nil
		return
	}

	allSessions, err := a.discoveryService.DiscoverSessions()
	if err != nil {
		a.sessionsForRepo = nil
		return
	}

	repoPath := a.repositories[a.repoSelectedIdx].Path

	var filtered []*state.Session
	for _, sess := range allSessions {
		if sess.RepoPath == repoPath {
			filtered = append(filtered, sess)
		}
	}
	a.sessionsForRepo = filtered

	// Adjust session selection
	if a.sessionSelectedIdx >= len(a.sessionsForRepo) {
		if len(a.sessionsForRepo) > 0 {
			a.sessionSelectedIdx = len(a.sessionsForRepo) - 1
		} else {
			a.sessionSelectedIdx = 0
		}
	}

	// Load the selected session into the main view
	if len(a.sessionsForRepo) > 0 && a.sessionSelectedIdx < len(a.sessionsForRepo) {
		a.loadSession(a.sessionsForRepo[a.sessionSelectedIdx].Name)
	}
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

	// Handle sidebar layout (unified repos + sessions + main view)
	if a.sidebarEnabled {
		return a.layoutWithSidebar(g, maxX, paneMaxY, currentMode)
	}

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

// layoutWithSidebar renders the unified layout with repos panel, sessions panel, and main view.
func (a *StructuredApp) layoutWithSidebar(g *gocui.Gui, maxX, paneMaxY int, currentMode input.Mode) error {
	// No status bar in sidebar mode - use full height
	maxY := paneMaxY + pane.StatusBarHeight
	fullHeight := maxY
	layout := pane.CalculateUnifiedSidebarLayout(maxX, fullHeight)

	// Delete old sidebar view if it exists (we now have repos and sessions views)
	g.DeleteView("sidebar")

	// Render repos panel (top-left)
	reposView, err := g.SetView("repos-panel", layout.Repos.X0, layout.Repos.Y0,
		layout.Repos.X1, layout.Repos.Y1, 0)
	if err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
			return err
		}
	}
	a.renderReposPanel(reposView)

	// Render sessions panel (bottom-left)
	sessionsView, err := g.SetView("sessions-panel", layout.Sessions.X0, layout.Sessions.Y0,
		layout.Sessions.X1, layout.Sessions.Y1, 0)
	if err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
			return err
		}
	}
	a.renderSessionsPanel(sessionsView)

	// Render main session view if we have a session loaded
	if len(a.sessions) > 0 && a.activeIdx < len(a.sessions) {
		session := a.sessions[a.activeIdx]
		view := a.views[session]

		// Handle resize
		width := layout.Main.Width()
		height := layout.Main.Height()
		view.Resize(width, height)

		mainView, err := g.SetView("main-view", layout.Main.X0, layout.Main.Y0,
			layout.Main.X1, layout.Main.Y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}

		a.configureStructuredView(mainView, session, true, currentMode)
		mainView.Clear()
		fmt.Fprint(mainView, view.Render())
	} else {
		// No session loaded, show empty main view
		mainView, err := g.SetView("main-view", layout.Main.X0, layout.Main.Y0,
			layout.Main.X1, layout.Main.Y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}
		mainView.Title = " No Session "
		mainView.FrameColor = gocui.ColorDefault
		mainView.TitleColor = gocui.ColorDefault
		mainView.Clear()
		fmt.Fprint(mainView, "\n  Select a repository and\n  session from the sidebar\n\n  Press 'n' to create\n  a new session")
	}

	// Delete status bar if it exists (not needed in sidebar mode)
	g.DeleteView("status-bar")

	// Handle terminal modal (same as non-sidebar mode)
	if currentMode.IsTerminal() && a.terminalCtrl != nil && a.terminalTerm != nil {
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

		session := a.ActiveSession()
		v.Title = fmt.Sprintf(" %s [Ctrl+Q to exit] ", session)
		v.Frame = true
		v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
		v.FrameColor = gocui.ColorGreen
		v.TitleColor = gocui.ColorGreen
		v.Wrap = false
		v.Editable = true
		v.Editor = gocui.EditorFunc(a.makeTerminalModalEditor())

		modalWidth := width - 2
		modalHeight := height - 2
		a.terminalTerm.Resize(modalHeight, modalWidth)
		a.terminalCtrl.Resize(modalWidth, modalHeight)

		v.Clear()
		ui.RenderTerminal(v, a.terminalTerm)

		if _, err := g.SetCurrentView("terminal-modal"); err != nil {
			return err
		}

		if a.terminalTerm.CursorVisible() {
			cx, cy := a.terminalTerm.Cursor()
			v.SetCursor(cx, cy)
			g.Cursor = true
		} else {
			g.Cursor = false
		}
	} else {
		g.DeleteView("terminal-modal")

		if currentMode.IsInput() {
			inputBuffer := a.input.InputBuffer()
			x0, y0, x1, y1 := ui.ModalDimensions(maxX, maxY, 50, 3)
			v, err := g.SetView("input-modal", x0, y0, x1, y1, 0)
			if err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
					return err
				}
			}

			// Custom input modal for session/repo creation
			title := " New Session (Enter=confirm, Esc=cancel) "
			if a.inputPurpose == "add_repo" {
				title = " Add Repository Path (Enter=confirm, Esc=cancel) "
			}
			v.Title = title
			v.Frame = true
			v.FrameRunes = []rune{'━', '┃', '┏', '┓', '┗', '┛'}
			v.FrameColor = gocui.ColorYellow
			v.Editable = true
			v.Clear()
			fmt.Fprintf(v, " %s", inputBuffer)
			v.Editor = gocui.EditorFunc(a.makeInputEditor())

			if _, err := g.SetCurrentView("input-modal"); err != nil {
				return err
			}
			g.Cursor = true
			v.SetCursor(len(inputBuffer)+1, 0)
		} else {
			g.DeleteView("input-modal")
			// Set focus based on which pane is focused
			switch a.focusedPane {
			case "repos":
				g.SetCurrentView("repos-panel")
			case "sessions":
				g.SetCurrentView("sessions-panel")
			default:
				g.SetCurrentView("sessions-panel")
			}
			g.Cursor = false
		}
	}

	return nil
}

// renderReposPanel draws the repository list in the repos panel.
func (a *StructuredApp) renderReposPanel(v *gocui.View) {
	v.Title = " [r] Repositories "
	v.Frame = true

	// Highlight if focused
	if a.focusedPane == "repos" {
		v.FrameColor = gocui.ColorCyan
		v.TitleColor = gocui.ColorCyan
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}
	v.Clear()

	if len(a.repositories) == 0 {
		fmt.Fprint(v, "\n  No repositories\n  configured.\n\n  Press 'a' to add.")
		return
	}

	for i, repo := range a.repositories {
		prefix := "  "
		if i == a.repoSelectedIdx {
			prefix = "> "
		}

		// Truncate name if too long
		displayName := repo.Name
		maxLen := pane.SidebarWidth - 4
		if len(displayName) > maxLen {
			displayName = displayName[:maxLen-2] + ".."
		}

		fmt.Fprintf(v, "%s%s\n", prefix, displayName)
	}
}

// renderSessionsPanel draws the session list for the selected repo.
func (a *StructuredApp) renderSessionsPanel(v *gocui.View) {
	repoName := ""
	if a.repoSelectedIdx < len(a.repositories) {
		repoName = a.repositories[a.repoSelectedIdx].Name
	}
	v.Title = fmt.Sprintf(" [s] Sessions - %s ", repoName)
	v.Frame = true

	// Highlight if focused
	if a.focusedPane == "sessions" {
		v.FrameColor = gocui.ColorCyan
		v.TitleColor = gocui.ColorCyan
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}
	v.Clear()

	if len(a.repositories) == 0 {
		fmt.Fprint(v, "\n  Add a repository\n  first using 'a'\n  in the repos panel.")
		return
	}

	if len(a.sessionsForRepo) == 0 {
		fmt.Fprint(v, "\n  No sessions for\n  this repository.\n\n  Press 'n' to create.")
		return
	}

	for i, sess := range a.sessionsForRepo {
		prefix := "  "
		if i == a.sessionSelectedIdx {
			prefix = "> "
		}

		// Format: branch (status)
		branchDisplay := sess.Branch
		if sess.Worktree != "" {
			branchDisplay += " [wt]"
		}

		statusIcon := ""
		switch sess.Status {
		case state.StatusActive:
			statusIcon = " [active]"
		case state.StatusTool:
			statusIcon = " [tool]"
		case state.StatusThinking:
			statusIcon = " [thinking]"
		case state.StatusNeedsInput:
			statusIcon = " [input]"
		}

		if sess.Attached {
			statusIcon = " [attached]"
		}

		fmt.Fprintf(v, "%s%s%s\n", prefix, branchDisplay, statusIcon)
	}

	// Add footer with hints
	height := v.InnerHeight()
	sessionCount := len(a.sessionsForRepo)
	if height > sessionCount+3 {
		fmt.Fprint(v, "\n───────────────────────\n")
		fmt.Fprint(v, " j/k:nav i:term n:new\n")
		fmt.Fprint(v, " x:del Ctrl+U/D:scroll")
	}
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
				if a.sidebarEnabled {
					// Unified sidebar navigation based on focused pane
					switch k {
					case 'k':
						a.navigateUp()
					case 'j':
						a.navigateDown()
					case 'h':
						// Move focus left (sessions -> repos)
						if a.focusedPane == "sessions" {
							a.focusedPane = "repos"
						}
					case 'l':
						// Move focus right (repos -> sessions)
						if a.focusedPane == "repos" {
							a.focusedPane = "sessions"
						}
					}
				} else {
					// Pane navigation (non-sidebar mode)
					switch k {
					case 'h', 'k':
						a.prevSession()
					case 'j', 'l':
						a.nextSession()
					}
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

	// Enter terminal mode with Enter (or select in sidebar mode)
	if err := a.gui.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsInput() {
			// Handle input submission
			inputText := a.input.ConsumeInputBuffer()
			switch a.inputPurpose {
			case "new_session":
				a.createNewSessionForRepo(inputText)
			case "add_repo":
				a.addRepository(inputText)
			}
			a.inputPurpose = ""
			return nil
		}
		if a.input.Mode().IsNormal() {
			if a.sidebarEnabled {
				// Enter terminal mode for current session
				if len(a.sessionsForRepo) > 0 || len(a.sessions) > 0 {
					return a.enterTerminalModal()
				}
				return nil
			}
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

	// Unified sidebar keybindings

	// 'r' - Focus repos pane
	if err := a.gui.SetKeybinding("", 'r', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			a.focusedPane = "repos"
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("r")
		}
		return nil
	}); err != nil {
		return err
	}

	// 's' - Focus sessions pane
	if err := a.gui.SetKeybinding("", 's', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			a.focusedPane = "sessions"
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("s")
		}
		return nil
	}); err != nil {
		return err
	}

	// 'n' - Create new session (when in sessions pane)
	if err := a.gui.SetKeybinding("", 'n', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			if a.focusedPane == "sessions" && len(a.repositories) > 0 {
				a.inputPurpose = "new_session"
				a.input.EnterInputMode()
			}
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("n")
		}
		return nil
	}); err != nil {
		return err
	}

	// 'a' - Add repo (when in repos pane)
	if err := a.gui.SetKeybinding("", 'a', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			if a.focusedPane == "repos" {
				a.inputPurpose = "add_repo"
				a.input.EnterInputMode()
			}
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("a")
		}
		return nil
	}); err != nil {
		return err
	}

	// 'd' or 'x' - Delete selected item based on focused pane
	deleteHandler := func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			switch a.focusedPane {
			case "repos":
				a.deleteSelectedRepo()
			case "sessions":
				a.deleteSelectedSessionFromRepo()
			}
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("d")
		}
		return nil
	}
	if err := a.gui.SetKeybinding("", 'd', gocui.ModNone, deleteHandler); err != nil {
		return err
	}
	if err := a.gui.SetKeybinding("", 'x', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			switch a.focusedPane {
			case "repos":
				a.deleteSelectedRepo()
			case "sessions":
				a.deleteSelectedSessionFromRepo()
			}
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("x")
		}
		return nil
	}); err != nil {
		return err
	}

	// 'R' - Refresh repos and sessions
	if err := a.gui.SetKeybinding("", 'R', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if a.input.Mode().IsNormal() && a.sidebarEnabled {
			a.refreshRepositories()
			a.refreshAvailableSessions()
		} else if a.input.Mode().IsTerminal() && a.terminalCtrl != nil {
			a.terminalCtrl.SendLiteralKeys("R")
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

// navigateUp moves up in the focused pane (repos or sessions).
func (a *StructuredApp) navigateUp() {
	switch a.focusedPane {
	case "repos":
		if len(a.repositories) == 0 {
			return
		}
		a.repoSelectedIdx = (a.repoSelectedIdx - 1 + len(a.repositories)) % len(a.repositories)
		a.refreshSessionsForSelectedRepo()
	case "sessions":
		if len(a.sessionsForRepo) == 0 {
			return
		}
		a.sessionSelectedIdx = (a.sessionSelectedIdx - 1 + len(a.sessionsForRepo)) % len(a.sessionsForRepo)
		if a.sessionSelectedIdx < len(a.sessionsForRepo) {
			a.loadSession(a.sessionsForRepo[a.sessionSelectedIdx].Name)
		}
	}
}

// navigateDown moves down in the focused pane (repos or sessions).
func (a *StructuredApp) navigateDown() {
	switch a.focusedPane {
	case "repos":
		if len(a.repositories) == 0 {
			return
		}
		a.repoSelectedIdx = (a.repoSelectedIdx + 1) % len(a.repositories)
		a.refreshSessionsForSelectedRepo()
	case "sessions":
		if len(a.sessionsForRepo) == 0 {
			return
		}
		a.sessionSelectedIdx = (a.sessionSelectedIdx + 1) % len(a.sessionsForRepo)
		if a.sessionSelectedIdx < len(a.sessionsForRepo) {
			a.loadSession(a.sessionsForRepo[a.sessionSelectedIdx].Name)
		}
	}
}

// createNewSessionForRepo creates a new session for the currently selected repository.
func (a *StructuredApp) createNewSessionForRepo(branchName string) {
	if branchName == "" || len(a.repositories) == 0 || a.repoSelectedIdx >= len(a.repositories) {
		return
	}

	repo := a.repositories[a.repoSelectedIdx]

	// Check if branch exists
	branches, _ := git.ListBranches(repo.Path)
	branchExists := false
	for _, b := range branches {
		if b == branchName {
			branchExists = true
			break
		}
	}

	sessionName, err := a.sessionManager.CreateSession(repo.Path, branchName, !branchExists)
	if err != nil {
		return // Silently fail
	}

	// Refresh and load
	a.refreshSessionsForSelectedRepo()
	a.refreshAvailableSessions()

	// Find and select the new session
	for i, sess := range a.sessionsForRepo {
		if sess.Name == sessionName {
			a.sessionSelectedIdx = i
			a.loadSession(sessionName)
			break
		}
	}
}

// addRepository adds a new repository to the config.
func (a *StructuredApp) addRepository(path string) {
	if path == "" {
		return
	}

	// Validate it's a git repo
	if _, err := git.FindRepoRoot(path); err != nil {
		return // Silently fail - not a git repo
	}

	if err := a.config.AddRepository(path); err != nil {
		return // Silently fail
	}

	a.refreshRepositories()

	// Select the newly added repo
	for i, repo := range a.repositories {
		if strings.HasSuffix(repo.Path, strings.TrimPrefix(path, "~")) || repo.Path == path {
			a.repoSelectedIdx = i
			a.refreshSessionsForSelectedRepo()
			break
		}
	}
}

// deleteSelectedRepo removes the selected repository from the config.
func (a *StructuredApp) deleteSelectedRepo() {
	if len(a.repositories) == 0 || a.repoSelectedIdx >= len(a.repositories) {
		return
	}

	repo := a.repositories[a.repoSelectedIdx]
	if err := a.config.RemoveRepository(repo.Path); err != nil {
		return // Silently fail
	}

	// Adjust selection
	if a.repoSelectedIdx > 0 {
		a.repoSelectedIdx--
	}
	a.refreshRepositories()
}

// deleteSelectedSessionFromRepo kills the selected session in the sessions pane.
func (a *StructuredApp) deleteSelectedSessionFromRepo() {
	if len(a.sessionsForRepo) == 0 || a.sessionSelectedIdx >= len(a.sessionsForRepo) {
		return
	}

	sess := a.sessionsForRepo[a.sessionSelectedIdx]

	// Delete via session manager (kills tmux session, optionally removes worktree)
	a.sessionManager.DeleteSession(sess.Name, false) // Don't remove worktree by default

	// Remove from views if loaded
	if _, ok := a.views[sess.Name]; ok {
		delete(a.views, sess.Name)
	}

	// Remove from sessions list if it's the current one
	for i, s := range a.sessions {
		if s == sess.Name {
			a.sessions = append(a.sessions[:i], a.sessions[i+1:]...)
			if a.activeIdx >= len(a.sessions) && a.activeIdx > 0 {
				a.activeIdx--
			}
			break
		}
	}

	// Refresh sessions
	a.refreshSessionsForSelectedRepo()
	a.refreshAvailableSessions()
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

// DiscoveryService returns the discovery service for external use.
func (a *StructuredApp) DiscoveryService() *discovery.Service {
	return a.discoveryService
}

// SessionManager returns the session manager for external use.
func (a *StructuredApp) SessionManager() *session.Manager {
	return a.sessionManager
}

// Config returns the config for external use.
func (a *StructuredApp) Config() *config.Config {
	return a.config
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
