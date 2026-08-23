# Discord Bot Channel Plugin

ActonOS Chat Channel integration for Discord servers and Direct Messages.

## Features
- Channels & DMs outbound messaging (`/channels/{id}/messages`)
- Rich Embeds support (`embed_title` metadata)
- Bot mention stripping (`<@!id>`) and automatic `@agent` / `/agent` command routing
- Device Pairing PIN security

## Permissions
- `net_outbound`: `["discord.com"]`
- `secrets`: `["discord_bot_token"]`
- `storage`: `true`
