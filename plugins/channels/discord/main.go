package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/actonos/plugin-sdk/sdk"
)

// DiscordConfig defines the root configuration model for the Discord channel plugin.
type DiscordConfig struct {
	PollIntervalSeconds int              `json:"poll_interval_seconds"`
	Accounts            []DiscordAccount `json:"accounts"`
}

// DiscordAccount represents an individual configured Discord bot instance.
type DiscordAccount struct {
	sdk.ChannelAccount
	EnableEmbeds    bool   `json:"enable_embeds"`
	DiscordBotToken string `json:"discord_bot_token,omitempty"`
}

type GatewayState struct {
	LastHeartbeat     time.Time
	HeartbeatInterval time.Duration
	LastSequence      *int64
	SessionID         string
}

type DiscordChannel struct {
	sdk.BaseChannel
	mu               sync.Mutex
	wsConns          map[string]sdk.WebSocketConn
	gatewayStates    map[string]*GatewayState
	lastDialAttempts map[string]time.Time
}

type GatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Bot           bool   `json:"bot"`
}

type DiscordMessage struct {
	ID        string      `json:"id"`
	ChannelID string      `json:"channel_id"`
	GuildID   string      `json:"guild_id"`
	Author    DiscordUser `json:"author"`
	Content   string      `json:"content"`
	Type      int         `json:"type"`
}

func (d *DiscordChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	acc, token, err := resolveDiscordAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}

	channelID := sdk.FirstNonEmpty(msg.ChatID, msg.Recipient)
	if channelID == "" {
		return fmt.Errorf("recipient or channel_id is required")
	}

	if msg.WantsTyping() && acc.TypingEnabled() {
		sendDiscordTyping(ctx, token, channelID)
		if msg.IsTypingOnly() {
			return nil
		}
	} else if msg.IsTypingOnly() {
		return nil
	}

	if msg.Reaction != "" && acc.AckReactionEnabled() {
		addDiscordReaction(ctx, token, channelID, msg.ReplyToID, sdk.MapReactionForPlatform("discord", msg.Reaction))
		if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
			return nil
		}
	} else if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
		return nil
	}

	if name, _, data, ok := msg.AttachedFile(); ok {
		return sendDiscordFile(ctx, token, channelID, msg, acc, name, data)
	}

	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	chunks := sdk.SplitMessage(msg.Content, 1900)

	for i, chunk := range chunks {
		payload := map[string]any{
			"content": chunk,
		}

		if acc.ReplyQuoteEnabled() && i == 0 && msg.ReplyToID != "" {
			payload["message_reference"] = map[string]any{
				"message_id": msg.ReplyToID,
			}
		}

		// Optional Rich Embed support
		if i == 0 {
			if title, ok := msg.Metadata["embed_title"]; ok && title != "" {
				color := 0x5865F2 // Blurple
				if cStr, ok := msg.Metadata["embed_color"]; ok {
					if c, err := strconv.ParseInt(strings.TrimPrefix(cStr, "#"), 16, 64); err == nil {
						color = int(c)
					}
				}
				embed := map[string]any{
					"title":       title,
					"description": chunk,
					"color":       color,
					"footer": map[string]string{
						"text": "ActonOS AI Swarm",
					},
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				}
				if targetAgent, ok := msg.Metadata["agent"]; ok && targetAgent != "" {
					embed["author"] = map[string]string{
						"name": fmt.Sprintf("🤖 Agent: %s", targetAgent),
					}
				}
				payload["embeds"] = []map[string]any{embed}
				delete(payload, "content")
			}
		}

		authHeader := "Bot " + token
		resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, authHeader, map[string]string{
			"Content-Type": "application/json",
		}, payload)
		if err != nil {
			return fmt.Errorf("discord sendMessage failed: %w", err)
		}

		// If 404 (Unknown Channel - 10003), target might be a Direct Message (DM) User ID
		if resp.Status == 404 && strings.Contains(resp.Body, "10003") && msg.Recipient != "" {
			dmChannelID, dmErr := d.getOrCreateDMChannel(ctx, token, msg.Recipient)
			if dmErr == nil && dmChannelID != "" {
				dmURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", dmChannelID)
				resp, err = ctx.HTTP().DoWithAuth("POST", dmURL, authHeader, map[string]string{
					"Content-Type": "application/json",
				}, payload)
				if err != nil {
					return fmt.Errorf("discord sendMessage DM failed: %w", err)
				}
				channelID = dmChannelID
				reqURL = dmURL
			}
		}

		if resp.Status < 200 || resp.Status >= 300 {
			return fmt.Errorf("discord API returned HTTP status %d: %s", resp.Status, resp.Body)
		}
	}

	_ = ctx.EventBus().Emit("channel.discord.sent", map[string]string{
		"account_id": acc.AccountID,
		"channel_id": channelID,
		"chat_id":    channelID,
		"status":     "sent",
		"chunks":     strconv.Itoa(len(chunks)),
	})
	return nil
}

