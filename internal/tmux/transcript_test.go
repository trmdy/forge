package tmux

import "testing"

func TestTranscriptBufferAppendTrim(t *testing.T) {
	buf := NewTranscriptBuffer(3)
	buf.Append("one\ntwo")
	buf.Append("three\nfour")

	if got := buf.Lines(); len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}

	expected := []string{"two", "three", "four"}
	for i, want := range expected {
		if got := buf.Lines()[i]; got != want {
			t.Fatalf("line %d = %q, want %q", i, got, want)
		}
	}
}

func TestTranscriptBufferString(t *testing.T) {
	buf := NewTranscriptBuffer(2)
	buf.Append("alpha")
	buf.Append("beta")

	if got := buf.String(); got != "alpha\nbeta" {
		t.Fatalf("unexpected string: %q", got)
	}
}

func TestTranscriptBufferReset(t *testing.T) {
	buf := NewTranscriptBuffer(2)
	buf.Append("alpha")
	buf.Reset()

	if got := buf.Lines(); len(got) != 0 {
		t.Fatalf("expected empty buffer, got %d lines", len(got))
	}
}
