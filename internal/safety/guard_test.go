package safety

import "testing"

func TestIsKnown_DefaultsFalse(t *testing.T) {
	g := New(t.TempDir())
	known, err := g.IsKnown("1234567890@s.whatsapp.net")
	if err != nil {
		t.Fatalf("IsKnown: %v", err)
	}
	if known {
		t.Fatal("expected unknown recipient to report false")
	}
}

func TestMarkKnown_ThenIsKnown(t *testing.T) {
	g := New(t.TempDir())
	jid := "1234567890@s.whatsapp.net"

	if err := g.MarkKnown(jid); err != nil {
		t.Fatalf("MarkKnown: %v", err)
	}
	known, err := g.IsKnown(jid)
	if err != nil {
		t.Fatalf("IsKnown: %v", err)
	}
	if !known {
		t.Fatal("expected recipient to be known after MarkKnown")
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	jid := "1234567890@s.whatsapp.net"

	g1 := New(dir)
	if err := g1.MarkKnown(jid); err != nil {
		t.Fatalf("MarkKnown: %v", err)
	}

	g2 := New(dir)
	known, err := g2.IsKnown(jid)
	if err != nil {
		t.Fatalf("IsKnown: %v", err)
	}
	if !known {
		t.Fatal("expected state to persist to disk across instances")
	}
}
