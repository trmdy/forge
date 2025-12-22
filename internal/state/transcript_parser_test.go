package state

import "testing"

func TestParseTranscript(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		want    string
	}{
		{name: "error", input: "panic: boom", want: "error"},
		{name: "rate limit", input: "HTTP 429 rate limit", want: "rate_limited"},
		{name: "approval", input: "Proceed? (y/n)", want: "awaiting_approval"},
		{name: "none", input: "all good", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ParseTranscript(tt.input)
			if tt.wantNil {
				if info != nil {
					t.Fatalf("expected nil, got %v", info.State)
				}
				return
			}
			if info == nil {
				t.Fatal("expected non-nil state info")
			}
			if string(info.State) != tt.want {
				t.Fatalf("expected state %q, got %q", tt.want, info.State)
			}
		})
	}
}
