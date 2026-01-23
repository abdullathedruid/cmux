package input

import "testing"

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "NORMAL"},
		{ModeTerminal, "TERMINAL"},
		{ModeInput, "INPUT"},
		{Mode(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestMode_Predicates(t *testing.T) {
	if !ModeNormal.IsNormal() {
		t.Error("ModeNormal.IsNormal() should be true")
	}
	if !ModeTerminal.IsTerminal() {
		t.Error("ModeTerminal.IsTerminal() should be true")
	}
	if !ModeInput.IsInput() {
		t.Error("ModeInput.IsInput() should be true")
	}

	// Cross-check
	if ModeNormal.IsTerminal() {
		t.Error("ModeNormal.IsTerminal() should be false")
	}
	if ModeTerminal.IsNormal() {
		t.Error("ModeTerminal.IsNormal() should be false")
	}
}
