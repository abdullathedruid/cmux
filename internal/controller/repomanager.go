package controller

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/discovery"
	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/session"
	"github.com/abdullathedruid/cmux/internal/state"
)

const (
	repoManagerViewName    = "repo-manager"
	repoListViewName       = "repo-list"
	sessionListViewName    = "session-list"
	confirmDeleteViewName  = "confirm-delete"
	addRepoInputViewName   = "add-repo-input"
)

// FocusPanel indicates which panel has focus.
type FocusPanel int

const (
	FocusRepoList FocusPanel = iota
	FocusSessionList
)

// RepoManagerController manages the repository-centric session view.
type RepoManagerController struct {
	ctx             *Context
	discovery       *discovery.Service
	sessionManager  *session.Manager
	gui             *gocui.Gui
	visible         bool
	focus           FocusPanel
	repositories    []discovery.RepositoryInfo
	sessions        []*state.Session
	repoSelected    int
	sessionSelected int

	// Input state
	inputMode      bool
	inputBuffer    string
	inputPurpose   string // "add_repo" or "new_session"

	// Delete confirmation state
	confirmMode       bool
	confirmCallback   func(bool)
	pendingDeleteName string
}

// NewRepoManagerController creates a new repo manager controller.
func NewRepoManagerController(ctx *Context, disc *discovery.Service, sessMgr *session.Manager) *RepoManagerController {
	return &RepoManagerController{
		ctx:            ctx,
		discovery:      disc,
		sessionManager: sessMgr,
	}
}

// Name returns the view name.
func (c *RepoManagerController) Name() string {
	return repoManagerViewName
}

// IsVisible returns whether the controller is visible.
func (c *RepoManagerController) IsVisible() bool {
	return c.visible
}

// Show shows the repo manager.
func (c *RepoManagerController) Show(g *gocui.Gui) error {
	c.visible = true
	c.gui = g
	c.focus = FocusRepoList
	c.repoSelected = 0
	c.sessionSelected = 0
	c.inputMode = false
	c.confirmMode = false

	c.refresh()
	return c.Layout(g)
}

// Hide hides the repo manager.
func (c *RepoManagerController) Hide(g *gocui.Gui) error {
	c.visible = false
	g.DeleteView(repoListViewName)
	g.DeleteView(sessionListViewName)
	g.DeleteView(confirmDeleteViewName)
	g.DeleteView(addRepoInputViewName)
	return nil
}

// refresh reloads repositories and sessions.
func (c *RepoManagerController) refresh() {
	c.repositories = c.discovery.GetConfiguredRepositories()

	// Get sessions for selected repo
	if c.repoSelected < len(c.repositories) {
		c.refreshSessionsForRepo(c.repositories[c.repoSelected].Path)
	} else {
		c.sessions = nil
	}
}

// refreshSessionsForRepo loads sessions for a specific repository.
func (c *RepoManagerController) refreshSessionsForRepo(repoPath string) {
	allSessions, err := c.discovery.DiscoverSessions()
	if err != nil {
		c.sessions = nil
		return
	}

	absRepoPath, _ := filepath.Abs(repoPath)

	var filtered []*state.Session
	for _, sess := range allSessions {
		sessRepoAbs, _ := filepath.Abs(sess.RepoPath)
		if sessRepoAbs == absRepoPath {
			filtered = append(filtered, sess)
		}
	}
	c.sessions = filtered

	// Adjust selection
	if c.sessionSelected >= len(c.sessions) && len(c.sessions) > 0 {
		c.sessionSelected = len(c.sessions) - 1
	}
}

// Layout arranges the two-panel view.
func (c *RepoManagerController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Left panel: 30% width for repo list
	repoWidth := maxX * 30 / 100
	if repoWidth < 20 {
		repoWidth = 20
	}

	// Repo list view
	if err := c.layoutRepoList(g, 0, 0, repoWidth-1, maxY-1); err != nil {
		return err
	}

	// Session list view (remaining space)
	if err := c.layoutSessionList(g, repoWidth, 0, maxX-1, maxY-1); err != nil {
		return err
	}

	// Handle modals
	if c.inputMode {
		return c.layoutInputModal(g, maxX, maxY)
	}
	if c.confirmMode {
		return c.layoutConfirmModal(g, maxX, maxY)
	}

	return nil
}

