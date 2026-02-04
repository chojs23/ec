# ec (easy-conflict)

<img width="1613" height="848" alt="Image" src="https://github.com/user-attachments/assets/f6903327-15c6-4fc0-9427-5bd820ba02ec" />

![Demo](https://github.com/user-attachments/assets/15022303-9948-4fdd-a6e5-2f909213d6a9)

easy-conflict is a terminal native Git mergetool with a 3 pane IntelliJ like resolver. It focuses on conflict blocks and writes the resolved result back to the merged file.

## Features

1. 3 pane TUI with ours, result, and theirs
2. Diff3 base view when available via git merge-file
3. No args mode that lists conflicted files and lets you pick one
4. Non interactive modes for CI or scripts
5. Optional backup of the merged file

## Install

Option 1. Go install

```
go install github.com/chojs23/ec/cmd/ec@latest
```

Option 2. Install script

```
./scripts/install.sh
PREFIX=/usr/local ./scripts/install.sh
```

Option 3. Build from source

```
make build
```

## Quick start

1. Run with no args inside a git repo that has conflicts

```
ec
```

2. Use it as a mergetool

```
git mergetool --tool ec
```

## Neovim plugin (terminal buffer)

This repo includes a minimal Neovim plugin that opens ec in a terminal buffer.

Install with your plugin manager and ensure `ec` is on your PATH.

Minimal config:

```lua
require("ec").setup()
```

Lazy.nvim minimal config:

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

Config defaults:

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

Usage:

```
:Ec
:Ec --base <path> --local <path> --remote <path> --merged <path>
```

## Git mergetool configuration

Add this to your git config

```

[merge]
  tool = ec

[mergetool "ec"]
  cmd = ec "$BASE" "$LOCAL" "$REMOTE" "$MERGED"
  trustExitCode = true

```

Notes

1. ec does not run git add after you write
2. Git will still decide whether the merge is resolved based on the file contents

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

## Key bindings

Keybindings are vim-like by default.

## Resolver screen

The resolver shows three panes in one view.

Conflicts are shown as focused blocks. The center pane is the output that will be written to the merged file.

You can move between conflicts, choose a side, and apply it. The status line shows which conflict you are on and whether it is resolved.

Use e to open $EDITOR with the current result. When you exit the editor, the resolver reloads the merged file and keeps manual edits.

Blue: modified lines (changed vs base)

Green: added lines

Red: conflicted lines where both sides differ

### Navigation

- n / p: next and previous conflict
- j / k: vertical scroll
- H / L: horizontal scroll

#### Selection and apply

- h / l: select ours or theirs
- a: accept selection
- d: discard selection
- o, t, b, x: apply ours, theirs, both, or none
- O and T: apply ours or theirs to all

#### Other

- u: undo
- e: open $EDITOR with current result
- w: write merged file without quitting
- q: back to selector or quit

## Backup behavior

Backups are off by default. Use --backup to write a sibling file named <merged>.ec.bak before writing the result.

## Base view behavior

Base chunks come from git merge-file --diff3 output. If the base stage is missing for a file, the tool continues without a base view and prints a warning.

## License

MIT
