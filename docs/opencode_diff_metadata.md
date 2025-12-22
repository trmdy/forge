# OpenCode diff metadata notes

Source: best-effort parsing of OpenCode output in tmux panes.

## Signals parsed

- Diff stat lines (e.g. `path/to/file | 12 ++++--`)
- Diff summary lines (e.g. `2 files changed, 9 insertions(+), 3 deletions(-)`)
- Commit references:
  - `commit <sha>` patterns
  - URLs containing `/commit/<sha>`

## Extracted fields

- Files (paths)
- FilesChanged
- Insertions / Deletions
- Commits

## Caveats

- This is best-effort and depends on OpenCode emitting diff or git output.
- If OpenCode provides structured diff metadata via JSON events or export, prefer that in the future.
