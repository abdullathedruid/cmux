package ui

import (
	"strings"
	"testing"

	"github.com/abdullathedruid/cmux/internal/state"
)

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		attached bool
		status   state.SessionStatus
		want     string
	}{
		{true, state.StatusIdle, "●"},
		{false, state.StatusIdle, "○"},
		{false, state.StatusActive, "◐"},
		{false, state.StatusTool, "⚙"},
		{false, state.StatusThinking, "◑"},
	}

	for _, tt := range tests {
		got := StatusIcon(tt.attached, tt.status)
		if got != tt.want {
			t.Errorf("StatusIcon(%v, %v) = %q, want %q", tt.attached, tt.status, got, tt.want)
		}
	}
}

func TestStatusText(t *testing.T) {
	tests := []struct {
		attached bool
		status   state.SessionStatus
		want     string
	}{
		{true, state.StatusIdle, "ATTACHED"},
		{false, state.StatusIdle, "IDLE"},
		{false, state.StatusActive, "ACTIVE"},
		{false, state.StatusTool, "TOOL"},
		{false, state.StatusThinking, "THINKING"},
	}

	for _, tt := range tests {
		got := StatusText(tt.attached, tt.status)
		if got != tt.want {
			t.Errorf("StatusText(%v, %v) = %q, want %q", tt.attached, tt.status, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"ab", 3, "ab"},
		{"abcd", 3, "abc"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"test", 8, "test    "},
		{"test", 4, "test"},
		{"test", 2, "te"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		got := PadRight(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("PadRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"test", 8, "    test"},
		{"test", 4, "test"},
		{"test", 2, "te"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		got := PadLeft(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("PadLeft(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"test", 8, "  test  "},
		{"test", 4, "test"},
		{"ab", 5, " ab  "},
		{"", 4, "    "},
	}

	for _, tt := range tests {
		got := Center(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("Center(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestRenderStatusBar(t *testing.T) {
	result := RenderStatusBar(5, 2, true)

	if !strings.Contains(result, "5 sessions") {
		t.Error("status bar should contain session count")
	}
	if !strings.Contains(result, "2 attached") {
		t.Error("status bar should contain attached count")
	}
	if !strings.Contains(result, "3 idle") {
		t.Error("status bar should contain idle count")
	}
	if !strings.Contains(result, "dashboard") {
		t.Error("status bar should mention dashboard view")
	}

	result = RenderStatusBar(3, 0, false)
	if !strings.Contains(result, "list") {
		t.Error("status bar should mention list view")
	}
}

func TestCardRender(t *testing.T) {
	card := &Card{
		Title:    "test-card",
		Status:   "IDLE",
		Icon:     "○",
		Path:     "~/code/test",
		Note:     "Test note",
		Width:    30,
		Selected: false,
	}

	lines := card.Render()

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}

	// Check title is in first line
	if !strings.Contains(lines[0], "test-card") {
		t.Error("first line should contain title")
	}

	// Check status line
	if !strings.Contains(lines[1], "IDLE") {
		t.Error("second line should contain status")
	}

	// Check path line
	if !strings.Contains(lines[2], "~/code/test") {
		t.Error("third line should contain path")
	}
}

func TestCardRenderSelected(t *testing.T) {
	card := &Card{
		Title:    "selected",
		Status:   "ATTACHED",
		Icon:     "●",
		Path:     "/test",
		Width:    25,
		Selected: true,
	}

	lines := card.Render()

	// Selected cards use bold borders
	if !strings.Contains(lines[0], "┏") {
		t.Error("selected card should use bold top-left corner")
	}
	if !strings.Contains(lines[0], "┓") {
		t.Error("selected card should use bold top-right corner")
	}
}

func TestHelpText(t *testing.T) {
	text := HelpText()

	// Check for key sections
	if !strings.Contains(text, "Navigation") {
		t.Error("help text should contain Navigation section")
	}
	if !strings.Contains(text, "Session Management") {
		t.Error("help text should contain Session Management section")
	}
	if !strings.Contains(text, "h/j/k/l") {
		t.Error("help text should mention vim keys")
	}
	if !strings.Contains(text, "Enter") {
		t.Error("help text should mention Enter key")
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  int // expected minimum number of lines
	}{
		{"short", 10, 1},
		{"this is a longer text that should wrap", 10, 4}, // At least 4 lines
		{"line1\nline2\nline3", 100, 3},
		{"", 10, 1},
	}

	for _, tt := range tests {
		got := WrapText(tt.text, tt.width)
		if len(got) < tt.want {
			t.Errorf("WrapText(%q, %d) = %d lines, want at least %d", tt.text, tt.width, len(got), tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{30, "30s ago"},
		{90, "1m ago"},
		{3600, "1h ago"},
		{7200, "2h ago"},
		{86400, "1d ago"},
		{172800, "2d ago"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.seconds)
		if got != tt.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestStatusColor(t *testing.T) {
	// Just verify it returns something for each case
	tests := []struct {
		attached bool
		status   state.SessionStatus
	}{
		{true, state.StatusIdle},
		{false, state.StatusIdle},
		{false, state.StatusActive},
		{false, state.StatusTool},
		{false, state.StatusThinking},
	}

	for _, tt := range tests {
		got := StatusColor(tt.attached, tt.status)
		if got == "" {
			t.Errorf("StatusColor(%v, %v) returned empty string", tt.attached, tt.status)
		}
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		a, b int
		want int
	}{
		{1, 2, 2},
		{5, 3, 5},
		{0, 0, 0},
		{-1, -2, -1},
	}

	for _, tt := range tests {
		got := max(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b int
		want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, -2, -2},
	}

	for _, tt := range tests {
		got := min(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
