# ActonOS Plugin Developer Guide

This document explains the architecture, abstractions, and API surfaces provided by `github.com/actonos/plugin-sdk/sdk`.

---

## 1. Core Abstractions

### 1.1. Context (`sdk.Context`)
Every plugin handler receives a `Context` interface providing safe access to host capabilities:

| Method | Return Type | Description |
|:---|:---|:---|
| `ctx.Config()` | `ConfigStore` | Typed access to user-configured plugin settings defined via `config_schema`. |
| `ctx.HTTP()` | `HTTPClient` | Outbound HTTP requests filtered by manifest domain egress whitelist. |
| `ctx.WS()` | `WebSocketClient` | Sandboxed real-time WebSocket client (`Dial`, `SendText`, `SendBinary`, `Poll`). |
| `ctx.Vault()` | `VaultClient` | Retrieve authorized tokens and credentials from the Hardware Vault. |
| `ctx.Storage()` | `KVStorage` | Persistent SQLite key-value partition unique to this plugin. |
| `ctx.EventBus()` | `EventBus` | Publish events onto the ActonOS internal message bus. |
| `ctx.Log()` | `Logger` | Structured logging with levels (Debug, Info, Warn, Error). |

---

## 2. Developing Tools (`sdk.Tool`)

Tools are callable functions exposed to ActonOS ReAct Agent Swarms.

### Generic Typed Tool with Automatic JSON Schema
```go
type QueryInput struct {
    SearchTerm string `json:"search_term" jsonschema:"description=Keywords to search,required"`
    MaxResults int    `json:"max_results" jsonschema:"description=Maximum results to return"`
}

func init() {
    tool := sdk.NewTypedTool("search_docs", "Search documentation", func(ctx sdk.Context, in QueryInput) (*sdk.ToolResult, error) {
        // Business logic...
        return sdk.NewResultData("Found 2 documents", map[string]any{"count": 2}), nil
    })
    sdk.RegisterTool(tool)
}
```

The SDK automatically reflects field types and tags (`jsonschema:"description=...,required,enum=..."`) to generate the tool schema.

---

## 3. Developing Chat Channels (`sdk.ChannelAdapter`)

Chat channels connect external messaging services (Telegram, Discord, Slack, WhatsApp, Zalo) with ActonOS agents.

Every channel plugin **must** use the same account schema and the same inbound/outbound envelope so the host can drive typing, reactions, and quote-replies uniformly.

### 3.1. Canonical account schema

Root `config_schema` is `{ poll_interval_seconds, accounts[] }`. Each account item follows `spec/CHANNEL_ACCOUNT_SCHEMA.json`:

| Field | Required | Notes |
|:---|:---|:---|
| `account_id` | yes | `^[a-z0-9_-]+$` |
| `display_name` | | UI label |
| *credential* | yes | Platform-specific (`bot_token`, or WhatsApp `access_token` + `phone_number_id`) |
| `default_agent` | yes | `x-ui-widget: agent-selector` |
| `listen_target` | | Optional conversation/channel filter (alias: `listen_channel_id`) |
| `enable_typing_indicator` | | default `true` |
| `enable_ack_reaction` | | default `true` |
| `enable_reply_quote` | | default `true` |
| `ack_reaction_emoji` | | default `👀` |

Embed `sdk.ChannelAccount` in the plugin account struct. Legacy root-level tokens are still read as a synthesized `account_id=default` account.

### 3.2. Canonical I/O envelope

`acton_channel_send` / `acton_channel_poll` normalize aliases in both directions. Hosts should prefer typed fields; metadata aliases stay valid.

**Inbound** (`sdk.InboundMessage`): `kind`, `message_id`, `chat_id`, `thread_id`, `timestamp`, `reaction`.

**Outbound** (`sdk.OutboundMessage`): `kind` (`text` \| `typing` \| `reaction` \| `media`), `chat_id`, `reply_to_id`, `thread_id`, `reaction`, `action`, `typing`, `file_name`, `mime_type`, `file_data`.

| Host intent | How to send | Plugin mapping |
|:---|:---|:---|
| Typing | `kind=typing` or `typing=true` or empty content | Discord `POST /typing`, Telegram/Zalo `sendChatAction`, WhatsApp `typing_indicator`, Slack no-op |
| Ack / react | `kind=reaction` + `reaction` + `reply_to_id` | Discord reactions, Telegram/Zalo `setMessageReaction`, Slack `reactions.add`, WhatsApp `type=reaction` |
| Quote reply | `reply_to_id` with `enable_reply_quote` | Discord `message_reference`, Telegram/Zalo `reply_to_message_id`, Slack `thread_ts`, WhatsApp `context.message_id` |
| File / document | `file_name` + `file_data` (raw bytes; JSON is base64) + optional caption in `content` | `msg.AttachedFile()` then platform upload. Telegram `sendDocument`/`sendPhoto`, Discord `files[0]`, Zalo `sendDocument`/`sendPhoto`, Slack `files.upload`, WhatsApp `/media` then send |

