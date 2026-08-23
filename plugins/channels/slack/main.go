package main

import (
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

// SlackConfig defines the root configuration model for the Slack channel plugin.
type SlackConfig struct {
	PollIntervalSeconds int            `json:"poll_interval_seconds"`
	Accounts            []SlackAccount `json:"accounts"`
}

// SlackAccount represents an individual configured Slack bot instance.
type SlackAccount struct {
	AccountID       string `json:"account_id"`
	DisplayName     string `json:"display_name"`
	BotToken        string `json:"bot_token,omitempty"`
	DefaultAgent    string `json:"default_agent"`
	ListenChannelID string `json:"listen_channel_id"`
}

type SlackChannel struct {
	sdk.BaseChannel
}

type SlackHistoryResponse struct {
	OK       bool `json:"ok"`
	Messages []struct {
		Type    string `json:"type"`
		User    string `json:"user"`
		Text    string `json:"text"`
		TS      string `json:"ts"`
		BotID   string `json:"bot_id"`
		Subtype string `json:"subtype"`
	} `json:"messages"`
}

func (s *SlackChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	accountID := msg.AccountID
	if accountID == "" {
		accountID = msg.Metadata["account_id"]
	}
	if accountID == "" {
		accountID = "default"
	}

	token, err := getSlackBotToken(ctx, accountID)
	if err != nil {
		return err
	}

	// Prioritize channel_id from metadata, fallback to recipient
	channelID := msg.Metadata["channel_id"]
	if channelID == "" {
		channelID = msg.Recipient
	}
	if channelID == "" {
		return fmt.Errorf("recipient or channel_id is required")
	}

	payload := map[string]any{
		"channel": channelID,
		"text":    msg.Content,
	}

	if threadTS, ok := msg.Metadata["thread_ts"]; ok && threadTS != "" {
		payload["thread_ts"] = threadTS
	}

	resp, err := ctx.HTTP().PostJSONWithBearer("https://slack.com/api/chat.postMessage", token, payload)
	if err != nil {
		return fmt.Errorf("slack chat.postMessage failed: %w", err)
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

	_ = ctx.EventBus().Emit("channel.slack.sent", map[string]string{
		"account_id": accountID,
		"channel_id": channelID,
		"ts":         result.TS,
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
				AccountID:       "default",
				DisplayName:     "Default Slack Bot",
				ListenChannelID: "general",
			},
		}
	}

	var allInbound []sdk.InboundMessage

	for _, acc := range accounts {
		token, err := getSlackBotToken(ctx, acc.AccountID)
		if err != nil || token == "" {
			continue
		}

		channelID := acc.ListenChannelID
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
			inbound.Metadata["channel_id"] = channelID
			inbound.Metadata["ts"] = m.TS
			inbound.Metadata["account_id"] = acc.AccountID

			allInbound = append(allInbound, inbound)
			_ = ctx.EventBus().Emit("channel.slack.received", inbound)
		}

		if newestTS != lastTS && newestTS != "" {
			_ = ctx.Storage().Set(storageKey, newestTS)
		}
	}

	return allInbound, nil
}

func getSlackBotToken(ctx sdk.Context, accountID string) (string, error) {
	if accountID != "" && accountID != "default" {
		if token, err := ctx.Vault().GetSecret("slack_bot_tokens." + accountID); err == nil && token != "" {
			return token, nil
		}
		if token, err := ctx.Vault().GetSecret("slack_bot_token_" + accountID); err == nil && token != "" {
			return token, nil
		}
	}

	token, err := ctx.Vault().GetSecret("slack_bot_token")
	if err != nil || token == "" {
		return "", fmt.Errorf("missing slack bot token for account %q in vault", accountID)
	}
	return token, nil
}

func init() {
	ch := &SlackChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "slack",
			ChannelDisplayName: "Slack Bot",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
