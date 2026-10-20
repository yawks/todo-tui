# todo-tui

A keyboard-first terminal client for [Tududi](https://github.com/chrisvel/tududi) and [OpenTodo](https://open-todo.app), built with Bubble Tea.

## Setup

Create an API key in Tududi, then copy `config.example.json` to your platform config directory:

- macOS: `~/Library/Application Support/tududi-tui/config.json`
- Linux: `~/.config/tududi-tui/config.json`

The configuration directory keeps its historical name `tududi-tui` so existing settings and saved state remain available after the rename.

`backend` selects `tududi` (the default for existing configurations) or `open-todo`.
`base_url` is the server root, without `/api` or `/api/v1` (for example `http://localhost:3002`).

For OpenTodo, create a full API token in Settings → Integrations → API tokens, then use:

```json
{
  "backend": "open-todo",
  "base_url": "https://todo.example.com",
  "api_key": "ot_your-token"
}
```

Press `c` to view and edit the configuration. The API key is masked. `ctrl+s` validates and saves the file with owner-only permissions, then reconnects. `esc` cancels. Changing server or backend clears the current filters and selection.

OpenTodo maps labels to tags, open/dated/completed tasks to Not started/Planned/Done, and archived projects to Done. Other task statuses are refused when saving. High/Medium/Low map to OpenTodo priorities 1/2/4; an existing priority 3 is preserved while it remains Low. Existing due times and time zones are preserved. Create labels with `T` before assigning them to tasks. End-to-end encrypted accounts are not supported.

```sh
go build -o todo-tui .
./todo-tui
```

## Keys

| Key | Action |
| --- | --- |
| `j`/`k`, arrows | Move |
| `tab` | Switch sidebar/content |
| `enter` | Open sidebar item |
| `/` | Search tasks using the selected filters |
| `n` | New task (from any view) |
| `N` | New project |
| `T` | New tag |
| `e`, `d` | Edit, delete |
| `space` | Complete/reopen task |
| `u` | Undo the last completion while its toast is visible (6 seconds) |
| `h` | Toggle completed tasks |
| `v` | Month/week/working-week calendar |
| `r` | Refresh |
| `c` | View/edit configuration |
| `?` | Help |
| `q` | Quit |

In forms, use `tab` or the arrow keys to move, `ctrl+s` to save, and `enter` on the due-date field to open the calendar. Project and tag fields open keyboard selectors with `enter`.
Tag and project colors accept any value supported by Tududi (normally `#RRGGBB`); press `enter` on the Color field for the built-in palette or type a custom value directly.

In the calendar, use the arrows to select a day, `enter` to browse its tasks, `esc` to return to day navigation, and `[`/`]` to change period.

The active project, the last task filter, and the calendar view are restored from `state.json` in the same config directory. Use the explicit `All` entries to clear either filter.

The minimum supported terminal size is 80×20. The task detail panel is hidden automatically on narrower terminals.

Search updates while typing (title and description, case insensitive). Use `tab` to focus the “all tasks” checkbox and `space` or `enter` to toggle searching all tasks, including completed tasks, regardless of the selected filters. Press `tab` again to return to the search field. Use `↑`/`↓` to select a result and `enter` to view it. Press `e` to edit, `ctrl+s` to save, or `esc` to discard edits and return to the task view. From the view, `esc` returns to the results; another `esc` closes search.