Use `sdk.ApplyInboundEnvelope(&msg, chatID, messageID, threadID, timestamp)` when emitting inbound events.

```go
type MyChannel struct {
    sdk.BaseChannel
}

func (c *MyChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
    token, _ := ctx.Vault().GetSecret("channel_token")
    if msg.WantsTyping() {
        // emit platform typing, return if msg.IsTypingOnly()
    }
    if name, _, data, ok := msg.AttachedFile(); ok {
        contentType, body, err := sdk.EncodeMultipart(map[string]string{
            "to": sdk.FirstNonEmpty(msg.ChatID, msg.Recipient),
        }, "file", name, data)
        if err != nil {
            return err
        }
        _, err = ctx.HTTP().PostBinary("https://api.mychat.com/upload", contentType, body)
        return err
    }
    return ctx.HTTP().PostJSONWithBearer("https://api.mychat.com/send", token, map[string]any{
        "to":   sdk.FirstNonEmpty(msg.ChatID, msg.Recipient),
        "text": msg.Content,
    })
}

func (c *MyChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
    msg := sdk.NewInboundMessage("telegram", "bot_primary", "123456", "Alice", "@coder please fix this bug")
    sdk.ApplyInboundEnvelope(&msg, "888", "42", "", "")
    return []sdk.InboundMessage{msg}, nil
}
```

---

## 4. Developing SaaS Connectors (`sdk.Connector`)

Connectors bridge third-party SaaS APIs (GitHub, Linear, Slack, Google Workspace, Notion) with multi-action routing, OAuth token brokering, and automatic Tool bridging.

```go
func init() {
    conn := sdk.NewBaseConnector("github", "GitHub", "oauth2").
        WithSecretKey("github_access_token")

    // Register typed action
    sdk.RegisterTypedAction(conn, "list_repos", "List GitHub repositories for user", func(ctx sdk.Context, in ListReposInput) (any, error) {
        token, _ := conn.GetAuthToken(ctx)
        resp, err := ctx.HTTP().GetWithBearer("https://api.github.com/user/repos", token)
        if err != nil {
            return nil, err
        }
        var repos []map[string]any
        _ = resp.JSON(&repos)
        return repos, nil
    })

    // Register as SaaS Connector
    sdk.RegisterConnector(conn)

    // Bridge connector actions as callable Tools for ReAct Agent Swarms!
    for _, tool := range conn.AsTools() {
        sdk.RegisterTool(tool)
    }
}
```

---

## 5. Dynamic Configuration & Multi-Account Patterns (`config_schema`)

Plugins can declare a flexible, schema-driven settings configuration in their `manifest.json`. ActonOS Web UI reads this schema to automatically render configuration forms without writing custom frontend code.

### 5.1. Declaring `config_schema` in `manifest.json`

```json
{
  "config_schema": {
    "type": "object",
    "properties": {
      "poll_interval_seconds": {
        "type": "integer",
        "title": "Polling Interval (seconds)",
        "default": 3,
        "x-ui-group": "General Settings"
      },
      "accounts": {
        "type": "array",
        "title": "Bot Accounts",
        "x-ui-group": "Bot Accounts",
        "items": {
          "type": "object",
          "required": ["account_id", "bot_token", "default_agent"],
          "properties": {
            "account_id": {
              "type": "string",
              "title": "Account ID",
              "pattern": "^[a-z0-9_-]+$",
              "x-ui-placeholder": "bot_support"
            },
            "bot_token": {
              "type": "string",
              "title": "Bot Token",
              "x-secret": true,
              "x-ui-widget": "password"
            },
            "default_agent": {
              "type": "string",
              "title": "Default Agent",
              "x-ui-widget": "agent-selector"
            },
            "enable_typing_indicator": { "type": "boolean", "default": true },
            "enable_ack_reaction": { "type": "boolean", "default": true },
            "enable_reply_quote": { "type": "boolean", "default": true },
            "ack_reaction_emoji": { "type": "string", "default": "👀" }
          }
        }
      }
    }
  }
}
```

### 5.2. Reading Configuration in Plugin Code

```go
type MyPluginConfig struct {
    PollIntervalSeconds int              `json:"poll_interval_seconds"`
    Accounts            []AccountConfig  `json:"accounts"`
}

type AccountConfig struct {
    sdk.ChannelAccount
}

func (c *MyChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
    var cfg MyPluginConfig
    if err := ctx.Config().Bind(&cfg); err != nil {
        return nil, err
    }

    for _, acc := range cfg.Accounts {
        // Retrieve secret token saved in Hardware Vault by ActonOS
        token, _ := ctx.Vault().GetSecret("discord_bot_tokens." + acc.AccountID)
        // Poll for this account...
    }
    // ...
}
```

