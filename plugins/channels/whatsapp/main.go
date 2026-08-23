package main

import (
	"encoding/json"
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

type WhatsAppConfig struct {
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token,omitempty"`
	DefaultAgent  string `json:"default_agent"`
}

type WhatsAppChannel struct {
	sdk.BaseChannel
}

type WhatsAppWebhookPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func (w *WhatsAppChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	token, err := ctx.Vault().GetSecret("whatsapp_access_token")
	if err != nil || token == "" {
		return fmt.Errorf("missing whatsapp_access_token in vault: %w", err)
	}

	phoneID := msg.Metadata["phone_number_id"]
	if phoneID == "" {
		phoneID, _ = ctx.Vault().GetSecret("whatsapp_phone_number_id")
	}
	if phoneID == "" {
		var cfg WhatsAppConfig
		_ = ctx.Config().Bind(&cfg)
		phoneID = cfg.PhoneNumberID
	}
	if phoneID == "" {
		return fmt.Errorf("missing whatsapp_phone_number_id in vault/metadata/config")
	}

	recipient := msg.Recipient
	if recipient == "" {
		recipient = msg.Metadata["to"]
	}
	if recipient == "" {
		return fmt.Errorf("recipient phone number is required")
	}

	reqURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", phoneID)
	chunks := sdk.SplitMessage(msg.Content, 3900)

	for i, chunk := range chunks {
		payload := map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                recipient,
			"type":              "text",
			"text": map[string]string{
				"body": chunk,
			},
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			return fmt.Errorf("whatsapp send message failed (chunk %d): %w", i+1, err)
		}
		if resp.Status < 200 || resp.Status >= 300 {
			return fmt.Errorf("whatsapp API returned status %d: %s", resp.Status, resp.Body)
		}
	}

	_ = ctx.EventBus().Emit("channel.whatsapp.sent", map[string]string{
		"recipient": recipient,
		"status":    "sent",
	})
	return nil
}

func (w *WhatsAppChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg WhatsAppConfig
	_ = ctx.Config().Bind(&cfg)

	// Read pending webhook queue stored in KV storage buffer
	rawQueue, ok, _ := ctx.Storage().Get("pending_webhook_queue")
	if !ok || rawQueue == "" {
		return nil, nil
	}

	// Reset queue
	_ = ctx.Storage().Delete("pending_webhook_queue")

	var payload WhatsAppWebhookPayload
	if err := json.Unmarshal([]byte(rawQueue), &payload); err != nil {
		return nil, fmt.Errorf("parsing whatsapp webhook queue: %w", err)
	}

	var inboundMsgs []sdk.InboundMessage
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			contactsMap := make(map[string]string)
			for _, contact := range change.Value.Contacts {
				contactsMap[contact.WaID] = contact.Profile.Name
			}

			for _, m := range change.Value.Messages {
				if m.Text.Body == "" {
					continue
				}

				name := contactsMap[m.From]
				if name == "" {
					name = "WhatsAppUser_" + m.From
				}

				targetAgent, cleanText := sdk.ExtractAgentMention(m.Text.Body)
				if targetAgent == "" && cfg.DefaultAgent != "" {
					targetAgent = cfg.DefaultAgent
				}

				inbound := sdk.NewInboundMessage(
					"whatsapp",
					"default",
					m.From,
					name,
					cleanText,
				)
				inbound.TargetAgent = targetAgent
				inbound.Metadata["message_id"] = m.ID
				inbound.Metadata["from"] = m.From

				inboundMsgs = append(inboundMsgs, inbound)
				_ = ctx.EventBus().Emit("channel.whatsapp.received", inbound)
			}
		}
	}

	return inboundMsgs, nil
}

func init() {
	ch := &WhatsAppChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "whatsapp",
			ChannelDisplayName: "WhatsApp Cloud",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
