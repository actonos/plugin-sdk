package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	pluginType := fs.String("type", "tool", "Plugin capability type: 'tool', 'channel', or 'connector'")
	outputDir := fs.String("dir", "", "Target directory (defaults to plugin name)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("plugin name is required. Example: acton-plugin new my-tool --type=tool")
	}

	pluginName := remaining[0]
	targetDir := *outputDir
	if targetDir == "" {
		targetDir = pluginName
	}

	pluginTypeLower := strings.ToLower(*pluginType)
	switch pluginTypeLower {
	case "tool", "channel", "connector":
	default:
		return fmt.Errorf("invalid plugin type '%s', must be 'tool', 'channel', or 'connector'", *pluginType)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating directory '%s': %w", targetDir, err)
	}

	// 1. Generate manifest.json
	manifestContent := generateManifest(pluginName, pluginTypeLower)
	if err := os.WriteFile(filepath.Join(targetDir, "manifest.json"), []byte(manifestContent), 0644); err != nil {
		return err
	}

	// 2. Generate main.go
	mainGoContent := generateMainGo(pluginName, pluginTypeLower)
	if err := os.WriteFile(filepath.Join(targetDir, "main.go"), []byte(mainGoContent), 0644); err != nil {
		return err
	}

	// 3. Generate main_test.go
	testGoContent := generateTestGo(pluginName, pluginTypeLower)
	if err := os.WriteFile(filepath.Join(targetDir, "main_test.go"), []byte(testGoContent), 0644); err != nil {
		return err
	}

	// 4. Generate README.md
	readmeContent := generateReadme(pluginName, pluginTypeLower)
	if err := os.WriteFile(filepath.Join(targetDir, "README.md"), []byte(readmeContent), 0644); err != nil {
		return err
	}

	// 5. Generate .gitignore
	gitignoreContent := "dist/\n*.wasm\n*.sig\n*.actonpkg\n"
	if err := os.WriteFile(filepath.Join(targetDir, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		return err
	}

	fmt.Printf("✨ Created new %s plugin project '%s' in '%s'!\n\n", pluginTypeLower, pluginName, targetDir)
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", targetDir)
	fmt.Printf("  acton-plugin validate    # Validate manifest & permissions\n")
	fmt.Printf("  acton-plugin build       # Build WebAssembly binary\n")
	fmt.Printf("  acton-plugin test        # Run on local mock host\n")
	fmt.Printf("  acton-plugin pack        # Create .actonpkg distribution bundle\n")

	return nil
}

func generateManifest(name, pType string) string {
	cleanName := strings.ReplaceAll(name, "-", " ")
	cleanName = strings.Title(cleanName)

	switch pType {
	case "channel":
		return fmt.Sprintf(`{
  "id": "%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "Chat channel integration for %s",
  "author": "ActonOS Community",
  "license": "MIT",
  "capabilities": ["channel"],
  "permissions": {
    "net_outbound": ["*.example.com"],
    "secrets": ["bot_token"],
    "storage": true
  },
  "channels": [
    {
      "name": "%s",
      "display_name": "%s",
      "requires_pairing": true
    }
  ],
  "config_schema": {
    "type": "object",
    "properties": {
      "poll_interval_seconds": {
        "type": "integer",
        "title": "Polling Interval (seconds)",
        "default": 3,
        "minimum": 1,
        "maximum": 60,
        "x-ui-group": "General Settings"
      },
      "accounts": {
        "type": "array",
        "title": "Bot Accounts",
        "minItems": 1,
        "x-ui-group": "Bot Accounts",
        "items": {
          "type": "object",
          "required": ["account_id", "bot_token", "default_agent"],
          "properties": {
            "account_id": {
              "type": "string",
              "title": "Account ID",
              "pattern": "^[a-z0-9_-]+$",
              "x-ui-placeholder": "e.g. support_bot"
            },
            "display_name": { "type": "string", "title": "Display Name" },
            "bot_token": {
              "type": "string",
              "title": "Bot Token",
              "x-secret": true,
              "x-ui-widget": "password"
            },
            "default_agent": {
              "type": "string",
              "title": "Default Target Agent",
              "x-ui-widget": "agent-selector"
            },
            "enable_typing_indicator": { "type": "boolean", "title": "Live Typing Indicator", "default": true },
            "enable_ack_reaction": { "type": "boolean", "title": "Acknowledge with Reaction", "default": true },
            "enable_reply_quote": { "type": "boolean", "title": "Quote / Thread Reply", "default": true },
            "ack_reaction_emoji": { "type": "string", "title": "Acknowledgement Emoji", "default": "👀" }
          }
        }
      }
    }
  }
}
`, name, cleanName, cleanName, name, cleanName)

	case "connector":
		return fmt.Sprintf(`{
  "id": "%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "SaaS integration connector for %s",
  "author": "ActonOS Community",
  "license": "MIT",
  "capabilities": ["connector"],
  "permissions": {
    "net_outbound": ["api.example.com"],
    "secrets": ["oauth_access_token"],
    "storage": true
  },
  "connectors": [
    {
      "name": "%s",
      "display_name": "%s",
      "auth_type": "oauth2",
      "actions": ["list_items", "create_item"]
    }
  ]
}
`, name, cleanName, cleanName, name, cleanName)

	default: // "tool"
		return fmt.Sprintf(`{
  "id": "%s",
  "name": "%s",
  "version": "1.0.0",
  "description": "Agent tool extension for %s",
  "author": "ActonOS Community",
  "license": "MIT",
  "capabilities": ["tool"],
  "permissions": {
    "net_outbound": ["*"],
    "storage": false
  },
  "tools": [
    {
      "name": "%s",
      "description": "Executes %s action for ReAct agents",
      "category": "plugin",
      "parameters": {
        "type": "object",
        "required": ["input"],
        "properties": {
          "input": {
            "type": "string",
            "description": "Input argument text"
          }
        }
      }
    }
  ]
}
`, name, cleanName, cleanName, name, cleanName)
	}
}

