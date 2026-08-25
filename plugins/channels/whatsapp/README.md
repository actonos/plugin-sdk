# WhatsApp Cloud API Channel Plugin

ActonOS Chat Channel integration for WhatsApp Business Cloud API.

## Features
- Canonical `accounts[]` schema (typing, ack reaction, quote reply)
- Individual & Business text messaging
- Typing indicator + inbound acknowledgement reactions
- Quoted replies via `context.message_id`
- Webhook queue buffering and `@agent` command routing
- Device Pairing security

## Permissions
- `net_outbound`: `["graph.facebook.com"]`
- `secrets`: `["whatsapp_access_token", "whatsapp_phone_number_id"]`
- `storage`: `true`
