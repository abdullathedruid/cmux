package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/abdullathedruid/cmux/internal/state"
	"github.com/abdullathedruid/cmux/internal/ui"
)

const dashboardViewName = "dashboard"

// DashboardController manages the dashboard grid view.
type DashboardController struct {
	ctx *Context
}

// NewDashboardController creates a new dashboard controller.
func NewDashboardController(ctx *Context) *DashboardController {
	return &DashboardController{ctx: ctx}
}

// Name returns the view name.
func (c *DashboardController) Name() string {
	return dashboardViewName
}

// Layout sets up the dashboard view.
func (c *DashboardController) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// Main view takes most of the screen, leaving 1 line for status bar
	v, err := g.SetView(dashboardViewName, 0, 0, maxX-1, maxY-2, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}
	v.Title = " cmux "
	v.Wrap = false
	v.Frame = true

	return nil
}

// Keybindings sets up dashboard-specific keybindings.
// Note: j/k/h/l navigation is handled globally in app.go to work around tcell keybinding issues.
func (c *DashboardController) Keybindings(g *gocui.Gui) error {
	// Arrow key navigation (special keys work fine with view-specific bindings)
	if err := g.SetKeybinding(dashboardViewName, gocui.KeyArrowDown, gocui.ModNone, c.cursorDown); err != nil {
		return err
	}
	if err := g.SetKeybinding(dashboardViewName, gocui.KeyArrowUp, gocui.ModNone, c.cursorUp); err != nil {
		return err
	}

	// Actions
	if err := g.SetKeybinding(dashboardViewName, gocui.KeyEnter, gocui.ModNone, c.attach); err != nil {
		return err
	}
	if err := g.SetKeybinding(dashboardViewName, 'p', gocui.ModNone, c.popupAttach); err != nil {
		return err
	}
	if err := g.SetKeybinding(dashboardViewName, 'n', gocui.ModNone, c.newSession); err != nil {
		return err
	}
	if err := g.SetKeybinding(dashboardViewName, 'd', gocui.ModNone, c.deleteSession); err != nil {
		return err
	}
	if err := g.SetKeybinding(dashboardViewName, 'r', gocui.ModNone, c.refresh); err != nil {
		return err
	}

	return nil
}

// Card height constants
const (
	largeCardHeight   = 9 // title, status, last active, 5 tools, context, bottom border
	compactCardHeight = 3 // title, status+last active, bottom border
)

// Render renders the dashboard content.
func (c *DashboardController) Render(g *gocui.Gui) error {
	v, err := g.View(dashboardViewName)
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
		fmt.Fprintln(v, "  Press 'n' to create a new session.")
		return nil
	}

	// Get view dimensions for card sizing
	width, height := v.Size()
	cardWidth := 35 // Default card width
	cardsPerRow := max((width-4)/cardWidth, 1)
	if cardsPerRow > 3 {
		cardsPerRow = 3 // Max 3 cards per row
	}
	// Account for: 2-space left margin + 2-space gaps between cards
	// Total margin = 2 + 2*(cardsPerRow-1) = 2*cardsPerRow
	cardWidth = (width - 2*cardsPerRow) / cardsPerRow

	// Determine card size based on available vertical space
	cardSize := c.calculateCardSize(repos, cardsPerRow, height)

	for _, repo := range repos {
		// Repository header
		fmt.Fprintf(v, "\n  %s\n", repo.Name)

		// Render sessions as cards in a grid
		c.renderCards(v, repo.Sessions, cardWidth, cardsPerRow, selectedName, cardSize)
	}

	return nil
}

// calculateCardSize determines whether to use large or compact cards based on terminal height.
func (c *DashboardController) calculateCardSize(repos []*state.Repository, cardsPerRow, availableHeight int) ui.CardSize {
	// Calculate total rows and height needed for large cards
	totalRows := 0
	for _, repo := range repos {
		sessionCount := len(repo.Sessions)
		rows := (sessionCount + cardsPerRow - 1) / cardsPerRow // ceiling division
		totalRows += rows
	}

	// Height calculation:
	// - Each repo header: 2 lines (blank + name)
	// - Each row of cards: cardHeight lines + 1 blank line after
	// - Some buffer for frame/borders
	repoCount := len(repos)
	repoHeaderLines := repoCount * 2
	largeCardsHeight := totalRows*(largeCardHeight+1) + repoHeaderLines

	// Use large cards if they fit, otherwise compact
	if largeCardsHeight <= availableHeight {
		return ui.CardSizeLarge
	}
	return ui.CardSizeCompact
}

