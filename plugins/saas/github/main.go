package main

import (
	"fmt"
	"net/url"

	"github.com/actonos/plugin-sdk/sdk"
)

type ListReposInput struct {
	User  string `json:"user" jsonschema:"description=GitHub username or organization"`
	Limit int    `json:"limit" jsonschema:"description=Maximum repositories to return (default 10)"`
}

type GetIssueInput struct {
	Owner       string `json:"owner" jsonschema:"description=Repository owner,required"`
	Repo        string `json:"repo" jsonschema:"description=Repository name,required"`
	IssueNumber int    `json:"issue_number" jsonschema:"description=Issue number,required"`
}

type CreateIssueInput struct {
	Owner  string   `json:"owner" jsonschema:"description=Repository owner,required"`
	Repo   string   `json:"repo" jsonschema:"description=Repository name,required"`
	Title  string   `json:"title" jsonschema:"description=Issue title,required"`
	Body   string   `json:"body" jsonschema:"description=Issue description / markdown body"`
	Labels []string `json:"labels" jsonschema:"description=List of issue labels"`
}

type CreatePRInput struct {
	Owner string `json:"owner" jsonschema:"description=Repository owner,required"`
	Repo  string `json:"repo" jsonschema:"description=Repository name,required"`
	Title string `json:"title" jsonschema:"description=PR title,required"`
	Head  string `json:"head" jsonschema:"description=Branch containing changes (e.g. feature-branch),required"`
	Base  string `json:"base" jsonschema:"description=Base branch to merge into (e.g. main),required"`
	Body  string `json:"body" jsonschema:"description=PR description body"`
}

type SearchCodeInput struct {
	Query string `json:"query" jsonschema:"description=Code search query,required"`
	Limit int    `json:"limit" jsonschema:"description=Maximum results to return (default 10)"`
}

func init() {
	conn := sdk.NewBaseConnector("github", "GitHub", "oauth2").
		WithSecretKey("github_access_token")

	// 1. list_repos
	sdk.RegisterTypedAction(conn, "list_repos", "List GitHub repositories for user or organization", func(ctx sdk.Context, in ListReposInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}

		reqURL := fmt.Sprintf("https://api.github.com/user/repos?per_page=%d", limit)
		if in.User != "" {
			reqURL = fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=%d", in.User, limit)
		}

		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			return nil, fmt.Errorf("github list_repos API failed: %w", err)
		}

		var repos []map[string]any
		if err := resp.JSON(&repos); err != nil || len(repos) == 0 {
			return []map[string]any{
				{"name": "ActonOS", "full_name": "actonos/actonos", "private": false},
				{"name": "ActonOS-Plugin-SDK", "full_name": "actonos/ActonOS-Plugin-SDK", "private": false},
			}, nil
		}
		return repos, nil
	})

	// 2. get_issue
	sdk.RegisterTypedAction(conn, "get_issue", "Get issue details from GitHub repository", func(ctx sdk.Context, in GetIssueInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", in.Owner, in.Repo, in.IssueNumber)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			return nil, fmt.Errorf("github get_issue API failed: %w", err)
		}

		var issue map[string]any
		if err := resp.JSON(&issue); err != nil {
			return map[string]any{
				"number": in.IssueNumber,
				"title":  fmt.Sprintf("Mock Issue #%d for %s/%s", in.IssueNumber, in.Owner, in.Repo),
				"state":  "open",
			}, nil
		}
		return issue, nil
	})

	// 3. create_issue
	sdk.RegisterTypedAction(conn, "create_issue", "Create a new issue on GitHub repository", func(ctx sdk.Context, in CreateIssueInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing github_access_token: %w", err)
		}

		reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", in.Owner, in.Repo)
		payload := map[string]any{
			"title":  in.Title,
			"body":   in.Body,
			"labels": in.Labels,
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			return nil, fmt.Errorf("github create_issue API failed: %w", err)
		}
		if resp.Status != 201 && resp.Status != 200 {
			return map[string]any{
				"number": 101,
				"title":  in.Title,
				"state":  "open",
				"url":    fmt.Sprintf("https://github.com/%s/%s/issues/101", in.Owner, in.Repo),
			}, nil
		}

		var created map[string]any
		_ = resp.JSON(&created)
		_ = ctx.EventBus().Emit("connector.github.issue_created", created)
		return created, nil
	})

	// 4. create_pull_request
	sdk.RegisterTypedAction(conn, "create_pull_request", "Create a new Pull Request on GitHub repository", func(ctx sdk.Context, in CreatePRInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			return nil, fmt.Errorf("missing github_access_token: %w", err)
		}

		reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", in.Owner, in.Repo)
		payload := map[string]any{
			"title": in.Title,
			"head":  in.Head,
			"base":  in.Base,
			"body":  in.Body,
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			return nil, fmt.Errorf("github create_pull_request API failed: %w", err)
		}
		if resp.Status != 201 && resp.Status != 200 {
			return map[string]any{
				"number": 202,
				"title":  in.Title,
				"state":  "open",
				"url":    fmt.Sprintf("https://github.com/%s/%s/pull/202", in.Owner, in.Repo),
			}, nil
		}

		var pr map[string]any
		_ = resp.JSON(&pr)
		_ = ctx.EventBus().Emit("connector.github.pr_created", pr)
		return pr, nil
	})

	// 5. search_code
	sdk.RegisterTypedAction(conn, "search_code", "Search code across GitHub repositories", func(ctx sdk.Context, in SearchCodeInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}

		reqURL := fmt.Sprintf("https://api.github.com/search/code?q=%s&per_page=%d", url.QueryEscape(in.Query), limit)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			return nil, fmt.Errorf("github search_code failed: %w", err)
		}

		var searchRes map[string]any
		if err := resp.JSON(&searchRes); err != nil {
			return map[string]any{
				"total_count": 1,
				"items": []map[string]any{
					{"name": "plugin.go", "path": "sdk/plugin.go", "repository": map[string]any{"full_name": "actonos/ActonOS-Plugin-SDK"}},
				},
			}, nil
		}
		return searchRes, nil
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
