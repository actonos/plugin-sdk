package sdk

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
)

//go:embed VERSION
var rawVersion string

// Global SDK and ABI Version Constants
var (
	// Version is the semantic version of the ActonOS Plugin SDK.
	Version = strings.TrimSpace(rawVersion)

	// ABIVersion defines the low-level WebAssembly Host-Guest syscall ABI protocol version.
	ABIVersion = "1.0.0"

	// MinActonOSVersion defines the minimum compatible ActonOS Host daemon version.
	MinActonOSVersion = "2.0.0"

	// WazeroRuntimeVersion represents the compatible Wazero runtime engine version.
	WazeroRuntimeVersion = "v1.12.0"
)

func init() {
	if Version == "" {
		Version = "2.0.0"
	}
}

// BuildInfo provides structured metadata about the SDK and runtime environment.
type BuildInfo struct {
	Version              string `json:"version"`
	ABIVersion           string `json:"abi_version"`
	MinActonOSVersion    string `json:"min_actonos_version"`
	WazeroRuntimeVersion string `json:"wazero_runtime_version"`
	GoVersion            string `json:"go_version"`
	Platform             string `json:"platform"`
}

// GetBuildInfo returns the runtime build and version metadata.
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:              Version,
		ABIVersion:           ABIVersion,
		MinActonOSVersion:    MinActonOSVersion,
		WazeroRuntimeVersion: WazeroRuntimeVersion,
		GoVersion:            runtime.Version(),
		Platform:             fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
