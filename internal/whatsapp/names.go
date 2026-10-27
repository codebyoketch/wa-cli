package whatsapp

import (
	"context"
	"fmt"
	"strings"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

// ResolveName returns the best display name for a user JID string, using
// only local data (whatsmeow's contact and LID stores, no network):
// address-book full name, first name, push name, business name, then the
// formatted phone number, then the raw ID. An @lid JID is first mapped to
// its phone-number JID, since address-book names are stored under that.
// Not meant for group JIDs — those have no contact entry and fall
// through to the raw ID.
func (c *Client) ResolveName(ctx context.Context, jidStr string) string {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return jidStr
	}
	jid = jid.ToNonAD()

	pn := jid
	if jid.Server == types.HiddenUserServer {
		if mapped, err := c.WA.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !mapped.IsEmpty() {
			pn = mapped
		}
	}

	var infos []types.ContactInfo
	if pn != jid {
		if info, err := c.WA.Store.Contacts.GetContact(ctx, pn); err == nil {
			infos = append(infos, info)
		}
	}
	if info, err := c.WA.Store.Contacts.GetContact(ctx, jid); err == nil {
		infos = append(infos, info)
	}
	if name := pickName(infos...); name != "" {
		return name
	}
	if pn.Server == types.DefaultUserServer {
		return formatPhone(pn.User)
	}
	return jid.User
}

// pickName returns the first non-empty name across infos, trying each
// field in priority order across all infos before moving to the next
// field (so a phone contact's push name doesn't beat a LID contact's
// address-book name).
func pickName(infos ...types.ContactInfo) string {
	fields := []func(types.ContactInfo) string{
		func(ci types.ContactInfo) string { return ci.FullName },
		func(ci types.ContactInfo) string { return ci.FirstName },
		func(ci types.ContactInfo) string { return ci.PushName },
		func(ci types.ContactInfo) string { return ci.BusinessName },
	}
	for _, field := range fields {
		for _, ci := range infos {
			if name := field(ci); name != "" && name != "-" {
				return name
			}
		}
	}
	return ""
}

// formatPhone renders a JID's digits as a phone number: Indian numbers as
// "+91 95419 88522", everything else as plain "+<digits>".
func formatPhone(digits string) string {
	if len(digits) == 12 && strings.HasPrefix(digits, "91") {
		return "+91 " + digits[2:7] + " " + digits[7:]
	}
	return "+" + digits
}

// DisplayText renders a stored message for display: text and captions,
// [media] tags, reactions, edits, deletes, and @mentions as @name. hidden
// is true for pure protocol messages (key distribution, history-sync
// notices, ...) that aren't something a person sent and shouldn't be
// shown at all.
func (c *Client) DisplayText(ctx context.Context, m msgstore.Message) (text string, hidden bool) {
	return displayText(decodeRawProto(m.RawProto), m, func(jid string) string {
		return c.ResolveName(ctx, jid)
	})
}

func displayText(msg *waProto.Message, m msgstore.Message, resolve func(string) string) (string, bool) {
	if msg == nil {
		// No raw content (e.g. a message just sent from the TUI) — fall
		// back to what msgstore recorded.
		return withMediaTag(m.MediaType, m.Text), false
	}
	if isProtocolOnly(msg) {
		return "", true
	}

	// REVOKE is enum value 0, which is also what GetType() returns on a
	// nil ProtocolMessage — so check presence before comparing types.
	pm := msg.GetProtocolMessage()
	var text string
	switch {
	case msg.GetReactionMessage() != nil:
		if r := msg.GetReactionMessage().GetText(); r != "" {
			text = "[reaction " + r + "]"
		} else {
			text = "[reaction removed]"
		}
	case pm != nil && pm.GetType() == waProto.ProtocolMessage_REVOKE:
		text = "[deleted]"
	case pm != nil && pm.GetType() == waProto.ProtocolMessage_MESSAGE_EDIT:
		edited := pm.GetEditedMessage()
		text = strings.TrimSpace("[edited] " + withMediaTag(classifyMessage(edited)))
		msg = edited // mentions live on the edited content
	case msg.GetSecretEncryptedMessage() != nil:
		text = "[edited]"
	case msg.GetTemplateMessage() != nil:
		text = msg.GetTemplateMessage().GetHydratedTemplate().GetHydratedContentText()
		if text == "" {
			text = "[template]"
		}
	default:
		text = withMediaTag(classifyMessage(msg))
		if text == "" {
			text = "[" + firstFieldName(msg) + "]"
		}
	}

	for _, jid := range mentionedJIDs(msg) {
		if at := strings.Index(jid, "@"); at > 0 {
			text = strings.ReplaceAll(text, "@"+jid[:at], "@"+resolve(jid))
		}
	}
	return text, false
}

func withMediaTag(mediaType, text string) string {
	switch {
	case mediaType != "" && text != "":
		return fmt.Sprintf("[%s] %s", mediaType, text)
	case mediaType != "":
		return fmt.Sprintf("[%s]", mediaType)
	}
	return text
}

// isProtocolOnly reports whether msg carries nothing a person sent: only
// encryption-key distribution / message metadata, or a protocolMessage
// other than a delete or edit.
func isProtocolOnly(msg *waProto.Message) bool {
	if msg == nil {
		return true
	}
	if pm := msg.GetProtocolMessage(); pm != nil {
		return pm.GetType() != waProto.ProtocolMessage_REVOKE && pm.GetType() != waProto.ProtocolMessage_MESSAGE_EDIT
	}
	return firstFieldName(msg) == ""
}

// firstFieldName returns the name of the first content field set on msg,
// ignoring plumbing fields, with the "Message" suffix trimmed (so
// locationMessage -> "location"). "" if there is no content field.
func firstFieldName(msg *waProto.Message) string {
	var name string
	msg.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		switch fd.Name() {
		case "messageContextInfo", "senderKeyDistributionMessage", "fastRatchetKeySenderKeyDistributionMessage":
			return true
		}
		name = strings.TrimSuffix(string(fd.Name()), "Message")
		return false
	})
	return name
}

// mentionedJIDs returns the JIDs @mentioned in msg's text or caption.
func mentionedJIDs(msg *waProto.Message) []string {
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo().GetMentionedJID()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo().GetMentionedJID()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo().GetMentionedJID()
	}
	return nil
}
