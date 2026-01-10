package fmail

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateMessageIDFormat(t *testing.T) {
	atomic.StoreUint32(&idCounter, 0)
	now := time.Date(2026, 1, 10, 15, 30, 0, 0, time.UTC)
	id := GenerateMessageID(now)
	require.Equal(t, "20260110-153000-0001", id)
}

func TestGenerateMessageIDConcurrentUnique(t *testing.T) {
	atomic.StoreUint32(&idCounter, 0)
	const total = 500
	now := time.Date(2026, 1, 10, 15, 30, 0, 0, time.UTC)

	results := make(chan string, total)
	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			results <- GenerateMessageID(now)
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, total)
	for id := range results {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = struct{}{}
	}
	require.Len(t, seen, total)
}
