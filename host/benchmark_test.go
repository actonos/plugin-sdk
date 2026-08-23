package host_test

import (
	"context"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func BenchmarkWazeroWasmToolExecution(b *testing.B) {
	wasmPath := "../examples/weather-tool/dist/plugin.wasm"
	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		b.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.MockHTTPRoute("https://api.open-meteo.com/v1/forecast", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"current_weather":{"temperature":24.5,"windspeed":10.2,"weathercode":1,"is_day":1,"time":"2026-08-24T00:00"}}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		b.Fatalf("loading plugin: %v", err)
	}
	defer runner.Close()

	inputJSON := []byte(`{"city":"Tokyo"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := runner.ExecuteTool("get_weather", inputJSON)
		if err != nil || res.Error != "" {
			b.Fatalf("failed: %v", err)
		}
	}
}
