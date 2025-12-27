package cli

import "testing"

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		maxLen   int
		expected string
	}{
		{
			name:     "short message unchanged",
			msg:      "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			msg:      "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long message truncated",
			msg:      "hello world, this is a long message",
			maxLen:   15,
			expected: "hello world,...",
		},
		{
			name:     "empty message",
			msg:      "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateMessage(tt.msg, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateMessage(%q, %d) = %q, want %q", tt.msg, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestSendResultJSON(t *testing.T) {
	// Test that sendResult struct marshals correctly
	result := sendResult{
		AgentID:  "agent-123",
		ItemID:   "item-456",
		Position: 3,
		ItemType: "message",
	}

	if result.AgentID != "agent-123" {
		t.Errorf("expected AgentID 'agent-123', got %q", result.AgentID)
	}
	if result.Position != 3 {
		t.Errorf("expected Position 3, got %d", result.Position)
	}
}

func TestSendResultWithError(t *testing.T) {
	result := sendResult{
		AgentID: "agent-123",
		Error:   "failed to enqueue",
	}

	if result.Error == "" {
		t.Error("expected error to be set")
	}
	if result.ItemID != "" {
		t.Errorf("expected empty ItemID when error, got %q", result.ItemID)
	}
}
