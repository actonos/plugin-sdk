package sdk

// Context provides access to all sandboxed Host capabilities.
type Context interface {
	// HTTP returns the sandboxed outbound HTTP client.
	HTTP() HTTPClient

	// WS returns the sandboxed WebSocket client.
	WS() WebSocketClient

	// Config provides access to structured user-configured settings.
	Config() ConfigStore

	// Vault provides access to authorized Hardware Vault secrets.
	Vault() VaultClient

	// Storage provides isolated SQLite key-value persistence.
	Storage() KVStorage

	// EventBus provides access to the internal message broker.
	EventBus() EventBus

	// Workspace provides sandboxed file operations in the ActonOS User Workspace.
	Workspace() WorkspaceClient

	// Log provides structured logging to ActonOS daemon.
	Log() Logger
}

// WebSocketClient establishes sandboxed WebSocket connections managed by Host daemon.
type WebSocketClient interface {
	// Dial connects to a WebSocket endpoint.
	Dial(url string, headers map[string]string) (WebSocketConn, error)
}

// WebSocketConn represents an active WebSocket connection.
type WebSocketConn interface {
	// SendText sends a text message.
	SendText(message string) error

	// SendBinary sends binary payload.
	SendBinary(data []byte) error

	// SendJSON serializes and sends a JSON payload.
	SendJSON(v any) error

	// Poll checks for incoming messages without blocking. Returns (payload, hasMessage, error).
	Poll() ([]byte, bool, error)

	// PollJSON attempts to deserialize the next pending message. Returns (hasMessage, error).
	PollJSON(target any) (bool, error)

	// Close terminates the WebSocket connection.
	Close() error

	// HandleID returns the raw connection handle ID assigned by Host.
	HandleID() int32
}

// ConfigStore provides typed access to user-configured plugin settings defined in manifest.
type ConfigStore interface {
	// Get retrieves a string config setting by key.
	Get(key string) (string, bool)

	// GetString retrieves a string config setting with a fallback default.
	GetString(key string, defaultVal string) string

	// GetInt retrieves an integer config setting with a fallback default.
	GetInt(key string, defaultVal int) int

	// GetBool retrieves a boolean config setting with a fallback default.
	GetBool(key string, defaultVal bool) bool

	// GetJSON deserializes a structured sub-key into target struct.
	GetJSON(key string, target any) (bool, error)

	// Bind unmarshals the entire plugin configuration root into a target struct.
	Bind(target any) error
}

// HTTPClient allows performing outbound HTTP calls subject to egress firewall.
type HTTPClient interface {
	Get(url string) (*HTTPResponse, error)
	GetWithBearer(url string, token string) (*HTTPResponse, error)
	Post(url string, contentType string, body string) (*HTTPResponse, error)
	PostBinary(url string, contentType string, body []byte) (*HTTPResponse, error)
	PostJSON(url string, body any) (*HTTPResponse, error)
	PostJSONWithBearer(url string, token string, body any) (*HTTPResponse, error)
	Do(req HTTPRequest) (*HTTPResponse, error)
	DoWithAuth(method, url, authHeader string, headers map[string]string, body any) (*HTTPResponse, error)
}

// VaultClient allows retrieving authorized secrets from the Hardware Vault.
type VaultClient interface {
	GetSecret(key string) (string, error)
}

// KVStorage provides key-value storage for the plugin.
type KVStorage interface {
	Get(key string) (string, bool, error)
	Set(key string, value string) error
	Delete(key string) error
	GetJSON(key string, target any) (bool, error)
	SetJSON(key string, value any) error
}

// EventBus allows emitting events onto the system bus.
type EventBus interface {
	Emit(topic string, payload any) error
}

// Logger allows logging messages with appropriate severity.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// WorkspaceClient provides sandboxed file operations in the ActonOS User Workspace.
type WorkspaceClient interface {
	// SaveFile stores binary data into the workspace at the target path.
	SaveFile(path string, data []byte, mimeType string) (*WorkspaceFileResponse, error)

	// SaveText stores text content into the workspace at the target path.
	SaveText(path string, text string) (*WorkspaceFileResponse, error)

	// ReadFile retrieves a workspace file node by path or ID.
	ReadFile(pathOrID string) (*WorkspaceFileResponse, error)

	// ReadBinary retrieves a workspace file and returns decoded binary bytes.
	ReadBinary(pathOrID string) ([]byte, *WorkspaceFileResponse, error)
}

