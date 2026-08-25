# ActonOS Built-in Plugins Suite

This document lists all official, ready-to-deploy built-in plugins provided in the ActonOS ecosystem. All plugins are compiled to WebAssembly (`wasip1`/`wasm32-wasi`) and run inside the Wazero JIT sandbox.

---

## 💬 1. Chat Channels (`plugins/channels/`)

| Plugin Name | ID | Capabilities | Required Vault Secrets | Egress Whitelist | Features |
|:---|:---|:---|:---|:---|:---|
| **Telegram** | `channel-telegram` | `channel` | `telegram_bot_token` | `api.telegram.org` | Canonical `accounts[]`, Markdown, quote replies, `sendChatAction` typing, `setMessageReaction` ack, `@agent` routing |
| **Discord** | `channel-discord` | `channel` | `discord_bot_tokens.*` | `discord.com`, `gateway.discord.gg` | Canonical `accounts[]`, Gateway + REST, rich embeds, typing, reactions, `message_reference` quotes |
| **WhatsApp** | `channel-whatsapp` | `channel` | `whatsapp_access_token`, `whatsapp_phone_number_id` | `graph.facebook.com` | Canonical `accounts[]`, Cloud API text, typing indicator, message reactions, quoted replies, webhook buffering |
| **Slack** | `channel-slack` | `channel` | `slack_bot_token` | `slack.com` | Canonical `accounts[]`, `chat.postMessage`, thread replies (`thread_ts`), `reactions.add` ack |
| **Zalo** | `channel-zalo` | `channel` | `zalo_bot_token` | `bot-api.zaloplatforms.com` | Canonical `accounts[]`, Markdown, `sendChatAction` typing, reactions, quote replies, media |

---

## 🛠️ 2. SaaS Connectors & Agent Tools (`plugins/saas/`)

| Plugin Name | ID | Auth Type | Required Vault Secrets | Egress Whitelist | Callable Actions / Agent Tools (`conn.AsTools()`) |
|:---|:---|:---|:---|:---|:---|
| **Google Workspace** | `connector-google-workspace` | `oauth2` | `google_workspace_access_token` | `gmail.googleapis.com`, `www.googleapis.com` | • `connector_google-workspace_send_email`<br>• `connector_google-workspace_create_calendar_event`<br>• `connector_google-workspace_list_drive_files`<br>• `connector_google-workspace_get_doc_content` |
| **GitHub** | `connector-github` | `oauth2` | `github_access_token` | `api.github.com` | • `connector_github_list_repos`<br>• `connector_github_get_issue`<br>• `connector_github_create_issue`<br>• `connector_github_create_pull_request`<br>• `connector_github_search_code` |
| **Notion** | `connector-notion` | `bearer` | `notion_api_key` | `api.notion.com` | • `connector_notion_search_pages`<br>• `connector_notion_create_page`<br>• `connector_notion_query_database`<br>• `connector_notion_append_block_children` |
| **Slack SaaS** | `connector-slack` | `oauth2` | `slack_bot_token` | `slack.com` | • `connector_slack_post_message`<br>• `connector_slack_list_channels`<br>• `connector_slack_get_history` |
| **Linear** | `connector-linear` | `api_key` | `linear_api_key` | `api.linear.app` | • `connector_linear_list_issues`<br>• `connector_linear_create_issue`<br>• `connector_linear_update_issue_status` |
| **Jira Cloud** | `connector-jira` | `oauth2` | `jira_api_token`, `jira_cloud_id` | `api.atlassian.com` | • `connector_jira_search_issues_jql`<br>• `connector_jira_create_issue`<br>• `connector_jira_transition_issue` |
| **Figma** | `connector-figma` | `oauth2` | `figma_access_token` | `api.figma.com` | • `connector_figma_get_file`<br>• `connector_figma_get_comments`<br>• `connector_figma_post_comment` |

---

## 📦 How to Build and Package All Plugins

Build and package any plugin into a distributable `.actonpkg` bundle using the CLI toolchain:

```bash
# 1. Build WASM binary
./build/acton-plugin.exe build -src plugins/channels/telegram -out dist/channel-telegram.wasm

# 2. Validate manifest and permissions
./build/acton-plugin.exe validate -manifest plugins/channels/telegram/manifest.json

# 3. Package into .actonpkg bundle
./build/acton-plugin.exe pack -manifest plugins/channels/telegram/manifest.json -wasm dist/channel-telegram.wasm -out dist/channel-telegram.actonpkg
```
