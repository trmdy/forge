# Vault Feature Test Plan

This document provides a comprehensive test plan for the native credential vault feature.

## Test Environment Setup

### Prerequisites
- Swarm binary built: `go build ./cmd/swarm/...`
- At least one AI coding agent installed (Claude, Codex, Gemini, or OpenCode)
- Existing auth files for at least one adapter (logged in)

### Test Data Locations
```
Vault: ~/.config/swarm/vault/
Claude: ~/.claude.json, ~/.config/claude-code/auth.json
Codex: ~/.codex/auth.json (or $CODEX_HOME/auth.json)
Gemini: ~/.gemini/settings.json
OpenCode: ~/.opencode/auth.json
```

---

## 1. Unit Tests (Automated)

Run all automated tests:
```bash
go test ./internal/vault/... -v
```

### 1.1 Path Functions (`paths.go`)
| Test | Command | Expected |
|------|---------|----------|
| ParseAdapter valid | `TestParseAdapter` | "claude" → AdapterClaude |
| ParseAdapter aliases | `TestParseAdapter` | "anthropic" → AdapterClaude |
| ParseAdapter invalid | `TestParseAdapter` | "unknown" → "" |
| Adapter.Provider() | `TestAdapterProvider` | AdapterClaude → "anthropic" |
| AllAdapters | `TestAllAdapters` | Returns 4 adapters |
| DefaultVaultPath | `TestDefaultVaultPath` | Absolute path containing ".config/swarm/vault" |
| ProfilesPath | `TestProfilesPath` | Appends "/profiles" |
| ProfilePath | `TestProfilePath` | Correct nested path |

### 1.2 Profile Functions (`profile.go`)
| Test | Command | Expected |
|------|---------|----------|
| Backup invalid name | `TestBackupInvalidProfileName` | ErrInvalidProfileName |
| Backup no auth | `TestBackupNoAuthFiles` | ErrNoAuthFiles |
| Activate invalid name | `TestActivateInvalidProfileName` | ErrInvalidProfileName |
| Activate missing | `TestActivateProfileNotFound` | ErrProfileNotFound |
| Delete invalid name | `TestDeleteInvalidProfileName` | ErrInvalidProfileName |
| Delete missing | `TestDeleteProfileNotFound` | ErrProfileNotFound |
| Get invalid name | `TestGetInvalidProfileName` | ErrInvalidProfileName |
| Get missing | `TestGetProfileNotFound` | ErrProfileNotFound |
| List empty vault | `TestListEmptyVault` | Empty slice, no error |
| GetActive no profiles | `TestGetActiveNoProfiles` | nil, no error |
| Clear no files | `TestClearNoFiles` | No error |
| CopyFile | `TestCopyFile` | Content matches |
| CopyFile creates dirs | `TestCopyFileCreatesParentDir` | Parent dirs created |
| Profile lifecycle | `TestProfileLifecycle` | Get→List→Delete works |

### 1.3 Encrypted Vault (`vault.go`)
| Test | Command | Expected |
|------|---------|----------|
| Initialize/Unlock | `TestVault_InitializeAndUnlock` | State transitions correct |
| Wrong password | `TestVault_UnlockWrongPassword` | ErrInvalidPassword |
| Store/Retrieve/Delete | `TestVault_StoreRetrieveDelete` | CRUD operations work |
| List secrets | `TestVault_List` | Returns all names |
| Persistence | `TestVault_Persistence` | Survives restart |
| Locked operations | `TestVault_LockedOperations` | ErrVaultLocked |
| Update preserves CreatedAt | `TestVault_UpdateSecret` | CreatedAt unchanged |
| Change password | `TestVault_ChangePassword` | Old fails, new works |
| ResolveCredential | `TestVault_ResolveCredential` | vault:, env:, literal work |

---

## 2. CLI Integration Tests (Manual)

### 2.1 Vault Initialization

```bash
# Test: Initialize vault directory
swarm vault init

# Expected: Creates ~/.config/swarm/vault/profiles/{anthropic,openai,google,opencode}/
# Verify:
ls -la ~/.config/swarm/vault/profiles/
```

```bash
# Test: Re-initialize (idempotent)
swarm vault init

# Expected: No error, directories unchanged
```

### 2.2 Vault Status

```bash
# Test: Status with no profiles
swarm vault status

# Expected output:
# ADAPTER   ACTIVE PROFILE  AUTH FILES  SAVED PROFILES
# claude    (none)          yes         0
# codex     (none)          yes         0
# gemini    (none)          yes         0
# opencode  (none)          no          0
```

```bash
# Test: JSON output
swarm vault status --json

# Expected: Valid JSON array with adapter, has_auth, profile_count
```

### 2.3 Vault Paths

```bash
# Test: Show all paths
swarm vault paths

# Expected: Lists primary and secondary paths for each adapter with (exists) or (not found)
```

