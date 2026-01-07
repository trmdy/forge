---
id: swarm-hc5y
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:25:25.907703934+01:00
type: task
priority: 2
---
# Add launchd plist for macOS

Create a launchd plist for running forged on macOS.

**File**: contrib/launchd/com.forge.forged.plist (new file)

**Changes Required**:
1. Create plist with Label, ProgramArguments, RunAtLoad, KeepAlive
2. Configure log output paths
3. Create README with installation instructions

**Installation steps to document**:
- Copy to ~/Library/LaunchAgents/
- launchctl load ~/Library/LaunchAgents/com.forge.forged.plist
- launchctl start com.forge.forged

See docs/design/scheduler-daemon-tasks.md#task-52 for full details.


