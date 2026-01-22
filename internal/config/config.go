// Package config handles application configuration.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds application configuration.
type Config struct {
	// DataDir is the directory for persistent data (notes, etc.)
	DataDir string `yaml:"-"`

	// SessionPrefix is prepended to tmux session names (empty by default)
	SessionPrefix string `yaml:"session_prefix"`

	// ClaudeCommand is the command to run Claude Code
	ClaudeCommand string `yaml:"claude_command"`

	// DefaultShell is the shell to use when Claude exits
	DefaultShell string `yaml:"default_shell"`

	// WorktreeDir is the subdirectory name for worktrees inside repos
	WorktreeDir string `yaml:"worktree_dir"`

	// RefreshInterval is how often to refresh session state (in seconds)
	RefreshInterval int `yaml:"refresh_interval"`

	// Keys contains keybinding configuration
	Keys KeyBindings `yaml:"keys"`

	// Theme contains theme/appearance configuration
	Theme Theme `yaml:"theme"`
}

// KeyBindings holds all configurable keybindings.
type KeyBindings struct {
	Quit       string `yaml:"quit"`
	ToggleView string `yaml:"toggle_view"`
	Help       string `yaml:"help"`
	Search     string `yaml:"search"`
	Worktree   string `yaml:"worktree"`
	EditNote   string `yaml:"edit_note"`
	NewWizard  string `yaml:"new_wizard"`
	NavDown    string `yaml:"nav_down"`
	NavUp      string `yaml:"nav_up"`
	NavLeft    string `yaml:"nav_left"`
	NavRight   string `yaml:"nav_right"`
	Popup      string `yaml:"popup"`
	NewSession string `yaml:"new_session"`
	Delete     string `yaml:"delete"`
	Refresh    string `yaml:"refresh"`
	Diff       string `yaml:"diff"`
}

// Theme holds theme configuration.
type Theme struct {
	Colors ThemeColors            `yaml:"colors"`
	Status map[string]StatusStyle `yaml:"status"`
}

// ThemeColors holds color configuration.
type ThemeColors struct {
	SelectionBg string `yaml:"selection_bg"`
	SelectionFg string `yaml:"selection_fg"`
	StatusBarBg string `yaml:"statusbar_bg"`
	StatusBarFg string `yaml:"statusbar_fg"`
}

// StatusStyle holds style configuration for a status type.
type StatusStyle struct {
	Icon  string `yaml:"icon"`
	Color string `yaml:"color"`
	Label string `yaml:"label"`
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		DataDir:         defaultDataDir(),
		SessionPrefix:   "",
		ClaudeCommand:   "claude",
		DefaultShell:    getDefaultShell(),
		WorktreeDir:     ".worktrees",
		RefreshInterval: 2,
		Keys:            DefaultKeyBindings(),
		Theme:           DefaultTheme(),
	}
}

// DefaultKeyBindings returns the default keybindings.
func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
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
}

// DefaultTheme returns the default theme configuration.
func DefaultTheme() Theme {
	return Theme{
		Colors: ThemeColors{
			SelectionBg: "blue",
			SelectionFg: "white",
			StatusBarBg: "blue",
			StatusBarFg: "white",
		},
		Status: map[string]StatusStyle{
			"attached": {
				Icon:  "\u25cf", // ●
				Color: "green",
				Label: "ATTACHED",
			},
			"active": {
				Icon:  "\u25d0", // ◐
				Color: "yellow",
				Label: "ACTIVE",
			},
			"tool": {
				Icon:  "\u2699", // ⚙
				Color: "cyan",
				Label: "TOOL",
			},
			"thinking": {
				Icon:  "\u25d1", // ◑
				Color: "yellow",
				Label: "THINKING",
			},
			"input": {
				Icon:  "\U0001F514", // 🔔
				Color: "magenta",
				Label: "INPUT",
			},
			"stopped": {
				Icon:  "\u2713", // ✓
				Color: "green",
				Label: "DONE",
			},
			"idle": {
				Icon:  "\u25cb", // ○
				Color: "white",
				Label: "IDLE",
			},
		},
	}
}

