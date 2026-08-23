# Zalo Official Account (OA) Channel Plugin

ActonOS Chat Channel integration for Zalo OA OpenAPI v3.0 serving Vietnamese users.

## Features
- Customer support messaging via `/v3.0/oa/message/cs`
- Webhook queue parsing (`user_send_text`)
- Multi-Agent routing with Vietnamese `@agent` syntax support
- Device Pairing security

## Permissions
- `net_outbound`: `["openapi.zalo.me"]`
- `secrets`: `["zalo_oa_access_token"]`
- `storage`: `true`