func (c *RepoManagerController) layoutRepoList(g *gocui.Gui, x0, y0, x1, y1 int) error {
	v, err := g.SetView(repoListViewName, x0, y0, x1, y1, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = " Repositories "
	v.Frame = true
	v.Wrap = false
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.handleRepoListKeys)

	if c.focus == FocusRepoList && !c.inputMode && !c.confirmMode {
		v.FrameColor = gocui.ColorCyan
		v.TitleColor = gocui.ColorCyan
		g.SetCurrentView(repoListViewName)
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}

	v.Clear()
	c.renderRepoList(v)

	return nil
}

func (c *RepoManagerController) layoutSessionList(g *gocui.Gui, x0, y0, x1, y1 int) error {
	v, err := g.SetView(sessionListViewName, x0, y0, x1, y1, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	repoName := ""
	if c.repoSelected < len(c.repositories) {
		repoName = c.repositories[c.repoSelected].Name
	}
	v.Title = fmt.Sprintf(" Sessions - %s ", repoName)
	v.Frame = true
	v.Wrap = false
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.handleSessionListKeys)

	if c.focus == FocusSessionList && !c.inputMode && !c.confirmMode {
		v.FrameColor = gocui.ColorCyan
		v.TitleColor = gocui.ColorCyan
		g.SetCurrentView(sessionListViewName)
	} else {
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}

	v.Clear()
	c.renderSessionList(v)

	return nil
}

func (c *RepoManagerController) layoutInputModal(g *gocui.Gui, maxX, maxY int) error {
	width := 60
	height := 5
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(addRepoInputViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	title := " Add Repository "
	if c.inputPurpose == "new_session" {
		title = " New Session - Enter Branch Name "
	}
	v.Title = title
	v.Frame = true
	v.FrameColor = gocui.ColorYellow
	v.TitleColor = gocui.ColorYellow
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.handleInputKeys)

	v.Clear()
	fmt.Fprintf(v, "\n  > %s_\n", c.inputBuffer)
	fmt.Fprint(v, "\n  Enter: Confirm  Esc: Cancel")

	g.SetCurrentView(addRepoInputViewName)
	return nil
}

func (c *RepoManagerController) layoutConfirmModal(g *gocui.Gui, maxX, maxY int) error {
	width := 50
	height := 7
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(confirmDeleteViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = " Delete Session "
	v.Frame = true
	v.FrameColor = gocui.ColorRed
	v.TitleColor = gocui.ColorRed
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.handleConfirmKeys)

	v.Clear()
	fmt.Fprintf(v, "\n  Delete: %s\n", c.pendingDeleteName)
	fmt.Fprint(v, "\n  Also remove worktree?\n")
	fmt.Fprint(v, "\n  [y] Yes  [n] No  [Esc] Cancel")

	g.SetCurrentView(confirmDeleteViewName)
	return nil
}

func (c *RepoManagerController) renderRepoList(v *gocui.View) {
	if len(c.repositories) == 0 {
		fmt.Fprint(v, "\n  No repositories\n  configured.\n\n  Press 'a' to add\n  a repository.")
		return
	}

	for i, repo := range c.repositories {
		prefix := "  "
		if i == c.repoSelected {
			prefix = "> "
		}

		// Truncate name if needed
		name := repo.Name
		maxLen := 18
		if len(name) > maxLen {
			name = name[:maxLen-2] + ".."
		}

		fmt.Fprintf(v, "%s%s\n", prefix, name)
	}

	// Footer
	fmt.Fprint(v, "\n───────────────────\n")
	fmt.Fprint(v, " [j/k] Navigate\n")
	fmt.Fprint(v, " [a] Add  [d] Remove\n")
	fmt.Fprint(v, " [Tab/l] Sessions\n")
	fmt.Fprint(v, " [q/Esc] Close")
}

