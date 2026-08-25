package sdk

import "strings"

// Canonical metadata aliases shared by every chat channel plugin.
const (
	MetaAccountID        = "account_id"
	MetaChatID           = "chat_id"
	MetaChannelID        = "channel_id"
	MetaMessageID        = "message_id"
	MetaMsgID            = "msg_id"
	MetaReplyToID        = "reply_to_id"
	MetaReplyToMsgID     = "reply_to_msg_id"
	MetaReplyToMessageID = "reply_to_message_id"
	MetaReplyToTS        = "reply_to_ts"
	MetaThreadID         = "thread_id"
	MetaThreadTS         = "thread_ts"
	MetaReaction         = "reaction"
	MetaTyping           = "typing"
	MetaAction           = "action"
	MetaTS               = "ts"
	MetaTimestamp        = "timestamp"
	MetaFrom             = "from"
	MetaFileName         = "file_name"
	MetaMIMEType         = "mime_type"
)

var mediaMetaKeys = []string{"photo", "image_url", "document", "file_url", "voice", "voice_url", "audio_url"}

var emojiToSlackName = map[string]string{
	"👀":  "eyes",
	"👍":  "+1",
	"👎":  "-1",
	"❤️": "heart",
	"❤":  "heart",
	"✅":  "white_check_mark",
	"⚡":  "zap",
	"🎉":  "tada",
	"❌":  "x",
	"🔥":  "fire",
}

var slackNameToEmoji = map[string]string{
	"eyes":             "👀",
	"+1":               "👍",
	"thumbsup":         "👍",
	"-1":               "👎",
	"thumbsdown":       "👎",
	"heart":            "❤️",
	"white_check_mark": "✅",
	"zap":              "⚡",
	"tada":             "🎉",
	"x":                "❌",
	"fire":             "🔥",
}

// FirstNonEmpty returns the first non-empty trimmed string.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// Normalize lifts metadata aliases into canonical envelope fields and mirrors those fields
// back into Metadata so the host and plugins share one contract regardless of who wrote what.
func (m *InboundMessage) Normalize() {
	if m == nil {
		return
	}
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	m.AccountID = FirstNonEmpty(m.AccountID, m.Metadata[MetaAccountID])
	if m.AccountID == "" {
		m.AccountID = "default"
	}
	m.Metadata[MetaAccountID] = m.AccountID

	m.ChatID = FirstNonEmpty(m.ChatID, m.Metadata[MetaChatID], m.Metadata[MetaChannelID], m.Metadata[MetaFrom])
	if m.ChatID != "" {
		setMetaIfEmpty(m.Metadata, MetaChatID, m.ChatID)
		setMetaIfEmpty(m.Metadata, MetaChannelID, m.ChatID)
	}

	m.MessageID = FirstNonEmpty(m.MessageID, m.Metadata[MetaMessageID], m.Metadata[MetaMsgID], m.Metadata[MetaTS])
	if m.MessageID != "" {
		setMetaIfEmpty(m.Metadata, MetaMessageID, m.MessageID)
		setMetaIfEmpty(m.Metadata, MetaMsgID, m.MessageID)
	}

	m.ThreadID = FirstNonEmpty(m.ThreadID, m.Metadata[MetaThreadID], m.Metadata[MetaThreadTS])
	if m.ThreadID != "" {
		setMetaIfEmpty(m.Metadata, MetaThreadID, m.ThreadID)
		setMetaIfEmpty(m.Metadata, MetaThreadTS, m.ThreadID)
	}

	m.Timestamp = FirstNonEmpty(m.Timestamp, m.Metadata[MetaTimestamp], m.Metadata["date"], m.Metadata[MetaTS])
	if m.Timestamp != "" {
		setMetaIfEmpty(m.Metadata, MetaTimestamp, m.Timestamp)
	}

	m.Reaction = FirstNonEmpty(m.Reaction, m.Metadata[MetaReaction])
	if m.Reaction != "" {
		m.Metadata[MetaReaction] = m.Reaction
	}

	if m.Kind == "" {
		switch {
		case m.Reaction != "" && strings.TrimSpace(m.Content) == "":
			m.Kind = MessageKindReaction
		case hasMediaMeta(m.Metadata):
			m.Kind = MessageKindMedia
		default:
			m.Kind = MessageKindText
		}
	}
}

// ApplyInboundEnvelope sets canonical conversation fields then Normalize()s alias metadata.
func ApplyInboundEnvelope(msg *InboundMessage, chatID, messageID, threadID, timestamp string) {
	if msg == nil {
		return
	}
	if chatID != "" {
		msg.ChatID = chatID
	}
	if messageID != "" {
		msg.MessageID = messageID
	}
	if threadID != "" {
		msg.ThreadID = threadID
	}
	if timestamp != "" {
		msg.Timestamp = timestamp
	}
	msg.Normalize()
}

