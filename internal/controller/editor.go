package controller

import (
	"fmt"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
)

const editorViewName = "editor"

// EditorController manages the note editor modal.
type EditorController struct {
	ctx         *Context
	visible     bool
	sessionName string
	content     string
	cursorPos   int
	onSave      func(sessionName, content string) error
	gui         *gocui.Gui
}

// Edit handles key input for the editor modal.
func (c *EditorController) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch {
	case key == gocui.KeyEsc:
		c.cancel(c.gui, v)
		return true
	case key == gocui.KeyCtrlS:
		c.save(c.gui, v)
		return true
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		c.backspace(c.gui, v)
		return true
	case key == gocui.KeyEnter:
		c.newline(c.gui, v)
		return true
	case ch != 0 && mod == gocui.ModNone:
		c.content += string(ch)
		c.Render(c.gui)
		return true
	}
	return false
}

// NewEditorController creates a new editor controller.
func NewEditorController(ctx *Context, onSave func(sessionName, content string) error) *EditorController {
	return &EditorController{
		ctx:    ctx,
		onSave: onSave,
	}
}

// Name returns the view name.
func (c *EditorController) Name() string {
	return editorViewName
}

// IsVisible returns whether the editor is visible.
func (c *EditorController) IsVisible() bool {
	return c.visible
}

// Show shows the editor for a session.
func (c *EditorController) Show(g *gocui.Gui, sessionName, currentNote string) error {
	c.sessionName = sessionName
	c.content = currentNote
	c.cursorPos = len(currentNote)
	c.visible = true
	c.gui = g
	return c.Layout(g)
}

// Hide hides the editor.
func (c *EditorController) Hide(g *gocui.Gui) error {
	c.visible = false
	return g.DeleteView(editorViewName)
}

// Layout sets up the editor view.
func (c *EditorController) Layout(g *gocui.Gui) error {
	if !c.visible {
		return nil
	}

	maxX, maxY := g.Size()

	// Center the modal
	width := 60
	height := 15
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	v, err := g.SetView(editorViewName, x0, y0, x0+width, y0+height, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = fmt.Sprintf(" Edit Note: %s ", c.sessionName)
	v.Wrap = true
	v.Frame = true
	v.Editable = true
	v.Editor = gocui.EditorFunc(c.Edit)

	// Set as top view
	if _, err := g.SetCurrentView(editorViewName); err != nil {
		return err
	}

	return c.Render(g)
}

// Keybindings sets up editor keybindings.
// Note: Key handling is done via the custom Editor interface instead.
func (c *EditorController) Keybindings(g *gocui.Gui) error {
	return nil
}

// Render renders the editor content.
func (c *EditorController) Render(g *gocui.Gui) error {
	v, err := g.View(editorViewName)
	if err != nil {
		return err
	}

	v.Clear()

	fmt.Fprintln(v, c.content+"_")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "───────────────────────────────────────────")
	fmt.Fprintln(v, "Ctrl+S: Save  Esc: Cancel")

	return nil
}

func (c *EditorController) save(g *gocui.Gui, v *gocui.View) error {
	// Get content from view buffer
	content := c.content

	// Call save handler
	if c.onSave != nil {
		if err := c.onSave(c.sessionName, content); err != nil {
			return err
		}
	}

	return c.Hide(g)
}

func (c *EditorController) cancel(g *gocui.Gui, v *gocui.View) error {
	return c.Hide(g)
}

func (c *EditorController) backspace(g *gocui.Gui, v *gocui.View) error {
	if len(c.content) > 0 {
		c.content = c.content[:len(c.content)-1]
	}
	return c.Render(g)
}

func (c *EditorController) newline(g *gocui.Gui, v *gocui.View) error {
	c.content += "\n"
	return c.Render(g)
}
