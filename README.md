# ec (easy-conflict)

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

## Git mergetool configuration

Add this to your git config

```
[merge]
    tool = ec

[mergetool "easy-conflict"]
    cmd = ec "$BASE" "$LOCAL" "$REMOTE" "$MERGED"
    trustExitCode = true
```

Notes

1. easy-conflict does not run git add after you write
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

Navigation

1. n and p: next and previous conflict
2. j and k: vertical scroll
3. H and L: horizontal scroll

Selection and apply

1. h and l: select ours or theirs
2. a: accept selection
3. d: discard selection
4. o, t, b, x: apply ours, theirs, both, or none
5. O and T: apply ours or theirs to all

Other

1. u: undo
2. e: open $EDITOR with current result
3. w: write merged file without quitting
4. q: back to selector or quit

## Backup behavior

Backups are off by default. Use --backup to write a sibling file named <merged>.easy-conflict.bak before writing the result.

## Base view behavior

Base chunks come from git merge-file --diff3 output. If the base stage is missing for a file, the tool continues without a base view and prints a warning.
