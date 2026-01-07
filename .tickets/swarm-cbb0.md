---
id: swarm-cbb0
status: closed
deps: []
links: []
created: 2025-12-23T15:07:45.622151808+01:00
type: epic
priority: 0
---
# EPIC: Native Credential Vault

Replace external CAAM dependency with native Swarm credential vault. Provides secure storage for AI coding agent auth files (Claude, Codex, Gemini), instant profile switching, cooldown tracking, and credential sync across nodes.

Key requirements:
- Store auth files in ~/.config/swarm/vault/profiles/{provider}/{profile}/
- Support backup/activate/switch operations for instant auth switching (<100ms)
- Track cooldowns and usage per profile
- Enable credential sync to remote nodes via SSH
- Remove all CAAM references from codebase

This is critical infrastructure for multi-account management in production deployments.


