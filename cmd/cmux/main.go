// Package main provides the entry point for cmux.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/abdullathedruid/cmux/internal/app"
)

var structured = flag.Bool("structured", false, "Use structured view mode (renders from hooks/transcripts instead of terminal emulation)")

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: cmux [flags] <session-name> [session-name...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "cmux is a terminal multiplexer for managing multiple tmux sessions")
		fmt.Fprintln(os.Stderr, "with vim-like modal keybindings.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  -structured  Use structured view mode (renders Claude sessions from")
		fmt.Fprintln(os.Stderr, "               hooks/transcripts instead of terminal emulation)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Modes:")
		fmt.Fprintln(os.Stderr, "  NORMAL   - Navigate between panes (h/j/k/l, 1-9)")
		fmt.Fprintln(os.Stderr, "  TERMINAL - Send input to the active pane (i or Enter to enter)")
		fmt.Fprintln(os.Stderr, "  INPUT    - Text input for creating new worktrees (N)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Keys (Normal mode):")
		fmt.Fprintln(os.Stderr, "  h/j/k/l  - Navigate between panes")
		fmt.Fprintln(os.Stderr, "  1-9      - Jump to pane N")
		fmt.Fprintln(os.Stderr, "  i/Enter  - Enter terminal mode")
		fmt.Fprintln(os.Stderr, "  N        - Create new worktree and pane")
		fmt.Fprintln(os.Stderr, "  q        - Quit")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Keys (Terminal mode):")
		fmt.Fprintln(os.Stderr, "  Ctrl+Q   - Return to normal mode")
		os.Exit(1)
	}

	sessions := flag.Args()

	if *structured {
		// Use structured view mode (renders from hooks/transcripts)
		application, err := app.NewStructuredApp()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
			os.Exit(1)
		}

		if err := application.InitSessions(sessions); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing sessions: %v\n", err)
			os.Exit(1)
		}

		if err := application.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Use traditional terminal emulation mode
		application, err := app.NewPocApp()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
			os.Exit(1)
		}

		if err := application.InitSessions(sessions); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing sessions: %v\n", err)
			os.Exit(1)
		}

		if err := application.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}
