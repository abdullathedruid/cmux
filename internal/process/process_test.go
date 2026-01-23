package process

import (
	"os"
	"testing"
)

func TestExtractCommandName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/usr/local/bin/node", "node"},
		{"/bin/zsh", "zsh"},
		{"node", "node"},
		{"node /path/to/script.js", "node"},
		{"/usr/bin/python3 -m http.server", "python3"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		got := extractCommandName(tt.input)
		if got != tt.want {
			t.Errorf("extractCommandName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetCommandLine(t *testing.T) {
	// Test with current process - should return something containing "go"
	pid := os.Getpid()
	cmdLine, err := GetCommandLine(pid)
	if err != nil {
		t.Fatalf("GetCommandLine(%d) error: %v", pid, err)
	}
	if cmdLine == "" {
		t.Errorf("GetCommandLine(%d) returned empty string", pid)
	}
}

func TestGetCommandLineInvalidPID(t *testing.T) {
	// PID 0 or negative PIDs should fail
	_, err := GetCommandLine(-1)
	if err == nil {
		t.Error("expected error for invalid PID")
	}
}

func TestGetChildProcesses(t *testing.T) {
	// Get children of current process (likely none in test)
	pid := os.Getpid()
	children, err := GetChildProcesses(pid)
	if err != nil {
		t.Fatalf("GetChildProcesses(%d) error: %v", pid, err)
	}
	// Just verify it doesn't error - we may or may not have children
	_ = children
}

func TestGetActiveApp(t *testing.T) {
	// Test with current process
	pid := os.Getpid()
	name, cmdLine, err := GetActiveApp(pid)
	if err != nil {
		t.Fatalf("GetActiveApp(%d) error: %v", pid, err)
	}
	if name == "" {
		t.Errorf("GetActiveApp(%d) returned empty name", pid)
	}
	if cmdLine == "" {
		t.Errorf("GetActiveApp(%d) returned empty cmdLine", pid)
	}
}
