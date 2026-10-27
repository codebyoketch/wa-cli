// Package tui implements wa-cli's full-screen terminal interface,
// launched by running `wa` with no subcommand.
//
// Architecture: whatsmeow's connection stays open for the life of the
// program (like `wa watch`), but instead of printing to stdout directly,
// incoming messages are bridged into Bubble Tea's own event loop via
// tea.Program.Send from internal/whatsapp.Client's OnIncomingMessage
// callback. All chatstore/msgstore reads and the actual send happen in
// tea.Cmd functions (Bubble Tea's mechanism for async work), never
// directly in Update — keeping the UI responsive during network calls.
package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/safety"
	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

// Run connects, launches the full-screen TUI, and blocks until the user
// quits (Ctrl+C). Requires an existing login (`wa login`).
func Run(a *app.App) error {
	ctx := context.Background()

	// whatsmeow logs warnings/errors directly to stdout by default,
	// which corrupts Bubble Tea's alt-screen rendering (raw writes
	// desync Bubble Tea's tracking of what's already on screen, showing
	// up as garbled/duplicated boxes). Redirect stdout to a log file
	// before constructing any waLog logger — including the one
	// internal/whatsapp.New builds for the whatsmeow client itself —
	// then hand Bubble Tea the real terminal explicitly so its own
	// rendering is unaffected by the swap.
	realStdout := os.Stdout
	logPath := filepath.Join(a.Config.DataDir, "tui.log")
	if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		os.Stdout = logFile
		defer func() {
			os.Stdout = realStdout
			logFile.Close()
		}()
	}

	openDebugLog(a.Config.DataDir)

	dbLog := waLog.Stdout("Database", "WARN", true)

	container, err := store.Open(ctx, a.Config.DataDir, dbLog)
	if err != nil {
		return err
	}

	cs := chatstore.New(a.Config.DataDir)
	ms := msgstore.New(a.Config.DataDir)
	guard := safety.New(a.Config.DataDir)

	client, err := whatsapp.New(ctx, container, dbLog, cs, ms)
	if err != nil {
		return err
	}
	if !client.IsLoggedIn() {
		return fmt.Errorf("not logged in: run 'wa login' first")
	}
	client.SetNotifications(a.Config.NotifyEnabled, a.Config.NotifyGroups, a.Config.NotifyShowPreview)

	m := newModel(ctx, client, cs, ms, guard)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(realStdout))

	client.OnIncomingMessage(func(msg msgstore.Message) {
		p.Send(incomingMsg{msg: msg})
	})
	client.WA.AddEventHandler(func(evt interface{}) {
		dlog("wa event %T", evt)
		switch evt.(type) {
		case *events.PushName, *events.Contact, *events.BusinessName, *events.AppStateSyncComplete, *events.HistorySync:
			p.Send(namesChangedMsg{})
		case *events.Connected:
			p.Send(connectedMsg{})
		}
	})

	if err := client.WA.Connect(); err != nil {
		return fmt.Errorf("connecting to WhatsApp: %w", err)
	}
	defer client.Disconnect()

	_, err = p.Run()
	return err
}

type focusArea int

const (
	focusChatList focusArea = iota
	focusInput
)

type model struct {
	ctx    context.Context
	client *whatsapp.Client
	cs     *chatstore.Store
	ms     *msgstore.Store
	guard  *safety.Guard

	chats       []chatstore.Chat
	selectedJID string // highlighted in the chat list; "" means none yet
	openJID     string // shown in the message pane; "" until Enter opens one
	filter      string // typed in the chat list; narrows it by name

	// names maps a 1:1 chat JID to whatsapp.Client.ResolveName's result.
	names map[string]string
	// namesRefreshPending is set while a debounced names reload is
	// scheduled, so a burst of name events triggers only one reload.
	namesRefreshPending bool
	// backfilled records chats whose older history was already requested
	// from the phone this session.
	backfilled map[string]bool
	// groupNames maps a group JID to its real name, fetched via
	// ListGroups since chatstore's own name field for groups only ever
	// comes from HistorySync, which isn't reliable.
	groupNames map[string]string

	messages []displayMsg

	viewport viewport.Model
	input    textinput.Model

	focus         focusArea
	width, height int
	statusLine    string
	ready         bool
	quitting      bool
}

// --- tea.Msg types for async results ---

// displayMsg is a stored message plus its rendered form, computed off the
// UI thread since resolving names touches the database.
type displayMsg struct {
	msgstore.Message
	Sender string // resolved sender name; "" for your own messages
	Body   string // whatsapp.Client.DisplayText output
}

