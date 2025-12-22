# OpenCode usage metrics notes

Source: local `opencode` CLI help output and a no-usage `opencode stats` run on this host.

## Relevant CLI commands

- `opencode stats`: shows token usage + cost statistics.
  - Flags: `--days`, `--project`, `--tools`.
  - No JSON output flag exposed in help output.
- `opencode run --format json`: advertises raw JSON events for a run.
- `opencode export <sessionID>`: exports session data as JSON.

## Observed stats output fields

The `opencode stats` output is a text table. Fields observed in the table:

- Sessions
- Messages
- Days
- Total Cost (USD)
- Avg Cost/Day (USD)
- Avg Tokens/Session
- Median Tokens/Session
- Input
- Output
- Cache Read
- Cache Write

## Integration implications

- If OpenCode JSON events expose usage, prefer that for per-session metrics.
- If JSON events are unavailable, the stats table is parseable but would be a best-effort adapter.
- Storing metrics likely needs a structured field (agent metadata or account usage stats).
- Session IDs are required for `opencode export`, so the adapter must surface them if we rely on export.
