// Package main provides the entry point for cmux.
package main

import (
	"fmt"
	"os"

	"github.com/abdullathedruid/cmux/internal/app"
	"github.com/abdullathedruid/cmux/internal/config"
)

func main() {
	// Load configuration
	cfg := config.Default()

	// Ensure data directory exists
	if err := cfg.EnsureDataDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}

	// Create and run the application
	application, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting cmux: %v\n", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
