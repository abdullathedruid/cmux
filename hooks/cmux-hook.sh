#!/bin/bash
# cmux-hook.sh - Minimal Claude Code hook for cmux
#
# Appends raw hook events as JSONL (one JSON object per line).
# All parsing and state management is done by cmux.
#
# Output: $TMPDIR/cmux/events/<tmux-session>.jsonl
#
# Installation: Add to ~/.claude/settings.json:
# {
#   "hooks": {
#     "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "Stop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "SubagentStop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "PermissionRequest": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}],
#     "Notification": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-hook.sh"}]}]
#   }
# }

DIR="${TMPDIR:-/tmp}/cmux/events"
mkdir -p "$DIR"

# Get tmux session name (skip if not in tmux)
SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null) || exit 0
[ -z "$SESSION" ] && exit 0

# Read event, add timestamp and tmux session, append as single line
# Create parent directory in case session name contains slashes
OUTFILE="$DIR/$SESSION.jsonl"
mkdir -p "$(dirname "$OUTFILE")"
jq -c --arg ts "$(date -Iseconds)" --arg tmux "$SESSION" '. + {ts: $ts, tmux_session: $tmux}' \
    >> "$OUTFILE"
