---
id: swarm-xw03
status: closed
deps: [swarm-v419]
links: []
created: 2025-12-23T15:09:24.215665586+01:00
type: task
priority: 1
---
# Add vault package tests

Create comprehensive tests for the vault package.

Test files to create:
- internal/vault/vault_test.go
- internal/vault/profile_test.go
- internal/vault/paths_test.go

Test cases for vault.go:
- NewVault creates directory structure
- NewVault handles existing vault
- Vault operations with missing directory

Test cases for profile.go:
- Backup copies all auth files correctly
- Backup creates profile directory
- Backup overwrites existing profile (with warning)
- Activate restores files to correct locations
- Activate preserves file permissions (600)
- Activate fails gracefully if profile missing
- List returns all profiles for adapter
- List returns empty for unknown adapter
- Delete removes profile directory
- Delete fails for non-existent profile
- GetActive detects current profile via hash
- GetActive returns empty when no match

Test cases for paths.go:
- AuthPaths returns correct paths for Claude
- AuthPaths returns correct paths for Codex
- AuthPaths respects CODEX_HOME env var
- AuthPaths returns correct paths for Gemini
- AuthPaths returns correct paths for OpenCode

Integration tests:
- Full backup -> activate cycle
- Multiple profiles for same adapter
- Profile isolation (activating one doesn't affect others)

Use testutil.TempDir for isolated test environments.
Mock home directory to avoid touching real auth files.


