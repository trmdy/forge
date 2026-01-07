---
id: swarm-gn87
status: closed
deps: [swarm-v419]
links: []
created: 2025-12-23T15:08:49.688856638+01:00
type: task
priority: 1
---
# Implement vault node sync

Add commands to sync vault profiles to/from remote nodes.

Commands:
- swarm vault push <node> [--profile provider/name] - Push vault to remote node
- swarm vault pull <node> [--profile provider/name] - Pull vault from remote node

Implementation:
1. Use existing SSH executor to transfer files
2. Tar/compress vault directory for efficient transfer
3. Support selective sync (single profile or all)
4. Handle conflicts (prompt or --force flag)

Push operation:
1. Tar vault/profiles/ directory (exclude metadata db)
2. SSH to node, extract to ~/.config/swarm/vault/profiles/
3. Preserve file permissions (auth files should be 600)

Pull operation:
1. SSH to node, tar remote vault/profiles/
2. Extract locally, merge or replace based on flags

Security considerations:
- Auth files contain sensitive tokens
- Use secure temp files during transfer
- Clean up on failure
- Warn if transferring to untrusted nodes

Flags:
- --all: Sync all profiles (default: prompt for selection)
- --force: Overwrite without confirmation
- --dry-run: Show what would be transferred


