// Package terminal provides tmux control mode connection management.
package terminal

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"

	"github.com/creack/pty"
)

// outputPattern matches "%output %<pane-id> <data>" lines.
// The line may be prefixed with DCS escape sequences like \033P1000p
var outputPattern = regexp.MustCompile(`%output %(\d+) (.*)$`)

// ControlMode manages a tmux -CC (control mode) connection.
type ControlMode struct {
	session  string
	cmd      *exec.Cmd
	pty      *os.File
	outputCh chan []byte
	doneCh   chan struct{}
	mu       sync.Mutex
}

// NewControlMode creates a new control mode connection for the given session.
func NewControlMode(session string) *ControlMode {
	return &ControlMode{
		session:  session,
		outputCh: make(chan []byte, 100),
		doneCh:   make(chan struct{}),
	}
}

// Start begins the tmux control mode connection with the given dimensions.
func (c *ControlMode) Start(width, height int) error {
	c.cmd = exec.Command("tmux", "-CC", "attach-session", "-t", c.session)

	// Start with a PTY (tmux needs a real terminal)
	var err error
	c.pty, err = pty.Start(c.cmd)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	// Set the PTY size
	pty.Setsize(c.pty, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})

	// Monitor process exit
	go func() {
		c.cmd.Wait()
	}()

	// Start reading output
	go c.readOutput()

	// Force a full redraw by resizing to a different size first, then back
	// This tricks tmux into thinking the terminal changed and needs a full redraw
	c.Resize(width-1, height-1)
	c.Resize(width, height)

	return nil
}

// Resize tells tmux about the new window size.
func (c *ControlMode) Resize(width, height int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Resize the PTY
	pty.Setsize(c.pty, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})

	// Tell tmux to refresh with the new size
	cmd := fmt.Sprintf("refresh-client -C %d,%d\n", width, height)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// readOutput reads lines from tmux control mode and parses them.
func (c *ControlMode) readOutput() {
	defer close(c.outputCh)

	scanner := bufio.NewScanner(c.pty)
	// Increase buffer size for large outputs
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse %output lines
		if data, ok := parseOutputLine(line); ok {
			select {
			case c.outputCh <- data:
			case <-c.doneCh:
				return
			}
		}
	}
}

// parseOutputLine parses a "%output %N <data>" line and returns decoded data.
func parseOutputLine(line string) ([]byte, bool) {
	matches := outputPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}

	// matches[2] contains the octal-encoded data
	data := decodeOctal(matches[2])
	return data, true
}

// decodeOctal converts \NNN octal sequences to bytes.
func decodeOctal(s string) []byte {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+3 < len(s) {
			// Check if next 3 chars are octal digits
			if isOctalDigit(s[i+1]) && isOctalDigit(s[i+2]) && isOctalDigit(s[i+3]) {
				val, _ := strconv.ParseInt(s[i+1:i+4], 8, 32)
				result = append(result, byte(val))
				i += 4
				continue
			}
			// Handle \\ (escaped backslash)
			if i+1 < len(s) && s[i+1] == '\\' {
				result = append(result, '\\')
				i += 2
				continue
			}
		}
		result = append(result, s[i])
		i++
	}
	return result
}

func isOctalDigit(b byte) bool {
	return b >= '0' && b <= '7'
}

// SendKeys sends tmux key commands (e.g., "Enter", "C-c", "Up").
func (c *ControlMode) SendKeys(keys string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := fmt.Sprintf("send-keys -t %s %s\n", c.session, keys)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// SendLiteralKeys sends literal key input to tmux (quoted).
func (c *ControlMode) SendLiteralKeys(keys string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := fmt.Sprintf("send-keys -t %s -l %q\n", c.session, keys)
	_, err := c.pty.Write([]byte(cmd))
	return err
}

// OutputChan returns the channel that receives terminal output data.
func (c *ControlMode) OutputChan() <-chan []byte {
	return c.outputCh
}

// Session returns the tmux session name.
func (c *ControlMode) Session() string {
	return c.session
}

// Close terminates the control mode connection.
func (c *ControlMode) Close() error {
	close(c.doneCh)
	if c.pty != nil {
		c.pty.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}
