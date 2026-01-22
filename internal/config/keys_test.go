package config

import (
	"testing"

	"github.com/jesseduffield/gocui"
)

func TestParseKey_SingleChar(t *testing.T) {
	tests := []struct {
		input    string
		wantRune rune
	}{
		{"q", 'q'},
		{"v", 'v'},
		{"?", '?'},
		{"/", '/'},
	}

	for _, tt := range tests {
		key, err := ParseKeyPreserveCase(tt.input)
		if err != nil {
			t.Errorf("ParseKeyPreserveCase(%q) error = %v", tt.input, err)
			continue
		}
		if !key.IsRune() {
			t.Errorf("ParseKeyPreserveCase(%q) expected rune, got special key", tt.input)
			continue
		}
		if key.Rune() != tt.wantRune {
			t.Errorf("ParseKeyPreserveCase(%q) = %q, want %q", tt.input, key.Rune(), tt.wantRune)
		}
	}
}

func TestParseKey_UppercasePreserved(t *testing.T) {
	key, err := ParseKeyPreserveCase("N")
	if err != nil {
		t.Fatalf("ParseKeyPreserveCase(N) error = %v", err)
	}
	if !key.IsRune() {
		t.Fatal("ParseKeyPreserveCase(N) expected rune, got special key")
	}
	if key.Rune() != 'N' {
		t.Errorf("ParseKeyPreserveCase(N) = %q, want 'N'", key.Rune())
	}
}

func TestParseKey_SpecialKeys(t *testing.T) {
	tests := []struct {
		input   string
		wantKey gocui.Key
	}{
		{"enter", gocui.KeyEnter},
		{"space", gocui.KeySpace},
		{"esc", gocui.KeyEsc},
		{"escape", gocui.KeyEsc},
		{"tab", gocui.KeyTab},
		{"backspace", gocui.KeyBackspace2},
		{"up", gocui.KeyArrowUp},
		{"down", gocui.KeyArrowDown},
		{"left", gocui.KeyArrowLeft},
		{"right", gocui.KeyArrowRight},
	}

	for _, tt := range tests {
		key, err := ParseKeyPreserveCase(tt.input)
		if err != nil {
			t.Errorf("ParseKeyPreserveCase(%q) error = %v", tt.input, err)
			continue
		}
		if key.IsRune() {
			t.Errorf("ParseKeyPreserveCase(%q) expected special key, got rune", tt.input)
			continue
		}
		if key.GocuiKey() != tt.wantKey {
			t.Errorf("ParseKeyPreserveCase(%q) = %v, want %v", tt.input, key.GocuiKey(), tt.wantKey)
		}
	}
}

func TestParseKey_CtrlKeys(t *testing.T) {
	tests := []struct {
		input   string
		wantKey gocui.Key
	}{
		{"ctrl+c", gocui.KeyCtrlC},
		{"ctrl+s", gocui.KeyCtrlS},
		{"Ctrl+A", gocui.KeyCtrlA},
	}

	for _, tt := range tests {
		key, err := ParseKeyPreserveCase(tt.input)
		if err != nil {
			t.Errorf("ParseKeyPreserveCase(%q) error = %v", tt.input, err)
			continue
		}
		if key.IsRune() {
			t.Errorf("ParseKeyPreserveCase(%q) expected ctrl key, got rune", tt.input)
			continue
		}
		if key.GocuiKey() != tt.wantKey {
			t.Errorf("ParseKeyPreserveCase(%q) = %v, want %v", tt.input, key.GocuiKey(), tt.wantKey)
		}
	}
}

func TestParseKey_Invalid(t *testing.T) {
	tests := []string{
		"",
		"ctrl+",
		"invalid_key",
		"ab",
	}

	for _, input := range tests {
		_, err := ParseKeyPreserveCase(input)
		if err == nil {
			t.Errorf("ParseKeyPreserveCase(%q) expected error, got nil", input)
		}
	}
}

func TestValidateKeys_NoDuplicates(t *testing.T) {
	keys := &KeyBindings{
		Quit:       "q",
		ToggleView: "v",
		Help:       "?",
		Search:     "/",
		Worktree:   "w",
		EditNote:   "e",
		NewWizard:  "N",
		NavDown:    "j",
		NavUp:      "k",
		NavLeft:    "h",
		NavRight:   "l",
		Popup:      "p",
		NewSession: "n",
		Delete:     "x",
		Refresh:    "r",
		Diff:       "d",
	}

	if err := ValidateKeys(keys); err != nil {
		t.Errorf("ValidateKeys() error = %v, want nil", err)
	}
}

func TestValidateKeys_WithDuplicates(t *testing.T) {
	keys := &KeyBindings{
		Quit:       "q",
		ToggleView: "q", // duplicate
		Help:       "?",
	}

	err := ValidateKeys(keys)
	if err == nil {
		t.Error("ValidateKeys() expected error for duplicate keys, got nil")
	}
}
