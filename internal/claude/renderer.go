package claude

import (
	"fmt"
	"strings"
	"time"
)

// Renderer renders a Claude session to a string for display.
type Renderer struct {
	width  int
	height int
}

// NewRenderer creates a renderer with the given dimensions.
func NewRenderer(width, height int) *Renderer {
	return &Renderer{width: width, height: height}
}

// Resize updates the renderer dimensions.
func (r *Renderer) Resize(width, height int) {
	r.width = width
	r.height = height
}

// Render renders a session to a string.
func (r *Renderer) Render(session *Session) string {
	if session == nil {
		return r.renderEmpty()
	}

	var sb strings.Builder

	// Calculate available space
	// Reserve: 1 line for status bar, 2 lines for input prompt area
	contentHeight := r.height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render messages (bottom-aligned, most recent visible)
	lines := r.renderMessages(session.Messages, contentHeight)

	// Pad to fill height
	for i := len(lines); i < contentHeight; i++ {
		sb.WriteString("\n")
	}
	for _, line := range lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Render current activity / input prompt
	sb.WriteString(r.renderActivityLine(session))
	sb.WriteString("\n")

	// Render status bar
	sb.WriteString(r.renderStatusBar(session))

	return sb.String()
}

func (r *Renderer) renderEmpty() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(r.centerText("No Claude session active", r.width))
	sb.WriteString("\n\n")
	sb.WriteString(r.centerText("Start Claude Code in a tmux session to connect", r.width))
	return sb.String()
}

func (r *Renderer) renderMessages(messages []Message, maxLines int) []string {
	var allLines []string
	var prevRole string
	var prevHadTools bool

	for _, msg := range messages {
		// Only show header when role changes (like a chat app grouping)
		showHeader := msg.Role != prevRole

		// Add gap between tool-only and text messages in same assistant group
		if !showHeader && msg.Role == "assistant" && prevHadTools && msg.TextPreview != "" && len(msg.ToolCalls) == 0 {
			allLines = append(allLines, "") // Gap before text after tools
		}

		msgLines := r.renderMessageGrouped(msg, showHeader)
		allLines = append(allLines, msgLines...)

		prevRole = msg.Role
		prevHadTools = len(msg.ToolCalls) > 0
	}

	// Return only the last maxLines
	if len(allLines) > maxLines {
		return allLines[len(allLines)-maxLines:]
	}
	return allLines
}

func (r *Renderer) renderMessageGrouped(msg Message, showHeader bool) []string {
	var lines []string

	switch msg.Role {
	case "user":
		if showHeader {
			lines = append(lines, "") // Blank line before new group
			lines = append(lines, r.styleUserHeader(msg.Timestamp))
		}
		lines = append(lines, r.wrapText(msg.Content, r.width-4, "    ")...)

	case "assistant":
		if showHeader {
			lines = append(lines, "") // Blank line before new group
			lines = append(lines, r.styleAssistantHeader(msg.Timestamp))
		}

		// Tool calls first (they usually precede text in Claude's responses)
		for _, tool := range msg.ToolCalls {
			lines = append(lines, r.renderToolCall(tool))
		}

		// Text content (with gap after tool calls)
		if msg.TextPreview != "" {
			if len(msg.ToolCalls) > 0 {
				lines = append(lines, "") // Gap between tools and text
			}
			lines = append(lines, r.wrapText(msg.TextPreview, r.width-4, "    ")...)
		}
	}

	return lines
}

func (r *Renderer) styleUserHeader(ts time.Time) string {
	timeStr := ts.Format("15:04:05")
	return fmt.Sprintf("\033[1;34m▶ You\033[0m \033[90m%s\033[0m", timeStr)
}

func (r *Renderer) styleAssistantHeader(ts time.Time) string {
	timeStr := ts.Format("15:04:05")
	return fmt.Sprintf("\033[1;32m◀ Claude\033[0m \033[90m%s\033[0m", timeStr)
}

