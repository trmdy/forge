package tmux

import "testing"

func TestHashSnapshot(t *testing.T) {
	hash := HashSnapshot("hello")
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if hash != HashSnapshot("hello") {
		t.Fatal("hash should be stable for same input")
	}

	if hash == HashSnapshot("hello!") {
		t.Fatal("hash should change when input changes")
	}
}
