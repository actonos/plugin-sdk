# Jira SaaS Connector & Tools Plugin

ActonOS SaaS Connector and Agent Tools integration for Atlassian Jira Cloud.

## Actions / Agent Tools
- `search_issues_jql`: Query tickets using JQL expressions
- `create_issue`: Create new issues (Bugs, Tasks, Stories)
- `transition_issue`: Progress tickets through workflow statuses

## Permissions
- `net_outbound`: `["api.atlassian.com"]`
- `secrets`: `["jira_api_token", "jira_cloud_id"]`
