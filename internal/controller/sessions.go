package controller

import (
	"fmt"
	"strings"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/git"
	"github.com/abdullathedruid/cmux/internal/ui"
)

const (
	sessionsViewName = "sessions"
	detailsViewName  = "details"
)

// SessionsController manages the sessions list view.
type SessionsController struct {
	ctx *Context
}

// NewSessionsController creates a new sessions controller.
func NewSessionsController(ctx *Context) *SessionsController {
	return &SessionsController{ctx: ctx}
}

// Name returns the view name.
func (c *SessionsController) Name() string {
	return sessionsViewName
}

// Layout sets up the sessions list view.
func (c *SessionsController) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// Sessions list on the left (1/3 width)
	splitX := maxX / 3
	if splitX < 25 {
		splitX = 25
	}

	v, err := g.SetView(sessionsViewName, 0, 0, splitX-1, maxY-2, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}
	v.Title = " Sessions "
	v.Wrap = false
	v.Frame = true
	v.Highlight = true
	v.SelBgColor = gocui.ColorBlue
	v.SelFgColor = gocui.ColorWhite

	// Details panel on the right
	dv, err := g.SetView(detailsViewName, splitX, 0, maxX-1, maxY-2, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}
	dv.Title = " Details "
	dv.Wrap = true
	dv.Frame = true

	return nil
}

// Keybindings sets up sessions-specific keybindings.
func (c *SessionsController) Keybindings(g *gocui.Gui) error {
	// Navigation
	if err := g.SetKeybinding(sessionsViewName, 'j', gocui.ModNone, c.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, 'k', gocui.ModNone, c.cursorUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, gocui.KeyArrowDown, gocui.ModNone, c.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, gocui.KeyArrowUp, gocui.ModNone, c.cursorUp); err != nil {
		return err
	}

	// Actions
	if err := g.SetKeybinding(sessionsViewName, gocui.KeyEnter, gocui.ModNone, c.attach); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, 'n', gocui.ModNone, c.newSession); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, 'd', gocui.ModNone, c.deleteSession); err != nil {
		return err
	}
	if err := g.SetKeybinding(sessionsViewName, 'r', gocui.ModNone, c.refresh); err != nil {
		return err
	}

	return nil
}

// Render renders the sessions list and details panel.
func (c *SessionsController) Render(g *gocui.Gui) error {
	if err := c.renderSessionsList(g); err != nil {
		return err
	}
	return c.renderDetails(g)
}

// renderSessionsList renders the sessions list.
func (c *SessionsController) renderSessionsList(g *gocui.Gui) error {
	v, err := g.View(sessionsViewName)
	if err != nil {
		return err
	}

	v.Clear()

	repos := c.ctx.State.GetRepositories()
	selectedName := c.ctx.State.GetSelectedSessionName()

	if len(repos) == 0 {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  No sessions found.")
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  Press 'n' to create")
		fmt.Fprintln(v, "  a new session.")
		return nil
	}

	lineIdx := 0
	selectedLine := 0

	for _, repo := range repos {
		// Repository header
		fmt.Fprintf(v, "\n%s\n", repo.Name)
		lineIdx += 2

		for _, sess := range repo.Sessions {
			// Build session line
			icon := ui.StatusIcon(sess.Attached, sess.Status)
			displayName := sess.Name
			if sess.RepoName != "" && strings.HasPrefix(sess.Name, sess.RepoName+"-") {
				displayName = strings.TrimPrefix(sess.Name, sess.RepoName+"-")
			}

			prefix := "  "
			if sess.Name == selectedName {
				prefix = "> "
				selectedLine = lineIdx
			}

			fmt.Fprintf(v, "%s%s %s\n", prefix, icon, displayName)
			lineIdx++
		}
	}

	// Set cursor position to selected line
	v.SetCursor(0, selectedLine)
	v.SetOrigin(0, max(0, selectedLine-5))

	return nil
}

// renderDetails renders the details panel.
func (c *SessionsController) renderDetails(g *gocui.Gui) error {
	v, err := g.View(detailsViewName)
	if err != nil {
		return err
	}

	v.Clear()

	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  No session selected")
		return nil
	}

	// Session details
	fmt.Fprintln(v, "")
	fmt.Fprintf(v, "  Name:     %s\n", sess.Name)

	worktreePath := sess.Worktree
	if worktreePath == "" {
		worktreePath = sess.RepoPath
	}
	fmt.Fprintf(v, "  Worktree: %s\n", git.ShortenPath(worktreePath))

	fmt.Fprintf(v, "  Branch:   %s\n", sess.Branch)
	fmt.Fprintf(v, "  Status:   %s\n", ui.StatusText(sess.Attached, sess.Status))

	// Created time
	if !sess.Created.IsZero() {
		fmt.Fprintf(v, "  Created:  %s\n", sess.Created.Format("2006-01-02 15:04"))
	}

	// Note
	if sess.Note != "" {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "  Note:")
		for _, line := range strings.Split(sess.Note, "\n") {
			fmt.Fprintf(v, "  %s\n", line)
		}
	}

	return nil
}

// Navigation handlers
func (c *SessionsController) cursorDown(g *gocui.Gui, v *gocui.View) error {
	c.ctx.State.SelectNext()
	return c.Render(g)
}

func (c *SessionsController) cursorUp(g *gocui.Gui, v *gocui.View) error {
	c.ctx.State.SelectPrev()
	return c.Render(g)
}

// Action handlers
func (c *SessionsController) attach(g *gocui.Gui, v *gocui.View) error {
	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		return nil
	}
	if c.ctx.OnAttach != nil {
		return c.ctx.OnAttach(sess.Name)
	}
	return nil
}

func (c *SessionsController) newSession(g *gocui.Gui, v *gocui.View) error {
	if c.ctx.OnNew != nil {
		return c.ctx.OnNew()
	}
	return nil
}

func (c *SessionsController) deleteSession(g *gocui.Gui, v *gocui.View) error {
	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		return nil
	}
	if c.ctx.OnDelete != nil {
		return c.ctx.OnDelete(sess.Name)
	}
	return nil
}

func (c *SessionsController) refresh(g *gocui.Gui, v *gocui.View) error {
	if c.ctx.OnRefresh != nil {
		return c.ctx.OnRefresh()
	}
	return nil
}
