// Package tui implements the Forge terminal user interface.
package tui

import (
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/models"
)

func sampleTranscriptLines() ([]string, []time.Time) {
	content := sampleTranscript()
	lines := strings.Split(content, "\n")
	now := time.Now()
	start := now.Add(-time.Duration(len(lines)) * 8 * time.Second)
	timestamps := make([]time.Time, len(lines))
	for i := range lines {
		timestamps[i] = start.Add(time.Duration(i) * 8 * time.Second)
	}
	return lines, timestamps
}

func sampleTranscript() string {
	return `$ opencode
[info] Starting OpenCode agent...
[info] Model: gpt-5
[info] Workspace: /home/user/projects/api-service

> Analyzing codebase structure...

Found 42 files in src/
Found 15 files in tests/

> Running initial assessment...

The codebase appears to be a REST API service with:
- Express.js server setup
- PostgreSQL database connection
- JWT authentication middleware
- Basic CRUD endpoints for users and products

> Ready for instructions.

$ What tests are failing?

> Checking test results...

Running: npm test

` + "```" + `
FAIL src/users.test.js
  ✓ should create a new user (45ms)
  ✗ should validate email format (12ms)
    Expected: validation error
    Received: success

PASS src/products.test.js
  ✓ should list products (23ms)
  ✓ should filter by category (31ms)
` + "```" + `

Error: 1 test failed in users.test.js
The email validation test expects an error but receives success.

> Investigating the issue...

Looking at src/validators/email.js:
The regex pattern is missing the TLD validation.

> Suggested fix:

` + "```javascript" + `
// Current (incorrect)
const emailRegex = /^[^@]+@[^@]+$/

// Fixed
const emailRegex = /^[^@]+@[^@]+\.[a-zA-Z]{2,}$/
` + "```" + `

? Apply this fix? [y/n]
`
}

func sampleQueueEditors() map[string]*queueEditorState {
	agents := []string{"Agent A1", "Agent B7", "Agent C3", "Agent D4"}
	editors := make(map[string]*queueEditorState, len(agents))
	for _, agent := range agents {
		editors[agent] = &queueEditorState{
			Items:       sampleQueueItems(),
			Selected:    0,
			Expanded:    make(map[string]bool),
			DeleteIndex: -1,
		}
	}
	return editors
}

func sampleQueueItems() []queueItem {
	return []queueItem{
		{
			ID:      "q-01",
			Kind:    models.QueueItemTypeMessage,
			Summary: "Review PR comments and propose fixes.",
			Status:  models.QueueItemStatusPending,
		},
		{
			ID:            "q-02",
			Kind:          models.QueueItemTypeConditional,
			Summary:       "Continue with deployment checklist.",
			Status:        models.QueueItemStatusPending,
			ConditionType: models.ConditionTypeWhenIdle,
		},
		{
			ID:              "q-03",
			Kind:            models.QueueItemTypePause,
			Summary:         "Waiting on reviewer",
			Status:          models.QueueItemStatusPending,
			DurationSeconds: 90,
		},
		{
			ID:       "q-04",
			Kind:     models.QueueItemTypeMessage,
			Summary:  "Draft follow-up message to the team.",
			Status:   models.QueueItemStatusDispatched,
			Attempts: 1,
		},
		{
			ID:       "q-05",
			Kind:     models.QueueItemTypeMessage,
			Summary:  "Summarize latest PR feedback and next steps.",
			Status:   models.QueueItemStatusFailed,
			Attempts: 2,
			Error:    "Rate limited by provider",
		},
	}
}

func sampleApprovals() map[string][]approvalItem {
	now := time.Now()
	return map[string][]approvalItem{
		"ws-1": {
			{
				ID:          "appr-101",
				Agent:       "Agent A1",
				RequestType: models.ApprovalRequestType("file_changes"),
				Summary:     "Apply config changes to auth middleware.",
				Status:      models.ApprovalStatusPending,
				Risk:        "medium",
				Details:     "Agent proposes edits to `auth.go` to allow token refresh.",
				Snippet:     "+ if tokenExpired {\n+   return refreshToken(ctx)\n+ }",
				CreatedAt:   now.Add(-8 * time.Minute),
			},
		},
		"ws-2": {
			{
				ID:          "appr-204",
				Agent:       "Agent B7",
				RequestType: models.ApprovalRequestType("shell_command"),
				Summary:     "Run database migration.",
				Status:      models.ApprovalStatusPending,
				Risk:        "high",
				Details:     "Command will apply pending migrations to production DB.",
				Snippet:     "make migrate-up ENV=prod",
				CreatedAt:   now.Add(-3 * time.Minute),
			},
			{
				ID:          "appr-205",
				Agent:       "Agent B7",
				RequestType: models.ApprovalRequestType("file_changes"),
				Summary:     "Update README badges and version.",
				Status:      models.ApprovalStatusPending,
				Risk:        "low",
				Details:     "Documentation-only change requested.",
				Snippet:     "+ Forge v0.3.1",
				CreatedAt:   now.Add(-1 * time.Minute),
			},
		},
	}
}

