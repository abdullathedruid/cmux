package controller

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/state"
)

const searchViewName = "search"

// SearchController manages the search/filter functionality.
type SearchController struct {
	ctx      *Context
	visible  bool
	query    string
	results  []*state.Session
	selected int
	onSelect func(sessionName string) error
	gui      *gocui.Gui
}

// Edit handles key input for the search modal.
func (c *SearchController) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc:
		c.close(c.gui, v)
		return true
	case key == gocui.KeyEnter:
		c.selectResult(c.gui, v)
		return true
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		c.backspace(c.gui, v)
		return true
	case key == gocui.KeyArrowDown || key == gocui.KeyCtrlN:
		c.cursorDown(c.gui, v)
		return true
	case key == gocui.KeyArrowUp || key == gocui.KeyCtrlP:
		c.cursorUp(c.gui, v)
		return true
	case ch != 0 && mod == gocui.ModNone:
		c.query += string(ch)
		c.selected = 0
		c.updateResults()
		c.Render(c.gui)
		return true
	}
	return false
}

// NewSearchController creates a new search controller.
func NewSearchController(ctx *Context, onSelect func(sessionName string) error) *SearchController {
	return &SearchController{
		ctx:      ctx,
		onSelect: onSelect,
	}
}

// Name returns the view name.
func (c *SearchController) Name() string {
	return searchViewName
}

// IsVisible returns whether the search is visible.
func (c *SearchController) IsVisible() bool {
	return c.visible
}

// Show shows the search modal.
func (c *SearchController) Show(g *gocui.Gui) error {
	c.visible = true
	c.query = ""
	c.selected = 0
	c.gui = g
	c.updateResults()
	return c.Layout(g)
}

// Hide hides the search modal.
func (c *SearchController) Hide(g *gocui.Gui) error {
	c.visible = false
	c.query = ""
	return g.DeleteView(searchViewName)
}

// Layout sets up the search view.
func (c *SearchController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the modal
	width := 50
	height := 15
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(searchViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}

	v.Title = " Search Sessions "
	v.Wrap = false
	v.Frame = true
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.Edit)

	// Set as top view
	if _, err := g.SetCurrentView(searchViewName); err != nil {
		return err
	}

	return c.Render(g)
}

// Keybindings sets up search keybindings.
// Note: Key handling is done via the custom Editor interface instead.
func (c *SearchController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the search content.
func (c *SearchController) Render(g *gocui.Gui) error {
	v, err := g.View(searchViewName)
	if err != nil {
		return err
	}

	v.Clear()

	// Search prompt
	fmt.Fprintf(v, " > %s_\n", c.query)
	fmt.Fprintln(v, " ────────────────────────────────────────────")

	if len(c.results) == 0 {
		if c.query == "" {
			fmt.Fprintln(v, " Type to search sessions...")
		} else {
			fmt.Fprintln(v, " No matching sessions")
		}
	} else {
		// Show up to 8 results
		maxResults := 8
		if len(c.results) < maxResults {
			maxResults = len(c.results)
		}

		for i := 0; i < maxResults; i++ {
			sess := c.results[i]
			prefix := "  "
			if i == c.selected {
				prefix = "> "
			}

			// Format: name (repo) [status]
			status := "idle"
			if sess.Attached {
				status = "attached"
			}
			fmt.Fprintf(v, "%s%s (%s) [%s]\n", prefix, sess.Name, sess.RepoName, status)
		}

		if len(c.results) > maxResults {
			fmt.Fprintf(v, " ... and %d more\n", len(c.results)-maxResults)
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: Select  Esc: Cancel")

	return nil
}

func (c *SearchController) updateResults() {
	if c.query == "" {
		c.results = c.ctx.State.GetSessions()
		return
	}

	query := strings.ToLower(c.query)
	allSessions := c.ctx.State.GetSessions()
	c.results = make([]*state.Session, 0)

	for _, sess := range allSessions {
		// Fuzzy match on name, repo name, branch, and note
		if fuzzyMatch(sess.Name, query) ||
			fuzzyMatch(sess.RepoName, query) ||
			fuzzyMatch(sess.Branch, query) ||
			fuzzyMatch(sess.Note, query) {
			c.results = append(c.results, sess)
		}
	}
}

// fuzzyMatch performs a simple fuzzy match (contains-based).
func fuzzyMatch(text, pattern string) bool {
	text = strings.ToLower(text)
	pattern = strings.ToLower(pattern)

	// Simple substring match
	if strings.Contains(text, pattern) {
		return true
	}

	// Character-by-character fuzzy match
	patternIdx := 0
	for i := 0; i < len(text) && patternIdx < len(pattern); i++ {
		if text[i] == pattern[patternIdx] {
			patternIdx++
		}
	}
	return patternIdx == len(pattern)
}

// Navigation handlers
func (c *SearchController) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if c.selected < len(c.results)-1 {
		c.selected++
	}
	return c.Render(g)
}

func (c *SearchController) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if c.selected > 0 {
		c.selected--
	}
	return c.Render(g)
}

func (c *SearchController) selectResult(g *gocui.Gui, v *gocui.View) error {
	if len(c.results) == 0 || c.selected >= len(c.results) {
		return nil
	}

	sess := c.results[c.selected]

	// Hide search
	if err := c.Hide(g); err != nil {
		return err
	}

	// Select the session in state
	c.ctx.State.SetSelectedSession(sess.Name)

	// If callback provided, call it (e.g., to attach)
	if c.onSelect != nil {
		return c.onSelect(sess.Name)
	}

	return nil
}

func (c *SearchController) close(g *gocui.Gui, v *gocui.View) error {
	return c.Hide(g)
}

func (c *SearchController) backspace(g *gocui.Gui, v *gocui.View) error {
	if len(c.query) > 0 {
		c.query = c.query[:len(c.query)-1]
		c.selected = 0
		c.updateResults()
	}
	return c.Render(g)
}
