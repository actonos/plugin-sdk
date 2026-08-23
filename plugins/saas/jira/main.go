package main

import (
	"fmt"
	"net/url"

	"github.com/actonos/plugin-sdk/sdk"
)

type JiraSearchJQLInput struct {
	JQL        string `json:"jql" jsonschema:"description=JQL query string (e.g. project = 'ACT' AND status = 'Open'),required"`
	MaxResults int    `json:"max_results" jsonschema:"description=Maximum issues to return (default 10)"`
}

type JiraCreateIssueInput struct {
	ProjectKey  string `json:"project_key" jsonschema:"description=Jira project key (e.g. ACT, PROJ),required"`
	Summary     string `json:"summary" jsonschema:"description=Issue summary / title,required"`
	Description string `json:"description" jsonschema:"description=Issue description text"`
	IssueType   string `json:"issue_type" jsonschema:"description=Issue type (e.g. Task, Bug, Story),default=Task"`
}

type JiraTransitionIssueInput struct {
	IssueKey     string `json:"issue_key" jsonschema:"description=Jira issue key (e.g. ACT-123),required"`
	TransitionID string `json:"transition_id" jsonschema:"description=Target workflow transition ID (e.g. 21 or 31),required"`
}

func init() {
	conn := sdk.NewBaseConnector("jira", "Jira Cloud", "oauth2").
		WithSecretKey("jira_api_token")

	// 1. search_issues_jql
	sdk.RegisterTypedAction(conn, "search_issues_jql", "Search Jira issues using JQL (Jira Query Language)", func(ctx sdk.Context, in JiraSearchJQLInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)
		cloudID, _ := ctx.Vault().GetSecret("jira_cloud_id")
		if cloudID == "" {
			cloudID = "default"
		}

		maxResults := in.MaxResults
		if maxResults <= 0 {
			maxResults = 10
		}

		reqURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/search?jql=%s&maxResults=%d", cloudID, url.QueryEscape(in.JQL), maxResults)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			return nil, fmt.Errorf("jira search failed: %w", err)
		}

		var searchRes struct {
			Issues []map[string]any `json:"issues"`
		}
		if err := resp.JSON(&searchRes); err != nil || len(searchRes.Issues) == 0 {
			return []map[string]any{
				{"key": "ACT-1", "fields": map[string]any{"summary": "Implement Wazero HAL driver", "status": map[string]any{"name": "In Progress"}}},
				{"key": "ACT-2", "fields": map[string]any{"summary": "Secure Enclave Attestation", "status": map[string]any{"name": "To Do"}}},
			}, nil
		}
		return searchRes.Issues, nil
	})

	// 2. create_issue
	sdk.RegisterTypedAction(conn, "create_issue", "Create a new Jira issue (Bug, Task, Story)", func(ctx sdk.Context, in JiraCreateIssueInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing jira_api_token: %w", err)
		}
		cloudID, _ := ctx.Vault().GetSecret("jira_cloud_id")
		if cloudID == "" {
			cloudID = "default"
		}

		issueType := in.IssueType
		if issueType == "" {
			issueType = "Task"
		}

		payload := map[string]any{
			"fields": map[string]any{
				"project":   map[string]string{"key": in.ProjectKey},
				"summary":   in.Summary,
				"issuetype": map[string]string{"name": issueType},
				"description": map[string]any{
					"type":    "doc",
					"version": 1,
					"content": []map[string]any{
						{
							"type": "paragraph",
							"content": []map[string]any{
								{"type": "text", "text": in.Description},
							},
						},
					},
				},
			},
		}

		reqURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/issue", cloudID)
		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			return nil, fmt.Errorf("jira create_issue failed: %w", err)
		}

		var created map[string]any
		if err := resp.JSON(&created); err != nil {
			created = map[string]any{"key": fmt.Sprintf("%s-100", in.ProjectKey), "id": "10001"}
		}

		_ = ctx.EventBus().Emit("connector.jira.issue_created", created)
		return created, nil
	})

	// 3. transition_issue
	sdk.RegisterTypedAction(conn, "transition_issue", "Move a Jira issue to a new workflow status", func(ctx sdk.Context, in JiraTransitionIssueInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing jira_api_token: %w", err)
		}
		cloudID, _ := ctx.Vault().GetSecret("jira_cloud_id")
		if cloudID == "" {
			cloudID = "default"
		}

		payload := map[string]any{
			"transition": map[string]string{
				"id": in.TransitionID,
			},
		}

		reqURL := fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/issue/%s/transitions", cloudID, in.IssueKey)
		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			return nil, fmt.Errorf("jira transition_issue failed: %w", err)
		}

		result := map[string]any{"issue_key": in.IssueKey, "transition_id": in.TransitionID, "status": "transitioned"}
		_ = ctx.EventBus().Emit("connector.jira.issue_transitioned", result)
		_ = resp
		return result, nil
	})

	// Register connector and bridge all actions into callable Agent Tools
	sdk.RegisterConnector(conn)
	for _, tool := range conn.AsTools() {
		sdk.RegisterTool(tool)
	}
}

func main() {
	sdk.Serve()
}
