package host_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/actonos/plugin-sdk/host"
)

func TestWasmCompilationAndExecution(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, "../examples/weather-tool")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compilation error: %v (output: %s)", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init error: %v", err)
	}
	defer mockHost.Close()

	// Configure mock HTTP response for weather API
	mockHost.MockHTTPRoute("https://api.open-meteo.com/v1/forecast", host.HTTPMockResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"current_weather":{"temperature":24.5,"windspeed":10.2,"weathercode":1,"is_day":1,"time":"2026-08-24T00:00"}}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("loading plugin wasm: %v", err)
	}
	defer runner.Close()

	// 2. Execute tool
	res, err := runner.ExecuteTool("get_weather", []byte(`{"city":"Tokyo"}`))
	if err != nil {
		t.Fatalf("execute tool error: %v", err)
	}

	t.Logf("Tool Result: Content='%s', Data=%+v, Error='%s'", res.Content, res.Data, res.Error)
	if res.Error != "" {
		t.Errorf("tool returned error: %s", res.Error)
	}

	if res.Content == "" {
		t.Errorf("expected non-empty content in tool result")
	}

	temp, ok := res.Data["temperature"].(float64)
	if !ok || temp != 24.5 {
		t.Errorf("expected temperature 24.5, got %v", res.Data["temperature"])
	}
}
