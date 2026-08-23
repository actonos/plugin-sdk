package main

import (
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

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
	token, err := ctx.Vault().GetSecret("slack_bot_token")
	if err != nil || token == "" {
		return fmt.Errorf("missing slack_bot_token in vault: %w", err)
	}

	channelID := msg.Recipient
	if channelID == "" {
		channelID = msg.Metadata["channel_id"]
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
		"channel_id": channelID,
		"ts":         result.TS,
	})
	return nil
}

func (s *SlackChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	token, err := ctx.Vault().GetSecret("slack_bot_token")
	if err != nil || token == "" {
		return nil, fmt.Errorf("missing slack_bot_token in vault: %w", err)
	}

	channelID, ok, _ := ctx.Storage().Get("listen_channel_id")
	if !ok || channelID == "" {
		channelID = "general"
	}

	lastTS, _, _ := ctx.Storage().Get("last_ts_" + channelID)
	reqURL := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=10", channelID)
	if lastTS != "" {
		reqURL += "&oldest=" + lastTS
	}

	resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
	if err != nil {
		return nil, fmt.Errorf("slack conversations.history failed: %w", err)
	}

	var history SlackHistoryResponse
	if err := resp.JSON(&history); err != nil {
		return nil, fmt.Errorf("parsing slack history: %w", err)
	}

	var inboundMsgs []sdk.InboundMessage
	newestTS := lastTS

	for _, m := range history.Messages {
		if m.BotID != "" || m.Subtype == "bot_message" {
			continue
		}
		if m.TS > newestTS {
			newestTS = m.TS
		}

		inbound := sdk.NewInboundMessage(
			"slack",
			"default",
			m.User,
			m.User,
			m.Text,
		)
		inbound.Metadata["channel_id"] = channelID
		inbound.Metadata["ts"] = m.TS

		inboundMsgs = append(inboundMsgs, inbound)
		_ = ctx.EventBus().Emit("channel.slack.received", inbound)
	}

	if newestTS != lastTS && newestTS != "" {
		_ = ctx.Storage().Set("last_ts_"+channelID, newestTS)
	}

	return inboundMsgs, nil
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
