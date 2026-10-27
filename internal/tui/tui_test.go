package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
)

// visibleLines mimics Bubble Tea's standard renderer: a frame taller than
// the terminal keeps only its bottom `height` lines.
func visibleLines(view string, height int) []string {
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return lines
}

func typeKeys(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestFilterOpenAndEsc(t *testing.T) {
	var m tea.Model = newModel(nil, nil, nil, nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(chatsLoadedMsg{chats: []chatstore.Chat{
		{JID: "1@s.whatsapp.net", Name: "Alice", LastMessageAt: 3},
		{JID: "2@g.us", Name: "Jobs Group", IsGroup: true, LastMessageAt: 2},
		{JID: "3@s.whatsapp.net", Name: "Bob", LastMessageAt: 1},
	}})

	// Moving the highlight previews the chat but keeps focus on the list.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.(model); got.selectedJID != "2@g.us" || got.openJID != "2@g.us" || got.focus != focusChatList || cmd == nil {
		t.Fatalf("after down: selected=%q open=%q focus=%d", got.selectedJID, got.openJID, got.focus)
	}

	// Tab switches focus to the input and back.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := m.(model); got.focus != focusInput || got.openJID != "2@g.us" {
		t.Fatalf("tab from list: focus=%d open=%q", got.focus, got.openJID)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := m.(model); got.focus != focusChatList {
		t.Fatalf("tab from input: focus=%d", got.focus)
	}

	// "j" filters (option 1: arrows only move), highlighting the first match.
	m = typeKeys(m, "jo")
	got := m.(model)
	if got.filter != "jo" || len(got.visibleChats()) != 1 || got.selectedJID != "2@g.us" {
		t.Fatalf("filter %q: visible=%d selected=%q", got.filter, len(got.visibleChats()), got.selectedJID)
	}
	if !strings.Contains(m.View(), "Search: jo") || strings.Contains(m.View(), "Alice") {
		t.Fatalf("view doesn't show only the filtered list")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.(model); got.filter != "j" {
		t.Fatalf("backspace: filter=%q", got.filter)
	}
	m = typeKeys(m, "ob")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.(model); got.filter != "" || len(got.visibleChats()) != 3 {
		t.Fatalf("esc in list should clear filter, got %q", got.filter)
	}

	// Enter opens the highlighted chat, clears the filter, focuses input.
	m = typeKeys(m, "bob")
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = m.(model)
	if got.openJID != "3@s.whatsapp.net" || got.focus != focusInput || got.filter != "" || cmd == nil {
		t.Fatalf("enter: open=%q focus=%d filter=%q cmd=%v", got.openJID, got.focus, got.filter, cmd != nil)
	}

	// Letters now go to the input box, not the filter.
	m = typeKeys(m, "hi")
	if got := m.(model); got.input.Value() != "hi" || got.filter != "" {
		t.Fatalf("typing in input: value=%q filter=%q", got.input.Value(), got.filter)
	}

	// Esc returns to the list with the open chat highlighted.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.(model); got.focus != focusChatList || got.selectedJID != "3@s.whatsapp.net" {
		t.Fatalf("esc from input: focus=%d selected=%q", got.focus, got.selectedJID)
	}
}

func TestCursorVisibleInListAndInput(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	const reverse = "\x1b[7m"

	var m tea.Model = newModel(nil, nil, nil, nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(chatsLoadedMsg{chats: []chatstore.Chat{{JID: "1@s.whatsapp.net", Name: "Alice"}}})
	if !strings.Contains(m.View(), reverse) {
		t.Fatal("no cursor shown while the chat list has focus")
	}

	m = typeKeys(m, strings.Repeat("x", 40))
	view := m.View()
	if n := strings.Count(view, "\n") + 1; n > 30 {
		t.Fatalf("long search grew the frame to %d lines", n)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), reverse) {
		t.Fatal("no cursor shown in the focused input box")
	}
}

func TestSelectionVisibleWithManyChats(t *testing.T) {
	m := newModel(nil, nil, nil, nil, nil)
	var updated tea.Model = m
	updated, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	chats := make([]chatstore.Chat, 921)
	for i := range chats {
		chats[i] = chatstore.Chat{JID: fmt.Sprintf("%d@s.whatsapp.net", i), Name: fmt.Sprintf("chat %d", i), LastMessageAt: int64(10000 - i)}
		if i%3 == 0 {
			chats[i].Name = fmt.Sprintf("chat %d with a very long name 🎉🎉🎉", i)
			chats[i].UnreadCount = 123
		}
	}
	updated, _ = updated.Update(chatsLoadedMsg{chats: chats})

	for i := 1; i <= 40; i++ {
		updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
		view := updated.View()
		if n := strings.Count(view, "\n") + 1; n > 30 {
			t.Fatalf("after %d downs: frame is %d lines tall in a 30-line terminal", i, n)
		}
		want := fmt.Sprintf("› chat %d", i)
		if !strings.Contains(strings.Join(visibleLines(view, 30), "\n"), want) {
			t.Fatalf("after %d downs: selected chat %q is not on screen", i, want)
		}
	}
}
