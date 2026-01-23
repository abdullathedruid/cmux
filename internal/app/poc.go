// Package app provides application lifecycle and orchestration.
package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/abdullathedruid/cmux/internal/input"
	"github.com/abdullathedruid/cmux/internal/pane"
	"github.com/abdullathedruid/cmux/internal/terminal"
	"github.com/abdullathedruid/cmux/internal/ui"
	"github.com/jesseduffield/gocui"
)

// PocApp is the new pane-centric application built on poc architecture.
type PocApp struct {
	gui      *gocui.Gui
	panes    *pane.Manager
	input    *input.Handler
	repoRoot string

	// Layout state for resize detection
	lastMaxX, lastMaxY int
	lastPaneCount      int
	lastLayouts        []pane.Layout
	firstCall          bool
}

// NewPocApp creates a new application instance.
func NewPocApp() (*PocApp, error) {
	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode: gocui.OutputTrue,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing GUI: %w", err)
	}

	repoRoot, err := getGitRepoRoot()
	if err != nil {
		repoRoot = "."
	}

	return &PocApp{
		gui:       g,
		panes:     pane.NewManager(),
		input:     input.NewHandler(),
		repoRoot:  repoRoot,
		firstCall: true,
	}, nil
}

// InitSessions initializes panes for the given tmux session names.
func (a *PocApp) InitSessions(sessions []string) error {
	maxX, maxY := a.gui.Size()
	layouts := pane.CalculateLayouts(len(sessions), maxX, maxY)

	for i, session := range sessions {
		layout := layouts[i]
		width := layout.Width()
		height := layout.Height()

		p := pane.New(i+1, session, width, height)
		if err := p.Start(width, height); err != nil {
			// Clean up already created panes
			a.panes.CloseAll()
			return fmt.Errorf("starting control mode for %s: %w", session, err)
		}

		a.panes.Add(p)

		// Start output processing goroutine
		go a.processOutput(p)
	}

	return nil
}

// processOutput reads from a pane's output channel and writes to its terminal.
func (a *PocApp) processOutput(p *pane.Pane) {
	for data := range p.OutputChan() {
		p.WriteToTerminal(data)
		a.gui.Update(func(g *gocui.Gui) error { return nil })
	}
}

// Run starts the main event loop.
func (a *PocApp) Run() error {
	defer a.Close()

	a.gui.SetManagerFunc(a.layout)

	if err := a.setupKeybindings(); err != nil {
		return fmt.Errorf("setting up keybindings: %w", err)
	}

	// Handle SIGINT/SIGTERM for clean exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		a.gui.Update(func(g *gocui.Gui) error {
			return gocui.ErrQuit
		})
	}()

	if err := a.gui.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) && err.Error() != "quit" {
		return fmt.Errorf("main loop: %w", err)
	}

	return nil
}

// Close cleans up all resources.
func (a *PocApp) Close() {
	a.panes.CloseAll()
	a.gui.Close()
}

