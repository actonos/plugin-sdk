package host

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tetratelabs/wazero/api"
)

// Return codes of acton_oauth.get_token, identical to the ActonOS core. A positive value is the
// length of the JSON answer, which the guest then reads with acton_sys.read_response.
const (
	OAuthErrDenied      int32 = -2 // the account is not this plugin's
	OAuthErrAuthExpired int32 = -3 // the user must sign in again
	OAuthErrUnavailable int32 = -4 // no token available (unknown account)
)

// SetOAuthToken makes acton_oauth.get_token return token for account.
func (h *MockHost) SetOAuthToken(account, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthTokens[account] = token
}

// ExpireOAuthAccount makes get_token answer "sign in again" for account.
func (h *MockHost) ExpireOAuthAccount(account string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthExpired[account] = true
}

// DenyOAuthAccount makes get_token refuse account, as the core does for another plugin's account.
func (h *MockHost) DenyOAuthAccount(account string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oauthDenied[account] = true
}

func (h *MockHost) registerOAuthModule(ctx context.Context) error {
	b := h.runtime.NewHostModuleBuilder("acton_oauth")
	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, ptr, length uint32) int32 {
			raw, ok := mod.Memory().Read(ptr, length)
			if !ok || len(raw) == 0 || len(raw) > 256 {
				return OAuthErrDenied
			}
			account := string(raw)
			h.mu.RLock()
			denied, expired := h.oauthDenied[account], h.oauthExpired[account]
			token, known := h.oauthTokens[account]
			h.mu.RUnlock()
			switch {
			case denied:
				return OAuthErrDenied
			case expired:
				return OAuthErrAuthExpired
			case !known:
				return OAuthErrUnavailable
			}
			body, _ := json.Marshal(map[string]any{
				"access_token": token,
				"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return int32(h.setPendingResponse(mod.Name(), body))
		}).
		Export("get_token")
	_, err := b.Instantiate(ctx)
	return err
}
