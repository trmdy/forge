---
id: swarm-d6di
status: closed
deps: [swarm-v419, swarm-hnyo]
links: []
created: 2025-12-23T15:09:39.70773214+01:00
type: task
priority: 1
---
# Integrate vault with account service

Connect the vault package with the account service for unified credential management.

Integration points:

1. Account creation from vault profile:
   - When a vault profile exists, auto-create corresponding account
   - Account.CredentialRef = 'vault:provider/profile'
   - Link cooldown tracking between account and vault profile

2. Auto-rotation with vault:
   - When account hits rate limit, mark vault profile as on cooldown
   - RotateAccount should activate the next vault profile
   - Emit events for profile switches

3. Sync cooldown state:
   - Store cooldown in both account service and vault metadata
   - On startup, reconcile cooldown state
   - ClearCooldown should clear in both places

4. Agent spawn integration:
   - When spawning agent, resolve vault: credential ref
   - Activate the correct vault profile before spawn
   - Pass resolved API key as environment variable

New functions in account service:
- SyncFromVault(ctx, vault) - Import vault profiles as accounts
- ActivateForAgent(ctx, accountID, agentID) - Activate vault profile and track usage

Configuration:
- Add vault.auto_sync: bool to config
- Add vault.path: string to override default location

This enables the flow:
1. User runs 'swarm vault backup claude work'
2. Account auto-created with credential_ref='vault:anthropic/work'
3. Agent spawned uses this account
4. Rate limit hit -> cooldown set -> next profile activated


