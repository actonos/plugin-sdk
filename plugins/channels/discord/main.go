package main

import (
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

// DiscordConfig defines the root configuration model for the Discord channel plugin.
type DiscordConfig struct {
	PollIntervalSeconds int              `json:"poll_interval_seconds"`
	Accounts            []DiscordAccount `json:"accounts"`
}

// DiscordAccount represents an individual configured Discord bot instance.
type DiscordAccount struct {
	AccountID       string `json:"account_id"`
	DisplayName     string `json:"display_name"`
	BotToken        string `json:"bot_token,omitempty"`
	DefaultAgent    string `json:"default_agent"`
	ListenChannelID string `json:"listen_channel_id"`
	EnableEmbeds    bool   `json:"enable_embeds"`
}

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
	accountID := msg.AccountID
	if accountID == "" {
		accountID = msg.Metadata["account_id"]
	}
	if accountID == "" {
		accountID = "default"
	}

	token, err := getBotToken(ctx, accountID)
	if err != nil {
		return err
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
		"account_id": accountID,
		"channel_id": channelID,
		"status":     "sent",
	})
	return nil
}

func (d *DiscordChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg DiscordConfig
	_ = ctx.Config().Bind(&cfg)

	// Fallback to default account if no accounts are explicitly defined
	accounts := cfg.Accounts
	if len(accounts) == 0 {
		accounts = []DiscordAccount{
			{
				AccountID:       "default",
				DisplayName:     "Default Discord Bot",
				ListenChannelID: "default",
				EnableEmbeds:    true,
			},
		}
	}

	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		token, err := getBotToken(ctx, acc.AccountID)
		if err != nil || token == "" {
			ctx.Log().Warn("Skipping Discord account due to missing token", "account_id", acc.AccountID, "err", err)
			continue
		}

		msgs, err := d.pollAccountMessages(ctx, token, acc)
		if err != nil {
			ctx.Log().Error("Failed polling messages for Discord account", "account_id", acc.AccountID, "err", err)
			continue
		}

		allInbound = append(allInbound, msgs...)
	}

	return allInbound, nil
}

func (d *DiscordChannel) pollAccountMessages(ctx sdk.Context, token string, acc DiscordAccount) ([]sdk.InboundMessage, error) {
	listenChannelID := acc.ListenChannelID
	if listenChannelID == "" {
		listenChannelID = "default"
	}

	storageKey := fmt.Sprintf("last_msg_id_%s_%s", acc.AccountID, listenChannelID)
	lastMsgID, _, _ := ctx.Storage().Get(storageKey)

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
			acc.AccountID,
			msg.Author.ID,
			msg.Author.Username,
			cleanContent,
		)
		inbound.Metadata["channel_id"] = msg.ChannelID
		inbound.Metadata["message_id"] = msg.ID
		inbound.Metadata["account_id"] = acc.AccountID

		// If no explicit @agent mention was found in text, fallback to account's DefaultAgent
		if inbound.TargetAgent == "" && acc.DefaultAgent != "" {
			inbound.TargetAgent = acc.DefaultAgent
		}

		inboundMsgs = append(inboundMsgs, inbound)
		_ = ctx.EventBus().Emit("channel.discord.received", inbound)
	}

	if newestID != lastMsgID && newestID != "" {
		_ = ctx.Storage().Set(storageKey, newestID)
	}

	return inboundMsgs, nil
}

func getBotToken(ctx sdk.Context, accountID string) (string, error) {
	// 1. Try account-specific scoped vault keys
	if accountID != "" && accountID != "default" {
		if token, err := ctx.Vault().GetSecret("discord_bot_tokens." + accountID); err == nil && token != "" {
			return token, nil
		}
		if token, err := ctx.Vault().GetSecret("discord_bot_token_" + accountID); err == nil && token != "" {
			return token, nil
		}
	}

	// 2. Fallback to default vault secret
	token, err := ctx.Vault().GetSecret("discord_bot_token")
	if err != nil || token == "" {
		return "", fmt.Errorf("missing discord bot token for account %q in vault", accountID)
	}
	return token, nil
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
			ChannelDisplayName: "Discord Bot Gateway",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