type chatsLoadedMsg struct{ chats []chatstore.Chat }
type namesLoadedMsg struct{ names map[string]string }
type namesChangedMsg struct{}
type refreshNamesMsg struct{}
type groupsLoadedMsg struct{ names map[string]string }
type messagesLoadedMsg struct {
	chatJID string
	msgs    []displayMsg
}
type incomingMsg struct{ msg msgstore.Message }
type connectedMsg struct{}
type sendResultMsg struct{ err error }
type errMsg struct{ err error }

func newModel(ctx context.Context, client *whatsapp.Client, cs *chatstore.Store, ms *msgstore.Store, guard *safety.Guard) model {
	ti := textinput.New()
	ti.Placeholder = "Type a message and press Enter..."
	ti.CharLimit = 4096
	ti.Prompt = "> "

	vp := viewport.New(0, 0)

	return model{
		ctx:         ctx,
		client:      client,
		cs:          cs,
		ms:          ms,
		guard:       guard,
		input:       ti,
		viewport:    vp,
		focus:       focusChatList,
		statusLine:  "Connecting...",
		names:       map[string]string{},
		groupNames:  map[string]string{},
		backfilled:  map[string]bool{},
	}
}

// Init satisfies tea.Model: it kicks off the initial chat list load as an
// async tea.Cmd so the first frame renders immediately. 1:1 chat names
// load once the chat list arrives (see chatsLoadedMsg); groups load once
// connected (see connectedMsg).
func (m model) Init() tea.Cmd {
	return loadChatsCmd(m.cs)
}

// backfillCmd asks the phone for older messages if chatJID's stored
// history doesn't reach back msgstore.RetainFor. It needs at least one
// stored message to anchor the request.
func backfillCmd(ctx context.Context, client *whatsapp.Client, ms *msgstore.Store, chatJID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := ms.List(chatJID)
		if err != nil || len(msgs) == 0 {
			return nil
		}
		if time.Since(time.UnixMilli(msgs[0].Timestamp)) >= msgstore.RetainFor {
			return nil
		}
		if err := client.RequestOlderMessages(ctx, msgs[0], 50); err != nil {
			return errMsg{fmt.Errorf("requesting older messages: %w", err)}
		}
		return nil
	}
}

// loadNamesCmd resolves display names for the given 1:1 chat JIDs from
// local data only (contacts + LID mappings, no network).
func loadNamesCmd(ctx context.Context, client *whatsapp.Client, jids []string) tea.Cmd {
	return func() tea.Msg {
		names := make(map[string]string, len(jids))
		for _, jid := range jids {
			names[jid] = client.ResolveName(ctx, jid)
		}
		return namesLoadedMsg{names}
	}
}

// userChatJIDs returns the JIDs of chats that are 1:1 (not groups),
// optionally only those with no resolved name yet.
func userChatJIDs(chats []chatstore.Chat, skip map[string]string) []string {
	var jids []string
	for _, c := range chats {
		if _, done := skip[c.JID]; !c.IsGroup && !done {
			jids = append(jids, c.JID)
		}
	}
	return jids
}

// loadGroupsCmd fetches real group names via the account's joined-groups
// list (internal/whatsapp.ListGroups, Phase 7). chatstore only ever gets
// a group's name from HistorySync, which has proven unreliable — this
// gives the TUI an authoritative fallback so groups aren't stuck showing
// a blank/JID-only name in the sidebar.
func loadGroupsCmd(ctx context.Context, client *whatsapp.Client) tea.Cmd {
	return func() tea.Msg {
		groups, err := client.ListGroups(ctx)
		if err != nil {
			return errMsg{err}
		}
		names := make(map[string]string, len(groups))
		for _, g := range groups {
			if g.Name != "" {
				names[g.JID] = g.Name
			}
		}
		return groupsLoadedMsg{names}
	}
}

func loadChatsCmd(cs *chatstore.Store) tea.Cmd {
	return func() tea.Msg {
		chats, err := cs.List()
		if err != nil {
			return errMsg{err}
		}
		return chatsLoadedMsg{chats}
	}
}

