package state

import (
	"errors"
	"testing"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestDefaultPollerConfig(t *testing.T) {
	config := DefaultPollerConfig()

	if config.ActiveInterval <= 0 {
		t.Error("expected positive ActiveInterval")
	}
	if config.IdleInterval <= 0 {
		t.Error("expected positive IdleInterval")
	}
	if config.InactiveInterval <= 0 {
		t.Error("expected positive InactiveInterval")
	}
	if config.MaxConcurrentPolls <= 0 {
		t.Error("expected positive MaxConcurrentPolls")
	}
	if config.FailureBackoffBase <= 0 {
		t.Error("expected positive FailureBackoffBase")
	}
	if config.FailureBackoffMax <= 0 {
		t.Error("expected positive FailureBackoffMax")
	}
}

func TestPollerShouldPoll(t *testing.T) {
	config := PollerConfig{
		ActiveInterval:     100 * time.Millisecond,
		IdleInterval:       200 * time.Millisecond,
		InactiveInterval:   500 * time.Millisecond,
		MaxConcurrentPolls: 5,
	}

	p := NewPoller(config, nil, nil)
	now := time.Now()

	tests := []struct {
		name       string
		agent      *models.Agent
		lastPolled time.Time
		expect     bool
	}{
		{
			name:       "working agent never polled",
			agent:      &models.Agent{ID: "a1", State: models.AgentStateWorking},
			lastPolled: time.Time{},
			expect:     true,
		},
		{
			name:       "working agent recently polled",
			agent:      &models.Agent{ID: "a2", State: models.AgentStateWorking},
			lastPolled: now.Add(-50 * time.Millisecond),
			expect:     false,
		},
		{
			name:       "working agent poll due",
			agent:      &models.Agent{ID: "a3", State: models.AgentStateWorking},
			lastPolled: now.Add(-150 * time.Millisecond),
			expect:     true,
		},
		{
			name:       "idle agent recently polled",
			agent:      &models.Agent{ID: "a4", State: models.AgentStateIdle},
			lastPolled: now.Add(-100 * time.Millisecond),
			expect:     false,
		},
		{
			name:       "idle agent poll due",
			agent:      &models.Agent{ID: "a5", State: models.AgentStateIdle},
			lastPolled: now.Add(-250 * time.Millisecond),
			expect:     true,
		},
		{
			name:       "paused agent recently polled",
			agent:      &models.Agent{ID: "a6", State: models.AgentStatePaused},
			lastPolled: now.Add(-300 * time.Millisecond),
			expect:     false,
		},
		{
			name:       "paused agent poll due",
			agent:      &models.Agent{ID: "a7", State: models.AgentStatePaused},
			lastPolled: now.Add(-600 * time.Millisecond),
			expect:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up poll state
			if !tt.lastPolled.IsZero() {
				p.pollStates[tt.agent.ID] = &agentPollState{
					agentID:      tt.agent.ID,
					lastPolledAt: tt.lastPolled,
				}
			}

			got := p.shouldPoll(tt.agent, now)
			if got != tt.expect {
				t.Errorf("shouldPoll() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestPollerShouldPollRespectsBackoff(t *testing.T) {
	p := NewPoller(DefaultPollerConfig(), nil, nil)
	now := time.Now()

	p.pollStates["a1"] = &agentPollState{
		agentID:      "a1",
		lastPolledAt: now.Add(-10 * time.Second),
		nextPollAt:   now.Add(10 * time.Second),
	}

	agent := &models.Agent{ID: "a1", State: models.AgentStateWorking}
	if p.shouldPoll(agent, now) {
		t.Error("expected backoff to skip polling")
	}

	if !p.shouldPoll(agent, now.Add(11*time.Second)) {
		t.Error("expected polling after backoff elapses")
	}
}

func TestPollerBackoffDuration(t *testing.T) {
	config := PollerConfig{
		FailureBackoffBase: 1 * time.Second,
		FailureBackoffMax:  5 * time.Second,
	}
	p := NewPoller(config, nil, nil)

	tests := []struct {
		count int
		want  time.Duration
	}{
		{count: 0, want: 0},
		{count: 1, want: 1 * time.Second},
		{count: 2, want: 2 * time.Second},
		{count: 3, want: 4 * time.Second},
		{count: 4, want: 5 * time.Second},
		{count: 5, want: 5 * time.Second},
	}

	for _, tt := range tests {
		got := p.backoffDuration(tt.count)
		if got != tt.want {
			t.Errorf("backoffDuration(%d) = %v, want %v", tt.count, got, tt.want)
		}
	}
}

func TestPollerFailureMarksStale(t *testing.T) {
	p := NewPoller(DefaultPollerConfig(), nil, nil)
	p.recordPollFailure("agent-1", errors.New("boom"))

	state := p.pollStates["agent-1"]
	if state == nil {
		t.Fatal("expected poll state to exist")
	}
	if !state.stale {
		t.Fatal("expected agent to be marked stale")
	}
	if state.failureCount != 1 {
		t.Fatalf("expected failureCount 1, got %d", state.failureCount)
	}
	if state.nextPollAt.IsZero() {
		t.Fatal("expected nextPollAt to be set")
	}
	if state.nextPollAt.Before(state.lastPolledAt) {
		t.Fatal("expected nextPollAt after lastPolledAt")
	}
}
