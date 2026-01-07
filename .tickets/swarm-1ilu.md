---
id: swarm-1ilu
status: closed
deps: []
links: []
created: 2026-01-07T11:41:51.354394562+01:00
type: bug
priority: 1
---
# Loop run fails if log directory missing

Loop entries created by 'forge up' set LogPath, but the loop runner only mkdirs logs when LogPath is empty. If the logs/loops directory doesn't exist yet, loop run fails immediately with 'open ... log: no such file or directory'. Ensure the runner creates parent dirs even when LogPath is already set.


