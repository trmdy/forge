---
id: swarm-qio2
status: closed
deps: [swarm-s3r0]
links: []
created: 2025-12-23T15:09:53.371141286+01:00
type: task
priority: 2
---
# Add vault documentation

Create documentation for the vault feature.

Files to create/update:

docs/vault.md (new):
- Overview of credential vault
- Why instant switching matters (rate limits, multi-account)
- Directory structure explanation
- Quick start guide
- Command reference with examples
- Security considerations
- Troubleshooting common issues

docs/quickstart.md (update):
- Add vault setup section
- Show backup/activate workflow

docs/cli.md (update):
- Add vault command group
- Document all subcommands

docs/config.md (update):
- Document vault configuration options
- Explain vault: credential reference format

AGENTS.md (update if needed):
- Note about vault for agent credential management

Example content for vault.md:

# Credential Vault

Swarm's credential vault provides instant switching between AI coding agent accounts. When you hit rate limits on Claude Max, GPT Pro, or Gemini, switch to another account in <100ms instead of waiting for browser OAuth.

## Quick Start

# Save your current Claude auth
swarm vault backup claude work

# Login to another account (via Claude's /login)
# Then save it too  
swarm vault backup claude personal

# Now switch instantly
swarm vault activate claude personal  # <100ms

## How It Works

The vault stores copies of auth files:
~/.config/swarm/vault/profiles/
├── anthropic/
│   ├── work/
│   │   └── .claude.json
│   └── personal/
│       └── .claude.json
...


