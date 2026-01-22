package config

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/gocui"
)

// Key represents a parsed key binding.
type Key struct {
	Value any // rune for single chars, gocui.Key for special keys
	Mod   gocui.Modifier
}

// ParseKey parses a key string into a gocui-compatible key value.
// Supported formats:
//   - Single character: "q", "v", "?", "/"
//   - Special keys: "enter", "space", "esc", "tab", "backspace"
//   - Arrow keys: "up", "down", "left", "right"
//   - Ctrl combinations: "ctrl+c", "ctrl+s"
//   - Uppercase for shift: "N" (shift+n)
func ParseKey(s string) (Key, error) {
	if s == "" {
		return Key{}, fmt.Errorf("empty key string")
	}

	s = strings.ToLower(strings.TrimSpace(s))

	// Check for ctrl combinations
	if char, found := strings.CutPrefix(s, "ctrl+"); found {
		if len(char) == 1 {
			ctrlKey, ok := ctrlKeyMap[char]
			if ok {
				return Key{Value: ctrlKey, Mod: gocui.ModNone}, nil
			}
		}
		return Key{}, fmt.Errorf("invalid ctrl combination: %s", s)
	}

	// Check for special keys
	if key, ok := specialKeyMap[s]; ok {
		return Key{Value: key, Mod: gocui.ModNone}, nil
	}

	// Single character (preserve original case from input)
	original := strings.TrimSpace(s)
	if len(original) == 1 {
		return Key{Value: rune(original[0]), Mod: gocui.ModNone}, nil
	}

	return Key{}, fmt.Errorf("unknown key: %s", s)
}

// ParseKeyPreserveCase parses a key string preserving the original case.
// This is needed because uppercase letters like "N" should remain uppercase.
func ParseKeyPreserveCase(s string) (Key, error) {
	if s == "" {
		return Key{}, fmt.Errorf("empty key string")
	}

	trimmed := strings.TrimSpace(s)
	lower := strings.ToLower(trimmed)

	// Check for ctrl combinations
	if char, found := strings.CutPrefix(lower, "ctrl+"); found {
		if len(char) == 1 {
			ctrlKey, ok := ctrlKeyMap[char]
			if ok {
				return Key{Value: ctrlKey, Mod: gocui.ModNone}, nil
			}
		}
		return Key{}, fmt.Errorf("invalid ctrl combination: %s", s)
	}

	// Check for special keys (case insensitive)
	if key, ok := specialKeyMap[lower]; ok {
		return Key{Value: key, Mod: gocui.ModNone}, nil
	}

	// Single character (preserve original case)
	if len(trimmed) == 1 {
		return Key{Value: rune(trimmed[0]), Mod: gocui.ModNone}, nil
	}

	return Key{}, fmt.Errorf("unknown key: %s", s)
}

// IsRune returns true if the key is a rune (single character).
func (k Key) IsRune() bool {
	_, ok := k.Value.(rune)
	return ok
}

// Rune returns the key as a rune, or 0 if not a rune.
func (k Key) Rune() rune {
	if r, ok := k.Value.(rune); ok {
		return r
	}
	return 0
}

// GocuiKey returns the key as a gocui.Key, or 0 if not a special key.
func (k Key) GocuiKey() gocui.Key {
	if key, ok := k.Value.(gocui.Key); ok {
		return key
	}
	return 0
}

// specialKeyMap maps string names to gocui special keys.
var specialKeyMap = map[string]gocui.Key{
	"enter":     gocui.KeyEnter,
	"space":     gocui.KeySpace,
	"esc":       gocui.KeyEsc,
	"escape":    gocui.KeyEsc,
	"tab":       gocui.KeyTab,
	"backspace": gocui.KeyBackspace2,
	"delete":    gocui.KeyDelete,
	"insert":    gocui.KeyInsert,
	"home":      gocui.KeyHome,
	"end":       gocui.KeyEnd,
	"pgup":      gocui.KeyPgup,
	"pageup":    gocui.KeyPgup,
	"pgdn":      gocui.KeyPgdn,
	"pagedown":  gocui.KeyPgdn,
	"up":        gocui.KeyArrowUp,
	"down":      gocui.KeyArrowDown,
	"left":      gocui.KeyArrowLeft,
	"right":     gocui.KeyArrowRight,
	"f1":        gocui.KeyF1,
	"f2":        gocui.KeyF2,
	"f3":        gocui.KeyF3,
	"f4":        gocui.KeyF4,
	"f5":        gocui.KeyF5,
	"f6":        gocui.KeyF6,
	"f7":        gocui.KeyF7,
	"f8":        gocui.KeyF8,
	"f9":        gocui.KeyF9,
	"f10":       gocui.KeyF10,
	"f11":       gocui.KeyF11,
	"f12":       gocui.KeyF12,
}

// ctrlKeyMap maps single characters to their ctrl+key equivalents.
var ctrlKeyMap = map[string]gocui.Key{
	"a": gocui.KeyCtrlA,
	"b": gocui.KeyCtrlB,
	"c": gocui.KeyCtrlC,
	"d": gocui.KeyCtrlD,
	"e": gocui.KeyCtrlE,
	"f": gocui.KeyCtrlF,
	"g": gocui.KeyCtrlG,
	"h": gocui.KeyCtrlH,
	"i": gocui.KeyCtrlI,
	"j": gocui.KeyCtrlJ,
	"k": gocui.KeyCtrlK,
	"l": gocui.KeyCtrlL,
	"m": gocui.KeyCtrlM,
	"n": gocui.KeyCtrlN,
	"o": gocui.KeyCtrlO,
	"p": gocui.KeyCtrlP,
	"q": gocui.KeyCtrlQ,
	"r": gocui.KeyCtrlR,
	"s": gocui.KeyCtrlS,
	"t": gocui.KeyCtrlT,
	"u": gocui.KeyCtrlU,
	"v": gocui.KeyCtrlV,
	"w": gocui.KeyCtrlW,
	"x": gocui.KeyCtrlX,
	"y": gocui.KeyCtrlY,
	"z": gocui.KeyCtrlZ,
}

// KeyToString converts a Key back to its string representation.
func KeyToString(k Key) string {
	if k.IsRune() {
		return string(k.Rune())
	}

	gKey := k.GocuiKey()

	// Check special keys
	for name, key := range specialKeyMap {
		if key == gKey {
			return name
		}
	}

	// Check ctrl keys
	for char, key := range ctrlKeyMap {
		if key == gKey {
			return "ctrl+" + char
		}
	}

	return ""
}
