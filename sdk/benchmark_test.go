package sdk_test

import (
	"fmt"
	"testing"

	"github.com/actonos/acton-plugin-sdk/sdk"
	"github.com/actonos/acton-plugin-sdk/sdk/abi"
)

type BenchmarkPayload struct {
	ID        string            `json:"id" jsonschema:"description=Record ID,required"`
	Title     string            `json:"title" jsonschema:"description=Record title,required"`
	Score     float64           `json:"score" jsonschema:"description=Relevance score"`
	Active    bool              `json:"active"`
	Tags      []string          `json:"tags"`
	Metadata  map[string]string `json:"metadata"`
	SubRecord struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"sub_record"`
}

func BenchmarkGenerateSchema(b *testing.B) {
	var payload BenchmarkPayload
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sdk.GenerateSchema(payload)
	}
}

func BenchmarkPackUnpackPtrLen(b *testing.B) {
	var ptr uint32 = 0x12345678
	var len uint32 = 0x0000ABCD
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packed := abi.PackPtrLen(ptr, len)
		p, l := abi.UnpackPtrLen(packed)
		if p != ptr || l != len {
			b.Fatal("mismatch")
		}
	}
}

func BenchmarkMemoryAllocFree(b *testing.B) {
	data := []byte("Benchmark linear memory allocation performance in ActonOS Plugin SDK")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ptr, length := abi.BytesToPtr(data)
		_ = abi.PtrToBytes(ptr, length)
		abi.Free(ptr, length)
	}
}

func BenchmarkTypedToolExecution(b *testing.B) {
	tool := sdk.NewTypedTool("bench_tool", "Benchmark tool", func(ctx sdk.Context, in BenchmarkPayload) (*sdk.ToolResult, error) {
		return sdk.NewResultData("Processed "+in.Title, map[string]any{"id": in.ID, "score": in.Score}), nil
	})

	inputJSON := []byte(`{"id":"item_123","title":"Benchmark Document","score":98.5,"active":true,"tags":["benchmark","performance","wasm"]}`)
	ctx := sdk.NewContext()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := tool.Execute(ctx, inputJSON)
		if err != nil || res.Error != "" {
			b.Fatalf("tool failed: %v, error: %s", err, res.Error)
		}
	}
}

func TestUnicodeAndEmojiHandling(t *testing.T) {
	multilingualString := "ActonOS 🇻🇳 Hệ điều hành AI cho thiết bị phần cứng & ReAct Swarms. こんにちは世界！ 🚀 ⚡ 🤖"

	ptr, length := abi.StringToPtr(multilingualString)
	if ptr == 0 {
		t.Fatal("allocation failed")
	}

	recovered := abi.PtrToString(ptr, length)
	if recovered != multilingualString {
		t.Fatalf("unicode mismatch:\nExpected: %s\nGot:      %s", multilingualString, recovered)
	}

	abi.Free(ptr, length)
}

func TestLargePayloadHandling(t *testing.T) {
	// 2 MB test buffer
	largeData := make([]byte, 2*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	ptr, length := abi.BytesToPtr(largeData)
	if ptr == 0 || length != uint32(len(largeData)) {
		t.Fatalf("large allocation failed: ptr=%d, len=%d", ptr, length)
	}

	recovered := abi.PtrToBytes(ptr, length)
	if len(recovered) != len(largeData) {
		t.Fatalf("length mismatch: expected %d, got %d", len(largeData), len(recovered))
	}

	abi.Free(ptr, length)
}

func TestMalformedToolInputHandling(t *testing.T) {
	tool := sdk.NewTypedTool("safe_tool", "Safe tool", func(ctx sdk.Context, in BenchmarkPayload) (*sdk.ToolResult, error) {
		return sdk.NewResult("ok"), nil
	})

	ctx := sdk.NewContext()

	// Malformed JSON should not crash/panic, but return error
	res, err := tool.Execute(ctx, []byte(`{invalid-json-payload}`))
	if err == nil && (res == nil || res.Error == "") {
		t.Errorf("expected error on malformed input JSON, got %v", res)
	}

	// Empty input JSON
	res, err = tool.Execute(ctx, []byte(`{}`))
	if err != nil || res.Error != "" {
		t.Errorf("empty JSON object should be accepted with zero-values, got err: %v, res: %+v", err, res)
	}
}

func TestToolPanicSafety(t *testing.T) {
	panicTool := sdk.NewTypedTool("panic_tool", "Panicking tool", func(ctx sdk.Context, in BenchmarkPayload) (*sdk.ToolResult, error) {
		panic("simulated fatal crash in tool handler")
	})

	sdk.RegisterTool(panicTool)

	// Direct call via exported WASM boundary
	inputJSON := []byte(fmt.Sprintf(`{"tool_name":"panic_tool","input":{"id":"123"}}`))
	ptr, length := abi.BytesToPtr(inputJSON)
	defer abi.Free(ptr, length)

	packed := sdk.ActonToolExecuteTestWrapper(ptr, length)
	resPtr, resLen := abi.UnpackPtrLen(packed)
	defer abi.Free(resPtr, resLen)

	resBytes := abi.PtrToBytes(resPtr, resLen)
	if len(resBytes) == 0 {
		t.Fatalf("expected error result on panic, got empty bytes")
	}

	t.Logf("Panic caught safely: %s", string(resBytes))
}
