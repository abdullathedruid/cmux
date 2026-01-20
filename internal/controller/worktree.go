package controller

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/git"
)

const worktreeViewName = "worktree"

// WorktreeController manages the worktree picker modal.
type WorktreeController struct {
	ctx         *Context
	visible     bool
	repoPath    string
	worktrees   []git.Worktree
	branches    []string
	selected    int
	mode        worktreeMode
	inputBuffer string
	gui         *gocui.Gui
}

type worktreeMode int

const (
	modeSelectWorktree worktreeMode = iota
	modeSelectBranch
	modeCreateBranch
)

// Edit handles key input for the worktree modal.
func (c *WorktreeController) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc || ch == 'q':
		c.close(c.gui, v)
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
	case ch == 'b' && c.mode == modeSelectWorktree:
		c.showBranches(c.gui, v)
		return true
	case ch == 'n' && (c.mode == modeSelectWorktree || c.mode == modeSelectBranch):
		c.newBranch(c.gui, v)
		return true
	case ch != 0 && mod == gocui.ModNone && c.mode == modeCreateBranch:
		// Only accept character input in branch creation mode
		c.inputBuffer += string(ch)
		c.Render(c.gui)
		return true
	}
	return false
}

// NewWorktreeController creates a new worktree controller.
func NewWorktreeController(ctx *Context) *WorktreeController {
	return &WorktreeController{ctx: ctx}
}

// Name returns the view name.
func (c *WorktreeController) Name() string {
	return worktreeViewName
}

// IsVisible returns whether the picker is visible.
func (c *WorktreeController) IsVisible() bool {
	return c.visible
}

// Show shows the worktree picker for the current directory.
func (c *WorktreeController) Show(g *gocui.Gui) error {
	// Get current working directory and find repo
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	repoPath, err := git.FindRepoRoot(cwd)
	if err != nil {
		return fmt.Errorf("not in a git repository")
	}

	c.repoPath = repoPath
	c.selected = 0
	c.mode = modeSelectWorktree
	c.inputBuffer = ""
	c.gui = g

	// Load worktrees
	worktrees, err := git.ListWorktrees(repoPath)
	if err != nil {
		return err
	}
	c.worktrees = worktrees

	// Load branches
	branches, err := git.ListBranches(repoPath)
	if err != nil {
		branches = []string{}
	}
	c.branches = branches

	c.visible = true
	return c.Layout(g)
}

// Hide hides the worktree picker.
func (c *WorktreeController) Hide(g *gocui.Gui) error {
	c.visible = false
	return g.DeleteView(worktreeViewName)
}

// Layout sets up the worktree picker view.
func (c *WorktreeController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the modal
	width := 60
	height := 20
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(worktreeViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = c.getTitle()
	v.Wrap = false
	v.Frame = true
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.Edit)

	// Set as top view
	if _, err := g.SetCurrentView(worktreeViewName); err != nil {
		return err
	}

	return c.Render(g)
}

func (c *WorktreeController) getTitle() string {
	switch c.mode {
	case modeSelectBranch:
		return " Select Branch "
	case modeCreateBranch:
		return " New Branch Name "
	default:
		return " Worktrees "
	}
}

