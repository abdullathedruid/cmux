// Package main provides the entry point for cmux.
package main

import (
	"fmt"
	"os"

	"github.com/abdullathedruid/cmux/internal/app"
)

func main() {
	sessions := os.Args[1:]

	application, err := app.NewStructuredApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) > 0 {
		// Direct mode: open specified sessions
		if err := application.InitSessions(sessions); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing sessions: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Discovery mode: show sidebar with discovered Claude sessions
		if err := application.InitWithDiscovery(); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing discovery mode: %v\n", err)
			os.Exit(1)
		}
	}

	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