func generateMainGo(name, pType string) string {
	switch pType {
	case "channel":
		return fmt.Sprintf(`package main

import (
	"github.com/actonos/plugin-sdk/sdk"
)

type %sChannel struct {
	sdk.BaseChannel
}

func (c *%sChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
	ctx.Log().Info("Sending outbound message", "recipient", msg.Recipient, "content", msg.Content)
	// Example: Call channel API using ctx.HTTP().PostJSON(...) or ctx.HTTP().PostJSONWithBearer(...)
	return nil
}

func (c *%sChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
	ctx.Log().Debug("Polling for new messages")
	// Example: Return parsed messages using sdk.NewInboundMessage(...)
	return nil, nil
}

func init() {
	ch := &%sChannel{
		BaseChannel: sdk.BaseChannel{
			ChannelName:        "%s",
			ChannelDisplayName: "%s",
			PairingRequired:    true,
		},
	}
	sdk.RegisterChannel(ch)
}

func main() {
	sdk.Serve()
}
`, toPascalCase(name), toPascalCase(name), toPascalCase(name), toPascalCase(name), name, strings.Title(strings.ReplaceAll(name, "-", " ")))

	case "connector":
		return fmt.Sprintf(`package main

import (
	"fmt"
	"github.com/actonos/plugin-sdk/sdk"
)

type ListItemsInput struct {
	Limit int `+"`"+`json:"limit" jsonschema:"description=Maximum items to fetch"`+"`"+`
}

func init() {
	conn := sdk.NewBaseConnector("%s", "%s", "oauth2").
		WithSecretKey("%s_access_token")

	sdk.RegisterTypedAction(conn, "list_items", "List items from %s", func(ctx sdk.Context, in ListItemsInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("missing token: %%w", err)
		}
		ctx.Log().Info("Fetching items with token", "limit", in.Limit)
		return map[string]any{"status": "ok", "token_present": token != ""}, nil
	})

	sdk.RegisterConnector(conn)
	for _, tool := range conn.AsTools() {
		sdk.RegisterTool(tool)
	}
}

func main() {
	sdk.Serve()
}
`, name, strings.Title(strings.ReplaceAll(name, "-", " ")), name, strings.Title(strings.ReplaceAll(name, "-", " ")))

	default: // "tool"
		return fmt.Sprintf(`package main

import (
	"fmt"
	"github.com/actonos/plugin-sdk/sdk"
)

// %sInput defines the JSON schema parameters for this tool.
type %sInput struct {
	Query string `+"`"+`json:"query" jsonschema:"description=Search query or input parameter,required"`+"`"+`
}

func init() {
	tool := sdk.NewTypedTool("%s", "Executes %s action", func(ctx sdk.Context, in %sInput) (*sdk.ToolResult, error) {
		ctx.Log().Info("Executing %s tool", "query", in.Query)

		if in.Query == "" {
			return sdk.NewResultError("query parameter is required"), nil
		}

		resultText := fmt.Sprintf("Processed query: %%s", in.Query)
		return sdk.NewResult(resultText), nil
	})

	sdk.RegisterTool(tool)
}

func main() {
	sdk.Serve()
}
`, toPascalCase(name), toPascalCase(name), name, name, toPascalCase(name), name)
	}
}

func generateTestGo(name, pType string) string {
	return fmt.Sprintf(`package main

import (
	"testing"
	"github.com/actonos/plugin-sdk/sdk"
)

func TestPluginLogic(t *testing.T) {
	ctx := sdk.NewContext()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}
`)
}

func generateReadme(name, pType string) string {
	return fmt.Sprintf(`# %s Plugin for ActonOS

This is an ActonOS %s plugin compiled to WebAssembly.

## Building & Testing

`+"```bash"+`
# 1. Validate manifest and permissions
acton-plugin validate

# 2. Compile to WebAssembly
acton-plugin build

# 3. Test with local Wazero mock host
acton-plugin test

# 4. Pack into distribution bundle
acton-plugin pack
`+"```"+`
`, strings.Title(strings.ReplaceAll(name, "-", " ")), pType)
}

func toPascalCase(s string) string {
	parts := strings.Split(s, "-")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}
