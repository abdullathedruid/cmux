package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Gather recent repos from existing sessions
	c.recentRepos = c.gatherRecentRepos()

	return c.Layout(g)
}

// gatherRecentRepos gets unique repo paths from existing sessions.
func (c *WizardController) gatherRecentRepos() []string {
	sessions := c.ctx.State.GetSessions()
	seen := make(map[string]bool)
	var repos []string

	for _, sess := range sessions {
		if sess.RepoPath != "" && !seen[sess.RepoPath] {
			seen[sess.RepoPath] = true
			repos = append(repos, sess.RepoPath)
		}
	}

	// Also add current directory if it's a git repo
	if cwd, err := os.Getwd(); err == nil {
		if root, err := git.FindRepoRoot(cwd); err == nil && !seen[root] {
			repos = append([]string{root}, repos...) // Prepend cwd
		}
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

	v, err := g.SetView(wizardViewName, x0, y0, x0+width, y0+height)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}

	v.Title = c.getTitle()
	v.Wrap = false
	v.Frame = true

	// Set as top view
	if err := g.SetCurrentView(wizardViewName); err != nil {
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
	default:
		return " New Session "
	}
}

// Keybindings sets up wizard keybindings.
func (c *WizardController) Keybindings(g *gocui.Gui) error {
	// Navigation
	if err := g.SetKeybinding(wizardViewName, 'j', gocui.ModNone, c.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(wizardViewName, 'k', gocui.ModNone, c.cursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(wizardViewName, gocui.KeyArrowDown, gocui.ModNone, c.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(wizardViewName, gocui.KeyArrowUp, gocui.ModNone, c.cursorUp); err != nil {
		return err
	}

	// Selection
	if err := g.SetKeybinding(wizardViewName, gocui.KeyEnter, gocui.ModNone, c.select_); err != nil {
		return err
	}

	// Cancel/Back
	if err := g.SetKeybinding(wizardViewName, gocui.KeyEsc, gocui.ModNone, c.back); err != nil {
		return err
	}

	// Backspace for text input
	if err := g.SetKeybinding(wizardViewName, gocui.KeyBackspace, gocui.ModNone, c.backspace); err != nil {
		return err
	}
	if err := g.SetKeybinding(wizardViewName, gocui.KeyBackspace2, gocui.ModNone, c.backspace); err != nil {
		return err
	}

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
		for i := 0; i < maxShow; i++ {
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}
			fmt.Fprintf(v, "%s%s\n", prefix, items[i])
		}
		if len(items) > maxShow {
			fmt.Fprintf(v, "  ... and %d more\n", len(items)-maxShow)
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

	selectedItem := items[c.selected]

	// Check if it's a worktree
	for _, wt := range c.worktrees {
		if strings.Contains(selectedItem, wt.Branch) {
			return c.createSessionForPath(g, wt.Path, wt.Branch)
		}
	}

	// It's a branch - create worktree
	worktreePath, err := git.CreateWorktree(c.selectedRepo, selectedItem, false)
	if err != nil {
		return err
	}

	return c.createSessionForPath(g, worktreePath, selectedItem)
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

	// Refresh and attach
	if c.ctx.OnRefresh != nil {
		c.ctx.OnRefresh()
	}
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
	}

	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WizardController) backspace(g *gocui.Gui, v *gocui.View) error {
	if c.step == stepEnterBranchName && len(c.inputBuffer) > 0 {
		c.inputBuffer = c.inputBuffer[:len(c.inputBuffer)-1]
		return c.Render(g)
	}
	return nil
}

// HandleRune handles character input.
func (c *WizardController) HandleRune(g *gocui.Gui, r rune) error {
	if c.step == stepEnterBranchName {
		c.inputBuffer += string(r)
		return c.Render(g)
	}
	return nil
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
