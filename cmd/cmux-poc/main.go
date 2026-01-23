// Package main provides the entry point for the new pane-centric cmux.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/abdullathedruid/cmux/internal/app"
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: cmux-poc <session-name> [session-name...]")
		os.Exit(1)
	}

	sessions := flag.Args()

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
