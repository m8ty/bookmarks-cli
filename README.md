# cli-bookmark

A small terminal UI for fuzzy-searching your shell history and saving per-directory command bookmarks. Pick a command with arrow keys, hit enter, and it runs in the current shell.

## Features

- Fuzzy search over recent zsh history
- Per-directory bookmarks (the same `cwd` always shows the same saved commands)
- Toggle between history and bookmarks with `Ctrl-T`
- Selected command is copied to the clipboard before running

## Requirements

- Go 1.24+
- macOS (uses `pbcopy` for clipboard)
- zsh (reads `~/.zsh_history`)

## Build

```sh
go build -o bookmark
```

## Run

```sh
./bookmark
```

Or install it on your `PATH`:

```sh
go install
```

Bookmarks are persisted to `~/.config/bookmarks/saved.json`.

## Keybinds

| Key      | Action                                |
|----------|---------------------------------------|
| `↑` / `↓` | Move selection (also `Ctrl-K` / `Ctrl-J`) |
| `Enter`  | Run the selected command              |
| `Ctrl-B` | Bookmark the selected command for the current directory |
| `Ctrl-D` | Delete the selected bookmark (only in bookmarks view) |
| `Ctrl-T` | Toggle between history and bookmarks  |
| `Esc`    | Quit without running anything         |

Type to filter the list. With nothing typed, the full list is shown.