```bash
# Test: Show specific adapter
swarm vault paths claude

# Expected: Only Claude paths shown
```

```bash
# Test: Invalid adapter
swarm vault paths invalid

# Expected: Error "unknown adapter: invalid"
```

### 2.4 Backup Operations

```bash
# Test: Backup current Claude auth
swarm vault backup claude work

# Expected:
# Backed up claude auth files to profile "work"
#   Files: .claude.json
#   Path: ~/.config/swarm/vault/profiles/anthropic/work
```

```bash
# Verify backup:
ls -la ~/.config/swarm/vault/profiles/anthropic/work/
cat ~/.config/swarm/vault/profiles/anthropic/work/meta.json

# Expected: .claude.json and meta.json present
```

```bash
# Test: Backup duplicate (should fail)
swarm vault backup claude work

# Expected: Error "profile \"work\" already exists"
```

```bash
# Test: Backup with --force
swarm vault backup claude work --force

# Expected: Success, files updated
```

```bash
# Test: Backup non-existent adapter auth
swarm vault backup opencode test

# Expected: Error about no auth files (if not logged in)
```

### 2.5 List Operations

```bash
# Test: List all profiles
swarm vault list

# Expected:
# ADAPTER  PROFILE  FILES  UPDATED     STATUS
# claude   work     1      just now    active
```

```bash
# Test: List specific adapter
swarm vault list claude

# Expected: Only claude profiles shown
```

```bash
# Test: JSON output
swarm vault list --json

# Expected: Valid JSON array with is_active flag
```

### 2.6 Activate Operations

```bash
# First, create a second profile:
# 1. Log out of Claude or modify ~/.claude.json
# 2. swarm vault backup claude personal

# Test: Activate profile
swarm vault activate claude work

# Expected: "Activated claude profile \"work\""
```

```bash
# Verify: Auth file content should match backup
diff ~/.claude.json ~/.config/swarm/vault/profiles/anthropic/work/.claude.json
# Expected: No differences
```

```bash
# Test: Activate with backup
swarm vault activate claude personal --backup-current

# Expected: Creates backup-YYYYMMDD-HHMMSS profile, then activates personal
```

```bash
# Test: Switch alias
swarm vault switch claude work

# Expected: Same as activate
```

```bash
# Test: Use alias
swarm vault use claude work

# Expected: Same as activate
```

```bash
# Test: Activate non-existent
swarm vault activate claude nonexistent

# Expected: Error "activation failed: profile not found"
```

### 2.7 Delete Operations

```bash
# Test: Delete with confirmation (interactive)
swarm vault delete claude backup-20241223-151234

# Expected: Prompts "Delete profile...? [y/N]:"
```

```bash
# Test: Delete with --force
swarm vault delete claude old-profile --force

# Expected: Immediate deletion, no prompt
```

```bash
# Test: Delete with -y flag
swarm vault delete claude another-profile -y

# Expected: Immediate deletion, no prompt
```

```bash
# Test: Delete non-existent
swarm vault delete claude nonexistent

# Expected: Error "profile not found"
```

### 2.8 Clear Operations

```bash
# WARNING: This removes your auth files!
# Test: Clear with confirmation
swarm vault clear claude

# Expected: Prompts "Remove N auth file(s) for claude? [y/N]:"
```

```bash
# Verify: Auth file removed
ls ~/.claude.json
# Expected: No such file
```

```bash
# Test: Clear with --force
swarm vault clear codex --force

# Expected: Immediate removal, no prompt
```

---

## 3. Credential Reference Tests

### 3.1 vault: Reference Resolution

```bash
# Setup: Create a vault profile
swarm vault backup claude test-profile

# Test: Resolve credential (requires test code or debug)
# In account service, ResolveCredential("vault:claude/test-profile") should:
# 1. Find ~/.config/swarm/vault/profiles/anthropic/test-profile/
# 2. Read .claude.json
# 3. Extract api_key, apiKey, or claudeApiKey field
# 4. Return the API key value
```

### 3.2 Reference Format Variations

| Reference | Expected Behavior |
|-----------|-------------------|
| `vault:claude/work` | Resolves from anthropic/work |
| `vault:anthropic/work` | Resolves from anthropic/work |
| `vault:codex/default` | Resolves from openai/default |
| `vault:openai/default` | Resolves from openai/default |
| `vault:gemini/main` | Resolves from google/main |
| `vault:google/main` | Resolves from google/main |
| `vault:opencode/dev` | Resolves from opencode/dev |
| `vault:invalid/test` | Error: "unknown adapter" |
| `vault:claude` | Error: "expected adapter/profile" |

---

## 4. Performance Tests

### 4.1 Activation Speed (<100ms target)

```bash
# Test: Measure activation time
time swarm vault activate claude work

# Expected: real < 0.1s (100ms)
```