func sendDiscordFile(ctx sdk.Context, token, channelID string, msg sdk.OutboundMessage, acc DiscordAccount, name string, data []byte) error {
	payload := map[string]any{}
	if strings.TrimSpace(msg.Content) != "" {
		payload["content"] = msg.Content
	}
	if acc.ReplyQuoteEnabled() && msg.ReplyToID != "" {
		payload["message_reference"] = map[string]any{"message_id": msg.ReplyToID}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding discord payload_json: %w", err)
	}
	contentType, body, err := sdk.EncodeMultipart(map[string]string{"payload_json": string(payloadJSON)}, "files[0]", name, data)
	if err != nil {
		return fmt.Errorf("encoding discord file upload: %w", err)
	}
	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "Bot "+token, map[string]string{
		"Content-Type": contentType,
	}, body)
	if err != nil {
		return fmt.Errorf("discord file upload failed: %w", err)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return fmt.Errorf("discord file upload returned HTTP %d: %s", resp.Status, resp.Body)
	}
	_ = ctx.EventBus().Emit("channel.discord.sent", map[string]string{
		"account_id": acc.AccountID,
		"channel_id": channelID,
		"chat_id":    channelID,
		"status":     "sent",
		"file_name":  name,
	})
	return nil
}

func (d *DiscordChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg DiscordConfig
	_ = ctx.Config().Bind(&cfg)

	accounts := cfg.Accounts
	if len(accounts) == 0 {
		defaultToken, _ := ctx.Vault().GetSecret("discord_bot_token")
		if defaultToken == "" {
			defaultToken = ctx.Config().GetString("discord_bot_token", "")
		}
		accounts = []DiscordAccount{
			{
				ChannelAccount: sdk.ChannelAccount{
					AccountID:   "default",
					DisplayName: "Default Discord Bot",
					BotToken:    defaultToken,
				},
			},
		}
	}

	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		if acc.AccountID == "" {
			acc.AccountID = "default"
		}
		token := getBotToken(ctx, acc)
		if token == "" {
			continue
		}

		// 1. Process real-time events from Discord Gateway WebSocket
		wsMsgs := d.pollGatewayWebSocket(ctx, acc, token)
		allInbound = append(allInbound, wsMsgs...)

		// 2. HTTP Polling fallback if configured
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

	d.mu.Lock()
	state := d.gatewayStates[acc.AccountID]
	if state == nil {
		state = &GatewayState{
			HeartbeatInterval: 41 * time.Second,
		}
		d.gatewayStates[acc.AccountID] = state
	}
	d.mu.Unlock()

	// Proactive Heartbeat check: Send Heartbeat to keep connection alive 24/7
	if !state.LastHeartbeat.IsZero() && time.Since(state.LastHeartbeat) >= state.HeartbeatInterval {
		var seq any = nil
		if state.LastSequence != nil {
			seq = *state.LastSequence
		}
		_ = conn.SendJSON(map[string]any{
			"op": 1,
			"d":  seq,
		})
		state.LastHeartbeat = time.Now()
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
			ctx.Log().Warn("Discord gateway connection lost, will reconnect", "account_id", acc.AccountID, "err", err)
			break
		}
		if !hasMsg || len(rawBytes) == 0 {
			break
		}

		var payload GatewayPayload
		if err := json.Unmarshal(rawBytes, &payload); err != nil {
			continue
		}

		if payload.S != nil {
			state.LastSequence = payload.S
		}

		switch payload.Op {
		case 10: // HELLO -> Read heartbeat_interval & Send IDENTIFY
			var helloData struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(payload.D, &helloData); err == nil && helloData.HeartbeatInterval > 0 {
				state.HeartbeatInterval = time.Duration(helloData.HeartbeatInterval) * time.Millisecond
			}

			// Send immediate first Heartbeat
			var seq any = nil
			if state.LastSequence != nil {
				seq = *state.LastSequence
			}
			_ = conn.SendJSON(map[string]any{"op": 1, "d": seq})
			state.LastHeartbeat = time.Now()

			// Send Opcode 2 IDENTIFY with GUILDS (1) + GUILD_MESSAGES (512) + DIRECT_MESSAGES (4096) + MESSAGE_CONTENT (32768) = 37377
			identifyPayload := GatewayPayload{
				Op: 2, // IDENTIFY
				D: json.RawMessage(fmt.Sprintf(`{
					"token": "%s",
					"intents": 37377,
					"properties": {
						"os": "actonos",
						"browser": "actonos-plugin",
						"device": "actonos-plugin"
					}
				}`, token)),
			}
			_ = conn.SendJSON(identifyPayload)
			ctx.Log().Info("Identified with Discord Gateway", "account_id", acc.AccountID, "intents", 37377)

		case 11: // HEARTBEAT_ACK
			state.LastHeartbeat = time.Now()

		case 1: // HEARTBEAT REQUESTED by Discord -> Reply immediately
			var seq any = nil
			if state.LastSequence != nil {
				seq = *state.LastSequence
			}
			_ = conn.SendJSON(map[string]any{"op": 1, "d": seq})
			state.LastHeartbeat = time.Now()

		case 7, 9: // RECONNECT or INVALID_SESSION -> Reset connection
			d.mu.Lock()
			delete(d.wsConns, acc.AccountID)
			d.mu.Unlock()
			ctx.Log().Info("Received reconnect/invalid session from Discord Gateway, resetting", "op", payload.Op)
			return messages

		case 0: // DISPATCH -> Process Events
			if payload.T == "READY" {
				var readyData struct {
					SessionID string      `json:"session_id"`
					User      DiscordUser `json:"user"`
				}
				if err := json.Unmarshal(payload.D, &readyData); err == nil {
					state.SessionID = readyData.SessionID
					ctx.Log().Info("Discord Gateway Session Ready", "bot_user", readyData.User.Username, "account_id", acc.AccountID)
				}
			} else if payload.T == "MESSAGE_CREATE" {
				var msg DiscordMessage
				if err := json.Unmarshal(payload.D, &msg); err == nil && !msg.Author.Bot {
					rawContent := strings.TrimSpace(msg.Content)
					if rawContent == "" {
						ctx.Log().Warn("Received empty message content from Discord. Ensure 'MESSAGE CONTENT INTENT' is enabled in Discord Developer Portal -> Bot -> Privileged Gateway Intents", "channel_id", msg.ChannelID)
						continue
					}

					cleanContent := cleanDiscordMentions(rawContent)
					if cleanContent == "" {
						cleanContent = rawContent
					}

					targetAgent, finalContent := sdk.ExtractAgentMention(cleanContent)
					if targetAgent == "" && acc.DefaultAgent != "" {
						targetAgent = acc.DefaultAgent
					}
					if finalContent == "" {
						finalContent = cleanContent
					}

					inbound := sdk.NewInboundMessage(
						"discord",
						acc.AccountID,
						msg.Author.ID,
						msg.Author.Username,
						finalContent,
					)
					inbound.TargetAgent = targetAgent
					if msg.GuildID != "" {
						inbound.Metadata["guild_id"] = msg.GuildID
					}
					sdk.ApplyInboundEnvelope(&inbound, msg.ChannelID, msg.ID, "", "")

					messages = append(messages, inbound)
					_ = ctx.EventBus().Emit("channel.discord.received", inbound)
					ctx.Log().Info("Discord message received", "from", msg.Author.Username, "channel_id", msg.ChannelID, "content", finalContent, "target_agent", targetAgent)

					if acc.TypingEnabled() {
						sendDiscordTyping(ctx, token, msg.ChannelID)
					}
					if acc.AckReactionEnabled() {
						addDiscordReaction(ctx, token, msg.ChannelID, msg.ID, sdk.MapReactionForPlatform("discord", acc.ReactionEmoji()))
					}
				}
			}
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
	if d.gatewayStates == nil {
		d.gatewayStates = make(map[string]*GatewayState)
	}
	if d.lastDialAttempts == nil {
		d.lastDialAttempts = make(map[string]time.Time)
	}

	if conn, exists := d.wsConns[accountID]; exists && conn != nil {
		return conn, nil
	}

	// 15-second cooldown between retry attempts if previous attempt failed
	if lastAttempt, exists := d.lastDialAttempts[accountID]; exists {
		if time.Since(lastAttempt) < 15*time.Second {
			return nil, fmt.Errorf("discord gateway dial on cooldown")
		}
	}
	d.lastDialAttempts[accountID] = time.Now()

	gatewayURL := "wss://gateway.discord.gg/?v=10&encoding=json"
	conn, err := ctx.WS().Dial(gatewayURL, nil)
	if err != nil {
		return nil, err
	}

	d.wsConns[accountID] = conn
	return conn, nil
}

func (d *DiscordChannel) pollHTTPMessages(ctx sdk.Context, token string, acc DiscordAccount) ([]sdk.InboundMessage, error) {
	listenChannelID := acc.ResolveListenTarget()
	if listenChannelID == "" {
		if stored, ok, _ := ctx.Storage().Get("listen_channel_id"); ok && stored != "" {
			listenChannelID = stored
		} else {
			listenChannelID = "default"
		}
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
		if cleanContent == "" {
			cleanContent = msg.Content
		}

		targetAgent, finalContent := sdk.ExtractAgentMention(cleanContent)
		if targetAgent == "" && acc.DefaultAgent != "" {
			targetAgent = acc.DefaultAgent
		}

		inbound := sdk.NewInboundMessage(
			"discord",
			acc.AccountID,
			msg.Author.ID,
			msg.Author.Username,
			finalContent,
		)
		inbound.TargetAgent = targetAgent
		sdk.ApplyInboundEnvelope(&inbound, msg.ChannelID, msg.ID, "", "")

		inboundMsgs = append(inboundMsgs, inbound)
		_ = ctx.EventBus().Emit("channel.discord.received", inbound)

		if acc.TypingEnabled() {
			sendDiscordTyping(ctx, token, msg.ChannelID)
		}
		if acc.AckReactionEnabled() {
			addDiscordReaction(ctx, token, msg.ChannelID, msg.ID, sdk.MapReactionForPlatform("discord", acc.ReactionEmoji()))
		}
	}

	if newestID != lastMsgID && newestID != "" {
		_ = ctx.Storage().Set(storageKey, newestID)
	}

	return inboundMsgs, nil
}

// sendDiscordTyping triggers the "[Bot] is typing..." status on Discord.
func sendDiscordTyping(ctx sdk.Context, token string, channelID string) {
	if token == "" || channelID == "" {
		return
	}
	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/typing", channelID)
	authHeader := "Bot " + token
	_, _ = ctx.HTTP().DoWithAuth("POST", reqURL, authHeader, map[string]string{
		"Content-Type": "application/json",
	}, nil)
}

// addDiscordReaction adds an emoji reaction to a specific Discord message.
func addDiscordReaction(ctx sdk.Context, token string, channelID string, messageID string, emoji string) {
	if token == "" || channelID == "" || messageID == "" || emoji == "" {
		return
	}
	encodedEmoji := url.PathEscape(emoji)
	reqURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages/%s/reactions/%s/@me", channelID, messageID, encodedEmoji)
	authHeader := "Bot " + token
	_, _ = ctx.HTTP().DoWithAuth("PUT", reqURL, authHeader, nil, nil)
}

func getBotToken(ctx sdk.Context, acc DiscordAccount) string {
	inline := sdk.FirstNonEmpty(acc.BotToken, acc.DiscordBotToken)
	return sdk.ResolveSecret(ctx, inline, sdk.AccountVaultKeys(acc.AccountID, "discord_bot_tokens", "discord_bot_token", "bot_token", "token")...)
}

func resolveDiscordAccount(ctx sdk.Context, accountID string) (DiscordAccount, string, error) {
	var cfg DiscordConfig
	_ = ctx.Config().Bind(&cfg)

	acc := DiscordAccount{ChannelAccount: sdk.ChannelAccount{AccountID: accountID}}
	for _, a := range cfg.Accounts {
		if a.AccountID == accountID {
			acc = a
			break
		}
	}
	if acc.AccountID == "" {
		acc.AccountID = "default"
	}
	token := getBotToken(ctx, acc)
	if token == "" {
		return acc, "", fmt.Errorf("missing discord bot token for account %q in vault or config", acc.AccountID)
	}
	return acc, token, nil
}

func (d *DiscordChannel) getOrCreateDMChannel(ctx sdk.Context, token string, recipientID string) (string, error) {
	cacheKey := "dm_chan_" + recipientID
	if cachedID, ok, _ := ctx.Storage().Get(cacheKey); ok && cachedID != "" {
		return cachedID, nil
	}

	reqURL := "https://discord.com/api/v10/users/@me/channels"
	authHeader := "Bot " + token
	payload := map[string]string{
		"recipient_id": recipientID,
	}

	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, authHeader, map[string]string{
		"Content-Type": "application/json",
	}, payload)
	if err != nil {
		return "", fmt.Errorf("creating DM channel: %w", err)
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return "", fmt.Errorf("DM channel creation failed status %d: %s", resp.Status, resp.Body)
	}

	var dmResp struct {
		ID string `json:"id"`
	}
	if err := resp.JSON(&dmResp); err != nil || dmResp.ID == "" {
		return "", fmt.Errorf("parsing DM channel response: %w", err)
	}

	_ = ctx.Storage().Set(cacheKey, dmResp.ID)
	return dmResp.ID, nil
}

func cleanDiscordMentions(raw string) string {
	parts := strings.Fields(raw)
	var clean []string
	for _, p := range parts {
		if strings.HasPrefix(p, "<@") && strings.HasSuffix(p, ">") {
			continue
		}
		clean = append(clean, p)
	}
	return strings.Join(clean, " ")
}

func init() {
	ch := &DiscordChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "discord",
			ChannelDisplayName: "Discord Bot (Gateway & REST)",
			PairingRequired:    true,
		},
		wsConns:       make(map[string]sdk.WebSocketConn),
		gatewayStates: make(map[string]*GatewayState),
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
