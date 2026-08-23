package sdk_test

import (
	"encoding/json"
	"testing"

	"github.com/actonos/acton-plugin-sdk/sdk"
	"github.com/actonos/acton-plugin-sdk/sdk/abi"
)

type CityWeatherInput struct {
	City  string `json:"city" jsonschema:"description=Name of city,required"`
	Units string `json:"units" jsonschema:"description=Temperature unit,enum=celsius|fahrenheit"`
}

type NestedConfig struct {
	Endpoint string `json:"endpoint"`
	Timeout  int    `json:"timeout" jsonschema:"description=Timeout in seconds"`
}

type ComplexInput struct {
	Query  string       `json:"query" jsonschema:"required"`
	Config NestedConfig `json:"config"`
	Tags   []string     `json:"tags"`
}

func TestSchemaGenerator(t *testing.T) {
	var input CityWeatherInput
	schemaJSON := sdk.GenerateSchema(input)

	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatalf("failed to unmarshal generated schema: %v", err)
	}

	if schema["type"] != "object" {
		t.Errorf("expected type object, got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	cityProp, ok := props["city"].(map[string]any)
	if !ok || cityProp["type"] != "string" || cityProp["description"] != "Name of city" {
		t.Errorf("invalid city property schema: %v", cityProp)
	}

	req, ok := schema["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "city" {
		t.Errorf("expected required field 'city', got %v", req)
	}
}

func TestNestedSchemaGenerator(t *testing.T) {
	var input ComplexInput
	schemaJSON := sdk.GenerateSchema(input)

	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatalf("failed to unmarshal generated schema: %v", err)
	}

	props := schema["properties"].(map[string]any)
	configProp, ok := props["config"].(map[string]any)
	if !ok || configProp["type"] != "object" {
		t.Fatalf("expected config to be object schema, got %v", configProp)
	}

	tagsProp, ok := props["tags"].(map[string]any)
	if !ok || tagsProp["type"] != "array" {
		t.Fatalf("expected tags to be array schema, got %v", tagsProp)
	}
}

func TestPackedPointer(t *testing.T) {
	var ptr uint32 = 123456
	var len uint32 = 789

	packed := abi.PackPtrLen(ptr, len)
	unpackedPtr, unpackedLen := abi.UnpackPtrLen(packed)

	if unpackedPtr != ptr {
		t.Errorf("expected ptr %d, got %d", ptr, unpackedPtr)
	}
	if unpackedLen != len {
		t.Errorf("expected len %d, got %d", len, unpackedLen)
	}
}

func TestMemoryAllocFree(t *testing.T) {
	data := []byte("Hello ActonOS WASM Linear Memory")
	ptr, length := abi.BytesToPtr(data)

	if ptr == 0 || length != uint32(len(data)) {
		t.Fatalf("invalid alloc result: ptr=%d, len=%d", ptr, length)
	}

	recovered := abi.PtrToBytes(ptr, length)
	if string(recovered) != string(data) {
		t.Errorf("expected '%s', got '%s'", string(data), string(recovered))
	}

	abi.Free(ptr, length)
}

func TestTypedToolExecution(t *testing.T) {
	weatherTool := sdk.NewTypedTool("get_weather", "Fetch weather", func(ctx sdk.Context, in CityWeatherInput) (*sdk.ToolResult, error) {
		if in.City == "" {
			return sdk.NewResultError("city is required"), nil
		}
		return sdk.NewResultData("Weather for "+in.City, map[string]any{
			"temp": 28.5,
			"city": in.City,
		}), nil
	})

	ctx := sdk.NewContext()
	res, err := weatherTool.Execute(ctx, []byte(`{"city":"Tokyo","units":"celsius"}`))
	if err != nil {
		t.Fatalf("tool execution error: %v", err)
	}

	if res.Content != "Weather for Tokyo" {
		t.Errorf("unexpected content: %s", res.Content)
	}
	if res.Data["temp"] != 28.5 {
		t.Errorf("unexpected temp data: %v", res.Data["temp"])
	}
}

func TestContextStorageAndVault(t *testing.T) {
	ctx := sdk.NewContext()

	// Test KV Storage
	err := ctx.Storage().Set("last_sync", "2026-08-24T00:00:00Z")
	if err != nil {
		t.Fatalf("storage set failed: %v", err)
	}

	val, ok, err := ctx.Storage().Get("last_sync")
	if err != nil || !ok || val != "2026-08-24T00:00:00Z" {
		t.Errorf("unexpected storage get: val=%s, ok=%v, err=%v", val, ok, err)
	}

	// Test JSON KV Storage
	type SyncState struct {
		Counter int `json:"counter"`
	}
	err = ctx.Storage().SetJSON("state", SyncState{Counter: 42})
	if err != nil {
		t.Fatalf("storage set JSON failed: %v", err)
	}

	var state SyncState
	ok, err = ctx.Storage().GetJSON("state", &state)
	if err != nil || !ok || state.Counter != 42 {
		t.Errorf("unexpected storage get JSON: state=%+v, ok=%v, err=%v", state, ok, err)
	}

	// Test Vault
	secret, err := ctx.Vault().GetSecret("telegram_bot_token")
	if err != nil {
		t.Fatalf("vault get secret failed: %v", err)
	}
	if secret == "" {
		t.Errorf("expected secret, got empty string")
	}
}