func (c *RepoManagerController) renderSessionList(v *gocui.View) {
	if len(c.repositories) == 0 {
		fmt.Fprint(v, "\n  No repository selected.\n\n  Add a repository first\n  using the left panel.")
		return
	}

	if len(c.sessions) == 0 {
		fmt.Fprint(v, "\n  No sessions for this\n  repository.\n\n  Press 'n' to create\n  a new session.")
		return
	}

	for i, sess := range c.sessions {
		prefix := "  "
		if i == c.sessionSelected {
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

	// Footer
	fmt.Fprint(v, "\n───────────────────────────────────────────\n")
	fmt.Fprint(v, " [j/k] Navigate  [Enter] Attach\n")
	fmt.Fprint(v, " [n] New  [x] Delete  [X] Force Delete\n")
	fmt.Fprint(v, " [Tab/h] Repos  [r] Refresh  [q] Close")
}

// Keybindings sets up view-specific keybindings.
func (c *RepoManagerController) Keybindings(g *gocui.Gui) error {
	return nil // Handled via Editor functions
}

// Render re-renders both panels.
func (c *RepoManagerController) Render(g *gocui.Gui) error {
	return c.Layout(g)
}

// Key handlers

func (c *RepoManagerController) handleRepoListKeys(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case ch == 'j' || key == gocui.KeyArrowDown:
		c.repoNext()
		return true
	case ch == 'k' || key == gocui.KeyArrowUp:
		c.repoPrev()
		return true
	case ch == 'l' || key == gocui.KeyTab || key == gocui.KeyArrowRight:
		c.focusSessions()
		return true
	case ch == 'a':
		c.startAddRepo()
		return true
	case ch == 'd':
		c.removeSelectedRepo()
		return true
	case key == gocui.KeyEnter:
		c.focusSessions()
		return true
	case ch == 'q' || key == gocui.KeyEsc:
		if c.ctx.OnQuit != nil {
			c.ctx.OnQuit()
		}
		return true
	}
	return false
}

func (c *RepoManagerController) handleSessionListKeys(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case ch == 'j' || key == gocui.KeyArrowDown:
		c.sessionNext()
		return true
	case ch == 'k' || key == gocui.KeyArrowUp:
		c.sessionPrev()
		return true
	case ch == 'h' || key == gocui.KeyArrowLeft:
		c.focusRepos()
		return true
	case key == gocui.KeyTab:
		c.focusRepos()
		return true
	case key == gocui.KeyEnter:
		c.attachSelectedSession()
		return true
	case ch == 'n':
		c.startNewSession()
		return true
	case ch == 'x':
		c.deleteSelectedSession(true) // With prompt
		return true
	case ch == 'X':
		c.deleteSelectedSession(false) // Without prompt (force)
		return true
	case ch == 'r':
		c.refresh()
		c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
		return true
	case ch == 'q' || key == gocui.KeyEsc:
		if c.ctx.OnQuit != nil {
			c.ctx.OnQuit()
		}
		return true
	}
	return false
}

func (c *RepoManagerController) handleInputKeys(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc:
		c.cancelInput()
		return true
	case key == gocui.KeyEnter:
		c.confirmInput()
		return true
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		if len(c.inputBuffer) > 0 {
			c.inputBuffer = c.inputBuffer[:len(c.inputBuffer)-1]
		}
		c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
		return true
	case ch != 0 && mod == gocui.ModNone:
		c.inputBuffer += string(ch)
		c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
		return true
	}
	return false
}

func (c *RepoManagerController) handleConfirmKeys(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc:
		c.cancelConfirm()
		return true
	case ch == 'y' || ch == 'Y':
		if c.confirmCallback != nil {
			c.confirmCallback(true)
		}
		c.cancelConfirm()
		return true
	case ch == 'n' || ch == 'N':
		if c.confirmCallback != nil {
			c.confirmCallback(false)
		}
		c.cancelConfirm()
		return true
	}
	return false
}

// Navigation

