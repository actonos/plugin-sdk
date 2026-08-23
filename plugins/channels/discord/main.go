package main

import (
	"fmt"
	"strings"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

type DiscordChannel struct {
	sdk.BaseChannel
}

type DiscordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"author"`
}

func (d *DiscordChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	token, err := ctx.Vault().GetSecret("discord_bot_token")
	if err != nil || token == "" {
		return fmt.Errorf("missing discord_bot_token in vault: %w", err)
	}

	channelID := msg.Recipient
	if channelID == "" {
		channelID = msg.Metadata["channel_id"]
	}
	if channelID == "" {
		return fmt.Errorf("recipient or channel_id is required")
	}

	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	payload := map[string]any{
		"content": msg.Content,
	}

	if title, ok := msg.Metadata["embed_title"]; ok {
		payload["embeds"] = []map[string]any{
			{
				"title":       title,
				"description": msg.Content,
				"color":       0x5865F2, // Discord Blurple
			},
		}
	}

	authHeader := "Bot " + token
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, authHeader, map[string]string{
		"Content-Type": "application/json",
	}, payload)
	if err != nil {
		return fmt.Errorf("discord sendMessage failed: %w", err)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return fmt.Errorf("discord API returned HTTP status %d: %s", resp.Status, resp.Body)
	}

	_ = ctx.EventBus().Emit("channel.discord.sent", map[string]string{
		"channel_id": channelID,
		"status":     "sent",
	})
	return nil
}

func (d *DiscordChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	token, err := ctx.Vault().GetSecret("discord_bot_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("missing discord_bot_token in vault: %w", err)
	}

	listenChannelID, ok, _ := ctx.Storage().Get("listen_channel_id")
	if !ok || listenChannelID == "" {
		listenChannelID = "default"
	}

	lastMsgID, _, _ := ctx.Storage().Get("last_msg_id_" + listenChannelID)
	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages?limit=10", listenChannelID)
	if lastMsgID != "" {
		reqURL += "&after=" + lastMsgID
	}

	authHeader := "Bot " + token
	resp, err := ctx.HTTP().DoWithAuth("GET", reqURL, authHeader, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("discord getMessages failed: %w", err)
	}

	var messages []DiscordMessage
	if err := resp.JSON(&messages); err != nil {
		return nil, fmt.Errorf("parsing discord messages: %w", err)
	}

	var inboundMsgs []sdk.InboundMessage
	newestID := lastMsgID

	for _, msg := range messages {
		if msg.Author.Bot {
			continue
		}
		newestID = msg.ID

		cleanContent := cleanDiscordMentions(msg.Content)
		inbound := sdk.NewInboundMessage(
			"discord",
			"default",
			msg.Author.ID,
			msg.Author.Username,
			cleanContent,
		)
		inbound.Metadata["channel_id"] = msg.ChannelID
		inbound.Metadata["message_id"] = msg.ID

		inboundMsgs = append(inboundMsgs, inbound)
		_ = ctx.EventBus().Emit("channel.discord.received", inbound)
	}

	if newestID != lastMsgID && newestID != "" {
		_ = ctx.Storage().Set("last_msg_id_"+listenChannelID, newestID)
	}

	return inboundMsgs, nil
}

func cleanDiscordMentions(content string) string {
	trimmed := strings.TrimSpace(content)
	// Strip <@!123456789> or <@123456789> tags
	for strings.HasPrefix(trimmed, "<@") {
		idx := strings.Index(trimmed, ">")
		if idx != -1 {
			trimmed = strings.TrimSpace(trimmed[idx+1:])
		} else {
			break
		}
	}
	return trimmed
}

func init() {
	ch := &DiscordChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "discord",
			ChannelDisplayName: "Discord Bot",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
