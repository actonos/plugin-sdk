package sdk

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/actonos/acton-plugin-sdk/sdk/abi"
)

type defaultContext struct {
	http     HTTPClient
	vault    VaultClient
	storage  KVStorage
	eventBus EventBus
	logger   Logger
}

// NewContext creates a default Context connected to Host ABI syscalls.
func NewContext() Context {
	return &defaultContext{
		http:     &defaultHTTPClient{},
		vault:    &defaultVaultClient{},
		storage:  &defaultKVStorage{},
		eventBus: &defaultEventBus{},
		logger:   &defaultLogger{},
	}
}

func (c *defaultContext) HTTP() HTTPClient       { return c.http }
func (c *defaultContext) Vault() VaultClient     { return c.vault }
func (c *defaultContext) Storage() KVStorage     { return c.storage }
func (c *defaultContext) EventBus() EventBus     { return c.eventBus }
func (c *defaultContext) Log() Logger            { return c.logger }

// --- HTTP Client Implementation ---

type defaultHTTPClient struct{}

func (h *defaultHTTPClient) Get(url string) (*HTTPResponse, error) {
	return h.Do(HTTPRequest{
		Method: "GET",
		URL:    url,
	})
}

func (h *defaultHTTPClient) GetWithBearer(url string, token string) (*HTTPResponse, error) {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return h.Do(HTTPRequest{
		Method:  "GET",
		URL:     url,
		Headers: headers,
	})
}

func (h *defaultHTTPClient) Post(url string, contentType string, body string) (*HTTPResponse, error) {
	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return h.Do(HTTPRequest{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    body,
	})
}

func (h *defaultHTTPClient) PostJSON(url string, body any) (*HTTPResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serializing json body: %w", err)
	}
	return h.Post(url, "application/json", string(b))
}

func (h *defaultHTTPClient) PostJSONWithBearer(url string, token string, body any) (*HTTPResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serializing json body: %w", err)
	}
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return h.Do(HTTPRequest{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    string(b),
	})
}

func (h *defaultHTTPClient) DoWithAuth(method, url, authHeader string, headers map[string]string, body any) (*HTTPResponse, error) {
	mergedHeaders := make(map[string]string)
	for k, v := range headers {
		mergedHeaders[k] = v
	}
	if authHeader != "" {
		mergedHeaders["Authorization"] = authHeader
	}

	bodyStr := ""
	if body != nil {
		switch b := body.(type) {
		case string:
			bodyStr = b
		case []byte:
			bodyStr = string(b)
		default:
			marshaled, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("marshaling body: %w", err)
			}
			bodyStr = string(marshaled)
			if mergedHeaders["Content-Type"] == "" {
				mergedHeaders["Content-Type"] = "application/json"
			}
		}
	}

	return h.Do(HTTPRequest{
		Method:  method,
		URL:     url,
		Headers: mergedHeaders,
		Body:    bodyStr,
	})
}

func (h *defaultHTTPClient) Do(req HTTPRequest) (*HTTPResponse, error) {
	if req.Method == "" {
		req.Method = "GET"
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling http request: %w", err)
	}

	reqPtr, reqLen := abi.BytesToPtr(reqBytes)
	defer abi.Free(reqPtr, reqLen)

	resLen := abi.NetHTTPRequest(reqPtr, reqLen)
	if resLen == 0 {
		return nil, fmt.Errorf("empty response from host http proxy")
	}

	resBuf := make([]byte, resLen)
	destPtr, _ := abi.BytesToPtr(resBuf)
	defer abi.Free(destPtr, resLen)

	abi.SysReadResponse(destPtr, resLen)
	resBytes := abi.PtrToBytes(destPtr, resLen)

	var resp HTTPResponse
	if err := json.Unmarshal(resBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing http response JSON: %w (raw: %s)", err, string(resBytes))
	}
	return &resp, nil
}

// --- Vault Client Implementation ---

type defaultVaultClient struct{}

func (v *defaultVaultClient) GetSecret(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("secret key cannot be empty")
	}
	keyPtr, keyLen := abi.StringToPtr(key)
	defer abi.Free(keyPtr, keyLen)

	resLen := abi.VaultGetSecret(keyPtr, keyLen)
	if resLen == 0 {
		return "", fmt.Errorf("secret not found or unauthorized: %s", key)
	}

	destBuf := make([]byte, resLen)
	destPtr, _ := abi.BytesToPtr(destBuf)
	defer abi.Free(destPtr, resLen)

	abi.SysReadResponse(destPtr, resLen)
	return abi.PtrToString(destPtr, resLen), nil
}

