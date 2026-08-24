# Zalo Bot Platform Channel Plugin

Official **Zalo Bot Platform** (`bot.zapps.me` / `bot-api.zaloplatforms.com`) chat channel integration for ActonOS AI agents.

## Features

- 🤖 **Zalo Bot API Support**: Integrates directly with the official [Zalo Bot Platform API](https://bot.zapps.me/docs/apis/getMe/).
- 📝 **Markdown Rich Text (`parse_mode: "markdown"`)**: Supports `**bold**`, `*italic*`, `# Headings`, ordered/unordered lists, quotes, and color annotations.
- ⌨️ **Typing Indicators (`sendChatAction`)**: Real-time `"typing"` status while agents formulate responses.
- 🖼️ **Photo & Attachment Support (`sendPhoto`)**: Sends images with markdown captions.
- 🔄 **Dual Receiving Modes**:
  - **Webhook Mode**: Receives `message.text.received`, `message.image.received`, `message.voice.received` webhook requests.
  - **Long-Polling Mode (`getUpdates`)**: Local polling for development and standalone environments.
- 🏢 **Multi-Bot Accounts**: Supports multiple bot instances with independent tokens and agent routing.
- 🛡️ **Hardware Vault Isolation**: Secure secret storage for `zalo_bot_token` and `zalo_tokens.<account_id>`.

## Endpoints Used

| Endpoint | Method | Description |
| :--- | :---: | :--- |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/getMe` | `POST` | Validates bot token & returns account info |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendMessage` | `POST` | Transmits markdown text messages up to 2000 chars |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendChatAction` | `POST` | Emits typing indicator to chat |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/sendPhoto` | `POST` | Sends image with caption |
| `https://bot-api.zaloplatforms.com/bot${TOKEN}/getUpdates` | `POST` | Long-polling message reception |

## Permissions

- `net_outbound`: `["bot-api.zaloplatforms.com", "openapi.zalo.me"]`
- `secrets`: `["zalo_bot_token", "zalo_tokens.*"]`
- `storage`: `true`
- `bus_events`: `["channel.zalo.received", "channel.zalo.sent", "channel.zalo.action"]`
