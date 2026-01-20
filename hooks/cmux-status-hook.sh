#!/bin/bash
# cmux-status-hook.sh - Claude Code hook for reporting session status to cmux
#
# This script is called by Claude Code hooks and writes status updates
# to a file that cmux reads to display session status.
#
# Installation:
# 1. Copy this script to a location in your PATH, or note its full path
# 2. Make it executable: chmod +x cmux-status-hook.sh
# 3. Add to your Claude Code settings (~/.claude/settings.json):
#
# {
#   "hooks": {
#     "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "Stop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "SubagentStop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}]
#   }
# }

set -e

# Status directory - must match what cmux expects
DIR="${TMPDIR:-/tmp}/cmux/sessions"
mkdir -p "$DIR"

# Get tmux session name (this works because the hook runs in the tmux pane)
SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null || echo "")
if [ -z "$SESSION" ]; then
    exit 0
fi

# Read hook input from stdin
INPUT=$(cat)

# Extract event type and tool name using basic string parsing
# (avoiding jq dependency for portability)
EVENT=$(echo "$INPUT" | grep -o '"hook_event_name":"[^"]*"' | cut -d'"' -f4)
TOOL=$(echo "$INPUT" | grep -o '"tool_name":"[^"]*"' | cut -d'"' -f4)

# Determine status based on event
case "$EVENT" in
    PreToolUse)
        STATUS="tool"
        ;;
    PostToolUse)
        STATUS="active"  # Stay active between tool calls
        TOOL=""
        ;;
    Stop|SubagentStop)
        STATUS="idle"    # Only idle when Claude finishes responding
        TOOL=""
        ;;
    UserPromptSubmit)
        STATUS="active"
        TOOL=""
        ;;
    *)
        STATUS="active"
        TOOL=""
        ;;
esac

# Write status file
TS=$(date +%s)
echo "{\"status\":\"$STATUS\",\"tool\":\"$TOOL\",\"ts\":$TS}" > "$DIR/$SESSION.status"