```bash
# Test: Measure with multiple files (Claude has 2)
time swarm vault activate claude work

# Expected: Still < 100ms
```

### 4.2 List Performance

```bash
# Setup: Create 10+ profiles
for i in {1..10}; do
  cp ~/.claude.json ~/.claude.json.bak
  echo '{"test": "'$i'"}' > ~/.claude.json
  swarm vault backup claude "profile-$i" --force
done
mv ~/.claude.json.bak ~/.claude.json

# Test: List all profiles
time swarm vault list

# Expected: < 100ms for 10 profiles
```

---

## 5. Error Handling Tests

### 5.1 Permission Errors

```bash
# Test: Vault directory not writable
chmod 000 ~/.config/swarm/vault/profiles/anthropic
swarm vault backup claude test

# Expected: Permission denied error
chmod 755 ~/.config/swarm/vault/profiles/anthropic  # Restore
```

### 5.2 Corrupted Metadata

```bash
# Test: Invalid meta.json
echo "invalid json" > ~/.config/swarm/vault/profiles/anthropic/work/meta.json
swarm vault list claude

# Expected: Profile skipped or error message
# Restore: swarm vault backup claude work --force
```

### 5.3 Missing Auth Files

```bash
# Test: Profile exists but auth file deleted
rm ~/.config/swarm/vault/profiles/anthropic/work/.claude.json
swarm vault activate claude work

# Expected: Error about missing auth file
```

---

## 6. Backwards Compatibility Tests

### 6.1 CAAM Deprecation Warning

```bash
# Test: Deprecated command shows warning
swarm accounts import-caam --help

# Expected: Shows "Command \"import-caam\" is deprecated..."
```

### 6.2 caam: Reference Still Works

```bash
# If you have a caam vault at ~/.local/share/caam/vault:
# ResolveCredential("caam:claude/user@example.com") should still work
# (Internal test only)
```

---

## 7. Multi-Adapter Tests

### 7.1 Cross-Adapter Operations

```bash
# Test: Profiles don't interfere
swarm vault backup claude work
swarm vault backup codex work
swarm vault backup gemini work

swarm vault list

# Expected: 3 profiles, one per adapter, each named "work"
```

```bash
# Test: Activate one doesn't affect others
swarm vault activate claude work

# Verify: Codex and Gemini auth files unchanged
md5sum ~/.codex/auth.json ~/.gemini/settings.json
# (should match before/after)
```

---

## 8. Edge Case Tests

### 8.1 Special Characters in Profile Names

```bash
# Test: Profile with spaces (should work with quotes)
swarm vault backup claude "my work profile"

# Test: Profile with special chars
swarm vault backup claude "work@company"
swarm vault backup claude "personal-2024"
```

### 8.2 Empty Vault

```bash
# Test: Operations on fresh install
rm -rf ~/.config/swarm/vault
swarm vault status

# Expected: Shows all adapters with 0 saved profiles
```

### 8.3 Concurrent Access

```bash
# Test: Two terminals, simultaneous backup
# Terminal 1: swarm vault backup claude t1
# Terminal 2: swarm vault backup claude t2

# Expected: Both succeed, no corruption
```

---

## 9. Integration with Swarm Accounts

### 9.1 Credential Reference in Config

```yaml
# In config.yaml:
accounts:
  - provider: anthropic
    profile_name: work
    credential_ref: "vault:claude/work"
```

```bash
# Test: Account should resolve credential from vault
swarm accounts list

# Expected: Account shows, credential resolves correctly
```

---

## 10. Cleanup Checklist

After testing, clean up test artifacts:

```bash
# Remove test profiles
swarm vault delete claude profile-1 --force
swarm vault delete claude profile-2 --force
# ... etc

# Or nuclear option:
rm -rf ~/.config/swarm/vault/profiles/*/profile-*
rm -rf ~/.config/swarm/vault/profiles/*/test*
```

---

## Test Results Summary

| Category | Tests | Pass | Fail | Skip |
|----------|-------|------|------|------|
| Unit Tests (automated) | 33 | | | |
| CLI Init/Status/Paths | 8 | | | |
| CLI Backup | 5 | | | |
| CLI List | 3 | | | |
| CLI Activate | 6 | | | |
| CLI Delete | 4 | | | |
| CLI Clear | 2 | | | |
| Credential Resolution | 8 | | | |
| Performance | 2 | | | |
| Error Handling | 3 | | | |
| Backwards Compatibility | 2 | | | |
| Multi-Adapter | 2 | | | |
| Edge Cases | 3 | | | |
| **Total** | **81** | | | |

---

## Notes

- Tests marked with "Internal test only" require code modification or debug access
- Performance tests may vary by disk speed
- Some tests require actual adapter installations (Claude, Codex, etc.)
- Backup your auth files before running destructive tests
