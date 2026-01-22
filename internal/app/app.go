// Package app provides the main application orchestration for cmux.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/config"
	"github.com/abdullathedruid/cmux/internal/controller"
	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/notes"
	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/abdullathedruid/cmux/internal/status"
	"github.com/abdullathedruid/cmux/internal/tmux"
)

// App is the main application.
type App struct {
	gui    *gocui.Gui
	config *config.Config
	state  *state.State
	tmux   tmux.Client
	notes  *notes.Store
	ctx    *controller.Context

	// Controllers
	dashboard *controller.DashboardController
	sessions  *controller.SessionsController
	statusBar *controller.StatusBarController
	help      *controller.HelpController
	worktree  *controller.WorktreeController
	editor    *controller.EditorController
	search    *controller.SearchController
	wizard    *controller.WizardController

	// Pending attach - when set, main loop exits and attaches to this session
	pendingAttach string
}

// New creates a new App.
func New(cfg *config.Config) (*App, error) {
	s := state.New()
	t := tmux.NewClient(cfg.ClaudeCommand)
	n := notes.NewStore(cfg.NotesFile())
	ctx := controller.NewContext(s, t)

	// Load existing notes
	if err := n.Load(); err != nil {
		// Non-fatal, continue without notes
	}

	app := &App{
		config: cfg,
		state:  s,
		tmux:   t,
		notes:  n,
		ctx:    ctx,
	}

	// Set up context callbacks
	ctx.OnAttach = app.attachSession
	ctx.OnPopupAttach = app.popupSession
	ctx.OnShowDiff = app.showDiff
	ctx.OnNew = app.newSession
	ctx.OnDelete = app.deleteSession
	ctx.OnRefresh = app.refresh
	ctx.OnQuit = app.quit
	ctx.OnToggleView = app.toggleView
	ctx.OnShowHelp = app.showHelp

	// Create controllers
	app.dashboard = controller.NewDashboardController(ctx)
	app.sessions = controller.NewSessionsController(ctx)
	app.statusBar = controller.NewStatusBarController(ctx)
	app.help = controller.NewHelpController(ctx)
	app.worktree = controller.NewWorktreeController(ctx)
	app.editor = controller.NewEditorController(ctx, app.saveNote)
	app.search = controller.NewSearchController(ctx, nil) // nil = just select, don't attach
	app.wizard = controller.NewWizardController(ctx)

	return app, nil
}

// initGui initializes or reinitializes the gocui GUI.
func (a *App) initGui() error {
	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode: gocui.OutputTrue,
	})
	if err != nil {
		return fmt.Errorf("initializing gui: %w", err)
	}

	a.gui = g
	a.gui.SetManagerFunc(a.layout)
	a.gui.Mouse = false
	a.gui.Cursor = false

	// Set up keybindings
	if err := a.setupKeybindings(); err != nil {
		a.gui.Close()
		return fmt.Errorf("setting up keybindings: %w", err)
	}

	return nil
}

// Run runs the application.
func (a *App) Run() error {
	for {
		// Initialize or reinitialize GUI
		if err := a.initGui(); err != nil {
			return err
		}

		// Initial refresh
		if err := a.refresh(); err != nil {
			a.gui.Close()
			return err
		}

		// Start background refresh
		stopRefresh := make(chan struct{})
		go a.backgroundRefresh(stopRefresh)

		// Run main loop
		err := a.gui.MainLoop()
		close(stopRefresh)
		a.gui.Close()

		// Check if we need to attach to a session
		if a.pendingAttach != "" {
			name := a.pendingAttach
			a.pendingAttach = ""
			// This blocks until the user detaches
			a.tmux.AttachSession(name)
			// Loop back to reinitialize GUI
			continue
		}

		// Normal exit
		if err == nil || err == gocui.ErrQuit {
			return nil
		}
		return err
	}
}