// Normalize lifts metadata aliases into canonical envelope fields for outbound dispatch.
func (m *OutboundMessage) Normalize() {
	if m == nil {
		return
	}
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}

	m.AccountID = FirstNonEmpty(m.AccountID, m.Metadata[MetaAccountID])
	if m.AccountID == "" {
		m.AccountID = "default"
	}
	m.Metadata[MetaAccountID] = m.AccountID

	m.ChatID = FirstNonEmpty(m.ChatID, m.Metadata[MetaChatID], m.Metadata[MetaChannelID], m.Recipient)
	if m.ChatID != "" {
		setMetaIfEmpty(m.Metadata, MetaChatID, m.ChatID)
		setMetaIfEmpty(m.Metadata, MetaChannelID, m.ChatID)
	}

	m.ReplyToID = FirstNonEmpty(
		m.ReplyToID,
		m.Metadata[MetaReplyToID],
		m.Metadata[MetaReplyToMsgID],
		m.Metadata[MetaReplyToMessageID],
		m.Metadata[MetaReplyToTS],
	)
	if m.ReplyToID != "" {
		setMetaIfEmpty(m.Metadata, MetaReplyToID, m.ReplyToID)
		setMetaIfEmpty(m.Metadata, MetaReplyToMsgID, m.ReplyToID)
	}

	m.ThreadID = FirstNonEmpty(m.ThreadID, m.Metadata[MetaThreadID], m.Metadata[MetaThreadTS])
	if m.ThreadID != "" {
		setMetaIfEmpty(m.Metadata, MetaThreadID, m.ThreadID)
		setMetaIfEmpty(m.Metadata, MetaThreadTS, m.ThreadID)
	}

	m.Reaction = FirstNonEmpty(m.Reaction, m.Metadata[MetaReaction])
	if m.Reaction != "" {
		m.Metadata[MetaReaction] = m.Reaction
	}

	m.Action = FirstNonEmpty(m.Action, m.Metadata[MetaAction])
	if m.Action != "" {
		m.Metadata[MetaAction] = m.Action
	}

	if !m.Typing {
		m.Typing = m.Metadata[MetaTyping] == "true" || m.Kind == MessageKindTyping || m.Action == "typing"
	}
	if m.Typing {
		m.Metadata[MetaTyping] = "true"
	}

	m.FileName = FirstNonEmpty(m.FileName, m.Metadata[MetaFileName])
	if m.FileName != "" {
		setMetaIfEmpty(m.Metadata, MetaFileName, m.FileName)
	}
	m.MIMEType = FirstNonEmpty(m.MIMEType, m.Metadata[MetaMIMEType])
	if m.MIMEType != "" {
		setMetaIfEmpty(m.Metadata, MetaMIMEType, m.MIMEType)
	}

	if len(m.FileData) > 0 {
		m.Kind = MessageKindMedia
	} else if m.Kind == "" {
		switch {
		case m.Typing && m.IsControlOnly() && m.Reaction == "":
			m.Kind = MessageKindTyping
		case m.Reaction != "" && m.IsControlOnly():
			m.Kind = MessageKindReaction
		case hasMediaMeta(m.Metadata):
			m.Kind = MessageKindMedia
		default:
			m.Kind = MessageKindText
		}
	}
}

// WantsTyping reports whether this outbound payload should emit a typing / chat-action indicator.
func (m OutboundMessage) WantsTyping() bool {
	if m.Kind == MessageKindTyping || m.Typing || m.Action == "typing" {
		return true
	}
	if m.Metadata != nil && m.Metadata[MetaTyping] == "true" {
		return true
	}
	return false
}

// HasMedia reports whether a file or platform media metadata is present.
func (m OutboundMessage) HasMedia() bool {
	return len(m.FileData) > 0 || strings.TrimSpace(m.FileName) != "" || hasMediaMeta(m.Metadata)
}

// AttachedFile returns the host-forwarded file payload, if any.
func (m OutboundMessage) AttachedFile() (name, mimeType string, data []byte, ok bool) {
	if len(m.FileData) == 0 {
		return "", "", nil, false
	}
	name = FirstNonEmpty(m.FileName, metadataValue(m.Metadata, MetaFileName), "file")
	mimeType = FirstNonEmpty(m.MIMEType, metadataValue(m.Metadata, MetaMIMEType))
	return name, mimeType, m.FileData, true
}

func metadataValue(meta map[string]string, key string) string {
	if meta == nil {
		return ""
	}
	return meta[key]
}

// IsControlOnly reports a payload with no text and no media (typing and/or reaction only).
func (m OutboundMessage) IsControlOnly() bool {
	return strings.TrimSpace(m.Content) == "" && !m.HasMedia()
}

// IsTypingOnly reports a control frame with no text, media, or reaction (typing / empty payload).
func (m OutboundMessage) IsTypingOnly() bool {
	return m.IsControlOnly() && m.Reaction == ""
}

// ChatAction returns the platform chat-action name (default "typing").
func (m OutboundMessage) ChatAction() string {
	if a := strings.TrimSpace(m.Action); a != "" && a != "typing" {
		return a
	}
	return "typing"
}

// MapReactionForPlatform converts a unicode emoji or Slack-style name into the form the
// target platform expects (Slack short names, unicode for everyone else).
func MapReactionForPlatform(platform, emoji string) string {
	e := strings.TrimSpace(emoji)
	if e == "" {
		return ""
	}
	if platform == "slack" {
		if name, ok := emojiToSlackName[e]; ok {
			return name
		}
		return strings.Trim(e, ":")
	}
	if unicode, ok := slackNameToEmoji[strings.Trim(e, ":")]; ok {
		return unicode
	}
	return e
}

func hasMediaMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	for _, k := range mediaMetaKeys {
		if strings.TrimSpace(meta[k]) != "" {
			return true
		}
	}
	return false
}

func setMetaIfEmpty(meta map[string]string, key, value string) {
	if meta == nil || value == "" {
		return
	}
	if strings.TrimSpace(meta[key]) == "" {
		meta[key] = value
	}
}
