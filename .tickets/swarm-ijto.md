---
id: swarm-ijto
status: closed
deps: []
links: []
created: 2025-12-28T07:02:53.61594113+01:00
type: task
priority: 1
---
# Rename Go module from github.com/opencode-ai/swarm to github.com/tOgg1/forge

Update the Go module path from github.com/opencode-ai/swarm to github.com/tOgg1/forge.

This involves:
1. Update go.mod module declaration
2. Update all import statements in every .go file (~100+ files)
3. Update proto file (proto/swarmd/v1/swarmd.proto) go_package option
4. Regenerate protobuf files
5. Update any references in documentation or scripts

This is part of the broader rename from Swarm to Forge.


