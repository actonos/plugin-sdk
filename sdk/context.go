package sdk

// Context provides access to all sandboxed Host capabilities.
type Context interface {
	// HTTP returns the sandboxed outbound HTTP client.
	HTTP() HTTPClient

	// Vault provides access to authorized Hardware Vault secrets.
	Vault() VaultClient

	// Storage provides isolated SQLite key-value persistence.
	Storage() KVStorage

	// EventBus provides access to the internal message broker.
	EventBus() EventBus

	// Log provides structured logging to ActonOS daemon.
	Log() Logger
}

// HTTPClient allows performing outbound HTTP calls subject to egress firewall.
type HTTPClient interface {
	Get(url string) (*HTTPResponse, error)
	GetWithBearer(url string, token string) (*HTTPResponse, error)
	Post(url string, contentType string, body string) (*HTTPResponse, error)
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
