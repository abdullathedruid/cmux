package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/git"
)

const wizardViewName = "wizard"

type wizardStep int

const (
	stepSelectRepo wizardStep = iota
	stepSelectAction
	stepSelectBranch
	stepEnterBranchName
	stepAddRepo
)

// WizardController manages the session creation wizard.
type WizardController struct {
	ctx           *Context
	visible       bool
	step          wizardStep
	recentRepos   []string
	selectedRepo  string
	branches      []string
	worktrees     []git.Worktree
	selected      int
	inputBuffer   string
	createNew     bool // true = create new worktree, false = use existing
	gui           *gocui.Gui
	preSelectedRepo string // If set, skip repo selection step
}

// Edit handles key input for the wizard modal.
func (c *WizardController) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc:
		c.back(c.gui, v)
		return true
	case key == gocui.KeyEnter:
		c.select_(c.gui, v)
		return true
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		c.backspace(c.gui, v)
		return true
	case key == gocui.KeyArrowDown || ch == 'j':
		c.cursorDown(c.gui, v)
		return true
	case key == gocui.KeyArrowUp || ch == 'k':
		c.cursorUp(c.gui, v)
		return true
	case ch != 0 && mod == gocui.ModNone && (c.step == stepEnterBranchName || c.step == stepAddRepo):
		// Accept character input in text entry modes
		c.inputBuffer += string(ch)
		c.Render(c.gui)
		return true
	}
	return false
}

// NewWizardController creates a new wizard controller.
func NewWizardController(ctx *Context) *WizardController {
	return &WizardController{ctx: ctx}
}

// Name returns the view name.
func (c *WizardController) Name() string {
	return wizardViewName
}

// IsVisible returns whether the wizard is visible.
func (c *WizardController) IsVisible() bool {
	return c.visible
}

// Show shows the wizard.
func (c *WizardController) Show(g *gocui.Gui) error {
	c.visible = true
	c.step = stepSelectRepo
	c.selected = 0
	c.inputBuffer = ""
	c.selectedRepo = ""
	c.gui = g

	// If a repo is pre-selected, skip repo selection
	if c.preSelectedRepo != "" {
		c.selectedRepo = c.preSelectedRepo
		c.preSelectedRepo = ""
		c.step = stepSelectAction

		// Load branches and worktrees
		worktrees, _ := git.ListWorktrees(c.selectedRepo)
		c.worktrees = worktrees
		branches, _ := git.ListBranches(c.selectedRepo)
		c.branches = branches
	} else {
		// Gather recent repos from existing sessions
		c.recentRepos = c.gatherRecentRepos()
	}

	return c.Layout(g)
}

// ShowWithRepo shows the wizard with a pre-selected repository.
func (c *WizardController) ShowWithRepo(g *gocui.Gui, repoPath string) error {
	c.preSelectedRepo = repoPath
	return c.Show(g)
}

// ShowAddRepo shows the wizard in add-repo mode.
func (c *WizardController) ShowAddRepo(g *gocui.Gui) error {
	c.visible = true
	c.step = stepAddRepo
	c.selected = 0
	c.inputBuffer = ""
	c.selectedRepo = ""
	c.gui = g
	return c.Layout(g)
}

// gatherRecentRepos gets unique repo paths from config, existing sessions, and cwd.
func (c *WizardController) gatherRecentRepos() []string {
	seen := make(map[string]bool)
	var repos []string

	// Helper to add a repo if not already seen
	addRepo := func(path string) {
		if path != "" && !seen[path] {
			seen[path] = true
			repos = append(repos, path)
		}
	}

	// 1. Add current directory first (if it's a git repo)
	if cwd, err := os.Getwd(); err == nil {
		if root, err := git.FindRepoRoot(cwd); err == nil {
			addRepo(root)
		}
	}

	// 2. Add configured repositories
	for _, repo := range c.ctx.Config.ExpandedRepositories() {
		// Resolve to git root in case user specified a subdirectory
		if root, err := git.FindRepoRoot(repo); err == nil {
			addRepo(root)
		}
	}

	// 3. Add repos from existing sessions
	for _, sess := range c.ctx.State.GetSessions() {
		addRepo(sess.RepoPath)
	}

	return repos
}

