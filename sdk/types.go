package sdk

import (
	"encoding/json"
	"fmt"
)

// LogLevel defines the severity level for system logging.
type LogLevel int32

const (
	LogLevelDebug LogLevel = 1
	LogLevelInfo  LogLevel = 2
	LogLevelWarn  LogLevel = 3
	LogLevelError LogLevel = 4
)

// Capability defines the type of feature exposed by a plugin.
type Capability string

const (
	CapabilityTool      Capability = "tool"
	CapabilityChannel   Capability = "channel"
	CapabilityConnector Capability = "connector"
)

// PluginManifest defines the complete metadata and permission declaration of a plugin.
type PluginManifest struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Description  string          `json:"description,omitempty"`
	Author       string          `json:"author,omitempty"`
	License      string          `json:"license,omitempty"`
	Capabilities []Capability    `json:"capabilities"`
	Permissions  *Permissions    `json:"permissions,omitempty"`
	Tools        []ToolInfo      `json:"tools,omitempty"`
	Channels     []ChannelInfo   `json:"channels,omitempty"`
	Connectors   []ConnectorInfo `json:"connectors,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
	Config       map[string]any  `json:"config,omitempty"`
}

// Permissions declares the sandbox access boundaries requested by the plugin.
type Permissions struct {
	NetOutbound []string `json:"net_outbound,omitempty"` // Whitelisted domains (e.g. *.telegram.org, api.github.com)
	Secrets     []string `json:"secrets,omitempty"`      // Hardware Vault keys requested
	Storage     bool     `json:"storage,omitempty"`      // Enable isolated SQLite KV storage
	BusEvents   []string `json:"bus_events,omitempty"`   // Allowed event bus topics
}

// ToolInfo describes a tool callable by ActonOS ReAct Agent swarms.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// ChannelInfo describes a messaging channel integration.
type ChannelInfo struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	RequiresPairing bool   `json:"requires_pairing,omitempty"`
}

// ConnectorInfo describes a SaaS integration connector.
type ConnectorInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	AuthType    string   `json:"auth_type,omitempty"` // "oauth2", "api_key", "bearer"
	Actions     []string `json:"actions,omitempty"`
}

// ToolResult represents the output from executing a tool.
type ToolResult struct {
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// NewResult creates a successful ToolResult with text content.
func NewResult(content string) *ToolResult {
	return &ToolResult{Content: content}
}

// NewResultData creates a successful ToolResult with structured data.
func NewResultData(content string, data map[string]any) *ToolResult {
	return &ToolResult{
		Content: content,
		Data:    data,
	}
}

// NewResultError creates a failed ToolResult with an error message.
func NewResultError(errMessage string) *ToolResult {
	if errMessage == "" {
		errMessage = "unknown error"
	}
	return &ToolResult{
		Content: "Error: " + errMessage,
		Error:   errMessage,
	}
}

// MessageKind identifies the semantic type of a channel event.
type MessageKind string

const (
	MessageKindText     MessageKind = "text"
	MessageKindTyping   MessageKind = "typing"
	MessageKindReaction MessageKind = "reaction"
	MessageKindMedia    MessageKind = "media"
)

// InboundMessage represents an incoming message received from a chat channel.
// Canonical envelope fields (Kind, MessageID, ChatID, ThreadID, Timestamp, Reaction)
// are the host contract. Metadata keeps platform-specific extras and backward-compatible aliases.
type InboundMessage struct {
	ChannelID   string            `json:"channel_id"`
	AccountID   string            `json:"account_id"`
	SenderID    string            `json:"sender_id"`
	SenderName  string            `json:"sender_name"`
	TargetAgent string            `json:"target_agent,omitempty"`
	MentionText string            `json:"mention_text,omitempty"`
	Content     string            `json:"content"`
	Kind        MessageKind       `json:"kind,omitempty"`
	MessageID   string            `json:"message_id,omitempty"`
	ChatID      string            `json:"chat_id,omitempty"`
	ThreadID    string            `json:"thread_id,omitempty"`
	Timestamp   string            `json:"timestamp,omitempty"`
	Reaction    string            `json:"reaction,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a message sent to a channel recipient.
// Canonical envelope fields are the host contract. Metadata aliases remain supported.
type OutboundMessage struct {
	ChannelID string            `json:"channel_id"`
	AccountID string            `json:"account_id"`
	Recipient string            `json:"recipient"`
	Content   string            `json:"content"`
	Kind      MessageKind       `json:"kind,omitempty"`
	ChatID    string            `json:"chat_id,omitempty"`
	ReplyToID string            `json:"reply_to_id,omitempty"`
	ThreadID  string            `json:"thread_id,omitempty"`
	Reaction  string            `json:"reaction,omitempty"`
	Action    string            `json:"action,omitempty"`
	Typing    bool              `json:"typing,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// HTTPRequest represents a sandboxed outbound HTTP request.
type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// HTTPResponse represents the response returned from the Host HTTP proxy.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// JSON deserializes the HTTP response body into a target struct.
func (r *HTTPResponse) JSON(v any) error {
	if r == nil || r.Body == "" {
		return fmt.Errorf("empty response body")
	}
	return json.Unmarshal([]byte(r.Body), v)
}

// ConnectorActionPayload represents an action dispatched to a SaaS connector.
type ConnectorActionPayload struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}
