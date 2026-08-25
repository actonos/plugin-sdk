# Telegram Bot Channel Plugin

ActonOS Chat Channel integration for Telegram with support for:
- Canonical `accounts[]` schema (typing, ack reaction, quote reply)
- Markdown / HTML formatting
- Reply-to-message threading (`reply_to_id`)
- Live typing (`sendChatAction`) and inbound acknowledgement reactions
- Device Pairing (6-digit PIN)
- Multi-Agent routing via `@agent` mentions (e.g. `@coder review this`)

## Permissions
- `net_outbound`: `["api.telegram.org"]`
- `secrets`: `["telegram_bot_token"]`
- `storage`: `true` (stores `last_update_id` for long polling)
