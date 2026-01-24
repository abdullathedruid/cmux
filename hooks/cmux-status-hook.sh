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

BASEDIR="${TMPDIR:-/tmp}/cmux"
STATUSDIR="$BASEDIR/sessions"
EVENTSDIR="$BASEDIR/events"
mkdir -p "$STATUSDIR" "$EVENTSDIR"

# Get tmux session name
SESSION=$(tmux display-message -p '#{session_name}' 2>/dev/null || echo "")
[ -z "$SESSION" ] && exit 0

# Read hook input
INPUT=$(cat)

# Log all hook data for debugging
LOGFILE="$BASEDIR/hook-debug.log"
echo "=== $(date -Iseconds) [tmux:$SESSION] ===" >> "$LOGFILE"
echo "$INPUT" | jq '.' >> "$LOGFILE" 2>/dev/null || echo "$INPUT" >> "$LOGFILE"
echo "" >> "$LOGFILE"

# Append event to JSONL file for structured view
# Create parent directory in case session name contains slashes
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EVENTFILE="$EVENTSDIR/$SESSION.jsonl"
mkdir -p "$(dirname "$EVENTFILE")"
echo "$INPUT" | jq -c --arg ts "$TIMESTAMP" --arg tmux "$SESSION" '. + {ts: $ts, tmux_session: $tmux}' >> "$EVENTFILE"

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
        STATUS="stopped"
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
# Create parent directory in case session name contains slashes
STATUSFILE="$STATUSDIR/$SESSION.status"
mkdir -p "$(dirname "$STATUSFILE")"
jq -n \
    --arg status "$STATUS" \
    --arg tool "$TOOL" \
    --arg transcript "$TRANSCRIPT" \
    --arg session_id "$SESSION_ID" \
    --argjson ts "$(date +%s)" \
    '{status: $status, tool: $tool, transcript_path: $transcript, session_id: $session_id, ts: $ts}' \
    > "$STATUSFILE"
