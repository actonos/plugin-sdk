package main

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/actonos/plugin-sdk/host"
	"github.com/actonos/plugin-sdk/sdk"
)

func TestBase64DecodeImage(t *testing.T) {
	sampleBytes := []byte("fake png binary data 12345")
	rawB64 := base64.StdEncoding.EncodeToString(sampleBytes)

	// Test case 1: Raw base64 string
	decoded, mime, err := decodeBase64Image(rawB64)
	if err != nil {
		t.Fatalf("unexpected error decoding raw base64: %v", err)
	}
	if string(decoded) != string(sampleBytes) {
		t.Errorf("expected %s, got %s", sampleBytes, decoded)
	}
	if mime != "image/png" {
		t.Errorf("expected default mime image/png, got %s", mime)
	}

	// Test case 2: Data URI with image/jpeg
	dataURI := "data:image/jpeg;base64," + rawB64
	decoded2, mime2, err := decodeBase64Image(dataURI)
	if err != nil {
		t.Fatalf("unexpected error decoding data URI: %v", err)
	}
	if string(decoded2) != string(sampleBytes) {
		t.Errorf("expected %s, got %s", sampleBytes, decoded2)
	}
	if mime2 != "image/jpeg" {
		t.Errorf("expected mime image/jpeg, got %s", mime2)
	}
}

func TestAspectRatioMapping(t *testing.T) {
	tests := map[string]string{
		"1:1":  "1024x1024",
		"16:9": "1536x1024",
		"9:16": "1024x1536",
		"4:3":  "1536x1024",
		"3:2":  "1536x1024",
		"":     "1024x1024",
	}

	for input, expected := range tests {
		actual := mapAspectRatioToSize("gpt-image-2", input)
		if actual != expected {
			t.Errorf("gpt-image-2 aspect ratio %s: expected %s, got %s", input, expected, actual)
		}
	}

	dalle3Tests := map[string]string{
		"16:9": "1792x1024",
		"9:16": "1024x1792",
		"1:1":  "1024x1024",
	}
	for input, expected := range dalle3Tests {
		actual := mapAspectRatioToSize("dall-e-3", input)
		if actual != expected {
			t.Errorf("dall-e-3 aspect ratio %s: expected %s, got %s", input, expected, actual)
		}
	}
}

func TestSchemaGeneration(t *testing.T) {
	schemaJSON := sdk.GenerateSchema(GenerateImageInput{})
	if len(schemaJSON) == 0 {
		t.Fatalf("expected non-empty schema JSON")
	}
}

func TestImageGeneratorPluginWasm(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "image-generator.wasm")

	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-trimpath", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("WASM build failed: %v\nOutput:\n%s", err, string(out))
	}

	ctx := context.Background()
	mockHost, err := host.NewMockHost(ctx)
	if err != nil {
		t.Fatalf("mock host init: %v", err)
	}
	defer mockHost.Close()

	mockHost.SetAllowedDomains([]string{"api.openai.com", "api.x.ai", "generativelanguage.googleapis.com", "api.nanobanana.ai"})
	mockHost.SetVaultSecret("openai_api_key", "sk-mock-openai-key-12345")
	mockHost.SetVaultSecret("grok_api_key", "xai-mock-grok-key-12345")
	mockHost.SetVaultSecret("nanobanana_api_key", "AIzaSyMockNanoBananaKey12345")

	// Mock OpenAI image generation API (gpt-image-2 returns b64_json)
	mockHost.MockHTTPRoute("https://api.openai.com/v1/images/generations", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}]}`,
	})

	// Mock OpenAI image edits API (multipart)
	mockHost.MockHTTPRoute("https://api.openai.com/v1/images/edits", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}]}`,
	})

	// Mock Google Generative Language interactions API (NanoBanana)
	mockHost.MockHTTPRoute("https://generativelanguage.googleapis.com/v1beta/interactions", host.HTTPMockResponse{
		Status: 200,
		Body:   `{"outputs":[{"type":"image","inline_data":{"mime_type":"image/png","data":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}}]}`,
	})

	runner, err := mockHost.LoadPluginFromFile(ctx, wasmPath)
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	defer runner.Close()

	// 1. Test generate_image tool with Text-to-Image (OpenAI)
	genInput := []byte(`{
		"prompt": "An astronaut riding a green horse on Mars, cinematic lighting",
		"aspect_ratio": "16:9",
		"session_id": "sess_user_001"
	}`)

	toolRes, err := runner.ExecuteTool("generate_image", genInput)
	if err != nil || toolRes.Error != "" {
		t.Fatalf("tool execution failed: %v, error: %s", err, toolRes.Error)
	}

	jobIDVal, ok := toolRes.Data["job_id"].(string)
	if !ok || jobIDVal == "" {
		t.Fatalf("expected job_id in tool response, got %v", toolRes.Data)
	}
	if toolRes.Data["model"] != "gpt-image-1.5" {
		t.Fatalf("expected default model gpt-image-1.5, got %v", toolRes.Data["model"])
	}
	if toolRes.Data["status"] != "COMPLETED" {
		t.Fatalf("expected status COMPLETED, got %v", toolRes.Data["status"])
	}
	directURL, ok := toolRes.Data["image_url"].(string)
	if !ok || !strings.Contains(directURL, "/api/workspace/raw?") {
		t.Fatalf("expected workspace raw image URL directly in generate_image result, got: %v", toolRes.Data["image_url"])
	}
	t.Logf("generate_image returned directly: %s (Image URL: %s)", toolRes.Content, directURL)

	// 2. Test generate_image tool with Binary Reference Image (Multipart)
	sampleImgBase64 := base64.StdEncoding.EncodeToString([]byte("fake png binary reference"))
	img2imgInput := []byte(`{
		"prompt": "Transform this character into cyberpunk style",
		"reference_images": [
			{
				"data_base64": "data:image/png;base64,` + sampleImgBase64 + `",
				"name": "character.png",
				"role": "subject"
			}
		],
		"session_id": "sess_user_002"
	}`)

	toolRes2, err := runner.ExecuteTool("generate_image", img2imgInput)
	if err != nil || toolRes2.Error != "" {
		t.Fatalf("img2img execution failed: %v, error: %s", err, toolRes2.Error)
	}
	t.Logf("generate_image (img2img) returned: %s", toolRes2.Content)

	// 4. Test generate_image with custom aspect ratio
	ratioInput := []byte(`{
		"prompt": "A futuristic banana city glowing at night",
		"aspect_ratio": "16:9",
		"quality": "auto",
		"session_id": "sess_user_003"
	}`)

	toolRes3, err := runner.ExecuteTool("generate_image", ratioInput)
	if err != nil || toolRes3.Error != "" {
		t.Fatalf("aspect ratio execution failed: %v, error: %s", err, toolRes3.Error)
	}
	if toolRes3.Data["model"] != "gpt-image-1.5" {
		t.Fatalf("expected configured model gpt-image-1.5, got %v", toolRes3.Data["model"])
	}
	t.Logf("generate_image (16:9) returned: %s", toolRes3.Content)
}