func (r *Renderer) renderToolCall(tool ToolCall) string {
	icon := r.toolIcon(tool.Name)
	statusIcon := r.statusIcon(tool.Status)

	summary := tool.InputSummary
	if summary == "" {
		summary = tool.Name
	}

	// Truncate if too long
	maxLen := r.width - 10
	if len(summary) > maxLen {
		summary = summary[:maxLen-3] + "..."
	}

	return fmt.Sprintf("    %s %s %s", statusIcon, icon, summary)
}

func (r *Renderer) toolIcon(name string) string {
	switch name {
	case "Bash":
		return "\033[33m$\033[0m"
	case "Read":
		return "\033[36m📖\033[0m"
	case "Edit":
		return "\033[35m✏\033[0m"
	case "Write":
		return "\033[35m📝\033[0m"
	case "Glob":
		return "\033[36m🔍\033[0m"
	case "Grep":
		return "\033[36m🔎\033[0m"
	case "Task":
		return "\033[33m🤖\033[0m"
	default:
		return "\033[90m⚙\033[0m"
	}
}

func (r *Renderer) statusIcon(status ToolStatus) string {
	switch status {
	case ToolPending:
		return "\033[90m○\033[0m"
	case ToolRunning:
		return "\033[33m◐\033[0m"
	case ToolComplete:
		return "\033[32m●\033[0m"
	case ToolFailed:
		return "\033[31m✗\033[0m"
	default:
		return "\033[90m○\033[0m"
	}
}

func (r *Renderer) renderActivityLine(session *Session) string {
	switch session.Status {
	case StatusIdle:
		return "\033[90m─── Ready for input ───\033[0m"

	case StatusThinking:
		return "\033[33m⠋ Thinking...\033[0m"

	case StatusTool:
		if session.CurrentTool != nil {
			return fmt.Sprintf("\033[33m⠋ Running %s...\033[0m", session.CurrentTool.Name)
		}
		return "\033[33m⠋ Working...\033[0m"

	case StatusNeedsInput:
		return r.renderPermissionPrompt(session)

	case StatusActive:
		return "\033[32m● Active\033[0m"

	default:
		return ""
	}
}

func (r *Renderer) renderPermissionPrompt(session *Session) string {
	if session.PendingPermission == nil {
		return "\033[1;33m⚠ Input needed\033[0m"
	}

	perm := session.PendingPermission
	var lines []string

	// Header with message or tool name
	header := fmt.Sprintf("\033[1;33m⚠ Permission needed: %s\033[0m", perm.ToolName)
	if perm.Message != "" {
		header = fmt.Sprintf("\033[1;33m⚠ %s\033[0m", perm.Message)
	}
	lines = append(lines, header)

	// Show what's being requested
	detail := SummarizeToolInput(perm.ToolName, perm.ToolInput)
	if detail != "" && detail != perm.ToolName {
		// Truncate if too long
		maxLen := r.width - 6
		if len(detail) > maxLen && maxLen > 3 {
			detail = detail[:maxLen-3] + "..."
		}
		lines = append(lines, fmt.Sprintf("    \033[90m%s\033[0m", detail))
	}

	// Keybinding hints (Claude uses 1/2/3 not y/n/a)
	lines = append(lines, "\033[36m    [1] Yes  [2] Always  [3] No\033[0m")

	return strings.Join(lines, "\n")
}

func (r *Renderer) renderStatusBar(session *Session) string {
	left := fmt.Sprintf(" %s ", session.TmuxSession)
	right := fmt.Sprintf(" %s ", session.Status)

	// Pad middle
	middle := r.width - len(left) - len(right)
	if middle < 0 {
		middle = 0
	}

	return fmt.Sprintf("\033[7m%s%s%s\033[0m", left, strings.Repeat(" ", middle), right)
}

func (r *Renderer) wrapText(text string, width int, prefix string) []string {
	if width <= 0 {
		width = 80
	}

	var lines []string
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, prefix)
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, prefix)
			continue
		}

		currentLine := prefix + words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) > width {
				lines = append(lines, currentLine)
				currentLine = prefix + word
			} else {
				currentLine += " " + word
			}
		}
		lines = append(lines, currentLine)
	}

	return lines
}

func (r *Renderer) centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}
