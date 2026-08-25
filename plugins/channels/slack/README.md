# Slack Bot Channel Plugin

ActonOS Chat Channel integration for Slack workspaces.

## Features
- Canonical `accounts[]` schema (ack reaction, quote/thread reply)
- Channels & Direct Messages outbound posting (`chat.postMessage`)
- Thread replies via canonical `reply_to_id` / `thread_id` (`thread_ts`)
- Inbound acknowledgement reactions (`reactions.add`)
- Conversations history polling and `@agent` mention routing
- Multi-channel pairing PIN security

## Permissions
- `net_outbound`: `["slack.com"]`
- `secrets`: `["slack_bot_token"]`
- `storage`: `true`
