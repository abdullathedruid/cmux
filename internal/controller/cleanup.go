package controller

import (
	"fmt"
	"time"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/git"
)

const cleanupViewName = "cleanup"

// cleanupStep represents the current step in the cleanup flow.
type cleanupStep int

const (
	stepSelectWorktrees cleanupStep = iota
	stepConfirmDelete
)

// OrphanedWorktree represents a worktree without an active tmux session.
type OrphanedWorktree struct {
	RepoName   string
	RepoPath   string
	Worktree   git.Worktree
	IsMerged   bool
	Age        time.Duration
	Selected   bool
}

// CleanupController manages the worktree cleanup modal.
type CleanupController struct {
	ctx          *Context
	visible      bool
	gui          *gocui.Gui
	step         cleanupStep
	orphans      []OrphanedWorktree
	cursor       int
	scrollOffset int
	viewHeight   int
}

// NewCleanupController creates a new cleanup controller.
func NewCleanupController(ctx *Context) *CleanupController {
	return &CleanupController{ctx: ctx}
}

// Name returns the view name.
func (c *CleanupController) Name() string {
	return cleanupViewName
}

// IsVisible returns whether the modal is visible.
func (c *CleanupController) IsVisible() bool {
	return c.visible
}

// Edit handles key input for the cleanup modal.
func (c *CleanupController) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch c.step {
	case stepSelectWorktrees:
		return c.handleSelectStep(v, key, ch)
	case stepConfirmDelete:
		return c.handleConfirmStep(v, key, ch)
	}
	return false
}

func (c *CleanupController) handleSelectStep(v *gocui.View, key gocui.Key, ch rune) bool {
	switch {
	case key == gocui.KeyEsc || ch == 'q':
		c.Hide(c.gui)
		return true
	case key == gocui.KeyArrowDown || ch == 'j':
		c.cursorDown()
		c.Render(c.gui)
		return true
	case key == gocui.KeyArrowUp || ch == 'k':
		c.cursorUp()
		c.Render(c.gui)
		return true
	case key == gocui.KeySpace:
		c.toggleCurrent()
		c.Render(c.gui)
		return true
	case ch == 'a':
		c.toggleAll()
		c.Render(c.gui)
		return true
	case key == gocui.KeyEnter:
		if c.selectedCount() > 0 {
			c.step = stepConfirmDelete
			c.Render(c.gui)
		}
		return true
	}
	return false
}

func (c *CleanupController) handleConfirmStep(v *gocui.View, key gocui.Key, ch rune) bool {
	switch {
	case key == gocui.KeyEsc || ch == 'n':
		c.step = stepSelectWorktrees
		c.Render(c.gui)
		return true
	case ch == 'y':
		c.performDelete()
		return true
	}
	return false
}

// Show shows the cleanup modal.
func (c *CleanupController) Show(g *gocui.Gui) error {
	c.gui = g
	c.step = stepSelectWorktrees
	c.cursor = 0
	c.scrollOffset = 0

	// Find orphaned worktrees
	if err := c.findOrphanedWorktrees(); err != nil {
		return err
	}

	c.visible = true
	return c.Layout(g)
}

// Hide hides the cleanup modal.
func (c *CleanupController) Hide(g *gocui.Gui) error {
	c.visible = false
	return g.DeleteView(cleanupViewName)
}

// Layout sets up the cleanup modal view.
func (c *CleanupController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the modal
	width := 60
	height := 20
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(cleanupViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	// Calculate visible height for items (height - header lines - footer lines - borders)
	// Header: 3 lines (empty, "Select worktrees...", empty)
	// Footer: 3 lines (empty, separator, controls x2)
	// Plus repo name lines
	c.viewHeight = height - 2 - 6 // borders and fixed content

	v.Title = c.getTitle()
	v.Wrap = false
	v.Frame = true
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.Edit)

	// Set as top view
	if _, err := g.SetCurrentView(cleanupViewName); err != nil {
		return err
	}

	return c.Render(g)
}

func (c *CleanupController) getTitle() string {
	if c.step == stepConfirmDelete {
		return " Confirm Worktree Cleanup "
	}
	return " Worktree Cleanup "
}

