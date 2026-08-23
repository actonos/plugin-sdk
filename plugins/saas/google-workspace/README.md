# Google Workspace Connector & Tools Plugin

ActonOS SaaS Connector and Agent Tools integration for Google Workspace (Gmail, Google Calendar, Google Drive, Google Docs).

## Actions / Agent Tools
- `send_email`: Send email via Gmail API (`users.messages.send`)
- `create_calendar_event`: Schedule meetings via Google Calendar API
- `list_drive_files`: Search documents on Google Drive
- `get_doc_content`: Retrieve text from Google Docs

## Permissions
- `net_outbound`: `["gmail.googleapis.com", "www.googleapis.com"]`
- `secrets`: `["google_workspace_access_token"]`
