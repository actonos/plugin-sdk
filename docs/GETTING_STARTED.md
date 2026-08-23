# Getting Started with ActonOS Plugin SDK

This guide will walk you through creating, compiling, testing, and distributing your first **ActonOS WebAssembly Plugin** in under 5 minutes using the **ActonOS Plugin SDK**.

---

## 1. Prerequisites

- **Go 1.22+** (Go 1.24+ or 1.26+ recommended with native `wasip1` / `wasm` support).
- Build the `acton-plugin` CLI:
  ```bash
  go build -o acton-plugin ./cmd/acton-plugin/
  ```

---

## 2. Scaffold a New Plugin

Use the `acton-plugin new` command to scaffold a project:

```bash
# Create a new Tool plugin
acton-plugin new currency-converter --type=tool

# Or create a Chat Channel plugin
acton-plugin new slack-channel --type=channel

# Or create a SaaS Connector plugin
acton-plugin new linear-connector --type=connector
```

This creates a directory with:
- `manifest.json`: Capabilities and sandbox permissions.
- `main.go`: Tool implementation using `github.com/actonos/acton-plugin-sdk/sdk`.
- `main_test.go`: Unit tests.
- `README.md`: Plugin documentation.

---

## 3. Implement Your Tool Logic

Open `currency-converter/main.go` and define your strongly-typed inputs and business logic:

```go
package main

import (
	"fmt"
	"github.com/actonos/acton-plugin-sdk/sdk"
)

type ConvertInput struct {
	From   string  `json:"from" jsonschema:"description=Source currency code (e.g. USD),required"`
	To     string  `json:"to" jsonschema:"description=Target currency code (e.g. EUR, VND),required"`
	Amount float64 `json:"amount" jsonschema:"description=Amount to convert,required"`
}

func init() {
	converterTool := sdk.NewTypedTool("convert_currency", "Convert currency rates in real-time", func(ctx sdk.Context, in ConvertInput) (*sdk.ToolResult, error) {
		ctx.Log().Info("Converting currency", "from", in.From, "to", in.To, "amount", in.Amount)

		// Perform outbound HTTP request via sandbox proxy
		resp, err := ctx.HTTP().Get(fmt.Sprintf("https://api.exchangerate-api.com/v4/latest/%s", in.From))
		if err != nil {
			return nil, err
		}

		// Return structured data for ReAct agents
		return sdk.NewResultData(fmt.Sprintf("%.2f %s = %.2f %s", in.Amount, in.From, in.Amount*1.1, in.To), map[string]any{
			"from":   in.From,
			"to":     in.To,
			"amount": in.Amount,
			"rate":   1.1,
		}), nil
	})

	sdk.RegisterTool(converterTool)
}

func main() {
	sdk.Serve()
}
```

---

## 4. Validate Manifest Permissions

```bash
acton-plugin validate
```

Ensures your manifest matches JSON schemas and adheres to Least-Privilege security guidelines.

---

## 5. Compile to WebAssembly

```bash
acton-plugin build
```

This automatically compiles the plugin with:
`GOOS=wasip1 GOARCH=wasm -buildmode=c-shared`
into `dist/plugin.wasm`.

---

## 6. Test in Local Mock Sandbox

Test your plugin in the embedded Wazero mock host without running the full ActonOS server:

```bash
acton-plugin test --input='{"from":"USD","to":"EUR","amount":100}'
```

---

## 7. Sign and Package for Distribution

```bash
# Generate keypair (first time)
acton-plugin sign --gen-key

# Sign bytecode and manifest
acton-plugin sign

# Package into .actonpkg bundle
acton-plugin pack
```

Upload the resulting `.actonpkg` file directly into ActonOS via **Web UI (Settings > Plugins > Upload)** or drop it into `/data/plugins/`.
