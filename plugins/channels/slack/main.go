package main

import (
	"fmt"
	"strconv"

	"github.com/actonos/plugin-sdk/sdk"
)

type SlackConfig struct {
	PollIntervalSeconds int            `json:"poll_interval_seconds"`
	Accounts            []SlackAccount `json:"accounts"`
}

type SlackAccount struct {
	sdk.ChannelAccount
}

type SlackChannel struct {
	sdk.BaseChannel
}

type SlackHistoryResponse struct {
	OK       bool `json:"ok"`
	Messages []struct {
		Type     string `json:"type"`
		User     string `json:"user"`
		Text     string `json:"text"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
		BotID    string `json:"bot_id"`
		Subtype  string `json:"subtype"`
	} `json:"messages"`
}

func (s *SlackChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	acc, token, err := resolveSlackAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}

	channelID := sdk.FirstNonEmpty(msg.ChatID, msg.Recipient)
	if channelID == "" {
		return fmt.Errorf("recipient or channel_id is required")
	}

	if msg.WantsTyping() {
		if acc.TypingEnabled() {
			sendSlackTyping(ctx, token, channelID)
		}
		if msg.IsTypingOnly() {
			return nil
		}
	} else if msg.IsTypingOnly() {
		return nil
	}

	if msg.Reaction != "" && acc.AckReactionEnabled() {
		target := sdk.FirstNonEmpty(msg.ReplyToID, msg.ThreadID)
		addSlackReaction(ctx, token, channelID, target, sdk.MapReactionForPlatform("slack", msg.Reaction))
		if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
			return nil
		}
	} else if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
		return nil
	}

	chunks := sdk.SplitMessage(msg.Content, 3900)
	var lastTS string

	for i, chunk := range chunks {
		payload := map[string]any{
			"channel": channelID,
			"text":    chunk,
		}

		if acc.ReplyQuoteEnabled() {
			threadTS := sdk.FirstNonEmpty(msg.ThreadID, msg.ReplyToID)
			if threadTS != "" {
				payload["thread_ts"] = threadTS
			}
		}

		resp, err := ctx.HTTP().PostJSONWithBearer("https://slack.com/api/chat.postMessage", token, payload)
		if err != nil {
			return fmt.Errorf("slack chat.postMessage failed (chunk %d): %w", i+1, err)
		}
		if resp.Status != 200 {
			return fmt.Errorf("slack API returned status %d: %s", resp.Status, resp.Body)
		}

		var result struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			TS    string `json:"ts"`
		}
		if err := resp.JSON(&result); err == nil && !result.OK {
			return fmt.Errorf("slack API error: %s", result.Error)
		}
		lastTS = result.TS
	}

	_ = ctx.EventBus().Emit("channel.slack.sent", map[string]string{
		"account_id": acc.AccountID,
		"channel_id": channelID,
		"chat_id":    channelID,
		"ts":         lastTS,
		"chunks":     strconv.Itoa(len(chunks)),
	})
	return nil
}

func (s *SlackChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg SlackConfig
	_ = ctx.Config().Bind(&cfg)

	accounts := cfg.Accounts
	if len(accounts) == 0 {
		accounts = []SlackAccount{
			{
				ChannelAccount: sdk.ChannelAccount{
					AccountID:   "default",
					DisplayName: "Default Slack Bot",
				},
			},
		}
	}

	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		if acc.AccountID == "" {
			acc.AccountID = "default"
		}
		token := getSlackBotToken(ctx, acc)
		if token == "" {
			continue
		}

		channelID := acc.ResolveListenTarget()
		if channelID == "" {
			if stored, ok, _ := ctx.Storage().Get("listen_channel_id"); ok && stored != "" {
				channelID = stored
			} else {
				channelID = "general"
			}
		}

		storageKey := fmt.Sprintf("last_ts_%s_%s", acc.AccountID, channelID)
		lastTS, _, _ := ctx.Storage().Get(storageKey)
		if lastTS == "" {
			lastTS, _, _ = ctx.Storage().Get("last_ts_" + channelID)
		}

		reqURL := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=10", channelID)
		if lastTS != "" {
			reqURL += "&oldest=" + lastTS
		}

		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			continue
		}

		var history SlackHistoryResponse
		if err := resp.JSON(&history); err != nil {
			continue
		}

		newestTS := lastTS
		for _, m := range history.Messages {
			if m.BotID != "" || m.Subtype == "bot_message" {
				continue
			}
			if m.TS > newestTS {
				newestTS = m.TS
			}

			targetAgent, cleanText := sdk.ExtractAgentMention(m.Text)
			if targetAgent == "" && acc.DefaultAgent != "" {
				targetAgent = acc.DefaultAgent
			}

			inbound := sdk.NewInboundMessage(
				"slack",
				acc.AccountID,
				m.User,
				"SlackUser_"+m.User,
				cleanText,
			)
			inbound.TargetAgent = targetAgent
			inbound.Metadata["ts"] = m.TS
			sdk.ApplyInboundEnvelope(&inbound, channelID, m.TS, m.ThreadTS, m.TS)

			allInbound = append(allInbound, inbound)
			_ = ctx.EventBus().Emit("channel.slack.received", inbound)

			if acc.TypingEnabled() {
				sendSlackTyping(ctx, token, channelID)
			}
			if acc.AckReactionEnabled() {
				addSlackReaction(ctx, token, channelID, m.TS, sdk.MapReactionForPlatform("slack", acc.ReactionEmoji()))
			}
		}

		if newestTS != lastTS && newestTS != "" {
			_ = ctx.Storage().Set(storageKey, newestTS)
		}
	}

	return allInbound, nil
}

func sendSlackTyping(ctx sdk.Context, token, channelID string) {
	// Slack Web API has no public typing indicator for classic bots. Accept the
	// canonical typing event so the host contract stays uniform across channels.
	_ = ctx
	_ = token
	_ = channelID
}

func addSlackReaction(ctx sdk.Context, token, channelID, timestamp, name string) {
	if token == "" || channelID == "" || timestamp == "" || name == "" {
		return
	}
	payload := map[string]string{
		"channel":   channelID,
		"timestamp": timestamp,
		"name":      name,
	}
	_, _ = ctx.HTTP().PostJSONWithBearer("https://slack.com/api/reactions.add", token, payload)
}

func getSlackBotToken(ctx sdk.Context, acc SlackAccount) string {
	return sdk.ResolveSecret(ctx, acc.BotToken, sdk.AccountVaultKeys(acc.AccountID, "slack_bot_tokens", "slack_bot_token", "bot_token", "token")...)
}

func resolveSlackAccount(ctx sdk.Context, accountID string) (SlackAccount, string, error) {
	var cfg SlackConfig
	_ = ctx.Config().Bind(&cfg)

	acc := SlackAccount{ChannelAccount: sdk.ChannelAccount{AccountID: accountID}}
	for _, a := range cfg.Accounts {
		if a.AccountID == accountID {
			acc = a
			break
		}
	}
	if acc.AccountID == "" {
		acc.AccountID = "default"
	}
	token := getSlackBotToken(ctx, acc)
	if token == "" {
		return acc, "", fmt.Errorf("missing slack bot token for account %q in vault or config", acc.AccountID)
	}
	return acc, token, nil
}

func init() {
	ch := &SlackChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "slack",
			ChannelDisplayName: "Slack Workspace Bot",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
