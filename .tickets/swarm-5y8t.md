---
id: swarm-5y8t
status: closed
deps: []
links: []
created: 2025-12-27T08:54:01.644763508+01:00
type: task
priority: 2
---
# Implement swarm doctor command

Add comprehensive environment and dependency check command.

## From UX_FEEDBACK_1.md and forge-cli-v2.md

## Command
```bash
swarm doctor
swarm doctor --json
```

## Checks to Perform

### Dependencies
- [ ] tmux installed and version compatible
- [ ] opencode available in PATH (and version)
- [ ] ssh client available
- [ ] git available

### Configuration
- [ ] Config file readable
- [ ] DB writable and migrations applied
- [ ] Required directories exist
- [ ] Log file writable

### Nodes (if configured)
- [ ] SSH connections working
- [ ] swarmd reachable (if daemon mode)
- [ ] Remote tmux accessible

### Accounts (if configured)
- [ ] Vault accessible
- [ ] Profiles readable
- [ ] Auth files present

### Port Conflicts
- [ ] OpenCode ports not conflicting (1455 warning from codex-auth)
- [ ] swarmd port available

## Output Format
```
Swarm Doctor
============

[✓] tmux 3.4 installed
[✓] opencode 0.5.0 installed
[✓] ssh client available
[✓] git 2.43.0 installed

[✓] Config: ~/.config/swarm/config.yaml
[✓] Database: ~/.local/share/swarm/swarm.db
[✓] Migrations: up to date

[!] Node "remote-1": SSH connection timeout
[✓] Node "local": ok

All checks passed (1 warning)
```

## JSON Output
Structured with check name, status, message, details