func (c *RepoManagerController) repoNext() {
	if len(c.repositories) == 0 {
		return
	}
	c.repoSelected = (c.repoSelected + 1) % len(c.repositories)
	c.refreshSessionsForRepo(c.repositories[c.repoSelected].Path)
	c.sessionSelected = 0
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) repoPrev() {
	if len(c.repositories) == 0 {
		return
	}
	c.repoSelected = (c.repoSelected - 1 + len(c.repositories)) % len(c.repositories)
	c.refreshSessionsForRepo(c.repositories[c.repoSelected].Path)
	c.sessionSelected = 0
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) sessionNext() {
	if len(c.sessions) == 0 {
		return
	}
	c.sessionSelected = (c.sessionSelected + 1) % len(c.sessions)
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) sessionPrev() {
	if len(c.sessions) == 0 {
		return
	}
	c.sessionSelected = (c.sessionSelected - 1 + len(c.sessions)) % len(c.sessions)
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) focusSessions() {
	c.focus = FocusSessionList
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) focusRepos() {
	c.focus = FocusRepoList
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

// Actions

func (c *RepoManagerController) startAddRepo() {
	c.inputMode = true
	c.inputBuffer = ""
	c.inputPurpose = "add_repo"
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) startNewSession() {
	if len(c.repositories) == 0 {
		return
	}
	c.inputMode = true
	c.inputBuffer = ""
	c.inputPurpose = "new_session"
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) cancelInput() {
	c.inputMode = false
	c.inputBuffer = ""
	c.inputPurpose = ""
	c.gui.DeleteView(addRepoInputViewName)
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) confirmInput() {
	input := strings.TrimSpace(c.inputBuffer)
	if input == "" {
		c.cancelInput()
		return
	}

	switch c.inputPurpose {
	case "add_repo":
		c.addRepository(input)
	case "new_session":
		c.createNewSession(input)
	}

	c.cancelInput()
}

func (c *RepoManagerController) addRepository(path string) {
	// Expand path
	expanded := path
	if strings.HasPrefix(expanded, "~") {
		// config.AddRepository handles expansion
	}

	// Validate it's a git repo
	if _, err := git.FindRepoRoot(expanded); err != nil {
		// TODO: show error message
		return
	}

	if err := c.ctx.Config.AddRepository(path); err != nil {
		return
	}

	c.refresh()
	// Select the newly added repo
	for i, repo := range c.repositories {
		if strings.HasSuffix(repo.Path, filepath.Base(path)) {
			c.repoSelected = i
			c.refreshSessionsForRepo(repo.Path)
			break
		}
	}
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) removeSelectedRepo() {
	if len(c.repositories) == 0 || c.repoSelected >= len(c.repositories) {
		return
	}

	repo := c.repositories[c.repoSelected]
	if err := c.ctx.Config.RemoveRepository(repo.Path); err != nil {
		return
	}

	// Adjust selection
	if c.repoSelected > 0 {
		c.repoSelected--
	}
	c.refresh()
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}

func (c *RepoManagerController) createNewSession(branchName string) {
	if len(c.repositories) == 0 || c.repoSelected >= len(c.repositories) {
		return
	}

	repo := c.repositories[c.repoSelected]

	// Check if branch exists or needs to be created
	branches, _ := git.ListBranches(repo.Path)
	branchExists := false
	for _, b := range branches {
		if b == branchName {
			branchExists = true
			break
		}
	}

	sessionName, err := c.sessionManager.CreateSession(repo.Path, branchName, !branchExists)
	if err != nil {
		return
	}

	// Refresh and attach
	c.refresh()
	if c.ctx.OnAttach != nil {
		c.ctx.OnAttach(sessionName)
	}
}

func (c *RepoManagerController) attachSelectedSession() {
	if len(c.sessions) == 0 || c.sessionSelected >= len(c.sessions) {
		return
	}

	sess := c.sessions[c.sessionSelected]
	if c.ctx.OnAttach != nil {
		c.ctx.OnAttach(sess.Name)
	}
}

func (c *RepoManagerController) deleteSelectedSession(withPrompt bool) {
	if len(c.sessions) == 0 || c.sessionSelected >= len(c.sessions) {
		return
	}

	sess := c.sessions[c.sessionSelected]

	if withPrompt {
		// Show confirmation modal
		c.confirmMode = true
		c.pendingDeleteName = sess.Name
		c.confirmCallback = func(removeWorktree bool) {
			c.sessionManager.DeleteSession(sess.Name, removeWorktree)
			c.refresh()
			c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
		}
		c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
	} else {
		// Force delete: remove both session and worktree
		c.sessionManager.DeleteSession(sess.Name, true)
		c.refresh()
		c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
	}
}

func (c *RepoManagerController) cancelConfirm() {
	c.confirmMode = false
	c.confirmCallback = nil
	c.pendingDeleteName = ""
	c.gui.DeleteView(confirmDeleteViewName)
	c.gui.Update(func(g *gocui.Gui) error { return c.Layout(g) })
}
