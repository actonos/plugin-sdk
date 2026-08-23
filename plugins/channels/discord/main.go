package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

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
	mu      sync.RWMutex
	wsConns map[string]sdk.WebSocketConn
}

// GatewayPayload defines the Discord Gateway WebSocket envelope.
type GatewayPayload struct {
	Op int             `json:"op"`
	T  string          `json:"t,omitempty"` // Event name, e.g. "MESSAGE_CREATE"
	S  *int64          `json:"s,omitempty"` // Sequence number
	D  json.RawMessage `json:"d"`           // Event data
}

type GatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type GatewayIdentify struct {
	Token      string            `json:"token"`
	Intents    int               `json:"intents"`
	Properties GatewayProperties `json:"properties"`
}

type GatewayProperties struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

type DiscordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id,omitempty"`
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
			continue
		}

		// 1. Process real-time events from Discord Gateway WebSocket via ctx.WS()
		wsMsgs := d.pollGatewayWebSocket(ctx, acc, token)
		allInbound = append(allInbound, wsMsgs...)

		// 2. HTTP Polling fallback if needed
		httpMsgs, err := d.pollHTTPMessages(ctx, token, acc)
		if err == nil && len(httpMsgs) > 0 {
			allInbound = append(allInbound, httpMsgs...)
		}
	}

	return allInbound, nil
}

func (d *DiscordChannel) pollGatewayWebSocket(ctx sdk.Context, acc DiscordAccount, token string) []sdk.InboundMessage {
	conn, err := d.getOrDialGateway(ctx, acc.AccountID)
	if err != nil || conn == nil {
		return nil
	}

	var messages []sdk.InboundMessage

	// Read pending WebSocket frames from Gateway stream
	for {
		rawBytes, hasMsg, err := conn.Poll()
		if err != nil {
			// Connection closed or broken -> cleanup for reconnection
			d.mu.Lock()
			delete(d.wsConns, acc.AccountID)
			d.mu.Unlock()
			break
		}
		if !hasMsg || len(rawBytes) == 0 {
			break
		}

		var payload GatewayPayload
		if err := json.Unmarshal(rawBytes, &payload); err != nil {
			continue
		}

		switch payload.Op {
		case 10: // HELLO -> Send IDENTIFY
			identifyPayload := GatewayPayload{
				Op: 2, // IDENTIFY
			}
			identifyData := GatewayIdentify{
				Token:   token,
				Intents: 37377, // GUILDS (1) | GUILD_MESSAGES (512) | DIRECT_MESSAGES (4096) | MESSAGE_CONTENT (32768)
				Properties: GatewayProperties{
					OS:      "actonos",
					Browser: "actonos-plugin",
					Device:  "actonos-plugin",
				},
			}
			b, _ := json.Marshal(identifyData)
			identifyPayload.D = json.RawMessage(b)
			_ = conn.SendJSON(identifyPayload)

		case 0: // DISPATCH -> Event received
			if payload.T == "MESSAGE_CREATE" {
				var msg DiscordMessage
				if err := json.Unmarshal(payload.D, &msg); err == nil && !msg.Author.Bot {
					cleanContent := cleanDiscordMentions(msg.Content)
					inbound := sdk.NewInboundMessage(
						"discord",
						acc.AccountID,
						msg.Author.ID,
						msg.Author.Username,
						cleanContent,
					)
					inbound.Metadata["channel_id"] = msg.ChannelID
					inbound.Metadata["guild_id"] = msg.GuildID
					inbound.Metadata["message_id"] = msg.ID
					inbound.Metadata["account_id"] = acc.AccountID

					if inbound.TargetAgent == "" && acc.DefaultAgent != "" {
						inbound.TargetAgent = acc.DefaultAgent
					}

					messages = append(messages, inbound)
					_ = ctx.EventBus().Emit("channel.discord.received", inbound)
				}
			}

		case 1: // HEARTBEAT requested -> Reply with Opcode 1
			_ = conn.SendJSON(map[string]any{"op": 1, "d": nil})
		}
	}

	return messages
}

func (d *DiscordChannel) getOrDialGateway(ctx sdk.Context, accountID string) (sdk.WebSocketConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.wsConns == nil {
		d.wsConns = make(map[string]sdk.WebSocketConn)
	}

	if conn, exists := d.wsConns[accountID]; exists && conn != nil {
		return conn, nil
	}

	gatewayURL := "wss://gateway.discord.gg/?v=10&encoding=json"
	conn, err := ctx.WS().Dial(gatewayURL, nil)
	if err != nil {
		return nil, err
	}

	d.wsConns[accountID] = conn
	return conn, nil
}

func (d *DiscordChannel) pollHTTPMessages(ctx sdk.Context, token string, acc DiscordAccount) ([]sdk.InboundMessage, error) {
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
	if err != nil || resp == nil || resp.Status < 200 || resp.Status >= 300 {
		return nil, err
	}

	var messages []DiscordMessage
	if err := resp.JSON(&messages); err != nil {
		return nil, err
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
	if accountID != "" && accountID != "default" {
		if token, err := ctx.Vault().GetSecret("discord_bot_tokens." + accountID); err == nil && token != "" {
			return token, nil
		}
		if token, err := ctx.Vault().GetSecret("discord_bot_token_" + accountID); err == nil && token != "" {
			return token, nil
		}
	}

	token, err := ctx.Vault().GetSecret("discord_bot_token")
	if err != nil || token == "" {
		return "", fmt.Errorf("missing discord bot token for account %q in vault", accountID)
	}
	return token, nil
}

func cleanDiscordMentions(content string) string {
	trimmed := strings.TrimSpace(content)
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
		wsConns: make(map[string]sdk.WebSocketConn),
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
