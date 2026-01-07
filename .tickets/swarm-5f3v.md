---
id: swarm-5f3v
status: closed
deps: [swarm-18rc]
links: []
created: 2025-12-28T12:25:12.695866637+01:00
type: task
priority: 2
---
# Add systemd service file for forged

Create a systemd service file for running forged on Linux systems.

**File**: contrib/systemd/forged.service (new file)

**Changes Required**:
1. Create service file with Type=simple, Restart=on-failure
2. Use template unit (forged@.service) for per-user instances
3. Add security hardening (NoNewPrivileges, PrivateTmp)
4. Output to journal
5. Create README with installation instructions

**Installation steps to document**:
- Copy to /etc/systemd/system/forged@.service
- systemctl enable forged@$USER
- systemctl start forged@$USER

See docs/design/scheduler-daemon-tasks.md#task-51 for full details.


