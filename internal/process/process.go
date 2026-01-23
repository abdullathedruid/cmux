// Package process provides cross-platform process inspection utilities.
package process

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Info contains information about a running process.
type Info struct {
	PID     int
	Command string // Full command line with arguments
}

// GetCommandLine returns the full command line for a process.
// Works on macOS and Linux using POSIX-compatible ps flags.
func GetCommandLine(pid int) (string, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ps command failed: %w: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetChildProcesses returns all direct child processes of the given PID.
// Works on macOS and Linux using POSIX ps.
func GetChildProcesses(pid int) ([]Info, error) {
	// Use ps to list all processes with their PPID, then filter
	// Format: PID PPID COMMAND (tab-separated for reliable parsing)
	cmd := exec.Command("ps", "-eo", "pid=,ppid=,args=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ps failed: %w: %s", err, stderr.String())
	}

	parentPID := strconv.Itoa(pid)
	var children []Info

	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse: "  PID  PPID COMMAND..."
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		ppid := fields[1]
		if ppid != parentPID {
			continue
		}

		childPID, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		// Reconstruct command from remaining fields
		cmdLine := strings.Join(fields[2:], " ")

		children = append(children, Info{
			PID:     childPID,
			Command: cmdLine,
		})
	}

	return children, nil
}

// GetActiveApp returns the "active" application running in a shell process.
// It looks at child processes of the shell to find what's actually running.
// Returns the command name (first word) and full command line.
func GetActiveApp(shellPID int) (name string, cmdLine string, err error) {
	children, err := GetChildProcesses(shellPID)
	if err != nil {
		return "", "", err
	}

	// No children means the shell itself is active (idle prompt)
	if len(children) == 0 {
		cmdLine, err := GetCommandLine(shellPID)
		if err != nil {
			return "", "", err
		}
		return extractCommandName(cmdLine), cmdLine, nil
	}

	// Return the first child (usually what the user ran)
	// For more complex cases with pipelines, we could return all children
	child := children[0]
	return extractCommandName(child.Command), child.Command, nil
}

// extractCommandName extracts the command name from a full command line.
// Handles paths like "/usr/local/bin/node" -> "node"
func extractCommandName(cmdLine string) string {
	// Split on whitespace to get the command (first word)
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return ""
	}

	cmd := parts[0]

	// Extract basename from path
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		cmd = cmd[idx+1:]
	}

	return cmd
}
