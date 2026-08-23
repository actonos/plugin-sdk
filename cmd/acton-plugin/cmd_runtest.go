package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
)

func runTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	wasmPath := fs.String("wasm", "dist/plugin.wasm", "Path to compiled .wasm file")
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json")
	toolName := fs.String("tool", "", "Specific tool name to test execute")
	inputJSON := fs.String("input", "{}", "JSON input payload for tool execution")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Read manifest if available
	var manifest sdk.PluginManifest
	if manifestBytes, err := os.ReadFile(*manifestPath); err == nil {
		_ = json.Unmarshal(manifestBytes, &manifest)
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		return fmt.Errorf("initializing mock host: %w", err)
	}
	defer mockHost.Close()

	// Apply manifest permissions to mock host
	if manifest.Permissions != nil {
		if len(manifest.Permissions.NetOutbound) > 0 {
			mockHost.SetAllowedDomains(manifest.Permissions.NetOutbound)
		}
		for _, secret := range manifest.Permissions.Secrets {
			mockHost.SetVaultSecret(secret, "mock_secret_value_for_"+secret)
		}
	}

	fmt.Printf("🚀 Instantiating WASM plugin '%s' in Wazero sandbox...\n", *wasmPath)
	runner, err := mockHost.LoadPluginFromFile(ctx, *wasmPath)
	if err != nil {
		return fmt.Errorf("loading plugin: %w", err)
	}
	defer runner.Close()

	fmt.Println("✅ acton_plugin_init succeeded!")

	// If a tool test was requested or tools are present in manifest
	targetTool := *toolName
	if targetTool == "" && len(manifest.Tools) > 0 {
		targetTool = manifest.Tools[0].Name
	}

	if targetTool != "" {
		fmt.Printf("\n🧪 Executing Tool '%s' with input: %s\n", targetTool, *inputJSON)
		res, err := runner.ExecuteTool(targetTool, []byte(*inputJSON))
		if err != nil {
			return fmt.Errorf("executing tool '%s': %w", targetTool, err)
		}

		if res.Error != "" {
			fmt.Printf("❌ Tool returned error: %s\n", res.Error)
		} else {
			fmt.Printf("✅ Tool executed successfully!\n")
			fmt.Printf("📄 Output Content: %s\n", res.Content)
			if len(res.Data) > 0 {
				dataJSON, _ := json.MarshalIndent(res.Data, "", "  ")
				fmt.Printf("📊 Output Data:\n%s\n", string(dataJSON))
			}
		}
	}

	// If channels are present in manifest
	if len(manifest.Channels) > 0 {
		chName := manifest.Channels[0].Name
		fmt.Printf("\n🧪 Testing Channel '%s' polling...\n", chName)
		msgs, err := runner.PollChannelMessages()
		if err != nil {
			fmt.Printf("⚠️  Channel poll error: %v\n", err)
		} else {
			fmt.Printf("✅ Polled %d messages\n", len(msgs))
		}
	}

	// Print recorded host logs
	logs := mockHost.GetLogs()
	if len(logs) > 0 {
		fmt.Println("\n📋 Plugin Log Output:")
		for _, l := range logs {
			fmt.Printf("   [%s] %s\n", logLevelToString(l.Level), l.Message)
		}
	}

	fmt.Println("\n🎉 All sandbox execution tests passed!")
	return nil
}

func logLevelToString(lvl int32) string {
	switch lvl {
	case 1:
		return "DEBUG"
	case 2:
		return "INFO"
	case 3:
		return "WARN"
	case 4:
		return "ERROR"
	default:
		return "LOG"
	}
}