// loadMessagesCmd loads chatJID's messages for display. markRead is false
// for a preview (just moving the highlight) so browsing doesn't clear
// unread counts.
func loadMessagesCmd(ctx context.Context, client *whatsapp.Client, cs *chatstore.Store, ms *msgstore.Store, chatJID string, markRead bool) tea.Cmd {
	return func() tea.Msg {
		if markRead {
			_ = cs.MarkRead(chatJID)
		}
		msgs, err := ms.List(chatJID)
		if err != nil {
			return errMsg{err}
		}
		out := make([]displayMsg, 0, len(msgs))
		for _, msg := range msgs {
			body, hidden := client.DisplayText(ctx, msg)
			if hidden {
				continue
			}
			dm := displayMsg{Message: msg, Body: body}
			if !msg.FromMe {
				dm.Sender = client.ResolveName(ctx, msg.SenderJID)
			}
			out = append(out, dm)
		}
		return messagesLoadedMsg{chatJID: chatJID, msgs: out}
	}
}

func sendCmd(ctx context.Context, client *whatsapp.Client, ms *msgstore.Store, guard *safety.Guard, jidStr, text string) tea.Cmd {
	return func() tea.Msg {
		jid, err := types.ParseJID(jidStr)
		if err != nil {
			return sendResultMsg{err: err}
		}
		id, err := client.SendText(ctx, jid, text)
		if err != nil {
			return sendResultMsg{err: err}
		}
		// WhatsApp doesn't echo this device's own sends back as events, so
		// save it here (as `wa chat send` does) or it vanishes from history.
		var raw string
		if b, err := proto.Marshal(&waProto.Message{Conversation: proto.String(text)}); err == nil {
			raw = base64.StdEncoding.EncodeToString(b)
		}
		if err := ms.Append(msgstore.Message{
			ID:        id,
			ChatJID:   jidStr,
			SenderJID: "me",
			Timestamp: time.Now().UnixMilli(),
			Text:      text,
			FromMe:    true,
			RawProto:  raw,
		}); err != nil {
			return sendResultMsg{err: fmt.Errorf("sent, but saving to history failed: %w", err)}
		}
		if guard != nil {
			_ = guard.MarkKnown(jidStr)
		}
		return sendResultMsg{err: nil}
	}
}

// Update satisfies tea.Model: it's the single dispatch point for every
// event the TUI reacts to (key presses, window resizes, and the async
// results of the tea.Cmds started from Init or from a previous Update).
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		return m, nil

	case chatsLoadedMsg:
		m.chats = msg.chats
		if m.statusLine == "Connecting..." {
			m.statusLine = ""
		}
		if m.selectedJID == "" && len(m.chats) > 0 {
			m.selectedJID = m.chats[0].JID
		}
		cmds := []tea.Cmd{m.preview()}
		if missing := userChatJIDs(m.chats, m.names); len(missing) > 0 {
			cmds = append(cmds, loadNamesCmd(m.ctx, m.client, missing))
		}
		return m, tea.Batch(cmds...)

	case namesLoadedMsg:
		for jid, name := range msg.names {
			m.names[jid] = name
		}
		return m, nil

	case namesChangedMsg:
		// whatsmeow learned a new name. Wait a second so a burst of
		// these (e.g. a history sync) costs one reload, not hundreds.
		if m.namesRefreshPending {
			return m, nil
		}
		m.namesRefreshPending = true
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshNamesMsg{} })

	case refreshNamesMsg:
		m.namesRefreshPending = false
		cmds := []tea.Cmd{loadNamesCmd(m.ctx, m.client, userChatJIDs(m.chats, nil))}
		if m.openJID != "" {
			cmds = append(cmds, loadMessagesCmd(m.ctx, m.client, m.cs, m.ms, m.openJID, m.focus == focusInput))
		}
		return m, tea.Batch(cmds...)

	case connectedMsg:
		// Fetching joined groups also stores every member's LID->phone
		// mapping, so names can resolve. Only safe once connected.
		return m, loadGroupsCmd(m.ctx, m.client)

	case groupsLoadedMsg:
		m.groupNames = msg.names
		return m, func() tea.Msg { return namesChangedMsg{} }

	case messagesLoadedMsg:
		if msg.chatJID != m.openJID {
			return m, nil // stale response for a chat we've since navigated away from
		}
		m.messages = msg.msgs
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case incomingMsg:
		cmds := []tea.Cmd{loadChatsCmd(m.cs)}
		if msg.msg.ChatJID == m.openJID {
			// Reload rather than append, so it gets the same rendering
			// (names, mentions, reactions) as the rest of the chat.
			cmds = append(cmds, loadMessagesCmd(m.ctx, m.client, m.cs, m.ms, m.openJID, m.focus == focusInput))
		}
		return m, tea.Batch(cmds...)

	case sendResultMsg:
		if msg.err != nil {
			m.statusLine = "send failed: " + msg.err.Error()
		}
		return m, nil

	case errMsg:
		m.statusLine = msg.err.Error()
		return m, nil

	case tea.KeyMsg:
		dlog("key %q focus=%d selected=%s", msg.String(), m.focus, m.selectedJID)
		return m.handleKey(msg)
	}

	// Anything else (e.g. the cursor's blink ticks) belongs to the input.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.focus == focusChatList {
		switch msg.Type {
		case tea.KeyUp:
			m.moveSelection(-1)
		case tea.KeyDown:
			m.moveSelection(1)
		case tea.KeyEnter, tea.KeyTab:
			if m.selectedJID == "" {
				return m, nil
			}
			m.openJID = m.selectedJID
			m.filter = ""
			m.focus = focusInput
			cmds := []tea.Cmd{m.input.Focus(), loadMessagesCmd(m.ctx, m.client, m.cs, m.ms, m.openJID, true)}
			if !m.backfilled[m.openJID] {
				m.backfilled[m.openJID] = true
				cmds = append(cmds, backfillCmd(m.ctx, m.client, m.ms, m.openJID))
			}
			return m, tea.Batch(cmds...)
		case tea.KeyEsc:
			m.setFilter("")
		case tea.KeyBackspace:
			if r := []rune(m.filter); len(r) > 0 {
				m.setFilter(string(r[:len(r)-1]))
			}
		case tea.KeyRunes, tea.KeySpace:
			m.setFilter(m.filter + string(msg.Runes))
		}
		return m, m.preview()
	}

	// focusInput
	switch msg.Type {
	case tea.KeyEsc, tea.KeyTab:
		m.focus = focusChatList
		m.input.Blur()
		m.selectedJID = m.openJID
		return m, nil
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" || m.openJID == "" {
			return m, nil
		}
		m.input.SetValue("")

		// Show it immediately rather than waiting for the send to
		// round-trip — sendResultMsg corrects the status line if it
		// actually failed.
		m.messages = append(m.messages, displayMsg{
			Message: msgstore.Message{
				ChatJID:   m.openJID,
				Text:      text,
				FromMe:    true,
				Timestamp: time.Now().UnixMilli(),
			},
			Body: text,
		})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		return m, sendCmd(m.ctx, m.client, m.ms, m.guard, m.openJID, text)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// preview shows the highlighted chat in the message pane, without marking
