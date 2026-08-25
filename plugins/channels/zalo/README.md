# Zalo Bot Platform Channel Plugin

Official **Zalo Bot Platform** (`bot.zapps.me` / `bot-api.zaloplatforms.com`) chat channel integration for ActonOS AI agents.

## Features

- 🤖 **Zalo Bot Platform API Integration**: Integrates directly with official Zalo Bot endpoints.
- 📝 **Markdown Rich Text & Resilient Parsing**: Full Markdown support (`**bold**`, `*italic*`, `# Headings`, code blocks, lists) with automatic plain-text fallback if Markdown syntax fails.
- ⚡ **Live Real-Time Typing Indicators (`typing`)**: Canonical outbound `kind=typing` plus inbound ack while the agent computes the answer.
- 😊 **Acknowledgement Reactions**: Canonical `kind=reaction` / inbound ack emoji (`setMessageReaction`).
- 💬 **Auto Quote & Threaded Replies**: Outbound `reply_to_id` maps to `reply_to_message_id`.
- 🖼️ **Multi-Media Message Support**:
  - **Photos / Images (`sendPhoto`)**: Public HTTPS image URLs, or host `file_data` as a data URI.
  - **Documents / Files (`sendDocument`)**: Public HTTPS URLs or host `file_data` as a data URI (Zalo Bot API is JSON+URL, not Telegram multipart).
  - **Voice / Audio Notes (`sendVoice`)**: Public HTTPS `.aac` URLs or host `file_data`.
  - Responses with HTTP 200 and `"ok": false` are treated as send failures.
- 👥 **Group Chat & Direct Chat Routing**: Seamless handling for both `PRIVATE` and `GROUP` chat types with `@agent` mention extraction.
- 🏢 **Canonical `accounts[]` Gateway**: Same account schema as Discord/Telegram/Slack/WhatsApp (`account_id`, `bot_token`, `default_agent`, typing/reaction/quote flags).
- 🛡️ **Hardware Vault Isolation**: Isolated secret storage for `zalo_bot_token` and `zalo_bot_tokens.<account_id>`.
- 🔄 **Dual Receiving Modes**:
  - **Long-Polling Mode (`getUpdates`)**: High-performance local polling with configurable interval, auto conflict resolution, and automatic `deleteWebhook` recovery.
  - **Webhook Mode**: Receives events queued through ActonOS Webhook gateway.

## Endpoints Used

| Endpoint | Method | Description |
| :--- | :---: | :--- |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/getMe` | `POST` | Validates bot token & returns bot name / ID |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendMessage` | `POST` | Transmits markdown text messages with chunking & quote replies |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendChatAction` | `POST` | Broadcasts real-time typing indicators |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/setMessageReaction` | `POST` | Acknowledges inbound messages with an emoji reaction |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendPhoto` | `POST` | Sends images with markdown captions |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendDocument` | `POST` | Sends documents with custom file names & captions |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendVoice` | `POST` | Sends voice/audio notes |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/getUpdates` | `POST` | Long-polling message updates with timeout |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/deleteWebhook` | `POST` | Clears active webhooks to ensure long-polling delivery |

## Permissions

- `net_outbound`: `["bot-api.zaloplatforms.com", "openapi.zalo.me", "bot.zapps.me", "api.zapps.me"]`
- `secrets`: `["zalo_bot_token", "zalo_bot_tokens.*", "zalo_tokens.*"]`
- `storage`: `true`
- `bus_events`: `["channel.zalo.received", "channel.zalo.sent", "channel.zalo.action"]`
