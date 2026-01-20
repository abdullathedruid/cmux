package controller

import (
	"fmt"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/ui"
)

const helpViewName = "help"

// HelpController manages the help modal.
type HelpController struct {
	ctx     *Context
	visible bool
}

// NewHelpController creates a new help controller.
func NewHelpController(ctx *Context) *HelpController {
	return &HelpController{ctx: ctx}
}

// Name returns the view name.
func (c *HelpController) Name() string {
	return helpViewName
}

// IsVisible returns whether the help is visible.
func (c *HelpController) IsVisible() bool {
	return c.visible
}

// Show shows the help modal.
func (c *HelpController) Show(g *gocui.Gui) error {
	c.visible = true
	return c.Layout(g)
}

// Hide hides the help modal.
func (c *HelpController) Hide(g *gocui.Gui) error {
	c.visible = false
	return g.DeleteView(helpViewName)
}

// Toggle toggles the help modal visibility.
func (c *HelpController) Toggle(g *gocui.Gui) error {
	if c.visible {
		return c.Hide(g)
	}
	return c.Show(g)
}

// Layout sets up the help view.
func (c *HelpController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the help modal
	width := 50
	height := 25
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(helpViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = " Help "
	v.Wrap = true
	v.Frame = true

	// Set as top view
	if _, err := g.SetCurrentView(helpViewName); err != nil {
		return err
	}

	return c.Render(g)
}

// Keybindings sets up help-specific keybindings.
func (c *HelpController) Keybindings(g *gocui.Gui) error {
	// Any key closes help
	if err := g.SetKeybinding(helpViewName, gocui.KeyEsc, gocui.ModNone, c.close); err != nil {
		return err
	}
	if err := g.SetKeybinding(helpViewName, '?', gocui.ModNone, c.close); err != nil {
		return err
	}
	if err := g.SetKeybinding(helpViewName, 'q', gocui.ModNone, c.close); err != nil {
		return err
	}
	if err := g.SetKeybinding(helpViewName, gocui.KeyEnter, gocui.ModNone, c.close); err != nil {
		return err
	}
	if err := g.SetKeybinding(helpViewName, gocui.KeySpace, gocui.ModNone, c.close); err != nil {
		return err
	}

	return nil
}

// Render renders the help content.
func (c *HelpController) Render(g *gocui.Gui) error {
	v, err := g.View(helpViewName)
	if err != nil {
		return err
	}

	v.Clear()
	fmt.Fprint(v, ui.HelpText())
	return nil
}

func (c *HelpController) close(g *gocui.Gui, v *gocui.View) error {
	return c.Hide(g)
}
