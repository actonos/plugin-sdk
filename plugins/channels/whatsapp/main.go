package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

const whatsappGraphAPI = "https://graph.facebook.com/v21.0"

type WhatsAppConfig struct {
	PollIntervalSeconds int               `json:"poll_interval_seconds"`
	Accounts            []WhatsAppAccount `json:"accounts"`
	// Legacy root-level fields kept for backward-compatible config binding.
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token,omitempty"`
	DefaultAgent  string `json:"default_agent"`
}

type WhatsAppAccount struct {
	sdk.ChannelAccount
	AccessToken   string `json:"access_token,omitempty"`
	PhoneNumberID string `json:"phone_number_id,omitempty"`
}

type WhatsAppChannel struct {
	sdk.BaseChannel
}

type WhatsAppWebhookPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Text      struct {
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
	acc, token, phoneID, err := resolveWhatsAppAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}

	recipient := sdk.FirstNonEmpty(msg.ChatID, msg.Recipient, msg.Metadata["to"])
	if recipient == "" {
		return fmt.Errorf("recipient phone number is required")
	}

	if msg.WantsTyping() && acc.TypingEnabled() {
		sendWhatsAppTyping(ctx, token, phoneID, msg.ReplyToID)
		if msg.IsTypingOnly() {
			return nil
		}
	} else if msg.IsTypingOnly() {
		return nil
	}

	if msg.Reaction != "" && acc.AckReactionEnabled() {
		addWhatsAppReaction(ctx, token, phoneID, recipient, msg.ReplyToID, sdk.MapReactionForPlatform("whatsapp", msg.Reaction))
		if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
			return nil
		}
	} else if msg.Kind == sdk.MessageKindReaction && msg.IsControlOnly() {
		return nil
	}

	if name, mime, data, ok := msg.AttachedFile(); ok {
		return sendWhatsAppFile(ctx, token, phoneID, recipient, msg, acc, name, mime, data)
	}

	reqURL := fmt.Sprintf("%s/%s/messages", whatsappGraphAPI, phoneID)
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
		if acc.ReplyQuoteEnabled() && i == 0 && msg.ReplyToID != "" {
			payload["context"] = map[string]string{"message_id": msg.ReplyToID}
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
		"account_id": acc.AccountID,
		"recipient":  recipient,
		"chat_id":    recipient,
		"status":     "sent",
	})
	return nil
}

func (w *WhatsAppChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	var cfg WhatsAppConfig
	_ = ctx.Config().Bind(&cfg)
	accounts := activeWhatsAppAccounts(cfg)

	rawQueue, ok, _ := ctx.Storage().Get("pending_webhook_queue")
	if !ok || rawQueue == "" {
		return nil, nil
	}
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

			phoneID := change.Value.Metadata.PhoneNumberID
			acc := matchWhatsAppAccount(accounts, phoneID)

			token := resolveWhatsAppToken(ctx, acc)
			resolvedPhoneID := sdk.FirstNonEmpty(acc.PhoneNumberID, phoneID)

			for _, m := range change.Value.Messages {
				if m.Text.Body == "" {
					continue
				}

				name := contactsMap[m.From]
				if name == "" {
					name = "WhatsAppUser_" + m.From
				}

				targetAgent, cleanText := sdk.ExtractAgentMention(m.Text.Body)
				if targetAgent == "" && acc.DefaultAgent != "" {
					targetAgent = acc.DefaultAgent
				}

				inbound := sdk.NewInboundMessage(
					"whatsapp",
					acc.AccountID,
					m.From,
					name,
					cleanText,
				)
				inbound.TargetAgent = targetAgent
				inbound.Metadata["from"] = m.From
				inbound.Metadata["phone_number_id"] = resolvedPhoneID
				sdk.ApplyInboundEnvelope(&inbound, m.From, m.ID, "", m.Timestamp)

				inboundMsgs = append(inboundMsgs, inbound)
				_ = ctx.EventBus().Emit("channel.whatsapp.received", inbound)

				if token != "" && resolvedPhoneID != "" {
					if acc.TypingEnabled() {
						sendWhatsAppTyping(ctx, token, resolvedPhoneID, m.ID)
					}
					if acc.AckReactionEnabled() {
						addWhatsAppReaction(ctx, token, resolvedPhoneID, m.From, m.ID, sdk.MapReactionForPlatform("whatsapp", acc.ReactionEmoji()))
					}
				}
			}
		}
	}

	return inboundMsgs, nil
}

func sendWhatsAppTyping(ctx sdk.Context, token, phoneID, messageID string) {
	if token == "" || phoneID == "" || messageID == "" {
		return
	}
	reqURL := fmt.Sprintf("%s/%s/messages", whatsappGraphAPI, phoneID)
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
		"typing_indicator": map[string]string{
			"type": "text",
		},
	}
	_, _ = ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
}

func addWhatsAppReaction(ctx sdk.Context, token, phoneID, recipient, messageID, emoji string) {
	if token == "" || phoneID == "" || recipient == "" || messageID == "" || emoji == "" {
		return
	}
	reqURL := fmt.Sprintf("%s/%s/messages", whatsappGraphAPI, phoneID)
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                recipient,
		"type":              "reaction",
		"reaction": map[string]string{
			"message_id": messageID,
			"emoji":      emoji,
		},
	}
	_, _ = ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
}

