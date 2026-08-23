package sdk_test

import (
	"encoding/json"
	"testing"

	"github.com/actonos/plugin-sdk/sdk"
	"github.com/actonos/plugin-sdk/sdk/abi"
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

type BotAccountConfig struct {
	AccountID    string `json:"account_id" jsonschema:"title=Account ID,placeholder=bot_support,required"`
	BotToken     string `json:"bot_token" jsonschema:"title=Bot Token,secret,widget=password,required"`
	DefaultAgent string `json:"default_agent" jsonschema:"title=Default Agent,widget=agent-selector"`
	Embeds       bool   `json:"embeds" jsonschema:"title=Enable Embeds"`
}

type PluginAppConfig struct {
	PollInterval int                `json:"poll_interval" jsonschema:"title=Poll Interval,group=General,order=1"`
	Accounts     []BotAccountConfig `json:"accounts" jsonschema:"title=Bot Accounts,group=Accounts,order=2"`
}

func TestDynamicConfigSchemaGeneration(t *testing.T) {
	var cfg PluginAppConfig
	schemaJSON := sdk.GenerateSchema(cfg)

	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatalf("failed to unmarshal generated schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in schema")
	}

	pollProp, ok := props["poll_interval"].(map[string]any)
	if !ok || pollProp["title"] != "Poll Interval" || pollProp["x-ui-group"] != "General" || pollProp["x-order"] != float64(1) {
		t.Errorf("invalid poll_interval schema: %v", pollProp)
	}

	accountsProp, ok := props["accounts"].(map[string]any)
	if !ok || accountsProp["type"] != "array" {
		t.Fatalf("expected accounts to be array schema: %v", accountsProp)
	}

	items, ok := accountsProp["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected items in accounts schema: %v", accountsProp)
	}

	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in items schema: %v", items)
	}

	tokenProp, ok := itemProps["bot_token"].(map[string]any)
	if !ok || tokenProp["x-secret"] != true || tokenProp["x-ui-widget"] != "password" {
		t.Errorf("expected bot_token to have x-secret:true and widget:password, got: %v", tokenProp)
	}

	agentProp, ok := itemProps["default_agent"].(map[string]any)
	if !ok || agentProp["x-ui-widget"] != "agent-selector" {
		t.Errorf("expected default_agent to have widget:agent-selector, got: %v", agentProp)
	}
}

func TestConfigStoreBinding(t *testing.T) {
	ctx := sdk.NewContext()

	rawConfig := `{
		"poll_interval": 5,
		"accounts": [
			{
				"account_id": "bot_cskh",
				"bot_token": "token_123",
				"default_agent": "agent-support",
				"embeds": true
			},
			{
				"account_id": "bot_devops",
				"bot_token": "token_456",
				"default_agent": "agent-ops",
				"embeds": false
			}
		]
	}`

	err := ctx.Storage().Set("__config", rawConfig)
	if err != nil {
		t.Fatalf("failed setting __config in storage: %v", err)
	}

	// Test GetInt, GetString
	if ctx.Config().GetInt("poll_interval", 1) != 5 {
		t.Errorf("expected poll_interval=5, got %d", ctx.Config().GetInt("poll_interval", 1))
	}

	// Test Bind
	var boundCfg PluginAppConfig
	if err := ctx.Config().Bind(&boundCfg); err != nil {
		t.Fatalf("failed binding config: %v", err)
	}

	if boundCfg.PollInterval != 5 {
		t.Errorf("expected PollInterval=5, got %d", boundCfg.PollInterval)
	}

	if len(boundCfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(boundCfg.Accounts))
	}

	if boundCfg.Accounts[0].AccountID != "bot_cskh" || boundCfg.Accounts[0].DefaultAgent != "agent-support" || !boundCfg.Accounts[0].Embeds {
		t.Errorf("invalid first account config: %+v", boundCfg.Accounts[0])
	}

	if boundCfg.Accounts[1].AccountID != "bot_devops" || boundCfg.Accounts[1].DefaultAgent != "agent-ops" || boundCfg.Accounts[1].Embeds {
		t.Errorf("invalid second account config: %+v", boundCfg.Accounts[1])
	}
}

func TestAgentMentionExtraction(t *testing.T) {
	agent, clean := sdk.ExtractAgentMention("@devops help check cluster status")
	if agent != "devops" || clean != "help check cluster status" {
		t.Errorf("unexpected mention parse: agent=%q, clean=%q", agent, clean)
	}

	agent, clean = sdk.ExtractAgentMention("/ask finance what is the MRR?")
	if agent != "finance" || clean != "what is the MRR?" {
		t.Errorf("unexpected /ask parse: agent=%q, clean=%q", agent, clean)
	}

	agent, clean = sdk.ExtractAgentMention("regular chat message")
	if agent != "" || clean != "regular chat message" {
		t.Errorf("unexpected plain text parse: agent=%q, clean=%q", agent, clean)
	}
}

