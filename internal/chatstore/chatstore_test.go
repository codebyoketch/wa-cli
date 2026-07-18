package chatstore

import "testing"

func TestUpsertThenGet(t *testing.T) {
	s := New(t.TempDir())
	err := s.Upsert(Chat{JID: "a@s.whatsapp.net", Name: "Alice", LastMessageAt: 100, LastMessagePreview: "hi"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, ok, err := s.Get("a@s.whatsapp.net")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected chat to be found")
	}
	if got.Name != "Alice" || got.LastMessagePreview != "hi" {
		t.Fatalf("unexpected chat: %+v", got)
	}
}

func TestUpsert_PartialUpdateDoesNotClobber(t *testing.T) {
	s := New(t.TempDir())
	jid := "a@s.whatsapp.net"

	if err := s.Upsert(Chat{JID: jid, Name: "Alice", LastMessageAt: 100, LastMessagePreview: "hi"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Partial update: bump unread count only, leave Name/LastMessageAt zero.
	if err := s.Upsert(Chat{JID: jid, UnreadCount: 3}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, _, err := s.Get(jid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Alice" {
		t.Fatalf("expected Name preserved, got %q", got.Name)
	}
	if got.LastMessageAt != 100 {
		t.Fatalf("expected LastMessageAt preserved, got %d", got.LastMessageAt)
	}
	if got.UnreadCount != 3 {
		t.Fatalf("expected UnreadCount updated, got %d", got.UnreadCount)
	}
}

func TestList_SortedByRecency(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Upsert(Chat{JID: "old@s.whatsapp.net", Name: "Old", LastMessageAt: 100})
	_ = s.Upsert(Chat{JID: "new@s.whatsapp.net", Name: "New", LastMessageAt: 200})
	_ = s.Upsert(Chat{JID: "none@s.whatsapp.net", Name: "NoActivity"})

	chats, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(chats) != 3 {
		t.Fatalf("expected 3 chats, got %d", len(chats))
	}
	if chats[0].JID != "new@s.whatsapp.net" || chats[1].JID != "old@s.whatsapp.net" {
		t.Fatalf("unexpected order: %+v", chats)
	}
	if chats[2].JID != "none@s.whatsapp.net" {
		t.Fatalf("expected no-activity chat last, got %+v", chats[2])
	}
}

func TestSearch_MatchesNameCaseInsensitive(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Upsert(Chat{JID: "a@s.whatsapp.net", Name: "Alice Wanjiru"})
	_ = s.Upsert(Chat{JID: "b@s.whatsapp.net", Name: "Bob Otieno"})

	results, err := s.Search("wanjiru")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Alice Wanjiru" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSearch_MatchesJID(t *testing.T) {
	s := New(t.TempDir())
	_ = s.Upsert(Chat{JID: "254700111222@s.whatsapp.net", Name: ""})

	results, err := s.Search("254700111222")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestMarkRead(t *testing.T) {
	s := New(t.TempDir())
	jid := "a@s.whatsapp.net"
	_ = s.Upsert(Chat{JID: jid, Name: "Alice", UnreadCount: 5})

	if err := s.MarkRead(jid); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	got, _, _ := s.Get(jid)
	if got.UnreadCount != 0 {
		t.Fatalf("expected unread cleared, got %d", got.UnreadCount)
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	jid := "a@s.whatsapp.net"

	s1 := New(dir)
	_ = s1.Upsert(Chat{JID: jid, Name: "Alice"})

	s2 := New(dir)
	got, ok, err := s2.Get(jid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || got.Name != "Alice" {
		t.Fatalf("expected state to persist, got ok=%v chat=%+v", ok, got)
	}
}
