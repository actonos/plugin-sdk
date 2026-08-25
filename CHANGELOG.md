# Changelog

All notable changes to the ActonOS Plugin SDK are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-08-25

Canonical multi-account config and a shared inbound/outbound envelope for chat channels (typing, reactions, quote replies).

WASM ABI remains `1.0.0`. Extra JSON fields are `omitempty`; older hosts ignore them.

### Added

- One `accounts[]` schema for Discord, Telegram, Slack, WhatsApp, and Zalo (`spec/CHANNEL_ACCOUNT_SCHEMA.json`):
  - `account_id`, `display_name`, platform credential, `default_agent`, `listen_target`
  - `enable_typing_indicator`, `enable_ack_reaction`, `enable_reply_quote`, `ack_reaction_emoji`
- Shared I/O envelope on `InboundMessage` / `OutboundMessage`:
  - `kind` (`text` | `typing` | `reaction` | `media`)
  - `message_id`, `chat_id`, `thread_id`, `reply_to_id`, `reaction`, `typing`, `action`
- `sdk.ChannelAccount`, `Normalize()`, `ApplyInboundEnvelope()`, and `MapReactionForPlatform()` so plugins and the host share one contract.
- `acton-plugin new --type=channel` scaffolds the same account schema.

Built-in channels honor the envelope:

| Intent | Mapping |
|:---|:---|
| Typing | Discord `POST /typing`; Telegram/Zalo `sendChatAction`; WhatsApp `typing_indicator` (Cloud API v21). Slack Web API has no typing endpoint (accepted as no-op). |
| Ack reaction | Default 👀 on inbound (Slack maps to `eyes`). |
| Quote reply | Discord `message_reference`; Telegram/Zalo `reply_to_message_id`; Slack `thread_ts`; WhatsApp `context.message_id`. |

WhatsApp is multi-account (`access_token` + `phone_number_id`).

### Changed

- Channel plugin manifests are `accounts[]`-first (UI group **Bot Accounts**). Discord credential field is `bot_token` (was mismatched `discord_bot_token` in schema).
- Telegram, Zalo, and WhatsApp still read legacy root-level tokens as `account_id=default`.
- Plugins `Normalize()` both directions so old metadata aliases still work: `chat_id`, `channel_id`, `reply_to_msg_id`, `thread_ts`, `typing=true`.

### Channel plugins

| Plugin | Version |
|:---|:---|
| `channel-discord` | 1.1.0 |
| `channel-telegram` | 1.1.0 |
| `channel-slack` | 1.1.0 |
| `channel-whatsapp` | 1.1.0 |
| `channel-zalo` | 1.1.0 |

SaaS connectors are unchanged.

### Compatibility

- **WASM ABI:** `1.0.0` (unchanged).
- **Config:** prefer `accounts[]`. Root `telegram_bot_token` / `zalo_bot_token` / WhatsApp `access_token` still work until the config is re-saved in the new form.
- **Host:** text chat still works on older `actond`. Typing-while-thinking, host-driven quote replies, and `native_channel_notify` `kind` / `reaction` / `reply_to_id` need the matching ActonOS daemon (channel envelope + router pulse).

### Upgrade

1. Install the `*.actonpkg` bundles from this release (or the plugin registry).
2. Open each channel plugin config and fill `accounts[]` (or keep legacy root tokens).
3. Update ActonOS if the daemon should send typing pulses and quote `reply_to_id` on agent replies.

## [1.0.0] - 2026-08-24

Initial public SDK, CLI (`acton-plugin`), mock host, and built-in channel / SaaS plugins.

[1.1.0]: https://github.com/actonos/plugin-sdk/releases/tag/v1.1.0
[1.0.0]: https://github.com/actonos/plugin-sdk/releases/tag/v1.0.0