func sampleAuditItems() []auditItem {
	now := time.Now()
	return []auditItem{
		{
			ID:         "evt-128",
			Timestamp:  now.Add(-2 * time.Minute),
			Type:       models.EventTypeMessageDispatched,
			EntityType: models.EntityTypeQueue,
			EntityID:   "q-02",
			Summary:    "Dispatched queued message to Agent A1.",
			Detail:     "Queue item q-02 sent to agent-a1; awaiting response.",
		},
		{
			ID:         "evt-127",
			Timestamp:  now.Add(-4 * time.Minute),
			Type:       models.EventTypeApprovalRequested,
			EntityType: models.EntityTypeAgent,
			EntityID:   "agent-b7",
			Summary:    "Approval requested for file changes.",
			Detail:     "Agent B7 proposed edits to auth middleware in ws-2.",
		},
		{
			ID:         "evt-124",
			Timestamp:  now.Add(-7 * time.Minute),
			Type:       models.EventTypeAgentStateChanged,
			EntityType: models.EntityTypeAgent,
			EntityID:   "agent-a1",
			Summary:    "Agent moved to Working.",
			Detail:     "State changed from Idle to Working (confidence: high).",
		},
		{
			ID:         "evt-119",
			Timestamp:  now.Add(-12 * time.Minute),
			Type:       models.EventTypeWorkspaceCreated,
			EntityType: models.EntityTypeWorkspace,
			EntityID:   "ws-4",
			Summary:    "Workspace ml-models created.",
			Detail:     "Workspace bound to gpu-box at /home/user/projects/ml-models.",
		},
		{
			ID:         "evt-115",
			Timestamp:  now.Add(-18 * time.Minute),
			Type:       models.EventTypeRateLimitDetected,
			EntityType: models.EntityTypeAccount,
			EntityID:   "acct-primary",
			Summary:    "Rate limit detected for primary account.",
			Detail:     "Cooldown started for 5 minutes (provider: openai).",
		},
		{
			ID:         "evt-109",
			Timestamp:  now.Add(-25 * time.Minute),
			Type:       models.EventTypeNodeOffline,
			EntityType: models.EntityTypeNode,
			EntityID:   "prod-02",
			Summary:    "Node went offline.",
			Detail:     "Lost heartbeat from prod-02; last seen 3m ago.",
		},
		{
			ID:         "evt-102",
			Timestamp:  now.Add(-40 * time.Minute),
			Type:       models.EventTypeAgentSpawned,
			EntityType: models.EntityTypeAgent,
			EntityID:   "agent-c3",
			Summary:    "Agent spawned for data-pipeline.",
			Detail:     "Codex agent started on workspace ws-3.",
		},
		{
			ID:         "evt-097",
			Timestamp:  now.Add(-55 * time.Minute),
			Type:       models.EventTypeError,
			EntityType: models.EntityTypeSystem,
			EntityID:   "scheduler",
			Summary:    "Scheduler error.",
			Detail:     "Dispatch loop encountered timeout contacting tmux.",
		},
	}
}

func sampleMailboxThreads() []mailThread {
	now := time.Now()
	return []mailThread{
		{
			ID:      "thread-201",
			Subject: "Approval needed: auth middleware update",
			Messages: []mailMessage{
				{
					ID:        "msg-701",
					From:      "Agent B7",
					Body:      "Requesting approval for auth middleware changes in ws-2.",
					CreatedAt: now.Add(-35 * time.Minute),
					Read:      true,
				},
				{
					ID:        "msg-702",
					From:      "Operator",
					Body:      "Please include the diff summary before approval.",
					CreatedAt: now.Add(-30 * time.Minute),
					Read:      true,
				},
				{
					ID:        "msg-703",
					From:      "Agent B7",
					Body:      "Diff summary: auth.go token refresh guard added; tests pending.",
					CreatedAt: now.Add(-25 * time.Minute),
					Read:      false,
				},
			},
		},
		{
			ID:      "thread-202",
			Subject: "Rate limit cooldown applied",
			Messages: []mailMessage{
				{
					ID:        "msg-710",
					From:      "Scheduler",
					Body:      "Cooldown started for account acct-primary (5 minutes).",
					CreatedAt: now.Add(-18 * time.Minute),
					Read:      false,
				},
			},
		},
		{
			ID:      "thread-203",
			Subject: "Workspace ws-3 error triage",
			Messages: []mailMessage{
				{
					ID:        "msg-720",
					From:      "Agent C3",
					Body:      "Investigating error from tmux capture; may be stale session.",
					CreatedAt: now.Add(-50 * time.Minute),
					Read:      true,
				},
				{
					ID:        "msg-721",
					From:      "Operator",
					Body:      "Restart session if no heartbeat in 10 minutes.",
					CreatedAt: now.Add(-45 * time.Minute),
					Read:      true,
				},
			},
		},
	}
}

var _ = []any{
	sampleTranscriptLines,
	sampleTranscript,
	sampleQueueEditors,
	sampleQueueItems,
	sampleApprovals,
	sampleAuditItems,
	sampleMailboxThreads,
}