// --- KV Storage Implementation ---

type defaultKVStorage struct{}

func (s *defaultKVStorage) Get(key string) (string, bool, error) {
	keyPtr, keyLen := abi.StringToPtr(key)
	defer abi.Free(keyPtr, keyLen)

	resLen := abi.StorageKVGet(keyPtr, keyLen)
	if resLen == 0 {
		return "", false, nil
	}

	destBuf := make([]byte, resLen)
	destPtr, _ := abi.BytesToPtr(destBuf)
	defer abi.Free(destPtr, resLen)

	abi.SysReadResponse(destPtr, resLen)
	return abi.PtrToString(destPtr, resLen), true, nil
}

func (s *defaultKVStorage) Set(key string, value string) error {
	keyPtr, keyLen := abi.StringToPtr(key)
	defer abi.Free(keyPtr, keyLen)

	valPtr, valLen := abi.StringToPtr(value)
	defer abi.Free(valPtr, valLen)

	code := abi.StorageKVSet(keyPtr, keyLen, valPtr, valLen)
	if code != 0 {
		return fmt.Errorf("storage kv_set failed with code: %d", code)
	}
	return nil
}

func (s *defaultKVStorage) Delete(key string) error {
	keyPtr, keyLen := abi.StringToPtr(key)
	defer abi.Free(keyPtr, keyLen)

	code := abi.StorageKVDelete(keyPtr, keyLen)
	if code != 0 {
		return fmt.Errorf("storage kv_delete failed with code: %d", code)
	}
	return nil
}

func (s *defaultKVStorage) GetJSON(key string, target any) (bool, error) {
	raw, ok, err := s.Get(key)
	if err != nil || !ok {
		return ok, err
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return true, fmt.Errorf("deserializing kv json for key %s: %w", key, err)
	}
	return true, nil
}

func (s *defaultKVStorage) SetJSON(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("serializing kv json for key %s: %w", key, err)
	}
	return s.Set(key, string(b))
}

// --- EventBus Implementation ---

type defaultEventBus struct{}

func (b *defaultEventBus) Emit(topic string, payload any) error {
	topicPtr, topicLen := abi.StringToPtr(topic)
	defer abi.Free(topicPtr, topicLen)

	var payloadBytes []byte
	switch p := payload.(type) {
	case string:
		payloadBytes = []byte(p)
	case []byte:
		payloadBytes = p
	default:
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshaling event payload: %w", err)
		}
	}

	payloadPtr, payloadLen := abi.BytesToPtr(payloadBytes)
	defer abi.Free(payloadPtr, payloadLen)

	code := abi.BusEmitEvent(topicPtr, topicLen, payloadPtr, payloadLen)
	if code != 0 {
		return fmt.Errorf("bus emit_event failed with code: %d", code)
	}
	return nil
}

// --- Logger Implementation ---

type defaultLogger struct{}

func (l *defaultLogger) logMsg(level LogLevel, msg string, args ...any) {
	formatted := msg
	if len(args) > 0 {
		var parts []string
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				parts = append(parts, fmt.Sprintf("%v=%v", args[i], args[i+1]))
			} else {
				parts = append(parts, fmt.Sprintf("%v", args[i]))
			}
		}
		formatted = fmt.Sprintf("%s [%s]", msg, strings.Join(parts, ", "))
	}

	ptr, length := abi.StringToPtr(formatted)
	defer abi.Free(ptr, length)
	abi.SysLog(int32(level), ptr, length)
}

func (l *defaultLogger) Debug(msg string, args ...any) { l.logMsg(LogLevelDebug, msg, args...) }
func (l *defaultLogger) Info(msg string, args ...any)  { l.logMsg(LogLevelInfo, msg, args...) }
func (l *defaultLogger) Warn(msg string, args ...any)  { l.logMsg(LogLevelWarn, msg, args...) }
func (l *defaultLogger) Error(msg string, args ...any) { l.logMsg(LogLevelError, msg, args...) }
