package main

import (
	"encoding/json"
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

type ZaloChannel struct {
	sdk.BaseChannel
}

type ZaloWebhookPayload struct {
	AppID     string `json:"app_id"`
	EventName string `json:"event_name"` // "user_send_text"
	Sender    struct {
		ID string `json:"id"`
	} `json:"sender"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Message struct {
		MsgID string `json:"msg_id"`
		Text  string `json:"text"`
	} `json:"message"`
}

func (z *ZaloChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	token, err := ctx.Vault().GetSecret("zalo_oa_access_token")
	if err != nil || token == "" {
		return fmt.Errorf("missing zalo_oa_access_token in vault: %w", err)
	}

	payload := map[string]any{
		"recipient": map[string]string{
			"user_id": msg.Recipient,
		},
		"message": map[string]string{
			"text": msg.Content,
		},
	}

	reqURL := "https://openapi.zalo.me/v3.0/oa/message/cs"
	headers := map[string]string{
		"access_token": token,
		"Content-Type": "application/json",
	}

	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "", headers, payload)
	if err != nil {
		return fmt.Errorf("zalo send message failed: %w", err)
	}
	if resp.Status != 200 {
		return fmt.Errorf("zalo API returned status %d: %s", resp.Status, resp.Body)
	}

	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := resp.JSON(&result); err == nil && result.Error != 0 {
		return fmt.Errorf("zalo API error (%d): %s", result.Error, result.Message)
	}

	_ = ctx.EventBus().Emit("channel.zalo.sent", map[string]string{
		"recipient": msg.Recipient,
		"status":    "sent",
	})
	return nil
}

func (z *ZaloChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	rawQueue, ok, _ := ctx.Storage().Get("pending_zalo_webhook")
	if !ok || rawQueue == "" {
		return nil, nil
	}

	// Reset queue
	_ = ctx.Storage().Delete("pending_zalo_webhook")

	var events []ZaloWebhookPayload
	if err := json.Unmarshal([]byte(rawQueue), &events); err != nil {
		return nil, fmt.Errorf("parsing zalo webhook queue: %w", err)
	}

	var inboundMsgs []sdk.InboundMessage
	for _, ev := range events {
		if ev.EventName == "user_send_text" && ev.Message.Text != "" {
			inbound := sdk.NewInboundMessage(
				"zalo",
				"default",
				ev.Sender.ID,
				"ZaloUser_"+ev.Sender.ID,
				ev.Message.Text,
			)
			inbound.Metadata["msg_id"] = ev.Message.MsgID
			inbound.Metadata["oa_id"] = ev.Recipient.ID

			inboundMsgs = append(inboundMsgs, inbound)
			_ = ctx.EventBus().Emit("channel.zalo.received", inbound)
		}
	}

	return inboundMsgs, nil
}

func init() {
	ch := &ZaloChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "zalo",
			ChannelDisplayName: "Zalo Official Account",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
