package sdk

import (
	"encoding/json"
	"fmt"
)

// ActionHandler is a function that executes a specific SaaS connector action.
type ActionHandler func(ctx Context, params []byte) (any, error)

// Connector defines the contract for SaaS service integrations (GitHub, Notion, Linear, Airtable, etc.)
type Connector interface {
	Name() string
	DisplayName() string
	AuthType() string // "oauth2", "api_key", "bearer"
	Actions() []string
	DispatchAction(ctx Context, action string, params []byte) (any, error)
}

// BaseConnector provides a flexible implementation for SaaS connectors with action routing.
type BaseConnector struct {
	ConnectorName        string
	ConnectorDisplayName string
	AuthenticationType   string
	SecretKeyName        string
	actionHandlers       map[string]ActionHandler
	actionDescriptions   map[string]string
	actionSchemas        map[string]json.RawMessage
}

// NewBaseConnector creates a new BaseConnector instance.
func NewBaseConnector(name, displayName, authType string) *BaseConnector {
	return &BaseConnector{
		ConnectorName:        name,
		ConnectorDisplayName: displayName,
		AuthenticationType:   authType,
		SecretKeyName:        name + "_access_token",
		actionHandlers:       make(map[string]ActionHandler),
		actionDescriptions:   make(map[string]string),
		actionSchemas:        make(map[string]json.RawMessage),
	}
}

func (b *BaseConnector) Name() string        { return b.ConnectorName }
func (b *BaseConnector) DisplayName() string { return b.ConnectorDisplayName }
func (b *BaseConnector) AuthType() string    { return b.AuthenticationType }

// WithSecretKey configures the specific secret key name used to lookup tokens in Hardware Vault.
func (b *BaseConnector) WithSecretKey(secretKey string) *BaseConnector {
	b.SecretKeyName = secretKey
	return b
}

// GetAuthToken retrieves the connector's OAuth/API token from the Hardware Vault.
func (b *BaseConnector) GetAuthToken(ctx Context) (string, error) {
	if b.SecretKeyName == "" {
		return "", fmt.Errorf("connector %s has no secret key configured", b.ConnectorName)
	}
	return ctx.Vault().GetSecret(b.SecretKeyName)
}

func (b *BaseConnector) Actions() []string {
	var names []string
	for k := range b.actionHandlers {
		names = append(names, k)
	}
	return names
}

// RegisterAction binds a named action to a handler function.
func (b *BaseConnector) RegisterAction(action string, handler ActionHandler) {
	if b.actionHandlers == nil {
		b.actionHandlers = make(map[string]ActionHandler)
	}
	b.actionHandlers[action] = handler
}

func (b *BaseConnector) DispatchAction(ctx Context, action string, params []byte) (any, error) {
	handler, exists := b.actionHandlers[action]
	if !exists {
		return nil, fmt.Errorf("unknown connector action: %s", action)
	}
	return handler(ctx, params)
}

// RegisterTypedAction binds a strongly-typed struct handler to a connector action with auto-generated schema.
func RegisterTypedAction[T any](b *BaseConnector, action, description string, handler func(ctx Context, input T) (any, error)) {
	var sample T
	schema := GenerateSchema(sample)

	if b.actionDescriptions == nil {
		b.actionDescriptions = make(map[string]string)
	}
	if b.actionSchemas == nil {
		b.actionSchemas = make(map[string]json.RawMessage)
	}
	b.actionDescriptions[action] = description
	b.actionSchemas[action] = schema

	b.RegisterAction(action, func(ctx Context, params []byte) (any, error) {
		var input T
		if len(params) > 0 && string(params) != "{}" {
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid action payload: %w", err)
			}
		}
		return handler(ctx, input)
	})
}

// AsTools converts all registered connector actions into callable Agent Tools.
// Enables ReAct Agent Swarms to seamlessly invoke SaaS actions (e.g. connector_github_list_repos).
func (b *BaseConnector) AsTools() []Tool {
	var tools []Tool
	for action, handler := range b.actionHandlers {
		actName := action
		actHandler := handler
		toolName := fmt.Sprintf("connector_%s_%s", b.ConnectorName, actName)
		desc := b.actionDescriptions[actName]
		if desc == "" {
			desc = fmt.Sprintf("Execute %s action on %s connector", actName, b.ConnectorDisplayName)
		}
		schema := b.actionSchemas[actName]
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		tools = append(tools, &connectorToolBridge{
			toolName:    toolName,
			description: desc,
			schema:      schema,
			handler:     actHandler,
		})
	}
	return tools
}

type connectorToolBridge struct {
	toolName    string
	description string
	schema      json.RawMessage
	handler     ActionHandler
}

func (c *connectorToolBridge) Name() string                  { return c.toolName }
func (c *connectorToolBridge) Description() string           { return c.description }
func (c *connectorToolBridge) Category() string              { return "connector" }
func (c *connectorToolBridge) Schema() json.RawMessage       { return c.schema }
func (c *connectorToolBridge) Execute(ctx Context, inputJSON []byte) (*ToolResult, error) {
	out, err := c.handler(ctx, inputJSON)
	if err != nil {
		return NewResultError(err.Error()), nil
	}
	if out == nil {
		return NewResult("success"), nil
	}
	switch v := out.(type) {
	case string:
		return NewResult(v), nil
	case map[string]any:
		return NewResultData("success", v), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return NewResult(fmt.Sprintf("%v", v)), nil
		}
		return NewResult(string(b)), nil
	}
}
