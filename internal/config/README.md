# Configuration

cmux supports YAML-based configuration for keybindings, colors, and status indicators.

## Config File Location

`~/.config/cmux/config.yaml`

All configuration is optional. If the file doesn't exist or a value is omitted, sensible defaults are used.

## Keybindings

### Key Format

Keys can be specified in the following formats:

| Format | Examples | Description |
|--------|----------|-------------|
| Single character | `"q"`, `"v"`, `"?"`, `"/"` | Lowercase letters, symbols |
| Uppercase | `"N"`, `"Q"` | Shift + letter |
| Special keys | `"enter"`, `"space"`, `"esc"`, `"tab"` | Named special keys |
| Arrow keys | `"up"`, `"down"`, `"left"`, `"right"` | Navigation keys |
| Ctrl combinations | `"ctrl+c"`, `"ctrl+s"` | Control key combos |
| Function keys | `"f1"`, `"f2"`, ... `"f12"` | Function keys |

### Available Keybindings

| Key | Default | Description |
|-----|---------|-------------|
| `quit` | `"q"` | Quit cmux |
| `toggle_view` | `"v"` | Toggle between dashboard and list view |
| `help` | `"?"` | Show help screen |
| `search` | `"/"` | Open search/filter |
| `worktree` | `"w"` | Open worktree picker |
| `edit_note` | `"e"` | Edit session note |
| `new_wizard` | `"N"` | Open new session wizard |
| `nav_down` | `"j"` | Navigate down |
| `nav_up` | `"k"` | Navigate up |
| `nav_left` | `"h"` | Navigate left |
| `nav_right` | `"l"` | Navigate right |
| `popup` | `"p"` | Open session in popup |
| `new_session` | `"n"` | Create new session in current directory |
| `delete` | `"x"` | Delete selected session |
| `refresh` | `"r"` | Refresh session list |
| `diff` | `"d"` | Show git diff in popup |

### Hardcoded Keys

Some keys cannot be changed:

- `Ctrl+C` - Always quits (safety)
- `Enter` - Always confirms/attaches
- `Arrow keys` - Always work for navigation (in addition to configured nav keys)
- `Esc` - Closes modals/popups

### Validation

Duplicate keybindings are not allowed. If two actions are assigned the same key, cmux will display an error on startup listing the conflicts.

## Theme

### Colors

Available color names: `"black"`, `"red"`, `"green"`, `"yellow"`, `"blue"`, `"magenta"`, `"cyan"`, `"white"`, `"default"`

| Setting | Default | Description |
|---------|---------|-------------|
| `selection_bg` | `"blue"` | Background color for selected items |
| `selection_fg` | `"white"` | Foreground color for selected items |
| `statusbar_bg` | `"blue"` | Status bar background |
| `statusbar_fg` | `"white"` | Status bar foreground |

### Status Indicators

Each session status can have a custom icon, color, and label:

| Status | Default Icon | Default Color | Default Label | Description |
|--------|-------------|---------------|---------------|-------------|
| `attached` | `●` | `green` | `ATTACHED` | Session is attached |
| `active` | `◐` | `yellow` | `ACTIVE` | Claude is actively responding |
| `tool` | `⚙` | `cyan` | `TOOL` | Claude is using a tool |
| `thinking` | `◑` | `yellow` | `THINKING` | Claude is thinking |
| `input` | `🔔` | `magenta` | `INPUT` | Waiting for user input |
| `stopped` | `✓` | `green` | `DONE` | Session completed |
| `idle` | `○` | `white` | `IDLE` | Session is idle |

## Example Configuration

```yaml
# ~/.config/cmux/config.yaml

# Custom keybindings
keys:
  quit: "Q"           # Shift+Q to quit
  toggle_view: "v"
  help: "?"
  search: "/"
  worktree: "w"
  edit_note: "e"
  new_wizard: "N"
  nav_down: "j"
  nav_up: "k"
  nav_left: "h"
  nav_right: "l"
  popup: "p"
  new_session: "n"
  delete: "ctrl+d"    # Ctrl+D to delete
  refresh: "r"
  diff: "d"

# Custom theme
theme:
  colors:
    selection_bg: "magenta"
    selection_fg: "white"
    statusbar_bg: "blue"
    statusbar_fg: "white"

  # Custom status indicators
  status:
    attached:
      icon: "★"
      color: "green"
      label: "CONNECTED"
    active:
      icon: "▶"
      color: "yellow"
      label: "RUNNING"
    idle:
      icon: "○"
      color: "white"
      label: "READY"
```

## Partial Configuration

You don't need to specify all values. Any omitted values use defaults:

```yaml
# Only change what you need
keys:
  quit: "Q"
  delete: "ctrl+d"

theme:
  colors:
    selection_bg: "green"
```

## Troubleshooting

### "duplicate keybindings found" error

This means two or more actions are assigned the same key. The error message will list which keys conflict:

```
Error loading configuration: duplicate keybindings found:
  key "x" is used by: Quit, Delete
```

Fix by assigning unique keys to each action.

### "invalid key" error

The key string couldn't be parsed. Check the key format section above for valid formats.

### Config not loading

Ensure the file is at `~/.config/cmux/config.yaml` and is valid YAML. You can validate YAML syntax with:

```bash
python3 -c "import yaml; yaml.safe_load(open('$HOME/.config/cmux/config.yaml'))"
```
