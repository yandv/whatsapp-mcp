package main

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *MessageStore {
	t.Helper()
	t.Chdir(t.TempDir())
	store, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func countMessages(t *testing.T, store *MessageStore) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMessageStore(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("stores and reads back a text message", func(t *testing.T) {
		store := newTestStore(t)
		if err := store.StoreChat("1@s.whatsapp.net", "Alice", now); err != nil {
			t.Fatal(err)
		}
		if err := store.StoreMessage("M1", "1@s.whatsapp.net", "1", "hello", now, true, "", "", "", nil, nil, nil, 0); err != nil {
			t.Fatal(err)
		}
		msgs, err := store.GetMessages("1@s.whatsapp.net", 10)
		if err != nil || len(msgs) != 1 || msgs[0].Content != "hello" || !msgs[0].IsFromMe {
			t.Fatalf("msgs %+v err %v", msgs, err)
		}
	})
	t.Run("skips a message with neither content nor media", func(t *testing.T) {
		store := newTestStore(t)
		store.StoreChat("1@s.whatsapp.net", "Alice", now)
		if err := store.StoreMessage("M1", "1@s.whatsapp.net", "1", "", now, false, "", "", "", nil, nil, nil, 0); err != nil {
			t.Fatal(err)
		}
		if n := countMessages(t, store); n != 0 {
			t.Errorf("stored %d rows", n)
		}
	})
	t.Run("keeps a media message without text", func(t *testing.T) {
		store := newTestStore(t)
		store.StoreChat("1@s.whatsapp.net", "Alice", now)
		store.StoreMessage("M1", "1@s.whatsapp.net", "1", "", now, false, "image", "a.jpg", "u", []byte{1}, []byte{2}, []byte{3}, 9)
		if n := countMessages(t, store); n != 1 {
			t.Errorf("stored %d rows", n)
		}
	})
	t.Run("storing the same id again replaces the row", func(t *testing.T) {
		store := newTestStore(t)
		store.StoreChat("1@s.whatsapp.net", "Alice", now)
		store.StoreMessage("M1", "1@s.whatsapp.net", "1", "old", now, false, "", "", "", nil, nil, nil, 0)
		store.StoreMessage("M1", "1@s.whatsapp.net", "1", "new", now, false, "", "", "", nil, nil, nil, 0)
		msgs, _ := store.GetMessages("1@s.whatsapp.net", 10)
		if len(msgs) != 1 || msgs[0].Content != "new" {
			t.Errorf("msgs %+v", msgs)
		}
	})
}
