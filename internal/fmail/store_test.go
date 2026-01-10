package fmail

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreTopicRoundTrip(t *testing.T) {
	root := t.TempDir()
	fixed := time.Date(2026, 1, 10, 15, 30, 0, 0, time.UTC)

	store, err := NewStore(root, WithNow(func() time.Time { return fixed }))
	require.NoError(t, err)

	msg := &Message{
		From: "Alice",
		To:   "Task",
		Body: "hello",
	}
	id, err := store.SaveMessage(msg)
	require.NoError(t, err)
	require.Equal(t, "alice", msg.From)
	require.Equal(t, "task", msg.To)

	path := store.TopicMessagePath("task", id)
	loaded, err := store.ReadMessage(path)
	require.NoError(t, err)
	require.Equal(t, id, loaded.ID)
	require.Equal(t, "task", loaded.To)

	list, err := store.ListTopicMessages("task")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, id, list[0].ID)
}

func TestStoreDMRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	require.NoError(t, err)

	msg := &Message{
		From: "alice",
		To:   "@Bob",
		Body: "hi",
	}
	id, err := store.SaveMessage(msg)
	require.NoError(t, err)

	list, err := store.ListDMMessages("bob")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, id, list[0].ID)
	require.Equal(t, "@bob", list[0].To)
}

func TestStoreMessageSizeLimit(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	require.NoError(t, err)

	tooLarge := strings.Repeat("a", MaxMessageSize)
	msg := &Message{
		From: "alice",
		To:   "task",
		Body: tooLarge,
	}
	_, err = store.SaveMessage(msg)
	require.ErrorIs(t, err, ErrMessageTooLarge)
}
