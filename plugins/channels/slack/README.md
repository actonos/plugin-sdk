# Slack Bot Channel Plugin

ActonOS Chat Channel integration for Slack workspaces.

## Features
- Channels & Direct Messages outbound posting (`chat.postMessage`)
- Thread replies via `thread_ts` metadata
- Conversations history polling and `@agent` mention routing
- Multi-channel pairing PIN security

## Permissions
- `net_outbound`: `["slack.com"]`
- `secrets`: `["slack_bot_token"]`
- `storage`: `true`