// Load loads configuration from the config file, falling back to defaults.
func Load() (*Config, error) {
	cfg := Default()

	configPath := cfg.ConfigFile()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, use defaults
			return cfg, nil
		}
		return nil, err
	}

	// Parse YAML into a temporary struct to merge with defaults
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return nil, err
	}

	// Merge file config with defaults (file values override defaults)
	mergeConfig(cfg, &fileCfg)

	// Validate keybindings
	if err := ValidateKeys(&cfg.Keys); err != nil {
		return nil, err
	}

	return cfg, nil
}

// mergeConfig merges file configuration into the default configuration.
// Only non-zero values from file are applied.
func mergeConfig(dst, src *Config) {
	if src.SessionPrefix != "" {
		dst.SessionPrefix = src.SessionPrefix
	}
	if src.ClaudeCommand != "" {
		dst.ClaudeCommand = src.ClaudeCommand
	}
	if src.DefaultShell != "" {
		dst.DefaultShell = src.DefaultShell
	}
	if src.WorktreeDir != "" {
		dst.WorktreeDir = src.WorktreeDir
	}
	if src.RefreshInterval != 0 {
		dst.RefreshInterval = src.RefreshInterval
	}

	// Merge keybindings
	mergeKeyBindings(&dst.Keys, &src.Keys)

	// Merge theme
	mergeTheme(&dst.Theme, &src.Theme)
}

// mergeKeyBindings merges keybindings from src into dst.
func mergeKeyBindings(dst, src *KeyBindings) {
	if src.Quit != "" {
		dst.Quit = src.Quit
	}
	if src.ToggleView != "" {
		dst.ToggleView = src.ToggleView
	}
	if src.Help != "" {
		dst.Help = src.Help
	}
	if src.Search != "" {
		dst.Search = src.Search
	}
	if src.Worktree != "" {
		dst.Worktree = src.Worktree
	}
	if src.EditNote != "" {
		dst.EditNote = src.EditNote
	}
	if src.NewWizard != "" {
		dst.NewWizard = src.NewWizard
	}
	if src.NavDown != "" {
		dst.NavDown = src.NavDown
	}
	if src.NavUp != "" {
		dst.NavUp = src.NavUp
	}
	if src.NavLeft != "" {
		dst.NavLeft = src.NavLeft
	}
	if src.NavRight != "" {
		dst.NavRight = src.NavRight
	}
	if src.Popup != "" {
		dst.Popup = src.Popup
	}
	if src.NewSession != "" {
		dst.NewSession = src.NewSession
	}
	if src.Delete != "" {
		dst.Delete = src.Delete
	}
	if src.Refresh != "" {
		dst.Refresh = src.Refresh
	}
	if src.Diff != "" {
		dst.Diff = src.Diff
	}
}

// mergeTheme merges theme configuration from src into dst.
func mergeTheme(dst, src *Theme) {
	// Merge colors
	if src.Colors.SelectionBg != "" {
		dst.Colors.SelectionBg = src.Colors.SelectionBg
	}
	if src.Colors.SelectionFg != "" {
		dst.Colors.SelectionFg = src.Colors.SelectionFg
	}
	if src.Colors.StatusBarBg != "" {
		dst.Colors.StatusBarBg = src.Colors.StatusBarBg
	}
	if src.Colors.StatusBarFg != "" {
		dst.Colors.StatusBarFg = src.Colors.StatusBarFg
	}

	// Merge status styles
	if src.Status != nil {
		for key, style := range src.Status {
			if existing, ok := dst.Status[key]; ok {
				// Merge individual fields
				if style.Icon != "" {
					existing.Icon = style.Icon
				}
				if style.Color != "" {
					existing.Color = style.Color
				}
				if style.Label != "" {
					existing.Label = style.Label
				}
				dst.Status[key] = existing
			} else {
				// New status type, add it
				dst.Status[key] = style
			}
		}
	}
}

// defaultDataDir returns the default data directory.
func defaultDataDir() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "cmux")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cmux"
	}
	return filepath.Join(home, ".config", "cmux")
}

// getDefaultShell returns the user's default shell.
func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/bash"
}

// NotesFile returns the path to the notes file.
func (c *Config) NotesFile() string {
	return filepath.Join(c.DataDir, "notes.json")
}

// ConfigFile returns the path to the config file.
func (c *Config) ConfigFile() string {
	return filepath.Join(c.DataDir, "config.yaml")
}

// EnsureDataDir creates the data directory if it doesn't exist.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0755)
}