// Keybindings sets up worktree picker keybindings.
// Note: Key handling is done via the custom Editor interface instead.
func (c *WorktreeController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the worktree picker content.
func (c *WorktreeController) Render(g *gocui.Gui) error {
	v, err := g.View(worktreeViewName)
	if err != nil {
		return err
	}

	v.Clear()

	switch c.mode {
	case modeSelectWorktree:
		c.renderWorktrees(v)
	case modeSelectBranch:
		c.renderBranches(v)
	case modeCreateBranch:
		c.renderNewBranchInput(v)
	}

	return nil
}

func (c *WorktreeController) renderWorktrees(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Existing worktrees:")
	fmt.Fprintln(v, "")

	if len(c.worktrees) == 0 {
		fmt.Fprintln(v, "  No worktrees found.")
	} else {
		for i, wt := range c.worktrees {
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}

			label := wt.Branch
			if wt.IsMain {
				label += " (main)"
			}
			shortPath := git.ShortenPath(wt.Path)
			fmt.Fprintf(v, "%s%s\n", prefix, label)
			fmt.Fprintf(v, "    %s\n", shortPath)
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ─────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Select  b: From branch  n: New branch")
	fmt.Fprintln(v, "  Esc: Cancel")
}

func (c *WorktreeController) renderBranches(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Select a branch to create worktree:")
	fmt.Fprintln(v, "")

	if len(c.branches) == 0 {
		fmt.Fprintln(v, "  No branches found.")
	} else {
		for i, branch := range c.branches {
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}
			fmt.Fprintf(v, "%s%s\n", prefix, branch)
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ─────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Select  Esc: Back")
}

func (c *WorktreeController) renderNewBranchInput(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Enter new branch name:")
	fmt.Fprintln(v, "")
	fmt.Fprintf(v, "  > %s_\n", c.inputBuffer)
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ─────────────────────────────────────────")
	fmt.Fprintln(v, "  Enter: Create  Esc: Cancel")
}

// Navigation handlers
func (c *WorktreeController) cursorDown(g *gocui.Gui, v *gocui.View) error {
	var maxItems int
	switch c.mode {
	case modeSelectWorktree:
		maxItems = len(c.worktrees)
	case modeSelectBranch:
		maxItems = len(c.branches)
	default:
		return nil
	}

	if c.selected < maxItems-1 {
		c.selected++
	}
	return c.Render(g)
}

func (c *WorktreeController) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if c.selected > 0 {
		c.selected--
	}
	return c.Render(g)
}

// Action handlers
func (c *WorktreeController) select_(g *gocui.Gui, v *gocui.View) error {
	switch c.mode {
	case modeSelectWorktree:
		if c.selected < len(c.worktrees) {
			wt := c.worktrees[c.selected]
			return c.createSessionForWorktree(g, wt.Path, wt.Branch)
		}
	case modeSelectBranch:
		if c.selected < len(c.branches) {
			branch := c.branches[c.selected]
			return c.createWorktreeAndSession(g, branch, false)
		}
	case modeCreateBranch:
		if c.inputBuffer != "" {
			return c.createWorktreeAndSession(g, c.inputBuffer, true)
		}
	}
	return nil
}

func (c *WorktreeController) showBranches(g *gocui.Gui, v *gocui.View) error {
	if c.mode != modeSelectWorktree {
		return nil
	}
	c.mode = modeSelectBranch
	c.selected = 0
	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WorktreeController) newBranch(g *gocui.Gui, v *gocui.View) error {
	if c.mode != modeSelectWorktree && c.mode != modeSelectBranch {
		return nil
	}
	c.mode = modeCreateBranch
	c.inputBuffer = ""
	v.Title = c.getTitle()
	return c.Render(g)
}

func (c *WorktreeController) close(g *gocui.Gui, v *gocui.View) error {
	// Go back a mode or close
	switch c.mode {
	case modeSelectBranch:
		c.mode = modeSelectWorktree
		c.selected = 0
		v.Title = c.getTitle()
		return c.Render(g)
	case modeCreateBranch:
		c.mode = modeSelectWorktree
		c.inputBuffer = ""
		v.Title = c.getTitle()
		return c.Render(g)
	default:
		return c.Hide(g)
	}
}

func (c *WorktreeController) backspace(g *gocui.Gui, v *gocui.View) error {
	if c.mode == modeCreateBranch && len(c.inputBuffer) > 0 {
		c.inputBuffer = c.inputBuffer[:len(c.inputBuffer)-1]
		return c.Render(g)
	}
	return nil
}

func (c *WorktreeController) createSessionForWorktree(g *gocui.Gui, path, branch string) error {
	// Hide picker
	if err := c.Hide(g); err != nil {
		return err
	}

	// Generate session name
	info, err := git.GetRepoInfo(path)
	if err != nil {
		return err
	}

	sessionName := fmt.Sprintf("%s-%s", info.Name, sanitizeBranch(branch))

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

func (c *WorktreeController) createWorktreeAndSession(g *gocui.Gui, branch string, createBranch bool) error {
	// Create the worktree
	worktreePath, err := git.CreateWorktree(c.repoPath, branch, createBranch)
	if err != nil {
		return err
	}

	return c.createSessionForWorktree(g, worktreePath, branch)
}

func sanitizeBranch(branch string) string {
	// Replace slashes and other problematic chars
	result := strings.ReplaceAll(branch, "/", "-")
	result = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, result)
	return result
}
