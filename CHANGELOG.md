# Changelog

## v0.2.0

- Add theme config support for TUI colors via ~/.config/ec/themes.json
- Document theme configuration, defaults, and hex color guidance
- Add tests for theme config loading and apply behavior

## v0.1.3

- Add a minimal Neovim plugin for running ec in a terminal buffer
- Default to a floating window UI in Neovim with optional keymap support
- Allow writing with unresolved conflicts (markers preserved)

## v0.1.2

- Initial release of ec
- 3-pane TUI resolver with navigation, apply/undo, and write
- Non-interactive modes: --check and --apply-all (ours/theirs/both/none)
- No-args mode with conflicted file selector
- Diff3 base view via git merge-file
- GitHub Actions release workflow for macOS/Linux