// Keybindings sets up cleanup modal keybindings.
func (c *CleanupController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the cleanup modal content.
func (c *CleanupController) Render(g *gocui.Gui) error {
	v, err := g.View(cleanupViewName)
	if err != nil {
		return err
	}

	v.Clear()
	v.Title = c.getTitle()

	switch c.step {
	case stepSelectWorktrees:
		c.renderSelectStep(v)
	case stepConfirmDelete:
		c.renderConfirmStep(v)
	}

	return nil
}

func (c *CleanupController) renderSelectStep(v *gocui.View) {
	if len(c.orphans) == 0 {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  No orphaned worktrees found.")
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  All worktrees have active tmux sessions.")
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  ─────────────────────────────────────────────────")
		fmt.Fprintln(v, "  Esc: Close")
		return
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Select worktrees to remove:")
	fmt.Fprintln(v, "")

	// Determine visible range
	visibleStart := c.scrollOffset
	visibleEnd := c.scrollOffset + c.viewHeight
	if visibleEnd > len(c.orphans) {
		visibleEnd = len(c.orphans)
	}

	// Show scroll indicator at top if needed
	if c.scrollOffset > 0 {
		fmt.Fprintln(v, "  ↑ more above")
	}

	// Group by repo but only render visible items
	repoGroups := make(map[string][]int)
	repoOrder := []string{}
	for i := range c.orphans {
		name := c.orphans[i].RepoName
		if _, exists := repoGroups[name]; !exists {
			repoOrder = append(repoOrder, name)
		}
		repoGroups[name] = append(repoGroups[name], i)
	}

	currentLine := 0
	for _, repoName := range repoOrder {
		indices := repoGroups[repoName]

		// Check if any items in this repo are visible
		repoHasVisible := false
		for _, idx := range indices {
			if idx >= visibleStart && idx < visibleEnd {
				repoHasVisible = true
				break
			}
		}

		if repoHasVisible {
			fmt.Fprintf(v, "  %s:\n", repoName)
		}

		for _, idx := range indices {
			// Only render if within visible range
			if idx < visibleStart || idx >= visibleEnd {
				currentLine++
				continue
			}

			orphan := &c.orphans[idx]

			// Cursor and checkbox
			prefix := "  "
			if idx == c.cursor {
				prefix = "> "
			}

			checkbox := "[ ]"
			if orphan.Selected {
				checkbox = "[x]"
			}

			// Status label
			status := "orphan"
			if orphan.IsMerged {
				status = "merged"
			}

			// Age
			age := formatAge(orphan.Age)

			fmt.Fprintf(v, "  %s%s %s (%s, %s)\n", prefix, checkbox, orphan.Worktree.Branch, status, age)
			currentLine++
		}
	}

	// Show scroll indicator at bottom if needed
	if visibleEnd < len(c.orphans) {
		fmt.Fprintln(v, "  ↓ more below")
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ─────────────────────────────────────────────────")
	selectedCount := c.selectedCount()
	if selectedCount > 0 {
		fmt.Fprintf(v, "  Space: Toggle  a: Select all  Enter: Confirm (%d)\n", selectedCount)
	} else {
		fmt.Fprintln(v, "  Space: Toggle  a: Select all  Enter: Confirm")
	}
	fmt.Fprintln(v, "  Esc: Cancel")
}

func (c *CleanupController) renderConfirmStep(v *gocui.View) {
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  The following worktrees will be removed:")
	fmt.Fprintln(v, "")

	// Collect selected items
	var selected []string
	for i := range c.orphans {
		if c.orphans[i].Selected {
			selected = append(selected, fmt.Sprintf("    - %s/%s", c.orphans[i].RepoName, c.orphans[i].Worktree.Branch))
		}
	}

	// Show items with scroll if needed
	maxVisible := c.viewHeight
	if len(selected) <= maxVisible {
		for _, item := range selected {
			fmt.Fprintln(v, item)
		}
	} else {
		// Show first few and indicate more
		for i := 0; i < maxVisible-1; i++ {
			fmt.Fprintln(v, selected[i])
		}
		fmt.Fprintf(v, "    ... and %d more\n", len(selected)-maxVisible+1)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  This action cannot be undone!")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  ─────────────────────────────────────────────────")
	fmt.Fprintln(v, "  y: Confirm and delete  n: Cancel")
}

func (c *CleanupController) findOrphanedWorktrees() error {
	c.orphans = nil

	// Get all active session worktree paths
	activeWorktrees := make(map[string]bool)
	for _, sess := range c.ctx.State.GetSessions() {
		if sess.Worktree != "" {
			activeWorktrees[sess.Worktree] = true
		}
	}

	// Get repositories from config
	repos := c.ctx.Config.ExpandedRepositories()

	// If no configured repos, try to use repos from current sessions
	if len(repos) == 0 {
		repoSet := make(map[string]bool)
		for _, sess := range c.ctx.State.GetSessions() {
			if sess.RepoPath != "" {
				repoSet[sess.RepoPath] = true
			}
		}
		for path := range repoSet {
			repos = append(repos, path)
		}
	}

	// Check each repo for orphaned worktrees
	for _, repoPath := range repos {
		worktrees, err := git.ListWorktrees(repoPath)
		if err != nil {
			continue
		}

		repoName := ""
		if info, err := git.GetRepoInfo(repoPath); err == nil {
			repoName = info.Name
		}

		for _, wt := range worktrees {
			// Skip main worktree
			if wt.IsMain {
				continue
			}

			// Check if this worktree has an active session
			if activeWorktrees[wt.Path] {
				continue
			}

			// This is an orphaned worktree
			orphan := OrphanedWorktree{
				RepoName: repoName,
				RepoPath: repoPath,
				Worktree: wt,
				Selected: false,
			}

			// Check if branch is merged
			if merged, err := git.IsBranchMerged(repoPath, wt.Branch); err == nil {
				orphan.IsMerged = merged
			}

			// Get last commit time
			if lastCommit, err := git.GetLastCommitTime(wt.Path); err == nil {
				orphan.Age = time.Since(lastCommit)
			}

			c.orphans = append(c.orphans, orphan)
		}
	}

	return nil
}

func (c *CleanupController) cursorDown() {
	if c.cursor < len(c.orphans)-1 {
		c.cursor++
		// Scroll down if cursor goes below visible area
		if c.cursor >= c.scrollOffset+c.viewHeight {
			c.scrollOffset = c.cursor - c.viewHeight + 1
		}
	}
}

func (c *CleanupController) cursorUp() {
	if c.cursor > 0 {
		c.cursor--
		// Scroll up if cursor goes above visible area
		if c.cursor < c.scrollOffset {
			c.scrollOffset = c.cursor
		}
	}
}

func (c *CleanupController) toggleCurrent() {
	if c.cursor >= 0 && c.cursor < len(c.orphans) {
		c.orphans[c.cursor].Selected = !c.orphans[c.cursor].Selected
	}
}

func (c *CleanupController) toggleAll() {
	// If any are unselected, select all; otherwise deselect all
	allSelected := true
	for i := range c.orphans {
		if !c.orphans[i].Selected {
			allSelected = false
			break
		}
	}

	for i := range c.orphans {
		c.orphans[i].Selected = !allSelected
	}
}

func (c *CleanupController) selectedCount() int {
	count := 0
	for i := range c.orphans {
		if c.orphans[i].Selected {
			count++
		}
	}
	return count
}

func (c *CleanupController) performDelete() {
	// Delete selected worktrees
	for i := range c.orphans {
		if c.orphans[i].Selected {
			_ = git.RemoveWorktree(c.orphans[i].RepoPath, c.orphans[i].Worktree.Path)
		}
	}

	// Hide modal and refresh
	c.Hide(c.gui)
	if c.ctx.OnRefresh != nil {
		c.ctx.OnRefresh()
	}
}

func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(d.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "<1m"
}

// FormatOrphanLabel creates a display label for an orphaned worktree.
func FormatOrphanLabel(orphan *OrphanedWorktree) string {
	status := "orphan"
	if orphan.IsMerged {
		status = "merged"
	}
	return fmt.Sprintf("%s (%s, %s)", orphan.Worktree.Branch, status, formatAge(orphan.Age))
}
