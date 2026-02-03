# Nightly compound loop (forge pilot)

Two scripts run in sequence:

1) `scripts/compound-review.sh` (22:30)
- Reviews last 24h activity (git log + recent compound logs + recent ledgers)
- Updates `AGENTS.md` with durable learnings
- No code changes, no pushes

2) `scripts/auto-compound.sh` (23:00)
- Picks top ready sv task (`sv task ready`)
- Runs Codex to implement
- Runs `make check`
- Optionally pushes + opens draft PR

Both scripts abort on a dirty working tree, except untracked `logs/` and `docs/review/`.

Task selection: `sv task ready` ordering (status -> priority -> readiness -> updated_at -> id).

## Requirements

- `codex` CLI on PATH
- `sv` CLI on PATH
- `rg`, `python3`, `git`
- `go`, `golangci-lint` for `make check`
- `gh` if you want PR creation

## Environment toggles

- `COMPOUND_PUSH=1` to push + open PR
- `FORGE_TEST_SKIP_NETWORK=1` to skip network tests

## Launchd (macOS)

Create these files under `~/Library/LaunchAgents/`.

`com.forge.compound-review.plist` (22:30):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.forge.compound-review</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/trmd/Code/oss--forge/repos/forge/scripts/compound-review.sh</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge</string>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>22</integer>
    <key>Minute</key>
    <integer>30</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge/logs/compound-review.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge/logs/compound-review.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/Users/trmd/.bun/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
```

`com.forge.auto-compound.plist` (23:00):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.forge.auto-compound</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/trmd/Code/oss--forge/repos/forge/scripts/auto-compound.sh</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge</string>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>23</integer>
    <key>Minute</key>
    <integer>0</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge/logs/auto-compound.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/trmd/Code/oss--forge/repos/forge/logs/auto-compound.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/Users/trmd/.bun/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
```

Load:

```bash
launchctl load ~/Library/LaunchAgents/com.forge.compound-review.plist
launchctl load ~/Library/LaunchAgents/com.forge.auto-compound.plist
```

Check:

```bash
launchctl list | rg forge\.compound
```

## Logs

- `logs/compound-review.log`
- `logs/auto-compound.log`
- `logs/compound/*.jsonl` (raw codex output)