// layout is the gocui manager function that arranges views.
func (a *PocApp) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	panes := a.panes.All()
	currentMode := a.input.Mode()
	activePaneIdx := a.panes.ActiveIndex()
	inputBuffer := a.input.InputBuffer()

	// Recalculate layouts if size or pane count changed
	var layouts []pane.Layout
	if maxX != a.lastMaxX || maxY != a.lastMaxY || len(panes) != a.lastPaneCount {
		layouts = pane.CalculateLayouts(len(panes), maxX, maxY)
		a.lastMaxX, a.lastMaxY = maxX, maxY
		a.lastPaneCount = len(panes)
	} else {
		layouts = a.lastLayouts
	}

	for i, p := range panes {
		if i >= len(layouts) {
			continue
		}
		layout := layouts[i]

		// Handle resize for this pane
		if i < len(a.lastLayouts) && layouts[i] != a.lastLayouts[i] {
			width := layout.Width()
			height := layout.Height()
			if width > 0 && height > 0 {
				p.Resize(width, height)
			}
		}

		v, err := g.SetView(p.ViewName, layout.X0, layout.Y0, layout.X1, layout.Y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}

		// Configure view styling
		isActive := i == activePaneIdx
		ui.ConfigurePaneView(v, p, isActive, currentMode)
		v.Editor = gocui.EditorFunc(a.makeTerminalEditor(p.Ctrl))

		// Render terminal buffer to view
		v.Clear()
		ui.RenderTerminal(v, p.Term)
	}

	// Handle input modal
	if currentMode.IsInput() {
		x0, y0, x1, y1 := ui.ModalDimensions(maxX, maxY, 50, 3)
		v, err := g.SetView("input-modal", x0, y0, x1, y1, 0)
		if err != nil {
			if !errors.Is(err, gocui.ErrUnknownView) && err.Error() != "unknown view" {
				return err
			}
		}
		ui.ConfigureInputModal(v, inputBuffer)
		v.Editor = gocui.EditorFunc(a.makeInputEditor())

		if _, err := g.SetCurrentView("input-modal"); err != nil {
			return err
		}
		g.Cursor = true
		v.SetCursor(len(inputBuffer)+1, 0)
	} else {
		// Delete modal if not in input mode
		g.DeleteView("input-modal")

		// Set focus to active pane and handle cursor
		if len(panes) > 0 && activePaneIdx < len(panes) {
			activePane := panes[activePaneIdx]
			if _, err := g.SetCurrentView(activePane.ViewName); err != nil && a.firstCall {
				return err
			}
			a.firstCall = false

			// Set cursor after view is focused
			if currentMode.IsTerminal() {
				x, y := activePane.Term.Cursor()
				if v, err := g.View(activePane.ViewName); err == nil {
					v.SetCursor(x, y)
				}
				g.Cursor = true
			} else {
				g.Cursor = false
			}
		}
	}

	// Save layouts for next comparison
	if len(layouts) == len(a.lastLayouts) {
		copy(a.lastLayouts, layouts)
	} else {
		a.lastLayouts = make([]pane.Layout, len(layouts))
		copy(a.lastLayouts, layouts)
	}

	return nil
}

// addNewPane creates and adds a new pane for the given tmux session.
func (a *PocApp) addNewPane(sessionName string) error {
	maxX, maxY := a.gui.Size()
	paneCount := a.panes.Count()
	layouts := pane.CalculateLayouts(paneCount+1, maxX, maxY)
	layout := layouts[paneCount]

	width := layout.Width()
	height := layout.Height()

	p := pane.New(paneCount+1, sessionName, width, height)
	if err := p.Start(width, height); err != nil {
		return fmt.Errorf("start control mode: %w", err)
	}

	// Start output processing goroutine
	go a.processOutput(p)

	a.panes.Add(p)
	a.panes.FocusLast()

	return nil
}

// createWorktreeAndSession creates a git worktree and a tmux session in it.
func (a *PocApp) createWorktreeAndSession(name string) error {
	worktreePath := fmt.Sprintf("%s/.worktrees/%s", a.repoRoot, name)

	// Create the worktree with a new branch
	cmd := exec.Command("git", "-C", a.repoRoot, "worktree", "add", "-b", name, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, out)
	}

	// Create a new tmux session in the worktree directory
	cmd = exec.Command("tmux", "new-session", "-d", "-s", name, "-c", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, out)
	}

	return nil
}

// makeTerminalEditor creates an editor function that forwards character input to tmux.
func (a *PocApp) makeTerminalEditor(ctrl *terminal.ControlMode) func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		// Only forward input in terminal mode
		if !a.input.Mode().IsTerminal() {
			return false
		}

		// Only handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			ctrl.SendLiteralKeys(string(ch))
			return true
		}
		return false
	}
}

// makeInputEditor creates an editor function for the input modal.
func (a *PocApp) makeInputEditor() func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return func(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
		if !a.input.Mode().IsInput() {
			return false
		}

		// Handle printable characters
		if ch != 0 && mod == gocui.ModNone {
			a.input.AppendToInputBuffer(ch)
			return true
		}
		return false
	}
}

// getGitRepoRoot returns the root directory of the current git repository.
func getGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
