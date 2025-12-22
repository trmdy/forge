package state

import (
	"context"
	"testing"
)

type fakeSnapshotSource struct {
	content string
}

func (f *fakeSnapshotSource) CapturePane(ctx context.Context, target string, history bool) (string, error) {
	return f.content, nil
}

func TestCaptureSnapshot(t *testing.T) {
	source := &fakeSnapshotSource{content: "hello\nworld\n"}

	snapshot, err := CaptureSnapshot(context.Background(), source, "%1", false)
	if err != nil {
		t.Fatalf("CaptureSnapshot failed: %v", err)
	}
	if snapshot.Content == "" {
		t.Fatal("expected snapshot content")
	}
	if snapshot.Hash == "" {
		t.Fatal("expected snapshot hash")
	}
	if snapshot.CapturedAt.IsZero() {
		t.Fatal("expected captured time")
	}
}