// layout is the gocui manager function.
func (a *App) layout(g *gocui.Gui) error {
	// Status bar at bottom
	if err := a.statusBar.Layout(g); err != nil {
		return fmt.Errorf("statusBar.Layout: %w", err)
	}

	// Main view depends on current mode
	if a.state.IsDashboardView() {
		// Delete list views if they exist
		g.DeleteView("sessions")
		g.DeleteView("details")

		if err := a.dashboard.Layout(g); err != nil {
			return fmt.Errorf("dashboard.Layout: %w", err)
		}
		if _, err := g.SetCurrentView("dashboard"); err != nil {
			return fmt.Errorf("SetCurrentView(dashboard): %w", err)
		}
	} else {
		// Delete dashboard view if it exists
		g.DeleteView("dashboard")

		if err := a.sessions.Layout(g); err != nil {
			return fmt.Errorf("sessions.Layout: %w", err)
		}
		if _, err := g.SetCurrentView("sessions"); err != nil {
			return fmt.Errorf("SetCurrentView(sessions): %w", err)
		}
	}

	// Render content
	if err := a.render(); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	// Help overlay (if visible)
	if a.help.IsVisible() {
		if err := a.help.Layout(g); err != nil {
			return fmt.Errorf("help.Layout: %w", err)
		}
	}

	// Worktree picker overlay (if visible)
	if a.worktree.IsVisible() {
		if err := a.worktree.Layout(g); err != nil {
			return fmt.Errorf("worktree.Layout: %w", err)
		}
	}

	// Editor overlay (if visible)
	if a.editor.IsVisible() {
		if err := a.editor.Layout(g); err != nil {
			return fmt.Errorf("editor.Layout: %w", err)
		}
	}

	// Search overlay (if visible)
	if a.search.IsVisible() {
		if err := a.search.Layout(g); err != nil {
			return fmt.Errorf("search.Layout: %w", err)
		}
	}

	// Wizard overlay (if visible)
	if a.wizard.IsVisible() {
		if err := a.wizard.Layout(g); err != nil {
			return fmt.Errorf("wizard.Layout: %w", err)
		}
	}

	return nil
}

// render renders all views.
func (a *App) render() error {
	if a.state.IsDashboardView() {
		if err := a.dashboard.Render(a.gui); err != nil {
			return fmt.Errorf("dashboard.Render: %w", err)
		}
	} else {
		if err := a.sessions.Render(a.gui); err != nil {
			return fmt.Errorf("sessions.Render: %w", err)
		}
	}
	if err := a.statusBar.Render(a.gui); err != nil {
		return fmt.Errorf("statusBar.Render: %w", err)
	}
	return nil
}

// setupKeybindings sets up global keybindings.
func (a *App) setupKeybindings() error {
	// Global quit
	if err := a.gui.SetKeybinding("", 'q', gocui.ModNone, a.quitHandler); err != nil {
		return fmt.Errorf("keybinding 'q': %w", err)
	}
	if err := a.gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, a.quitHandler); err != nil {
		return fmt.Errorf("keybinding 'Ctrl+C': %w", err)
	}

	// Toggle view
	if err := a.gui.SetKeybinding("", 'v', gocui.ModNone, a.toggleViewHandler); err != nil {
		return fmt.Errorf("keybinding 'v': %w", err)
	}

	// Help
	if err := a.gui.SetKeybinding("", '?', gocui.ModNone, a.helpHandler); err != nil {
		return fmt.Errorf("keybinding '?': %w", err)
	}

	// Worktree picker
	if err := a.gui.SetKeybinding("", 'w', gocui.ModNone, a.worktreeHandler); err != nil {
		return fmt.Errorf("keybinding 'w': %w", err)
	}

	// Note editor
	if err := a.gui.SetKeybinding("", 'e', gocui.ModNone, a.editNoteHandler); err != nil {
		return fmt.Errorf("keybinding 'e': %w", err)
	}

	// Search
	if err := a.gui.SetKeybinding("", '/', gocui.ModNone, a.searchHandler); err != nil {
		return fmt.Errorf("keybinding '/': %w", err)
	}

	// Session wizard (Shift+N)
	if err := a.gui.SetKeybinding("", 'N', gocui.ModNone, a.wizardHandler); err != nil {
		return fmt.Errorf("keybinding 'N': %w", err)
	}

	// Navigation keys (global to work around tcell keybinding issue)
	if err := a.gui.SetKeybinding("", 'j', gocui.ModNone, a.cursorDownHandler); err != nil {
		return fmt.Errorf("keybinding 'j': %w", err)
	}
	if err := a.gui.SetKeybinding("", 'k', gocui.ModNone, a.cursorUpHandler); err != nil {
		return fmt.Errorf("keybinding 'k': %w", err)
	}
	if err := a.gui.SetKeybinding("", 'h', gocui.ModNone, a.cursorLeftHandler); err != nil {
		return fmt.Errorf("keybinding 'h': %w", err)
	}
	if err := a.gui.SetKeybinding("", 'l', gocui.ModNone, a.cursorRightHandler); err != nil {
		return fmt.Errorf("keybinding 'l': %w", err)
	}

	// Set up controller-specific keybindings
	if err := a.dashboard.Keybindings(a.gui); err != nil {
		return fmt.Errorf("dashboard.Keybindings: %w", err)
	}
	if err := a.sessions.Keybindings(a.gui); err != nil {
		return fmt.Errorf("sessions.Keybindings: %w", err)
	}
	if err := a.help.Keybindings(a.gui); err != nil {
		return fmt.Errorf("help.Keybindings: %w", err)
	}
	if err := a.worktree.Keybindings(a.gui); err != nil {
		return fmt.Errorf("worktree.Keybindings: %w", err)
	}
	if err := a.editor.Keybindings(a.gui); err != nil {
		return fmt.Errorf("editor.Keybindings: %w", err)
	}
	if err := a.search.Keybindings(a.gui); err != nil {
		return fmt.Errorf("search.Keybindings: %w", err)
	}
	if err := a.wizard.Keybindings(a.gui); err != nil {
		return fmt.Errorf("wizard.Keybindings: %w", err)
	}

	return nil
}

