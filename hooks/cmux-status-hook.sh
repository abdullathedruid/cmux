#!/bin/bash
# cmux-status-hook.sh - Lightweight Claude Code hook for cmux
#
# This hook maps tmux sessions to Claude transcripts and tracks real-time status.
# Full history is read directly from the JSONL transcript by cmux.
#
# Requirements: jq
#
# Installation:
# 1. Copy this script to a location in your PATH, or note its full path
# 2. Make it executable: chmod +x cmux-status-hook.sh
# 3. Add to your Claude Code settings (~/.claude/settings.json):
#
# {
#   "hooks": {
#     "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "Stop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "SubagentStop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PermissionRequest": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "Notification": [{"matcher": "permission_prompt", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}]
#   }
# }

set -e

DIR="${TMPDIR:-/tmp}/cmux/sessions"
mkdir -p "$DIR"

# Get tmux session name
SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null || echo "")
[ -z "$SESSION" ] && exit 0

# Read hook input
INPUT=$(cat)

# Parse fields
EVENT=$(echo "$INPUT" | jq -r '.hook_event_name // empty')
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')
TRANSCRIPT=$(echo "$INPUT" | jq -r '.transcript_path // empty')
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')

# Determine status based on event
case "$EVENT" in
    PreToolUse)
        STATUS="tool"
        ;;
    PostToolUse|UserPromptSubmit)
        STATUS="active"
        TOOL=""
        ;;
    Stop|SubagentStop)
        STATUS="idle"
        TOOL=""
        ;;
    Notification|PermissionRequest)
        STATUS="needs_input"
        TOOL=""
        ;;
    *)
        STATUS="active"
        TOOL=""
        ;;
esac

# Write minimal status file
jq -n \
    --arg status "$STATUS" \
    --arg tool "$TOOL" \
    --arg transcript "$TRANSCRIPT" \
    --arg session_id "$SESSION_ID" \
    --argjson ts "$(date +%s)" \
    '{status: $status, tool: $tool, transcript_path: $transcript, session_id: $session_id, ts: $ts}' \
    > "$DIR/$SESSION.status"