// Hide hides the wizard.
func (c *WizardController) Hide(g *gocui.Gui) error {
	c.visible = false
	return g.DeleteView(wizardViewName)
}

// Layout sets up the wizard view.
func (c *WizardController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the modal
	width := 60
	height := 18
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(wizardViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = c.getTitle()
	v.Wrap = false
	v.Frame = true
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.Edit)

	// Set as top view
	if _, err := g.SetCurrentView(wizardViewName); err != nil {
		return err
	}

	return c.Render(g)
}

func (c *WizardController) getTitle() string {
	switch c.step {
	case stepSelectRepo:
		return " New Session - Select Repository "
	case stepSelectAction:
		return " New Session - Select Action "
	case stepSelectBranch:
		return " New Session - Select Branch "
	case stepEnterBranchName:
		return " New Session - Enter Branch Name "
	case stepAddRepo:
		return " Add Repository "
	default:
		return " New Session "
	}
}

// Keybindings sets up wizard keybindings.
// Note: Key handling is done via the custom Editor interface instead.
func (c *WizardController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the wizard content.
func (c *WizardController) Render(g *gocui.Gui) error {
	v, err := g.View(wizardViewName)
	if err != nil {
		return err
	}

	v.Clear()

	switch c.step {
	case stepSelectRepo:
		c.renderRepoSelection(v)
	case stepSelectAction:
		c.renderActionSelection(v)
	case stepSelectBranch:
		c.renderBranchSelection(v)
	case stepEnterBranchName:
		c.renderBranchInput(v)
	case stepAddRepo:
		c.renderAddRepoInput(v)
	}

	return nil
}

func (c *WizardController) renderRepoSelection(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Select a repository:")
	fmt.Fprintln(v, "")

	if len(c.recentRepos) == 0 {
		fmt.Fprintln(v, "  No repositories found.")
		fmt.Fprintln(v, "  Run cmux from within a git repository.")
	} else {
		for i, repo := range c.recentRepos {
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}
			fmt.Fprintf(v, "%s%s\n", prefix, git.ShortenPath(repo))
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ────────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Select  Esc: Cancel")
}

func (c *WizardController) renderActionSelection(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintf(v, "  Repository: %s\n", filepath.Base(c.selectedRepo))
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  What would you like to do?")
	fmt.Fprintln(v, "")

	actions := []string{
		"Create session on main branch",
		"Create session from existing branch/worktree",
		"Create session with new branch",
	}

	for i, action := range actions {
		prefix := "  "
		if i == c.selected {
			prefix = "> "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, action)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ────────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Select  Esc: Back")
}

func (c *WizardController) renderBranchSelection(v *gocui.View) {
	fmt.Fprintln(v, "")

	if c.createNew {
		fmt.Fprintln(v, "  Select base branch for new worktree:")
	} else {
		fmt.Fprintln(v, "  Select existing branch/worktree:")
	}
	fmt.Fprintln(v, "")

	// Show worktrees first, then branches
	items := c.getBranchItems()
	if len(items) == 0 {
		fmt.Fprintln(v, "  No branches found.")
	} else {
		maxShow := 10
		if len(items) < maxShow {
			maxShow = len(items)
		}

		// Calculate viewport offset to keep selected item visible
		viewportStart := 0
		if c.selected >= maxShow {
			viewportStart = c.selected - maxShow + 1
		}
		viewportEnd := viewportStart + maxShow
		if viewportEnd > len(items) {
			viewportEnd = len(items)
			viewportStart = viewportEnd - maxShow
			if viewportStart < 0 {
				viewportStart = 0
			}
		}

		// Show indicator if there are items above
		if viewportStart > 0 {
			fmt.Fprintf(v, "  ... and %d more above\n", viewportStart)
		}

		for i := viewportStart; i < viewportEnd; i++ {
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}
			fmt.Fprintf(v, "%s%s\n", prefix, items[i])
		}

		// Show indicator if there are items below
		if viewportEnd < len(items) {
			fmt.Fprintf(v, "  ... and %d more\n", len(items)-viewportEnd)
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ────────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Select  Esc: Back")
}

func (c *WizardController) renderBranchInput(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Enter new branch name:")
	fmt.Fprintln(v, "")
	fmt.Fprintf(v, "  > %s_\n", c.inputBuffer)
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ────────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Create  Esc: Back")
}

func (c *WizardController) renderAddRepoInput(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Enter repository path:")
	fmt.Fprintln(v, "")
	fmt.Fprintf(v, "  > %s_\n", c.inputBuffer)
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  (Use ~ for home directory)")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ────────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Add  Esc: Cancel")
}

func (c *WizardController) getBranchItems() []string {
	var items []string

	// Add worktrees (marked)
	for _, wt := range c.worktrees {
		label := wt.Branch
		if wt.IsMain {
			label += " [main worktree]"
		} else {
			label += " [worktree]"
		}
		items = append(items, label)
	}

	// Add branches not in worktrees
	worktreeBranches := make(map[string]bool)
	for _, wt := range c.worktrees {
		worktreeBranches[wt.Branch] = true
	}

	for _, branch := range c.branches {
		if !worktreeBranches[branch] {
			items = append(items, branch)
		}
	}

	return items
}

// Navigation handlers
func (c *WizardController) cursorDown(g *gocui.Gui, v *gocui.View) error {
	maxItems := c.getMaxItems()
	if c.selected < maxItems-1 {
		c.selected++
	}
	return c.Render(g)
}

func (c *WizardController) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if c.selected > 0 {
		c.selected--
	}
	return c.Render(g)
}

func (c *WizardController) getMaxItems() int {
	switch c.step {
	case stepSelectRepo:
		return len(c.recentRepos)
	case stepSelectAction:
		return 3
	case stepSelectBranch:
		return len(c.getBranchItems())
	default:
		return 0
	}
}

func (c *WizardController) select_(g *gocui.Gui, v *gocui.View) error {
	switch c.step {
	case stepSelectRepo:
		return c.selectRepo(g)
	case stepSelectAction:
		return c.selectAction(g)
	case stepSelectBranch:
		return c.selectBranch(g)
	case stepEnterBranchName:
		return c.createWithNewBranch(g)
	case stepAddRepo:
		return c.addRepository(g)
	}
	return nil
}

func (c *WizardController) selectRepo(g *gocui.Gui) error {
	if c.selected >= len(c.recentRepos) {
		return nil
	}

	c.selectedRepo = c.recentRepos[c.selected]
	c.step = stepSelectAction
	c.selected = 0

	// Load branches and worktrees
	worktrees, _ := git.ListWorktrees(c.selectedRepo)
	c.worktrees = worktrees

	branches, _ := git.ListBranches(c.selectedRepo)
	c.branches = branches

	v, _ := g.View(wizardViewName)
	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WizardController) selectAction(g *gocui.Gui) error {
	switch c.selected {
	case 0: // Main branch
		return c.createOnMainBranch(g)
	case 1: // Existing branch
		c.createNew = false
		c.step = stepSelectBranch
		c.selected = 0
	case 2: // New branch
		c.createNew = true
		c.step = stepEnterBranchName
		c.inputBuffer = ""
	}

	v, _ := g.View(wizardViewName)
	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WizardController) selectBranch(g *gocui.Gui) error {
	items := c.getBranchItems()
	if c.selected >= len(items) {
		return nil
	}

	// Worktrees are listed first in getBranchItems(), so we can use the index
	// to determine if user selected a worktree or a plain branch
	if c.selected < len(c.worktrees) {
		wt := c.worktrees[c.selected]
		return c.createSessionForPath(g, wt.Path, wt.Branch)
	}

	// It's a branch without a worktree - create one
	branchIndex := c.selected - len(c.worktrees)

	// Build the list of branches without worktrees (same logic as getBranchItems)
	worktreeBranches := make(map[string]bool)
	for _, wt := range c.worktrees {
		worktreeBranches[wt.Branch] = true
	}

	var branchesWithoutWorktree []string
	for _, branch := range c.branches {
		if !worktreeBranches[branch] {
			branchesWithoutWorktree = append(branchesWithoutWorktree, branch)
		}
	}

	if branchIndex >= len(branchesWithoutWorktree) {
		return nil
	}

	branchName := branchesWithoutWorktree[branchIndex]
	worktreePath, err := git.CreateWorktree(c.selectedRepo, branchName, false)
	if err != nil {
		return err
	}

	return c.createSessionForPath(g, worktreePath, branchName)
}

func (c *WizardController) createOnMainBranch(g *gocui.Gui) error {
	// Find main worktree
	for _, wt := range c.worktrees {
		if wt.IsMain {
			return c.createSessionForPath(g, wt.Path, wt.Branch)
		}
	}

	// Fallback to repo root
	info, err := git.GetRepoInfo(c.selectedRepo)
	if err != nil {
		return err
	}
	return c.createSessionForPath(g, c.selectedRepo, info.Branch)
}

func (c *WizardController) createWithNewBranch(g *gocui.Gui) error {
	if c.inputBuffer == "" {
		return nil
	}

	// Create worktree with new branch
	worktreePath, err := git.CreateWorktree(c.selectedRepo, c.inputBuffer, true)
	if err != nil {
		return err
	}

	return c.createSessionForPath(g, worktreePath, c.inputBuffer)
}

func (c *WizardController) createSessionForPath(g *gocui.Gui, path, branch string) error {
	// Hide wizard
	if err := c.Hide(g); err != nil {
		return err
	}

	// Generate session name
	repoName := filepath.Base(c.selectedRepo)
	sessionName := fmt.Sprintf("%s-%s", repoName, sanitizeBranchForSession(branch))

	// Check if session exists
	if c.ctx.TmuxClient.HasSession(sessionName) {
		// Attach to existing
		if c.ctx.OnAttach != nil {
			return c.ctx.OnAttach(sessionName)
		}
		return nil
	}

	// Create new session
	if err := c.ctx.TmuxClient.CreateSession(sessionName, path, true); err != nil {
		return err
	}

	// Refresh and select the new session
	if c.ctx.OnRefresh != nil {
		c.ctx.OnRefresh()
	}
	c.ctx.State.SetSelectedSession(sessionName)

	if c.ctx.OnAttach != nil {
		return c.ctx.OnAttach(sessionName)
	}
	return nil
}

func (c *WizardController) back(g *gocui.Gui, v *gocui.View) error {
	switch c.step {
	case stepSelectRepo:
		return c.Hide(g)
	case stepSelectAction:
		c.step = stepSelectRepo
		c.selected = 0
	case stepSelectBranch, stepEnterBranchName:
		c.step = stepSelectAction
		c.selected = 0
		c.inputBuffer = ""
	case stepAddRepo:
		return c.Hide(g)
	}

	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WizardController) backspace(g *gocui.Gui, v *gocui.View) error {
	if (c.step == stepEnterBranchName || c.step == stepAddRepo) && len(c.inputBuffer) > 0 {
		c.inputBuffer = c.inputBuffer[:len(c.inputBuffer)-1]
		return c.Render(g)
	}
	return nil
}

func (c *WizardController) addRepository(g *gocui.Gui) error {
	if c.inputBuffer == "" {
		return nil
	}

	path := c.inputBuffer

	// Validate it's a git repo
	if _, err := git.FindRepoRoot(path); err != nil {
		// TODO: Show error message in UI
		return nil
	}

	// Add to config
	if err := c.ctx.Config.AddRepository(path); err != nil {
		return err
	}

	// Refresh and close
	if c.ctx.OnRefresh != nil {
		c.ctx.OnRefresh()
	}

	return c.Hide(g)
}

func sanitizeBranchForSession(branch string) string {
	result := strings.ReplaceAll(branch, "/", "-")
	result = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, result)
	return result
}