// renderCards renders session cards in a grid.
func (c *DashboardController) renderCards(v *gocui.View, sessions []*state.Session, cardWidth, cardsPerRow int, selectedName string, cardSize ui.CardSize) {
	if len(sessions) == 0 {
		return
	}

	// Build cards
	cards := make([]*ui.Card, len(sessions))
	for i, sess := range sessions {
		cards[i] = c.buildCard(sess, cardWidth, sess.Name == selectedName, cardSize)
	}

	// Render in rows
	for i := 0; i < len(cards); i += cardsPerRow {
		end := i + cardsPerRow
		if end > len(cards) {
			end = len(cards)
		}
		rowCards := cards[i:end]

		// Get rendered lines for each card
		cardLines := make([][]string, len(rowCards))
		maxLines := 0
		for j, card := range rowCards {
			cardLines[j] = card.Render()
			if len(cardLines[j]) > maxLines {
				maxLines = len(cardLines[j])
			}
		}

		// Print each line of the row
		for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
			var line strings.Builder
			line.WriteString("  ")
			for cardIdx, cl := range cardLines {
				if lineIdx < len(cl) {
					line.WriteString(cl[lineIdx])
				} else {
					line.WriteString(strings.Repeat(" ", cardWidth))
				}
				if cardIdx < len(cardLines)-1 {
					line.WriteString("  ")
				}
			}
			fmt.Fprintln(v, line.String())
		}
		fmt.Fprintln(v, "")
	}
}

// buildCard builds a card for a session.
func (c *DashboardController) buildCard(sess *state.Session, width int, selected bool, cardSize ui.CardSize) *ui.Card {
	// Extract session display name (remove repo prefix if present)
	displayName := sess.Name
	if sess.RepoName != "" && strings.HasPrefix(sess.Name, sess.RepoName+"-") {
		displayName = strings.TrimPrefix(sess.Name, sess.RepoName+"-")
	}

	// Get status info
	icon := ui.StatusIcon(sess.Attached, sess.Status)
	status := ui.StatusText(sess.Attached, sess.Status)

	// Show tool summary instead of generic status when available
	// Truncate to fit card width (leave room for icon and padding)
	if sess.ToolSummary != "" {
		maxLen := width - 8 // account for borders, padding, icon
		if maxLen < 20 {
			maxLen = 20
		}
		status = ui.Truncate(sess.ToolSummary, maxLen)
	}

	// Format last active time
	lastActive := ""
	if !sess.LastActive.IsZero() {
		seconds := int64(time.Since(sess.LastActive).Seconds())
		lastActive = ui.FormatDuration(seconds)
	}

	// Get first line of note
	note := ""
	if sess.Note != "" {
		if idx := strings.Index(sess.Note, "\n"); idx != -1 {
			note = sess.Note[:idx]
		} else {
			note = sess.Note
		}
	}

	// Build tool history (5 tools for large cards, fewer for compact)
	var toolHistory []string
	maxTools := 5
	if cardSize == ui.CardSizeCompact {
		maxTools = 0 // Compact cards don't show tool history
	}
	for i := 0; i < len(sess.ToolHistory) && i < maxTools; i++ {
		entry := sess.ToolHistory[i]
		ts := entry.Timestamp.Local().Format("15:04:05")
		toolHistory = append(toolHistory, fmt.Sprintf("%s %s", ts, entry.Tool))
	}

	// Get last prompt (first line, truncated)
	lastPrompt := ""
	if sess.LastPrompt != "" {
		lastPrompt = sess.LastPrompt
		if idx := strings.Index(lastPrompt, "\n"); idx != -1 {
			lastPrompt = lastPrompt[:idx]
		}
	}

	return &ui.Card{
		Title:       displayName,
		Status:      status,
		Icon:        icon,
		LastActive:  lastActive,
		CurrentTool: sess.CurrentTool,
		Note:        note,
		LastPrompt:  lastPrompt,
		ToolHistory: toolHistory,
		Width:       width,
		Selected:    selected,
		BorderColor: ui.StatusColor(sess.Attached, sess.Status),
		Size:        cardSize,
	}
}

// Navigation handlers
func (c *DashboardController) cursorDown(g *gocui.Gui, v *gocui.View) error {
	c.ctx.State.SelectNext()
	return c.Render(g)
}

func (c *DashboardController) cursorUp(g *gocui.Gui, v *gocui.View) error {
	c.ctx.State.SelectPrev()
	return c.Render(g)
}

// Action handlers
func (c *DashboardController) attach(g *gocui.Gui, v *gocui.View) error {
	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		return nil
	}
	if c.ctx.OnAttach != nil {
		return c.ctx.OnAttach(sess.Name)
	}
	return nil
}

func (c *DashboardController) popupAttach(g *gocui.Gui, v *gocui.View) error {
	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		return nil
	}
	if c.ctx.OnPopupAttach != nil {
		return c.ctx.OnPopupAttach(sess.Name)
	}
	return nil
}

func (c *DashboardController) newSession(g *gocui.Gui, v *gocui.View) error {
	if c.ctx.OnNew != nil {
		return c.ctx.OnNew()
	}
	return nil
}

func (c *DashboardController) deleteSession(g *gocui.Gui, v *gocui.View) error {
	sess := c.ctx.State.GetSelectedSession()
	if sess == nil {
		return nil
	}
	if c.ctx.OnDelete != nil {
		return c.ctx.OnDelete(sess.Name)
	}
	return nil
}

func (c *DashboardController) refresh(g *gocui.Gui, v *gocui.View) error {
	if c.ctx.OnRefresh != nil {
		return c.ctx.OnRefresh()
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