// Handlers

// isModalOpen returns true if any modal is currently visible.
func (a *App) isModalOpen() bool {
	return a.help.IsVisible() || a.worktree.IsVisible() ||
		a.editor.IsVisible() || a.search.IsVisible() || a.wizard.IsVisible()
}

func (a *App) quitHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	return a.quit()
}

func (a *App) toggleViewHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	a.toggleView()
	return nil
}

func (a *App) helpHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	a.showHelp()
	return nil
}

func (a *App) worktreeHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	return a.worktree.Show(g)
}

func (a *App) editNoteHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	sess := a.state.GetSelectedSession()
	if sess == nil {
		return nil
	}
	return a.editor.Show(g, sess.Name, sess.Note)
}

func (a *App) searchHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	return a.search.Show(g)
}

func (a *App) wizardHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	return a.wizard.Show(g)
}

func (a *App) cursorDownHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	a.state.SelectNext()
	return a.render()
}

func (a *App) cursorUpHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	a.state.SelectPrev()
	return a.render()
}

func (a *App) cursorLeftHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	// For now, left/right do the same as up/down in list navigation
	// Could be used for horizontal grid navigation in future
	a.state.SelectPrev()
	return a.render()
}

func (a *App) cursorRightHandler(g *gocui.Gui, v *gocui.View) error {
	if a.isModalOpen() {
		return nil
	}
	a.state.SelectNext()
	return a.render()
}

// Actions

func (a *App) quit() error {
	return gocui.ErrQuit
}

func (a *App) toggleView() {
	a.state.ToggleView()
}

func (a *App) showHelp() {
	a.help.Toggle(a.gui)
}

func (a *App) refresh() error {
	// Get tmux sessions
	tmuxSessions, err := a.tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing tmux sessions: %w", err)
	}

	// Convert to state sessions
	sessions := make([]*state.Session, 0, len(tmuxSessions))
	for _, ts := range tmuxSessions {
		sess := a.convertSession(ts)
		// Load note for this session
		sess.Note = a.notes.Get(sess.Name)
		sessions = append(sessions, sess)
	}

	// Update state
	a.state.UpdateSessions(sessions)
	a.state.SelectFirst()

	return nil
}

// convertSession converts a tmux session to a state session.
func (a *App) convertSession(ts tmux.Session) *state.Session {
	sess := &state.Session{
		Name:     ts.Name,
		Attached: ts.Attached,
		Created:  ts.Created,
		Status:   state.StatusIdle,
	}

	// Read status from hook-written file
	fullStatus := status.ReadFullStatus(ts.Name)
	if fullStatus.Found {
		switch fullStatus.Status {
		case "tool":
			sess.Status = state.StatusTool
		case "active":
			sess.Status = state.StatusActive
		case "thinking":
			sess.Status = state.StatusThinking
		case "needs_input":
			sess.Status = state.StatusNeedsInput
		default:
			sess.Status = state.StatusIdle
		}
		sess.CurrentTool = fullStatus.Tool
		sess.ToolSummary = fullStatus.Summary
		sess.LastActive = fullStatus.LastActive
		sess.SessionID = fullStatus.SessionID
		sess.LastPrompt = fullStatus.LastPrompt

		// Convert tool history
		sess.ToolHistory = make([]state.ToolHistoryEntry, len(fullStatus.ToolHistory))
		for i, entry := range fullStatus.ToolHistory {
			sess.ToolHistory[i] = state.ToolHistoryEntry{
				Tool:      entry.Tool,
				Summary:   entry.Summary,
				Result:    entry.Result,
				Timestamp: entry.Timestamp,
			}
		}
	}

	// Try to get git info from the session path
	if ts.Path != "" {
		if info, err := git.GetRepoInfo(ts.Path); err == nil {
			sess.RepoPath = info.Root
			sess.RepoName = info.Name
			sess.Branch = info.Branch
		}

		// Check if it's a worktree
		if git.IsWorktreePath(ts.Path) {
			sess.Worktree = ts.Path
		} else {
			sess.Worktree = ts.Path
		}
	}

	// If no repo info, try to derive from session name
	if sess.RepoName == "" {
		// Session name format: {repo}-{worktree}
		sess.RepoName = ts.Name
	}

	return sess
}