// it read. No-op if it's already shown.
func (m *model) preview() tea.Cmd {
	if m.selectedJID == "" || m.selectedJID == m.openJID {
		return nil
	}
	m.openJID = m.selectedJID
	m.messages = nil
	m.viewport.SetContent("")
	return loadMessagesCmd(m.ctx, m.client, m.cs, m.ms, m.openJID, false)
}

// setFilter changes the chat-list filter and highlights the first match.
func (m *model) setFilter(f string) {
	m.filter = f
	if visible := m.visibleChats(); len(visible) > 0 {
		m.selectedJID = visible[0].JID
	}
}

// visibleChats returns the chats whose displayed name contains the filter
// (case-insensitive), in list order.
func (m model) visibleChats() []chatstore.Chat {
	if m.filter == "" {
		return m.chats
	}
	q := strings.ToLower(m.filter)
	var out []chatstore.Chat
	for _, c := range m.chats {
		if strings.Contains(strings.ToLower(m.chatName(c)), q) {
			out = append(out, c)
		}
	}
	return out
}

// chatName is the name shown for c in the chat list.
func (m model) chatName(c chatstore.Chat) string {
	name := c.Name
	if c.IsGroup && name == "" {
		name = m.groupNames[c.JID]
	} else if !c.IsGroup && m.names[c.JID] != "" {
		name = m.names[c.JID]
	}
	if name == "" {
		name = c.JID
	}
	return name
}

func (m *model) moveSelection(delta int) {
	visible := m.visibleChats()
	if len(visible) == 0 {
		return
	}
	idx := 0
	for i, c := range visible {
		if c.JID == m.selectedJID {
			idx = i
			break
		}
	}
	idx = max(0, min(idx+delta, len(visible)-1))
	m.selectedJID = visible[idx].JID
}

