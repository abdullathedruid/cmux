package controller

import (
	"fmt"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/ui"
	"github.com/abdullathedruid/cmux/internal/version"
)

const statusBarViewName = "statusbar"

// StatusBarController manages the status bar at the bottom.
type StatusBarController struct {
	ctx *Context
}

// NewStatusBarController creates a new status bar controller.
func NewStatusBarController(ctx *Context) *StatusBarController {
	return &StatusBarController{ctx: ctx}
}

// Name returns the view name.
func (c *StatusBarController) Name() string {
	return statusBarViewName
}

// Layout sets up the status bar view.
func (c *StatusBarController) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	v, err := g.SetView(statusBarViewName, 0, maxY-2, maxX-1, maxY, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Frame = false
	v.Wrap = false
	v.BgColor = gocui.ColorBlue
	v.FgColor = gocui.ColorWhite | gocui.AttrBold

	return nil
}

// Keybindings sets up status bar keybindings (none needed).
func (c *StatusBarController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the status bar content.
func (c *StatusBarController) Render(g *gocui.Gui) error {
	v, err := g.View(statusBarViewName)
	if err != nil {
		return err
	}

	v.Clear()

	sessionCount := c.ctx.State.SessionCount()
	attachedCount := c.ctx.State.AttachedCount()
	activeCount := c.ctx.State.ActiveCount()
	isDashboard := c.ctx.State.IsDashboardView()

	content := ui.RenderStatusBar(sessionCount, attachedCount, activeCount, isDashboard, version.Short())
	fmt.Fprint(v, " "+content)

	return nil
}
