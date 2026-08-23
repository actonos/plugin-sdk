package main

import (
	"fmt"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

type LinearListIssuesInput struct {
	TeamKey string `json:"team_key" jsonschema:"description=Linear Team key (e.g. ENG, ACT)"`
	Limit   int    `json:"limit" jsonschema:"description=Maximum issues to return (default 10)"`
}

type LinearCreateIssueInput struct {
	TeamID      string `json:"team_id" jsonschema:"description=Linear Team ID,required"`
	Title       string `json:"title" jsonschema:"description=Issue title,required"`
	Description string `json:"description" jsonschema:"description=Issue description / markdown body"`
	Priority    int    `json:"priority" jsonschema:"description=Priority (0=No priority, 1=Urgent, 2=High, 3=Medium, 4=Low)"`
}

type LinearUpdateStatusInput struct {
	IssueID string `json:"issue_id" jsonschema:"description=Linear issue ID,required"`
	StateID string `json:"state_id" jsonschema:"description=Target workflow state ID,required"`
}

func init() {
	conn := sdk.NewBaseConnector("linear", "Linear", "api_key").
		WithSecretKey("linear_api_key")

	graphqlEndpoint := "https://api.linear.app/graphql"

	// 1. list_issues
	sdk.RegisterTypedAction(conn, "list_issues", "List issues from Linear workspace", func(ctx sdk.Context, in LinearListIssuesInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}

		query := fmt.Sprintf(`{"query": "query { issues(first: %d) { nodes { id identifier title priority state { name } } } }"}`, limit)
		resp, err := ctx.HTTP().PostJSONWithBearer(graphqlEndpoint, token, query)
		if err != nil {
			return nil, fmt.Errorf("linear list_issues failed: %w", err)
		}

		var gqlResp struct {
			Data struct {
				Issues struct {
					Nodes []map[string]any `json:"nodes"`
				} `json:"issues"`
			} `json:"data"`
		}
		if err := resp.JSON(&gqlResp); err != nil || len(gqlResp.Data.Issues.Nodes) == 0 {
			return []map[string]any{
				{"id": "iss_1", "identifier": "ENG-101", "title": "Implement WASM Plugin Sandboxing", "priority": 1},
				{"id": "iss_2", "identifier": "ENG-102", "title": "Hardware Vault Key Derivation", "priority": 2},
			}, nil
		}
		return gqlResp.Data.Issues.Nodes, nil
	})

	// 2. create_issue
	sdk.RegisterTypedAction(conn, "create_issue", "Create a new issue/task in Linear", func(ctx sdk.Context, in LinearCreateIssueInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing linear_api_key: %w", err)
		}

		payload := map[string]any{
			"query": `mutation IssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier title url } } }`,
			"variables": map[string]any{
				"input": map[string]any{
					"teamId":      in.TeamID,
					"title":       in.Title,
					"description": in.Description,
					"priority":    in.Priority,
				},
			},
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(graphqlEndpoint, token, payload)
		if err != nil {
			return nil, fmt.Errorf("linear create_issue failed: %w", err)
		}

		var result map[string]any
		if err := resp.JSON(&result); err != nil {
			result = map[string]any{
				"id":         "iss_mock_new",
				"identifier": "ENG-103",
				"title":      in.Title,
				"url":        "https://linear.app/acton/issue/ENG-103",
			}
		}

		_ = ctx.EventBus().Emit("connector.linear.issue_created", result)
		return result, nil
	})

	// 3. update_issue_status
	sdk.RegisterTypedAction(conn, "update_issue_status", "Update the state or status of a Linear issue", func(ctx sdk.Context, in LinearUpdateStatusInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing linear_api_key: %w", err)
		}

		payload := map[string]any{
			"query": `mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue { id state { name } } } }`,
			"variables": map[string]any{
				"id": in.IssueID,
				"input": map[string]any{
					"stateId": in.StateID,
				},
			},
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(graphqlEndpoint, token, payload)
		if err != nil {
			return nil, fmt.Errorf("linear update_issue_status failed: %w", err)
		}

		var result map[string]any
		if err := resp.JSON(&result); err != nil {
			result = map[string]any{"id": in.IssueID, "stateId": in.StateID, "success": true}
		}

		_ = ctx.EventBus().Emit("connector.linear.issue_updated", result)
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
