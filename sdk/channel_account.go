package sdk

import "strings"

// DefaultAckReactionEmoji is the canonical acknowledgement emoji applied to inbound user messages.
const DefaultAckReactionEmoji = "👀"

// ChannelAccountFeatures holds the shared interactive flags for every chat channel account.
// Pointer bools distinguish "omitted → default true" from an explicit false in config JSON.
type ChannelAccountFeatures struct {
	EnableTypingIndicator *bool  `json:"enable_typing_indicator,omitempty"`
	EnableAckReaction     *bool  `json:"enable_ack_reaction,omitempty"`
	EnableReplyQuote      *bool  `json:"enable_reply_quote,omitempty"`
	AckReactionEmoji      string `json:"ack_reaction_emoji,omitempty"`
}

// ChannelAccount is the canonical multi-bot account record shared by all chat channel plugins.
// Platform-specific credentials (bot_token, access_token, phone_number_id, …) live on the
// embedding plugin struct; common routing and feature flags live here.
type ChannelAccount struct {
	AccountID       string `json:"account_id"`
	DisplayName     string `json:"display_name,omitempty"`
	DefaultAgent    string `json:"default_agent,omitempty"`
	BotToken        string `json:"bot_token,omitempty"`
	ListenTarget    string `json:"listen_target,omitempty"`
	ListenChannelID string `json:"listen_channel_id,omitempty"` // alias of ListenTarget
	ChannelAccountFeatures
}

// ChannelRootConfig is the canonical root config object for chat channel plugins.
type ChannelRootConfig struct {
	PollIntervalSeconds int `json:"poll_interval_seconds"`
}

// BoolOrDefault returns *v when set, otherwise def. Used for schema-defaulted feature flags.
func BoolOrDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// TypingEnabled reports whether live typing indicators should be emitted (default true).
func (f ChannelAccountFeatures) TypingEnabled() bool {
	return BoolOrDefault(f.EnableTypingIndicator, true)
}

// AckReactionEnabled reports whether inbound acknowledgement reactions should be applied (default true).
func (f ChannelAccountFeatures) AckReactionEnabled() bool {
	return BoolOrDefault(f.EnableAckReaction, true)
}

// ReplyQuoteEnabled reports whether outbound replies should quote the source message (default true).
func (f ChannelAccountFeatures) ReplyQuoteEnabled() bool {
	return BoolOrDefault(f.EnableReplyQuote, true)
}

// ReactionEmoji returns the configured acknowledgement emoji, defaulting to 👀.
func (f ChannelAccountFeatures) ReactionEmoji() string {
	if e := strings.TrimSpace(f.AckReactionEmoji); e != "" {
		return e
	}
	return DefaultAckReactionEmoji
}

// ResolveListenTarget returns listen_target, falling back to the legacy listen_channel_id alias.
func (a ChannelAccount) ResolveListenTarget() string {
	return FirstNonEmpty(a.ListenTarget, a.ListenChannelID)
}

// ResolveSecret returns the first non-empty credential: an inline config value, then Vault keys,
// then ConfigStore keys with the same names.
func ResolveSecret(ctx Context, inline string, keys ...string) string {
	if strings.TrimSpace(inline) != "" {
		return inline
	}
	if ctx == nil {
		return ""
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if secret, err := ctx.Vault().GetSecret(k); err == nil && secret != "" {
			return secret
		}
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if val := ctx.Config().GetString(k, ""); val != "" {
			return val
		}
	}
	return ""
}

// AccountVaultKeys builds the standard per-account then default Vault lookup chain.
// For account "sales" and prefix "telegram_bot_tokens" this yields:
//
//	telegram_bot_tokens.sales, telegram_bot_token_sales, accounts.sales.bot_token, <defaults...>
func AccountVaultKeys(accountID string, accountPrefix string, defaultKeys ...string) []string {
	var keys []string
	if accountID != "" && accountID != "default" {
		if accountPrefix != "" {
			keys = append(keys, accountPrefix+"."+accountID)
			// Derive telegram_bot_token_sales from telegram_bot_tokens
			singular := strings.TrimSuffix(accountPrefix, "s")
			if singular != accountPrefix {
				keys = append(keys, singular+"_"+accountID)
			}
		}
		keys = append(keys, "accounts."+accountID+".bot_token")
	}
	keys = append(keys, defaultKeys...)
	return keys
}
