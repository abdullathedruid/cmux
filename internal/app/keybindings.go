package app

import (
	"strings"

	"github.com/abdullathedruid/cmux/internal/input"
	"github.com/abdullathedruid/cmux/internal/terminal"
	"github.com/jesseduffield/gocui"
)

// setupKeybindings configures all keyboard handlers.
func (a *PocApp) setupKeybindings() error {
	g := a.gui

	// Helper to get control mode for active pane
	getActiveCtrl := func() *terminal.ControlMode {
		if p := a.panes.Active(); p != nil {
			return p.Ctrl
		}
		return nil
	}

	// Helper to check current mode
	getMode := func() input.Mode {
		return a.input.Mode()
	}

	// Helper to move to adjacent pane
	movePaneDirection := func(g *gocui.Gui, dx, dy int) error {
		if a.panes.Count() == 0 {
			return nil
		}

		// Simple navigation: cycle through panes
		// TODO: implement spatial navigation based on layout
		if dx > 0 || dy > 0 {
			a.panes.Next()
		} else if dx < 0 || dy < 0 {
			a.panes.Prev()
		}
		return nil
	}

	// === GLOBAL KEYBINDINGS ===

	// Ctrl+Q: Exit terminal mode (works in both modes)
	if err := g.SetKeybinding("", gocui.KeyCtrlQ, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		a.input.EnterNormalMode()
		return nil
	}); err != nil {
		return err
	}

	// === NORMAL MODE KEYBINDINGS ===

	// q: Quit application (normal mode only)
	if err := g.SetKeybinding("", 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("q")
			}
			return nil
		}
		return gocui.ErrQuit
	}); err != nil {
		return err
	}

	// N: Open new worktree input modal (normal mode only)
	if err := g.SetKeybinding("", 'N', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode.IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("N")
			}
			return nil
		}
		if mode.IsInput() {
			return nil // Already in input mode
		}
		a.input.EnterInputMode()
		return nil
	}); err != nil {
		return err
	}

	// i: Enter terminal mode
	if err := g.SetKeybinding("", 'i', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("i")
			}
			return nil
		}
		// Reset scroll position when entering terminal mode
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollToBottom()
		}
		a.input.EnterTerminalMode()
		return nil
	}); err != nil {
		return err
	}

	// Enter: Enter terminal mode or confirm input
	if err := g.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode.IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("Enter")
			}
			return nil
		}
		if mode.IsInput() {
			name := strings.TrimSpace(a.input.ConsumeInputBuffer())
			if name == "" {
				return nil
			}

			if err := a.createWorktreeAndSession(name); err != nil {
				// TODO: show error to user
				return nil
			}

			return a.addNewPane(name)
		}
		// Reset scroll position when entering terminal mode
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollToBottom()
		}
		a.input.EnterTerminalMode()
		return nil
	}); err != nil {
		return err
	}

	// h/j/k/l: Navigate panes (normal mode) or forward to terminal
	navKeys := map[rune]struct{ dx, dy int }{
		'h': {-1, 0},
		'l': {1, 0},
		'k': {0, -1},
		'j': {0, 1},
	}

	for key, dir := range navKeys {
		k := key
		d := dir
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode().IsTerminal() {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendLiteralKeys(string(k))
				}
				return nil
			}
			return movePaneDirection(g, d.dx, d.dy)
		}); err != nil {
			return err
		}
	}

	// 1-9: Jump to pane N
	for i := 1; i <= 9; i++ {
		paneIdx := i - 1
		key := rune('0' + i)
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode().IsTerminal() {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendLiteralKeys(string(key))
				}
				return nil
			}
			if paneIdx < a.panes.Count() {
				a.panes.SetActive(paneIdx)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Arrow keys: Navigate in normal mode, forward in terminal mode
	arrowMappings := map[gocui.Key]struct {
		dx, dy  int
		tmuxKey string
	}{
		gocui.KeyArrowLeft:  {-1, 0, "Left"},
		gocui.KeyArrowRight: {1, 0, "Right"},
		gocui.KeyArrowUp:    {0, -1, "Up"},
		gocui.KeyArrowDown:  {0, 1, "Down"},
	}

	for key, mapping := range arrowMappings {
		k := key
		m := mapping
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if getMode().IsTerminal() {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendKeys(m.tmuxKey)
				}
				return nil
			}
			return movePaneDirection(g, m.dx, m.dy)
		}); err != nil {
			return err
		}
	}

	// === NORMAL MODE SCROLLBACK KEYBINDINGS ===

	// Ctrl+U: Scroll up half page (normal mode)
	if err := g.SetKeybinding("", gocui.KeyCtrlU, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("C-u")
			}
			return nil
		}
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollUp(12) // Half page ~12 lines
		}
		return nil
	}); err != nil {
		return err
	}

	// Ctrl+D: Scroll down half page (normal mode)
	if err := g.SetKeybinding("", gocui.KeyCtrlD, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("C-d")
			}
			return nil
		}
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollDown(12)
		}
		return nil
	}); err != nil {
		return err
	}

	// Ctrl+B: Scroll up full page (normal mode)
	if err := g.SetKeybinding("", gocui.KeyCtrlB, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("C-b")
			}
			return nil
		}
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollUp(24) // Full page ~24 lines
		}
		return nil
	}); err != nil {
		return err
	}

	// Ctrl+F: Scroll down full page (normal mode)
	if err := g.SetKeybinding("", gocui.KeyCtrlF, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("C-f")
			}
			return nil
		}
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollDown(24)
		}
		return nil
	}); err != nil {
		return err
	}

	// G: Scroll to bottom (normal mode) - return to live view
	if err := g.SetKeybinding("", 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if getMode().IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendLiteralKeys("G")
			}
			return nil
		}
		if p := a.panes.Active(); p != nil {
			p.Scrollback.ScrollToBottom()
		}
		return nil
	}); err != nil {
		return err
	}

	// === TERMINAL AND INPUT MODE KEYBINDINGS ===

	// Esc: Cancel input mode or forward to terminal
	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		mode := getMode()
		if mode.IsInput() {
			a.input.ExitInputMode()
			return nil
		}
		if mode.IsTerminal() {
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys("Escape")
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Backspace: Delete char in input mode or forward to terminal
	for _, key := range []gocui.Key{gocui.KeyBackspace, gocui.KeyBackspace2} {
		k := key
		if err := g.SetKeybinding("", k, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			mode := getMode()
			if mode.IsInput() {
				a.input.BackspaceInputBuffer()
				return nil
			}
			if mode.IsTerminal() {
				if ctrl := getActiveCtrl(); ctrl != nil {
					return ctrl.SendKeys("BSpace")
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Special keys that only make sense in terminal mode
	terminalOnlyKeys := map[gocui.Key]string{
		gocui.KeyDelete: "DC",
		gocui.KeyHome:   "Home",
		gocui.KeyEnd:    "End",
		gocui.KeyPgup:   "PPage",
		gocui.KeyPgdn:   "NPage",
		gocui.KeySpace:  "Space",
		gocui.KeyTab:    "Tab",
	}

	for key, tmuxKey := range terminalOnlyKeys {
		tk := tmuxKey
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if !getMode().IsTerminal() {
				return nil
			}
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys(tk)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Ctrl key combinations (except Ctrl+Q which is mode switch)
	ctrlMappings := map[gocui.Key]string{
		gocui.KeyCtrlA:         "C-a",
		gocui.KeyCtrlB:         "C-b",
		gocui.KeyCtrlC:         "C-c",
		gocui.KeyCtrlD:         "C-d",
		gocui.KeyCtrlE:         "C-e",
		gocui.KeyCtrlF:         "C-f",
		gocui.KeyCtrlG:         "C-g",
		gocui.KeyCtrlH:         "C-h",
		gocui.KeyCtrlJ:         "C-j",
		gocui.KeyCtrlK:         "C-k",
		gocui.KeyCtrlL:         "C-l",
		gocui.KeyCtrlN:         "C-n",
		gocui.KeyCtrlO:         "C-o",
		gocui.KeyCtrlP:         "C-p",
		gocui.KeyCtrlR:         "C-r",
		gocui.KeyCtrlS:         "C-s",
		gocui.KeyCtrlT:         "C-t",
		gocui.KeyCtrlU:         "C-u",
		gocui.KeyCtrlV:         "C-v",
		gocui.KeyCtrlW:         "C-w",
		gocui.KeyCtrlX:         "C-x",
		gocui.KeyCtrlY:         "C-y",
		gocui.KeyCtrlZ:         "C-z",
		gocui.KeyCtrlBackslash: "C-\\",
	}

	for key, tmuxKey := range ctrlMappings {
		tk := tmuxKey
		if err := g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if !getMode().IsTerminal() {
				return nil
			}
			if ctrl := getActiveCtrl(); ctrl != nil {
				return ctrl.SendKeys(tk)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
