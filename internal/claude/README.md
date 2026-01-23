# Claude Package - Structured View Implementation

This package provides a structured view of Claude Code sessions, replacing terminal emulation with parsed hook events and transcript data.

## Simplified Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Hook Script (minimal)                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  cmux-hook.sh                                                           │
│       │                                                                 │
│       │  1. Get tmux session name                                       │
│       │  2. Add ts + tmux_session to JSON                               │
│       │  3. Append to JSONL file                                        │
│       │                                                                 │
│       └──▶ $TMPDIR/cmux/events/<tmux-session>.jsonl                    │
│                                                                         │
│            One line per event, e.g.:                                    │
│            {"hook_event_name":"PreToolUse","tool_name":"Bash",...,      │
│             "ts":"2026-01-23T21:30:00+00:00","tmux_session":"dev"}      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           cmux (Go)                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────┐         ┌─────────────────┐                       │
│  │  EventWatcher   │         │TranscriptReader │                       │
│  │                 │         │                 │                       │
│  │ • fsnotify on   │         │ • Polls JSONL   │                       │
│  │   events/*.jsonl│         │ • Skips progress│                       │
│  │ • Creates reader│         │ • Deduplicates  │                       │
│  │   per session   │         │   streaming     │                       │
│  └────────┬────────┘         └────────┬────────┘                       │
│           │                           │                                 │
│           └───────────┬───────────────┘                                 │
│                       ▼                                                 │
│              ┌───────────────┐                                          │
│              │     View      │                                          │
│              │               │                                          │
│              │ • Session     │◀─── UpdateFromHookEvent()                │
│              │ • Messages[]  │◀─── PollTranscript()                     │
│              │ • Status      │                                          │
│              │ • CurrentTool │                                          │
│              └───────┬───────┘                                          │
│                      │                                                  │
│                      ▼                                                  │
│              ┌───────────────┐                                          │
│              │   Renderer    │───▶ Formatted string for gocui           │
│              └───────────────┘                                          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Files

| File | Purpose |
|------|---------|
| `types.go` | Data structures: Session, Message, ToolCall, HookEvent |
| `events.go` | EventWatcher + EventReader (reads per-session JSONL) |
| `transcript.go` | TranscriptReader (parses Claude transcript for message text) |
| `view.go` | View (combines event + transcript data, manages state) |
| `renderer.go` | Renderer (formats session state for terminal display) |
| `integration.go` | PaneAdapter + SendKeys helpers for tmux |

## Data Flow

### Events (Real-time status, tool calls)

```
Claude Code
    │
    ▼
Hook Script ──append──▶ events/<session>.jsonl
                              │
                              │ fsnotify + poll
                              ▼
                        EventWatcher
                              │
                              │ OnEvent callback
                              ▼
                            View
```

### Transcript (Conversation text)

```
Claude Code ──write──▶ ~/.claude/projects/.../session.jsonl
                              │
                              │ poll (incremental read)
                              ▼
                       TranscriptReader
                              │
                              │ Messages()
                              ▼
                            View
```

## Hook Script

The hook is minimal - just append JSON with metadata:

```bash
#!/bin/bash
DIR="${TMPDIR:-/tmp}/cmux/events"
mkdir -p "$DIR"

SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0
[ -z "$SESSION" ] && exit 0

jq -c --arg ts "$(date -Iseconds)" --arg tmux "$SESSION" \
    '. + {ts: $ts, tmux_session: $tmux}' >> "$DIR/$SESSION.jsonl"
```

All parsing logic lives in cmux, not the hook.

## Usage Example

```go
package main

import (
    "fmt"
    "time"

    "cmux/internal/claude"
)

func main() {
    // Watch for events from all sessions
    watcher, _ := claude.NewEventWatcher(claude.EventsDir())

    // Map of views per tmux session
    views := make(map[string]*claude.View)

    watcher.OnEvent(func(tmuxSession string, event claude.HookEvent) {
        view, ok := views[tmuxSession]
        if !ok {
            view = claude.NewView(tmuxSession, 80, 24)
            views[tmuxSession] = view
        }
        view.UpdateFromHookEvent(event)
    })

    watcher.Start()

    // Poll loop
    for {
        for _, view := range views {
            view.PollTranscript()

            if view.IsDirty() {
                // Render to screen
                fmt.Print(view.Render())
            }
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```

## Input Handling

For structured views, input goes through `tmux send-keys`:

```go
// Permission prompts
claude.SendPermissionResponse(tmuxSession, true)  // y
claude.SendPermissionResponse(tmuxSession, false) // n

// Text input
claude.SendText(tmuxSession, "hello world")

// Interrupt
claude.SendInterrupt(tmuxSession) // Ctrl-C
```

## Memory Efficiency

- Events file: Only stores what hooks provide (no full tool outputs unless PostToolUse)
- Transcript: Skips `progress` entries (8MB+ each), stores text previews only
- Deduplication: Streaming messages update in place by message ID

Typical memory: ~100KB per active session.