func (m *model) layout() {
	mainWidth := m.width - sidebarWidth - 6
	if mainWidth < 10 {
		mainWidth = 10
	}
	inputHeight := 3
	helpHeight := 1
	statusHeight := 1
	vpHeight := m.height - inputHeight - helpHeight - statusHeight - 2
	if vpHeight < 3 {
		vpHeight = 3
	}
	m.viewport.Width = mainWidth
	m.viewport.Height = vpHeight
	m.input.Width = mainWidth - 4
	if len(m.messages) > 0 {
		m.viewport.SetContent(m.renderMessages())
	}
}

// View satisfies tea.Model: it renders the current frame (sidebar,
// message viewport, and input box) from m's state — pure and
// side-effect-free, as Bubble Tea requires.
func (m model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Loading wa-cli...\n"
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderChatList(),
		lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.renderInputBar(),
		),
	)

	var footer []string
	if m.statusLine != "" {
		footer = append(footer, statusStyle.Render(m.statusLine))
	}
	footer = append(footer, helpStyle.Render(
		"type to filter · ↑/↓ move · enter open/send · tab switch focus · esc back · ctrl+c quit"))

	frame := lipgloss.JoinVertical(lipgloss.Left, append([]string{body}, footer...)...)
	dlog("view %d lines, terminal height %d", strings.Count(frame, "\n")+1, m.height)
	return frame
}

func (m model) renderChatList() string {
	var b strings.Builder
	// While the list has focus, typing goes to the search, so show it with
	// a block cursor; otherwise just the title.
	if m.focus == focusChatList {
		search, room := "Search: "+m.filter, sidebarWidth-5 // text width minus cursor
		if over := ansi.StringWidth(search) - room; over > 0 {
			search = ansi.TruncateLeft(search, over+1, "…") // keep the end you're typing
		}
		b.WriteString(chatListHeaderStyle.Render(search) + lipgloss.NewStyle().Reverse(true).Render(" ") + "\n\n")
	} else {
		b.WriteString(chatListHeaderStyle.Render("Chats") + "\n\n")
	}

	visible := m.visibleChats()
	if len(visible) == 0 {
		b.WriteString(chatItemStyle.Render("(none)"))
	}

	// Only draw the chats that fit: a frame taller than the terminal gets
	// its top cut off by Bubble Tea, hiding the selection and input box.
	height := m.viewport.Height + 3
	rows := height - 2 // minus header and blank line
	start := 0
	for i, c := range visible {
		if c.JID == m.selectedJID && i >= rows {
			start = i - rows + 1
			break
		}
	}
	end := min(start+rows, len(visible))

	for _, c := range visible[start:end] {
		name := m.chatName(c)

		// Truncate by display width so each chat stays on one line (a
		// wrapped label would grow the sidebar past the terminal again).
		suffix := ""
		if c.UnreadCount > 0 {
			suffix = fmt.Sprintf(" (%d)", c.UnreadCount)
		}
		label := ansi.Truncate(name, sidebarWidth-4-ansi.StringWidth(suffix), "…") + suffix

		switch {
		case c.JID == m.selectedJID:
			b.WriteString(selectedChatStyle.Render("› "+label) + "\n")
		case c.UnreadCount > 0:
			b.WriteString("  " + unreadStyle.Render(label) + "\n")
		default:
			b.WriteString(chatItemStyle.Render("  "+label) + "\n")
		}
	}

	return sidebarStyle.Width(sidebarWidth).Height(height).Render(b.String())
}

func (m model) renderMessages() string {
	if m.openJID == "" {
		return "Pick a chat and press Enter."
	}
	if len(m.messages) == 0 {
		return "No local history for this chat yet."
	}

	width := m.viewport.Width
	if width <= 0 {
		width = 40
	}

	var b strings.Builder
	for _, msg := range m.messages {
		text := msg.Body
		ts := timestampStyle.Render(time.UnixMilli(msg.Timestamp).Local().Format("15:04"))

		var line string
		var align lipgloss.Position
		if msg.FromMe {
			line = fmt.Sprintf("%s %s", myMsgStyle.Render(text), ts)
			align = lipgloss.Right
		} else {
			name := senderNameStyle.Render(msg.Sender)
			line = fmt.Sprintf("%s %s: %s", ts, name, theirMsgStyle.Render(text))
			align = lipgloss.Left
		}

		b.WriteString(lipgloss.NewStyle().Width(width).Align(align).Render(line) + "\n")
	}
	return b.String()
}

func (m model) renderInputBar() string {
	style := inputBoxStyle
	if m.focus == focusInput {
		style = inputBoxFocusedStyle
	}
	return style.Width(m.viewport.Width).Render(m.input.View())
}
