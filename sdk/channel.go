package sdk

import "strings"

// ChannelAdapter defines the contract for messaging platform integrations (Telegram, Discord, Slack, WhatsApp, Webhooks, etc.)
type ChannelAdapter interface {
	// Name returns the unique channel identifier (e.g. "telegram", "discord", "slack", "whatsapp")
	Name() string

	// DisplayName returns human-readable channel label (e.g. "Telegram Bot", "Discord Gateway")
	DisplayName() string

	// RequiresPairing returns true if 6-digit PIN device pairing is required
	RequiresPairing() bool

	// SendMessage handles an outbound message to a user or chat group
	SendMessage(ctx Context, msg OutboundMessage) error

	// PollMessages fetches new incoming messages during the poll cycle
	PollMessages(ctx Context) ([]InboundMessage, error)
}

// BaseChannel provides default implementation for common channel methods.
type BaseChannel struct {
	ChannelName        string
	ChannelDisplayName string
	PairingRequired    bool
}

func (b *BaseChannel) Name() string          { return b.ChannelName }
func (b *BaseChannel) DisplayName() string   { return b.ChannelDisplayName }
func (b *BaseChannel) RequiresPairing() bool { return b.PairingRequired }
func (b *BaseChannel) PollMessages(ctx Context) ([]InboundMessage, error) {
	return nil, nil
}

// NewInboundMessage creates a structured InboundMessage and automatically extracts @agent mentions.
func NewInboundMessage(channelID, accountID, senderID, senderName, content string) InboundMessage {
	targetAgent, cleanContent := ExtractAgentMention(content)
	return InboundMessage{
		ChannelID:   channelID,
		AccountID:   accountID,
		SenderID:    senderID,
		SenderName:  senderName,
		TargetAgent: targetAgent,
		MentionText: content,
		Content:     cleanContent,
		Kind:        MessageKindText,
		Metadata:    make(map[string]string),
	}
}

// NewOutboundMessage creates a structured OutboundMessage.
func NewOutboundMessage(channelID, recipient, content string) OutboundMessage {
	return OutboundMessage{
		ChannelID: channelID,
		Recipient: recipient,
		Content:   content,
		Kind:      MessageKindText,
		Metadata:  make(map[string]string),
	}
}

// NewOutboundFile creates a media outbound message carrying a host-forwarded file.
func NewOutboundFile(channelID, recipient, caption, fileName, mimeType string, data []byte) OutboundMessage {
	msg := NewOutboundMessage(channelID, recipient, caption)
	msg.Kind = MessageKindMedia
	msg.FileName = fileName
	msg.MIMEType = mimeType
	msg.FileData = data
	return msg
}

// ExtractAgentMention parses @agent_name, /agent agent_name, or /ask agent_name from text.
// Returns the extracted agent identifier (or "") and the cleaned content without mention prefix.
// Matches ActonOS Daemon channel router specification.
func ExtractAgentMention(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	// Pattern 1: /agent <name> <content> or /ask <name> <content>
	if strings.HasPrefix(trimmed, "/agent ") || strings.HasPrefix(trimmed, "/ask ") {
		parts := strings.SplitN(trimmed, " ", 3)
		if len(parts) >= 2 {
			agentName := strings.TrimSpace(parts[1])
			content := ""
			if len(parts) == 3 {
				content = strings.TrimSpace(parts[2])
			}
			return agentName, content
		}
	}

	// Pattern 2: @<name> <content> at start of message
	if strings.HasPrefix(trimmed, "@") {
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			mention := strings.TrimPrefix(fields[0], "@")
			mention = strings.TrimRight(mention, ":,")
			content := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			return mention, content
		}
	}

	return "", trimmed
}
