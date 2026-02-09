# ec (easy-conflict)

Terminal Git merge conflict resolver with a 3 pane TUI and Neovim integration

[![codecov](https://codecov.io/gh/chojs23/ec/graph/badge.svg?token=AX5BFUF8YN)](https://codecov.io/gh/chojs23/ec)

<img width="1613" height="848" alt="ec 3 pane terminal merge conflict resolver" src="https://github.com/user-attachments/assets/f6903327-15c6-4fc0-9427-5bd820ba02ec" />

![ec demo resolving git merge conflicts in terminal](https://github.com/user-attachments/assets/7c3eba19-d72b-499b-b2e8-3e16f645407e)

ec (easy-conflict) is a 3-way terminal native Git merge conflict resolver. Supports diff3 base comparison, and helps you resolve merge conflicts faster without leaving the terminal.

## Features

1. 3 pane TUI with ours, result, and theirs
2. Diff3 base view when available via git merge-file
3. No args mode that lists conflicted files and lets you pick one
4. Non interactive modes for CI or scripts
5. Optional backup of the merged file

## Installation

### Homebrew

```
brew install chojs23/tap/ec
```

### Go install

```
go install github.com/chojs23/ec/cmd/ec@latest
```

### Via curl

```
curl -fsSL https://raw.githubusercontent.com/chojs23/ec/main/scripts/install.sh | sh
```

### Install script

Downloads the GitHub release binary. VERSION is optional and defaults to latest.

```
./scripts/install.sh
```

```
VERSION=v0.1.3 PREFIX=/usr/local ./scripts/install.sh
```

### Build from source

```
make build
```

### Arch Linux (AUR)

- build from source

```
pikaur -S easy-conflict
```

- binary

```
pikaur -S easy-conflict-bin
```

## Quick start

Run with no args inside a git repo that has conflicts

```
ec
```

### Notes

ec does not run git add after you write

Git will still decide whether the merge is resolved based on the file contents

## Git mergetool configuration

You can set ec as your git mergetool by adding this to your git config

```
git config --global merge.tool ec
git config --global mergetool.ec.cmd 'ec "$BASE" "$LOCAL" "$REMOTE" "$MERGED"'
git config --global mergetool.ec.trustExitCode true
```

## Usage

Interactive

```
ec <BASE> <LOCAL> <REMOTE> <MERGED>
ec --base <path> --local <path> --remote <path> --merged <path>
```

No args mode

```
ec
```

Non interactive

```
ec --check --merged <path>
ec --apply-all ours --base <path> --local <path> --remote <path> --merged <path>
```

## Neovim plugin (terminal buffer)

This repo includes a minimal Neovim plugin that opens ec in a terminal buffer.

Install with your plugin manager and ensure `ec` is on your PATH.

### Minimal config

```lua
require("ec").setup()
```

Using lazy.nvim

```lua
{
  "chojs23/ec",
  keys = {
    { "<leader>gr", ":Ec<CR>", desc = "Open ec" },
  },
}
```

<details>
<summary>Default config</summary>

```lua
{
  cmd = "ec",
  open_cmd = "tabnew",
  cwd = nil,
  float = true,
  close_on_exit = true,
}
```

Option notes:

- `cmd`: executable name or list with default args; `:Ec` args are appended.
- `open_cmd`: Vim command used when float is disabled or unavailable.
- `cwd`: working directory for ec; string path or function; defaults to `getcwd()`.
- `float`: enable floating window; table merges with float defaults.
- `close_on_exit`: close terminal on successful exit code 0.

Float defaults when `float = true`:

```lua
{
  width = 0.92,
  height = 0.86,
  border = "rounded",
  title = "ec",
  title_pos = "center",
  zindex = 50,
}
```

Float option notes:

- `width` and `height`: fractions of editor when <= 1, otherwise absolute size.
- `border`: floating window border style.
- `title`: title text for the float.
- `title_pos`: title alignment.
- `zindex`: float stacking order.

</details>

### Commands

```
:Ec
:Ec --base <path> --local <path> --remote <path> --merged <path>
```

## Resolver screen

The resolver shows three panes in one view.

Conflicts are shown as focused blocks. The center pane is the output that will be written to the merged file.

You can move between conflicts, choose a side, and apply it. The status line shows which conflict you are on and whether it is resolved.

Use `e` to open $EDITOR with the current result. When you exit the editor, the resolver reloads the merged file and keeps manual edits.

Blue: modified lines (changed vs base)

Green: added lines

Red: conflicted lines where both sides differ

## Key bindings

Keybindings are vim-like by default.

### Navigation

- n / p: next and previous conflict
- gg / G: jump to top / bottom
- j / k: vertical scroll
- H / L: horizontal scroll

### Selection and apply

- h / l: select ours or theirs
- a / space: accept selection
- o / t / b / x: apply ours, theirs, both, or none
- d: discard selection
- O / T: apply ours or theirs to all

### Other

- u: undo
- e: open $EDITOR with current result
- w / ctrl+s: write file without quitting
- q: back to selector or quit

## Theme configuration

The TUI can load colors from a theme config file.

Config path:

```
$XDG_CONFIG_HOME/ec/themes.json (defaults to ~/.config/ec/themes.json)
```

Example:

```
{
  "default": "warm",
  "themes": {
    "warm": {
      "header_bg": "94",
      "header_fg": "230",
      "added_bg": "58",
      "conflicted_bg": "88",
      "result_fg": "#f1f1f1",
      "modified_bg": "#2f5f7a"
    }
  }
}
```

Missing keys fall back to the built-in defaults.

Hex colors require a TrueColor-capable terminal to avoid 256-color downsampling.

Supported keys:
`title_fg`, `pane_border`, `selected_pane_border`, `side_pane_border`, `selected_side_border`,
`header_bg`, `header_fg`, `footer_bg`, `footer_fg`, `line_number`, `ours_highlight_bg`,
`ours_highlight_fg`, `theirs_highlight_bg`, `theirs_highlight_fg`, `result_fg`,
`result_highlight_bg`, `result_highlight_fg`, `modified_bg`, `modified_fg`, `added_bg`,
`added_fg`, `removed_bg`, `removed_fg`, `conflicted_bg`, `conflicted_fg`,
`insert_marker_fg`, `selected_hunk_marker_fg`, `selected_hunk_marker_bg`, `selected_hunk_bg`,
`status_resolved_fg`, `status_unresolved_fg`, `result_resolved_marker_fg`,
`result_resolved_border`, `result_unresolved_border`, `toast_bg`, `toast_fg`,
`selector_resolved_fg`, `selector_unresolved_fg`, `dim_foreground_light`,
`dim_foreground_dark`, `dim_foreground_muted`.

<details>
<summary>Default theme colors</summary>

| Key                         | Default |
| --------------------------- | ------- |
| `title_fg`                  | `170`   |
| `pane_border`               | `63`    |
| `selected_pane_border`      | `205`   |
| `side_pane_border`          | `255`   |
| `selected_side_border`      | `33`    |
| `header_bg`                 | `62`    |
| `header_fg`                 | `230`   |
| `footer_bg`                 | `236`   |
| `footer_fg`                 | `243`   |
| `line_number`               | `241`   |
| `ours_highlight_bg`         | `24`    |
| `ours_highlight_fg`         | `230`   |
| `theirs_highlight_bg`       | `52`    |
| `theirs_highlight_fg`       | `230`   |
| `result_fg`                 | `231`   |
| `result_highlight_bg`       | `60`    |
| `result_highlight_fg`       | `230`   |
| `modified_bg`               | `24`    |
| `modified_fg`               | `231`   |
| `added_bg`                  | `28`    |
| `added_fg`                  | `231`   |
| `removed_bg`                | `237`   |
| `removed_fg`                | `250`   |
| `conflicted_bg`             | `131`   |
| `conflicted_fg`             | `231`   |
| `insert_marker_fg`          | `196`   |
| `selected_hunk_marker_fg`   | `226`   |
| `selected_hunk_marker_bg`   | `88`    |
| `selected_hunk_bg`          | `236`   |
| `status_resolved_fg`        | `42`    |
| `status_unresolved_fg`      | `196`   |
| `result_resolved_marker_fg` | `42`    |
| `result_resolved_border`    | `42`    |
| `result_unresolved_border`  | `196`   |
| `toast_bg`                  | `22`    |
| `toast_fg`                  | `230`   |
| `selector_resolved_fg`      | `42`    |
| `selector_unresolved_fg`    | `196`   |
| `dim_foreground_light`      | `231`   |
| `dim_foreground_dark`       | `16`    |
| `dim_foreground_muted`      | `244`   |

</details>

## Backup behavior

Backups are off by default. Use --backup to write a sibling file named <merged>.ec.bak before writing the result.

## Base view behavior

Base chunks come from git merge-file --diff3 output. If the base stage is missing for a file, the tool continues without a base view and prints a warning.

## Contributing

New features and bug reports are welcome.

Feel free to open an issue or a pull request.

## License

MIT
