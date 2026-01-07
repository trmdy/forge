---
id: swarm-9ri3
status: closed
deps: []
links: []
created: 2025-12-27T09:32:11.938183319+01:00
type: bug
priority: 0
---
# Fix go.mod toolchain version (1.25.5 is invalid)

Go.mod declares go 1.25.5 which triggers toolchain download and breaks offline builds.

## From UX_FEEDBACK_2.md - Critical Issue A

## Problem
Your go.mod declares `go 1.25.5`, which:
- Triggers toolchain download
- Breaks offline builds
- Bites every new machine/CI environment

## Fix
1. Set `go` to a real, available version (e.g., `go 1.23.x`)
2. If you want toolchain pinning, use `toolchain go1.23.x` in addition

## Steps
```bash
# In go.mod, change:
go 1.25.5
# To:
go 1.23
toolchain go1.23.4  # or whatever specific version
```

## Verification
- `go build ./...` works offline
- CI builds pass without toolchain download
- New developer onboarding is smooth


