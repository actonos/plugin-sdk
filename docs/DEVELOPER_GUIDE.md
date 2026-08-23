# ActonOS Plugin Developer Guide

This document explains the architecture, abstractions, and API surfaces provided by `github.com/actonos/plugin-sdk/sdk`.

---

## 1. Core Abstractions

### 1.1. Context (`sdk.Context`)
Every plugin handler receives a `Context` interface providing safe access to host capabilities:

| Method | Return Type | Description |
|:---|:---|:---|
| `ctx.HTTP()` | `HTTPClient` | Outbound HTTP requests filtered by manifest domain egress whitelist. |
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
