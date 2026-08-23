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

Chat channels connect external messaging services (Telegram, Discord, Slack, WhatsApp, Webhooks) with ActonOS agents.

```go
type MyChannel struct {
    sdk.BaseChannel
}

func (c *MyChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
    token, _ := ctx.Vault().GetSecret("channel_token")
    return ctx.HTTP().PostJSONWithBearer("https://api.mychat.com/send", token, map[string]any{
        "to": msg.Recipient,
        "text": msg.Content,
    })
}

func (c *MyChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
    // Return received messages using sdk.NewInboundMessage (automatically parses @agent mentions)
    msg := sdk.NewInboundMessage("telegram", "bot_primary", "123456", "Alice", "@coder please fix this bug")
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
            }
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
    AccountID    string `json:"account_id"`
    DefaultAgent string `json:"default_agent"`
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

