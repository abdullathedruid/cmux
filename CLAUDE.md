# CMUX

Terminal multiplexer with vim-like modal keybindings for managing tmux sessions.

## Build & Run

```bash
go build -o cmux ./cmd/cmux
cmux session1 session2 ...
```

## Architecture

Pane-centric, modal-aware, event-driven. Uses gocui for TUI, midterm for terminal emulation.

**Modes**: Normal (navigation) → Terminal (pass-through to tmux) → Input (text capture)

## Directory Structure

```
cmd/cmux/           Entry point
internal/
  app/              PocApp orchestration, keybindings
  pane/             Pane, Manager, Layout (responsive grid), SafeTerminal
  terminal/         Tmux control mode (-CC) PTY management
  input/            Modal input handling (Normal/Terminal/Input)
  config/           YAML config (~/.config/cmux/config.yaml), key parsing
  ui/               Theme, colors, rendering
  git/              Worktree operations, repo detection
  controller/       Higher-level controllers (dashboard, wizard, etc.)
  state/            Session state
  tmux/             Tmux wrappers
```

## Key Patterns

- **Thread safety**: RWMutex on Manager, Handler, SafeTerminal, ControlMode
- **Layout**: Responsive grid (1=full, 2=side-by-side, 3+=rows of 2)
- **Config merging**: Defaults → YAML overrides non-zero values

## Default Keybindings

| Key | Action |
|-----|--------|
| h/j/k/l | Navigation |
| 1-9 | Jump to pane |
| i/Enter | Terminal mode |
| Ctrl+Q | Exit terminal mode |
| N | New worktree (input mode) |
| q | Quit |

## Dependencies

- gocui: TUI framework
- midterm: VT100 terminal emulator
- creack/pty: PTY support
