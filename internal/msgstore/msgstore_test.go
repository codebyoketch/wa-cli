package msgstore

import "testing"

func TestAppendThenList(t *testing.T) {
	s := New(t.TempDir())
	jid := "a@s.whatsapp.net"

	_ = s.Append(Message{ID: "1", ChatJID: jid, Text: "hello", Timestamp: 100})
	_ = s.Append(Message{ID: "2", ChatJID: jid, Text: "world", Timestamp: 200})

	msgs, err := s.List(jid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" || msgs[1].Text != "world" {
		t.Fatalf("unexpected order: %+v", msgs)
	}
}

func TestAppend_TrimsToMaxPerChat(t *testing.T) {
	s := New(t.TempDir())
	jid := "a@s.whatsapp.net"

	for i := 0; i < MaxPerChat+10; i++ {
		_ = s.Append(Message{ID: string(rune('a' + i%26)), ChatJID: jid, Timestamp: int64(i)})
	}

	msgs, err := s.List(jid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != MaxPerChat {
		t.Fatalf("expected trimmed to %d, got %d", MaxPerChat, len(msgs))
	}
	// Oldest should have been dropped — the most recent MaxPerChat
	// timestamps should be 10..MaxPerChat+9.
	if msgs[0].Timestamp != 10 {
		t.Fatalf("expected oldest remaining timestamp 10, got %d", msgs[0].Timestamp)
	}
}

func TestList_UnknownChatReturnsEmpty(t *testing.T) {
	s := New(t.TempDir())
	msgs, err := s.List("nonexistent@s.whatsapp.net")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty, got %d messages", len(msgs))
	}
}

func TestGet_FindsByID(t *testing.T) {
	s := New(t.TempDir())
	jid := "a@s.whatsapp.net"
	_ = s.Append(Message{ID: "abc123", ChatJID: jid, Text: "target"})
	_ = s.Append(Message{ID: "def456", ChatJID: jid, Text: "other"})

	msg, ok, err := s.Get(jid, "abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || msg.Text != "target" {
		t.Fatalf("expected to find target message, got ok=%v msg=%+v", ok, msg)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := New(t.TempDir())
	_, ok, err := s.Get("a@s.whatsapp.net", "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Fatal("expected not found")
	}
}

func TestChatsAreIsolated(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Append(Message{ID: "1", ChatJID: "a@s.whatsapp.net", Text: "a-msg"})
	_ = s.Append(Message{ID: "2", ChatJID: "b@s.whatsapp.net", Text: "b-msg"})

	aMsgs, _ := s.List("a@s.whatsapp.net")
	bMsgs, _ := s.List("b@s.whatsapp.net")

	if len(aMsgs) != 1 || len(bMsgs) != 1 {
		t.Fatalf("expected 1 message each, got a=%d b=%d", len(aMsgs), len(bMsgs))
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	jid := "a@s.whatsapp.net"

	s1 := New(dir)
	_ = s1.Append(Message{ID: "1", ChatJID: jid, Text: "hi"})

	s2 := New(dir)
	msgs, err := s2.List(jid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "hi" {
		t.Fatalf("expected state to persist, got %+v", msgs)
	}
}
