package host_test

import (
	"context"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func BenchmarkWazeroWasmToolExecution(b *testing.B) {
	wasmPath := "../plugins/saas/figma/dist/plugin.wasm"
	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		b.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.MockHTTPRoute("https://api.figma.com/v1/files/sample_key", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"name":"ActonOS Design System","version":"1.0"}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		b.Fatalf("loading plugin: %v", err)
	}
	defer runner.Close()

	inputJSON := []byte(`{"file_key":"sample_key"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := runner.ExecuteTool("get_file", inputJSON)
		if err != nil || res.Error != "" {
			b.Fatalf("failed: %v", err)
		}
	}
}
