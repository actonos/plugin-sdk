package sdk

import (
	"encoding/json"
	"fmt"
)

// Tool represents a callable tool within the ActonOS system.
type Tool interface {
	Name() string
	Description() string
	Category() string
	Schema() json.RawMessage
	Execute(ctx Context, inputJSON []byte) (*ToolResult, error)
}

// BaseTool provides default implementation for metadata.
type BaseTool struct {
	ToolName        string
	ToolDescription string
	ToolCategory    string
	ToolSchema      json.RawMessage
}

func (b *BaseTool) Name() string                  { return b.ToolName }
func (b *BaseTool) Description() string           { return b.ToolDescription }
func (b *BaseTool) Category() string              { return b.ToolCategory }
func (b *BaseTool) Schema() json.RawMessage       { return b.ToolSchema }

// TypedToolFunc is a handler function that takes a strongly-typed input and returns a result.
type TypedToolFunc[T any] func(ctx Context, input T) (*ToolResult, error)

type genericTypedTool[T any] struct {
	name        string
	description string
	category    string
	schema      json.RawMessage
	handler     TypedToolFunc[T]
}

func (g *genericTypedTool[T]) Name() string            { return g.name }
func (g *genericTypedTool[T]) Description() string     { return g.description }
func (g *genericTypedTool[T]) Category() string        { return g.category }
func (g *genericTypedTool[T]) Schema() json.RawMessage { return g.schema }

func (g *genericTypedTool[T]) Execute(ctx Context, inputJSON []byte) (*ToolResult, error) {
	var input T
	if len(inputJSON) > 0 && string(inputJSON) != "{}" {
		if err := json.Unmarshal(inputJSON, &input); err != nil {
			return nil, fmt.Errorf("invalid tool input payload: %w", err)
		}
	}
	return g.handler(ctx, input)
}

// NewTypedTool creates an ergonomic type-safe Tool from a Go struct and a handler function.
// Schema is automatically generated from struct T.
func NewTypedTool[T any](name, description string, handler TypedToolFunc[T]) Tool {
	var sample T
	schema := GenerateSchema(sample)

	return &genericTypedTool[T]{
		name:        name,
		description: description,
		category:    "wasm",
		schema:      schema,
		handler:     handler,
	}
}
