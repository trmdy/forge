---
id: swarm-s3r0
status: closed
deps: [swarm-v419]
links: []
created: 2025-12-23T15:08:20.818650736+01:00
type: task
priority: 0
---
# Implement vault CLI commands

Create internal/cli/vault.go with vault management commands:

Commands to implement:
- swarm vault init - Initialize vault directory structure
- swarm vault backup <adapter> <profile> - Save current auth to vault
- swarm vault activate <adapter> <profile> - Restore auth from vault (aliased as 'switch', 'use')
- swarm vault list [adapter] - List saved profiles with active indicator
- swarm vault delete <adapter> <profile> - Remove profile (with confirmation)
- swarm vault status - Show active profile for each adapter
- swarm vault paths [adapter] - Show auth file locations for each adapter
- swarm vault clear <adapter> - Remove current auth files (logout state)

Flags:
- --force: Skip confirmation prompts
- --backup-current: Auto-backup current profile before activating new one
- --json: Output in JSON format for scripting

Output formatting:
- Use table format for list/status
- Show cooldown status if integrated with account service
- Color-code active vs inactive profiles

Example outputs:
$ swarm vault status
ADAPTER   ACTIVE PROFILE   COOLDOWN
claude    work@company     -
codex     personal         47m remaining
gemini    (none)           -

$ swarm vault list claude
PROFILE          LAST USED      STATUS
work@company     2h ago         active
personal         1d ago         -
backup           5d ago         -


