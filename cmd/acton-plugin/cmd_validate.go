package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

var idRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)
var semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+`)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "manifest.json", "Path to manifest.json file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest file '%s': %w", *manifestPath, err)
	}

	var manifest sdk.PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("invalid manifest JSON syntax: %w", err)
	}

	var errors []string
	var warnings []string

	// 1. Required fields
	if manifest.ID == "" {
		errors = append(errors, "missing required field 'id'")
	} else if !idRegex.MatchString(manifest.ID) {
		errors = append(errors, fmt.Sprintf("invalid 'id' format '%s', must contain only lowercase letters, digits, dashes or underscores", manifest.ID))
	}

	if manifest.Name == "" {
		errors = append(errors, "missing required field 'name'")
	}

	if manifest.Version == "" {
		errors = append(errors, "missing required field 'version'")
	} else if !semverRegex.MatchString(manifest.Version) {
		errors = append(errors, fmt.Sprintf("invalid 'version' '%s', must follow semantic versioning (e.g. 1.0.0)", manifest.Version))
	}

	if len(manifest.Capabilities) == 0 {
		errors = append(errors, "missing required field 'capabilities' (must declare at least one: 'tool', 'channel', 'connector')")
	}

	// 2. Capability matches & Duplicate checks
	toolNames := make(map[string]bool)
	for _, t := range manifest.Tools {
		if t.Name == "" {
			errors = append(errors, "tool item has empty 'name'")
		} else if toolNames[t.Name] {
			errors = append(errors, fmt.Sprintf("duplicate tool name '%s'", t.Name))
		}
		toolNames[t.Name] = true
	}

	channelNames := make(map[string]bool)
	for _, c := range manifest.Channels {
		if c.Name == "" {
			errors = append(errors, "channel item has empty 'name'")
		} else if channelNames[c.Name] {
			errors = append(errors, fmt.Sprintf("duplicate channel name '%s'", c.Name))
		}
		channelNames[c.Name] = true
	}

	connNames := make(map[string]bool)
	for _, conn := range manifest.Connectors {
		if conn.Name == "" {
			errors = append(errors, "connector item has empty 'name'")
		} else if connNames[conn.Name] {
			errors = append(errors, fmt.Sprintf("duplicate connector name '%s'", conn.Name))
		}
		connNames[conn.Name] = true
	}

	for _, cap := range manifest.Capabilities {
		switch cap {
		case sdk.CapabilityTool:
			if len(manifest.Tools) == 0 {
				warnings = append(warnings, "declared capability 'tool' but 'tools' list is empty")
			}
		case sdk.CapabilityChannel:
			if len(manifest.Channels) == 0 {
				warnings = append(warnings, "declared capability 'channel' but 'channels' list is empty")
			}
		case sdk.CapabilityConnector:
			if len(manifest.Connectors) == 0 {
				warnings = append(warnings, "declared capability 'connector' but 'connectors' list is empty")
			}
		default:
			errors = append(errors, fmt.Sprintf("unknown capability '%s'", cap))
		}
	}

	// 3. Permissions & SSRF checks
	if manifest.Permissions != nil {
		for _, domain := range manifest.Permissions.NetOutbound {
			domainLower := strings.ToLower(strings.TrimSpace(domain))
			if domainLower == "*" {
				warnings = append(warnings, "permission net_outbound uses wildcard '*', allowing connections to any domain. Consider specifying exact domains (Least-Privilege principle).")
			} else if domainLower == "localhost" || domainLower == "127.0.0.1" || domainLower == "::1" ||
				domainLower == "169.254.169.254" || domainLower == "0.0.0.0" || strings.HasPrefix(domainLower, "192.168.") ||
				strings.HasPrefix(domainLower, "10.") || strings.HasPrefix(domainLower, "172.") {
				errors = append(errors, fmt.Sprintf("forbidden private/local IP in net_outbound '%s' (SSRF violation)", domain))
			}
		}
	}

	// Output report
	fmt.Printf("🔍 Validating '%s'...\n\n", *manifestPath)
	fmt.Printf("📋 Plugin ID:    %s\n", manifest.ID)
	fmt.Printf("📦 Name:         %s (v%s)\n", manifest.Name, manifest.Version)
	fmt.Printf("⚙️  Capabilities: %v\n\n", manifest.Capabilities)

	if len(warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, w := range warnings {
			fmt.Printf("   - %s\n", w)
		}
		fmt.Println()
	}

	if len(errors) > 0 {
		fmt.Println("❌ Errors:")
		for _, e := range errors {
			fmt.Printf("   - %s\n", e)
		}
		fmt.Println()
		return fmt.Errorf("manifest validation failed with %d error(s)", len(errors))
	}

	fmt.Println("✅ Manifest is valid and ready for deployment!")
	return nil
}
