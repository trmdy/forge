---
id: swarm-j8b1
status: closed
deps: []
links: []
created: 2025-12-22T12:09:53.037259185+01:00
type: bug
priority: 1
---
# Restore ssh.ApplySSHConfig helper (SSH config parsing missing)

ApplySSHConfig exists as stub returning opts unchanged (see internal/ssh/executor.go). Needs real SSH config parsing integration (restore parser from swarm-y6b or implement) so host/user/port/key/proxyjump are applied.


