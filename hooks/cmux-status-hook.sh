#!/bin/bash
# cmux-status-hook.sh - Claude Code hook for reporting session status to cmux
#
# This script is called by Claude Code hooks and writes status updates
# to a file that cmux reads to display session status.
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
#     "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "Stop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "SubagentStop": [{"hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}],
#     "Notification": [{"matcher": "permission_prompt", "hooks": [{"type": "command", "command": "/path/to/cmux-status-hook.sh"}]}]
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

# Parse with jq
EVENT=$(echo "$INPUT" | jq -r '.hook_event_name // empty')
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')

# Build summary based on tool type (no truncation - CLI handles display)
SUMMARY=""
case "$TOOL" in
    Read)
        FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
        [ -n "$FILE" ] && SUMMARY="Reading ${FILE##*/}"
        ;;
    Edit)
        FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
        [ -n "$FILE" ] && SUMMARY="Editing ${FILE##*/}"
        ;;
    Write)
        FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
        [ -n "$FILE" ] && SUMMARY="Writing ${FILE##*/}"
        ;;
    Bash)
        CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
        [ -n "$CMD" ] && SUMMARY="Running: $CMD"
        ;;
    Grep)
        PATTERN=$(echo "$INPUT" | jq -r '.tool_input.pattern // empty')
        [ -n "$PATTERN" ] && SUMMARY="Searching: $PATTERN"
        ;;
    Glob)
        PATTERN=$(echo "$INPUT" | jq -r '.tool_input.pattern // empty')
        [ -n "$PATTERN" ] && SUMMARY="Finding: $PATTERN"
        ;;
    Task)
        DESC=$(echo "$INPUT" | jq -r '.tool_input.description // empty')
        [ -n "$DESC" ] && SUMMARY="Agent: $DESC"
        ;;
    WebFetch)
        URL=$(echo "$INPUT" | jq -r '.tool_input.url // empty')
        [ -n "$URL" ] && SUMMARY="Fetching: $URL"
        ;;
    WebSearch)
        QUERY=$(echo "$INPUT" | jq -r '.tool_input.query // empty')
        [ -n "$QUERY" ] && SUMMARY="Searching: $QUERY"
        ;;
    TodoWrite)
        SUMMARY="Updating todos"
        ;;
    LSP)
        OP=$(echo "$INPUT" | jq -r '.tool_input.operation // empty')
        FILE=$(echo "$INPUT" | jq -r '.tool_input.filePath // empty')
        if [ -n "$OP" ] && [ -n "$FILE" ]; then
            SUMMARY="LSP $OP: ${FILE##*/}"
        elif [ -n "$OP" ]; then
            SUMMARY="LSP: $OP"
        fi
        ;;
    *)
        [ -n "$TOOL" ] && SUMMARY="$TOOL"
        ;;
esac

# Determine status based on event
case "$EVENT" in
    PreToolUse)
        STATUS="tool"
        ;;
    PostToolUse|Stop|SubagentStop|UserPromptSubmit)
        STATUS="active"
        [ "$EVENT" = "Stop" ] || [ "$EVENT" = "SubagentStop" ] && STATUS="idle"
        TOOL=""
        SUMMARY=""
        ;;
    Notification)
        STATUS="needs_input"
        TOOL=""
        SUMMARY="Waiting for permission"
        ;;
    *)
        STATUS="active"
        TOOL=""
        SUMMARY=""
        ;;
esac

# Write status file as JSON
jq -n \
    --arg status "$STATUS" \
    --arg tool "$TOOL" \
    --arg summary "$SUMMARY" \
    --argjson ts "$(date +%s)" \
    '{status: $status, tool: $tool, summary: $summary, ts: $ts}' \
    > "$DIR/$SESSION.status"