func sendWhatsAppFile(ctx sdk.Context, token, phoneID, recipient string, msg sdk.OutboundMessage, acc WhatsAppAccount, name, mime string, data []byte) error {
	kind := sdk.FileKind(name, mime)
	waType := "document"
	switch kind {
	case "photo":
		waType = "image"
	case "voice":
		waType = "audio"
	case "video":
		waType = "video"
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	fields := map[string]string{
		"messaging_product": "whatsapp",
		"type":              mime,
	}
	contentType, body, err := sdk.EncodeMultipart(fields, "file", name, data)
	if err != nil {
		return fmt.Errorf("encoding whatsapp media upload: %w", err)
	}
	uploadURL := fmt.Sprintf("%s/%s/media", whatsappGraphAPI, phoneID)
	uploadResp, err := ctx.HTTP().DoWithAuth("POST", uploadURL, "Bearer "+token, map[string]string{
		"Content-Type": contentType,
	}, body)
	if err != nil {
		return fmt.Errorf("whatsapp media upload failed: %w", err)
	}
	if uploadResp.Status < 200 || uploadResp.Status >= 300 {
		return fmt.Errorf("whatsapp media upload returned HTTP %d: %s", uploadResp.Status, uploadResp.Body)
	}
	var uploaded struct {
		ID string `json:"id"`
	}
	if err := uploadResp.JSON(&uploaded); err != nil || uploaded.ID == "" {
		return fmt.Errorf("whatsapp media upload missing id: %s", uploadResp.Body)
	}

	media := map[string]string{"id": uploaded.ID}
	if strings.TrimSpace(msg.Content) != "" && waType != "audio" {
		media["caption"] = msg.Content
	}
	if waType == "document" {
		media["filename"] = name
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                recipient,
		"type":              waType,
		waType:              media,
	}
	if acc.ReplyQuoteEnabled() && msg.ReplyToID != "" {
		payload["context"] = map[string]string{"message_id": msg.ReplyToID}
	}
	reqURL := fmt.Sprintf("%s/%s/messages", whatsappGraphAPI, phoneID)
	resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
	if err != nil {
		return fmt.Errorf("whatsapp send media failed: %w", err)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return fmt.Errorf("whatsapp send media returned HTTP %d: %s", resp.Status, resp.Body)
	}
	_ = ctx.EventBus().Emit("channel.whatsapp.sent", map[string]string{
		"account_id": acc.AccountID,
		"recipient":  recipient,
		"chat_id":    recipient,
		"status":     "sent",
		"file_name":  name,
	})
	return nil
}

func activeWhatsAppAccounts(cfg WhatsAppConfig) []WhatsAppAccount {
	if len(cfg.Accounts) > 0 {
		return cfg.Accounts
	}
	return []WhatsAppAccount{
		{
			ChannelAccount: sdk.ChannelAccount{
				AccountID:    "default",
				DisplayName:  "Default WhatsApp Cloud",
				DefaultAgent: cfg.DefaultAgent,
			},
			AccessToken:   cfg.AccessToken,
			PhoneNumberID: cfg.PhoneNumberID,
		},
	}
}

func matchWhatsAppAccount(accounts []WhatsAppAccount, phoneNumberID string) WhatsAppAccount {
	if phoneNumberID != "" {
		for _, acc := range accounts {
			if acc.PhoneNumberID == phoneNumberID {
				return acc
			}
		}
	}
	if len(accounts) > 0 {
		return accounts[0]
	}
	return WhatsAppAccount{ChannelAccount: sdk.ChannelAccount{AccountID: "default"}}
}

func resolveWhatsAppToken(ctx sdk.Context, acc WhatsAppAccount) string {
	return sdk.ResolveSecret(ctx, acc.AccessToken, sdk.AccountVaultKeys(acc.AccountID, "whatsapp_tokens", "whatsapp_access_token")...)
}

func resolveWhatsAppAccount(ctx sdk.Context, accountID string) (WhatsAppAccount, string, string, error) {
	var cfg WhatsAppConfig
	_ = ctx.Config().Bind(&cfg)
	accounts := activeWhatsAppAccounts(cfg)

	acc := WhatsAppAccount{ChannelAccount: sdk.ChannelAccount{AccountID: accountID}}
	found := false
	for _, a := range accounts {
		if a.AccountID == accountID || (accountID == "default" && a.AccountID == "") {
			acc = a
			if acc.AccountID == "" {
				acc.AccountID = "default"
			}
			found = true
			break
		}
	}
	if !found && len(accounts) == 1 && (accountID == "" || accountID == "default") {
		acc = accounts[0]
		if acc.AccountID == "" {
			acc.AccountID = "default"
		}
	}

	token := resolveWhatsAppToken(ctx, acc)
	if token == "" {
		return acc, "", "", fmt.Errorf("missing whatsapp_access_token for account %q", acc.AccountID)
	}
	phoneID := sdk.FirstNonEmpty(acc.PhoneNumberID, sdk.ResolveSecret(ctx, "", "whatsapp_phone_number_id"))
	if phoneID == "" {
		return acc, "", "", fmt.Errorf("missing whatsapp_phone_number_id for account %q", acc.AccountID)
	}
	return acc, token, phoneID, nil
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
