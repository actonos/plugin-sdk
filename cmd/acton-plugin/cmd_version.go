package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/actonos/plugin-sdk/sdk"
)

// Injected during build via -ldflags:
// -ldflags "-X main.GitCommit=... -X main.BuildDate=..."
var (
	GitCommit = "dev"
	BuildDate = "2026-08-24T18:00:00Z"
)

func init() {
	if GitCommit == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					GitCommit = setting.Value
					if len(GitCommit) > 7 {
						GitCommit = GitCommit[:7]
					}
				}
				if setting.Key == "vcs.time" {
					BuildDate = setting.Value
				}
			}
		}
	}
}

type CLIVersionInfo struct {
	CLIVersion           string `json:"cli_version"`
	SDKVersion           string `json:"sdk_version"`
	ABIVersion           string `json:"abi_version"`
	MinActonOSVersion    string `json:"min_actonos_version"`
	WazeroRuntimeVersion string `json:"wazero_runtime_version"`
	GoVersion            string `json:"go_version"`
	Platform             string `json:"platform"`
	GitCommit            string `json:"git_commit"`
	BuildDate            string `json:"build_date"`
}

func getCLIVersionInfo() CLIVersionInfo {
	return CLIVersionInfo{
		CLIVersion:           sdk.Version,
		SDKVersion:           sdk.Version,
		ABIVersion:           sdk.ABIVersion,
		MinActonOSVersion:    sdk.MinActonOSVersion,
		WazeroRuntimeVersion: sdk.WazeroRuntimeVersion,
		GoVersion:            runtime.Version(),
		Platform:             fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		GitCommit:            GitCommit,
		BuildDate:            BuildDate,
	}
}

func runVersion(args []string) error {
	fsFlags := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOutput := fsFlags.Bool("json", false, "Output version information in JSON format")
	shortOutput := fsFlags.Bool("short", false, "Output only version number")

	if err := fsFlags.Parse(args); err != nil {
		return err
	}

	info := getCLIVersionInfo()

	if *shortOutput {
		fmt.Printf("v%s\n", info.CLIVersion)
		return nil
	}

	if *jsonOutput {
		b, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}

	fmt.Println("=================================================================")
	fmt.Println("⚡ ActonOS Plugin Developer CLI (acton-plugin)")
	fmt.Println("=================================================================")
	fmt.Printf("%-24s v%s\n", "CLI Version:", info.CLIVersion)
	fmt.Printf("%-24s v%s\n", "SDK Core Version:", info.SDKVersion)
	fmt.Printf("%-24s v%s\n", "WASM ABI Protocol:", info.ABIVersion)
	fmt.Printf("%-24s v%s+\n", "Min ActonOS Daemon:", info.MinActonOSVersion)
	fmt.Printf("%-24s %s\n", "Wazero JIT Runtime:", info.WazeroRuntimeVersion)
	fmt.Printf("%-24s %s (Target: wasip1/wasm)\n", "Go Compiler Version:", info.GoVersion)
	fmt.Printf("%-24s %s\n", "Target Platform:", info.Platform)
	fmt.Printf("%-24s %s\n", "Git Commit:", info.GitCommit)
	fmt.Printf("%-24s %s\n", "Build Timestamp:", info.BuildDate)
	fmt.Println("=================================================================")
	fmt.Println("Documentation: https://github.com/actonos/plugin-sdk")
	fmt.Println(strings.Repeat("-", 65))

	return nil
}