func (a *App) attachSession(name string) error {
	// Check if we're inside tmux
	if a.tmux.IsInsideTmux() {
		// Use switch-client
		return a.tmux.SwitchSession(name)
	}

	// Set pending attach and exit main loop
	// The Run() loop will handle the actual attach and reinitialize after detach
	a.pendingAttach = name
	return gocui.ErrQuit
}

func (a *App) popupSession(name string) error {
	// Popup only works when inside tmux and when tmux supports it
	if !a.tmux.IsInsideTmux() {
		// Fall back to regular attach when not inside tmux
		return a.attachSession(name)
	}

	// Don't popup the same session we're currently in - causes recursive display issues
	if a.tmux.GetCurrentSession() == name {
		// Fall back to switch-client (effectively a no-op when same session)
		return a.tmux.SwitchSession(name)
	}

	if !a.tmux.SupportsPopup() {
		// Fall back to switch-client when popup not supported
		return a.tmux.SwitchSession(name)
	}

	// Open in popup - this blocks until popup closes
	return a.tmux.DisplayPopup(name)
}

func (a *App) showDiff(name string) error {
	// Get the session to find its worktree path
	sess := a.state.GetSession(name)
	if sess == nil {
		return nil
	}

	// Determine the working directory
	workdir := sess.Worktree
	if workdir == "" {
		workdir = sess.RepoPath
	}
	if workdir == "" {
		return nil // No git directory available
	}

	// Popup only works when inside tmux and when tmux supports it
	if !a.tmux.IsInsideTmux() || !a.tmux.SupportsPopup() {
		return nil // Can't show diff without popup support
	}

	// Open diff in popup - this blocks until popup closes
	return a.tmux.DisplayDiffPopup(workdir)
}

func (a *App) newSession() error {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Try to get repo info
	var sessionName string
	info, err := git.GetRepoInfo(cwd)
	if err == nil {
		// In a git repo - use repo name
		branch := info.Branch
		if git.IsWorktreePath(cwd) {
			branch = git.ExtractWorktreeName(cwd)
		}
		if branch == "" || branch == "main" || branch == "master" {
			sessionName = info.Name + "-main"
		} else {
			sessionName = fmt.Sprintf("%s-%s", info.Name, sanitize(branch))
		}
	} else {
		// Not in a git repo - use directory name + timestamp
		dirName := filepath.Base(cwd)
		sessionName = fmt.Sprintf("%s-%d", sanitize(dirName), time.Now().Unix())
	}

	// Check if session already exists
	if a.tmux.HasSession(sessionName) {
		// Attach to it instead
		return a.attachSession(sessionName)
	}

	// Create the session
	if err := a.tmux.CreateSession(sessionName, cwd, true); err != nil {
		return fmt.Errorf("creating session %q: %w", sessionName, err)
	}

	// Refresh and attach
	if err := a.refresh(); err != nil {
		return fmt.Errorf("refreshing after session create: %w", err)
	}

	return a.attachSession(sessionName)
}

func (a *App) deleteSession(name string) error {
	if err := a.tmux.KillSession(name); err != nil {
		return fmt.Errorf("killing session %q: %w", name, err)
	}
	// Also delete the note and status file
	a.notes.Delete(name)
	status.CleanupStatus(name)
	return a.refresh()
}

func (a *App) saveNote(sessionName, content string) error {
	// Save to notes store
	if err := a.notes.Set(sessionName, content); err != nil {
		return fmt.Errorf("saving note for %q: %w", sessionName, err)
	}
	// Update state
	a.state.UpdateNote(sessionName, content)
	return nil
}

func (a *App) backgroundRefresh(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(a.config.RefreshInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.gui.Update(func(g *gocui.Gui) error {
				if err := a.refresh(); err != nil {
					return nil // Don't crash on refresh errors
				}
				return a.render()
			})
		}
	}
}

// sanitize removes problematic characters from session names.
func sanitize(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == '/' || c == ' ' {
			result = append(result, '-')
		}
	}
	return string(result)
}
