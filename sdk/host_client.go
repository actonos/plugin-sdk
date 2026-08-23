package sdk

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/actonos/plugin-sdk/sdk/abi"
)

type defaultContext struct {
	http     HTTPClient
	ws       WebSocketClient
	config   ConfigStore
	vault    VaultClient
	storage  KVStorage
	eventBus EventBus
	logger   Logger
}

// NewContext creates a default Context connected to Host ABI syscalls.
func NewContext() Context {
	storage := &defaultKVStorage{}
	return &defaultContext{
		http:     &defaultHTTPClient{},
		ws:       &defaultWebSocketClient{},
		config:   &defaultConfigStore{storage: storage},
		vault:    &defaultVaultClient{},
		storage:  storage,
		eventBus: &defaultEventBus{},
		logger:   &defaultLogger{},
	}
}

func (c *defaultContext) HTTP() HTTPClient     { return c.http }
func (c *defaultContext) WS() WebSocketClient  { return c.ws }
func (c *defaultContext) Config() ConfigStore   { return c.config }
func (c *defaultContext) Vault() VaultClient   { return c.vault }
func (c *defaultContext) Storage() KVStorage   { return c.storage }
func (c *defaultContext) EventBus() EventBus   { return c.eventBus }
func (c *defaultContext) Log() Logger          { return c.logger }

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

// --- WebSocket Client Implementation ---

type defaultWebSocketClient struct{}

func (w *defaultWebSocketClient) Dial(url string, headers map[string]string) (WebSocketConn, error) {
	if url == "" {
		return nil, fmt.Errorf("websocket url cannot be empty")
	}

	urlPtr, urlLen := abi.StringToPtr(url)
	defer abi.Free(urlPtr, urlLen)

	var hPtr, hLen uint32
	if len(headers) > 0 {
		hBytes, err := json.Marshal(headers)
		if err == nil && len(hBytes) > 0 {
			hPtr, hLen = abi.BytesToPtr(hBytes)
			defer abi.Free(hPtr, hLen)
		}
	}

	handleID := abi.WSConnect(urlPtr, urlLen, hPtr, hLen)
	if handleID < 0 {
		return nil, fmt.Errorf("websocket connect failed (handle=%d)", handleID)
	}

	return &defaultWebSocketConn{handleID: handleID}, nil
}

type defaultWebSocketConn struct {
	handleID int32
}

func (c *defaultWebSocketConn) HandleID() int32 {
	return c.handleID
}

func (c *defaultWebSocketConn) SendText(message string) error {
	msgPtr, msgLen := abi.StringToPtr(message)
	defer abi.Free(msgPtr, msgLen)

	code := abi.WSSend(c.handleID, 1, msgPtr, msgLen)
	if code != 0 {
		return fmt.Errorf("websocket send_text failed with code: %d", code)
	}
	return nil
}

func (c *defaultWebSocketConn) SendBinary(data []byte) error {
	dataPtr, dataLen := abi.BytesToPtr(data)
	defer abi.Free(dataPtr, dataLen)

	code := abi.WSSend(c.handleID, 2, dataPtr, dataLen)
	if code != 0 {
		return fmt.Errorf("websocket send_binary failed with code: %d", code)
	}
	return nil
}

func (c *defaultWebSocketConn) SendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("serializing websocket json payload: %w", err)
	}
	return c.SendText(string(b))
}

func (c *defaultWebSocketConn) Poll() ([]byte, bool, error) {
	resLen := abi.WSPoll(c.handleID)
	if resLen < 0 {
		return nil, false, fmt.Errorf("websocket connection closed or invalid handle")
	}
	if resLen == 0 {
		return nil, false, nil
	}

	destBuf := make([]byte, resLen)
	destPtr, _ := abi.BytesToPtr(destBuf)
	defer abi.Free(destPtr, uint32(resLen))

	abi.SysReadResponse(destPtr, uint32(resLen))
	return abi.PtrToBytes(destPtr, uint32(resLen)), true, nil
}

func (c *defaultWebSocketConn) PollJSON(target any) (bool, error) {
	data, ok, err := c.Poll()
	if err != nil || !ok {
		return ok, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return true, fmt.Errorf("deserializing websocket message json: %w", err)
	}
	return true, nil
}

func (c *defaultWebSocketConn) Close() error {
	code := abi.WSClose(c.handleID)
	if code != 0 {
		return fmt.Errorf("websocket close failed with code: %d", code)
	}
	return nil
}

// --- Config Store Implementation ---

type defaultConfigStore struct {
	storage KVStorage
}

func (c *defaultConfigStore) Get(key string) (string, bool) {
	var rawMap map[string]any
	if ok, _ := c.storage.GetJSON("__config", &rawMap); ok && rawMap != nil {
		if val, exists := rawMap[key]; exists {
			return fmt.Sprintf("%v", val), true
		}
	}
	val, ok, _ := c.storage.Get(key)
	return val, ok
}

func (c *defaultConfigStore) GetString(key string, defaultVal string) string {
	if val, ok := c.Get(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func (c *defaultConfigStore) GetInt(key string, defaultVal int) int {
	if val, ok := c.Get(key); ok {
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
			return n
		}
	}
	return defaultVal
}

func (c *defaultConfigStore) GetBool(key string, defaultVal bool) bool {
	if val, ok := c.Get(key); ok {
		lower := strings.ToLower(strings.TrimSpace(val))
		if lower == "true" || lower == "1" || lower == "yes" {
			return true
		}
		if lower == "false" || lower == "0" || lower == "no" {
			return false
		}
	}
	return defaultVal
}

func (c *defaultConfigStore) GetJSON(key string, target any) (bool, error) {
	var rawMap map[string]json.RawMessage
	if ok, err := c.storage.GetJSON("__config", &rawMap); ok && rawMap != nil {
		if rawVal, exists := rawMap[key]; exists {
			if err := json.Unmarshal(rawVal, target); err != nil {
				return true, err
			}
			return true, nil
		}
	} else if err != nil {
		return false, err
	}
	return c.storage.GetJSON(key, target)
}

func (c *defaultConfigStore) Bind(target any) error {
	raw, ok, err := c.storage.Get("__config")
	if err != nil {
		return fmt.Errorf("reading plugin config: %w", err)
	}
	if !ok || raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
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
