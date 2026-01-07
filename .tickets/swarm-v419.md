---
id: swarm-v419
status: closed
deps: [swarm-cbb0]
links: []
created: 2025-12-23T15:08:08.023971548+01:00
type: task
priority: 0
---
# Implement vault package core

Create internal/vault/ package with core vault functionality:

Files to create:
- internal/vault/vault.go - Main Vault struct and operations
- internal/vault/profile.go - Profile management (backup, activate, list, delete)
- internal/vault/paths.go - Auth file path detection per adapter (Claude, Codex, Gemini, OpenCode)
- internal/vault/sync.go - Profile sync operations for remote nodes

Core types:
- Vault: manages profiles directory and metadata
- Profile: represents a saved auth profile with provider, name, auth files
- AuthPaths: maps adapters to their auth file locations

Key functions:
- NewVault(configDir string) - Initialize vault at ~/.config/swarm/vault/
- Backup(adapter, profileName string) - Copy current auth files to vault
- Activate(adapter, profileName string) - Restore auth files from vault (<100ms target)
- List(adapter string) - List saved profiles
- Delete(adapter, profileName string) - Remove profile from vault
- GetActive(adapter string) - Detect currently active profile via content hash
- Status() - Show active profile for each adapter

Auth file locations to support:
- Claude: ~/.claude.json, ~/.config/claude-code/auth.json
- Codex: ~/.codex/auth.json (or $CODEX_HOME/auth.json)
- Gemini: ~/.gemini/settings.json
- OpenCode: ~/.opencode/auth.json (verify actual path)

Must be fast - file operations only, no network calls.


