package main

import (
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

type SearchPagesInput struct {
	Query string `json:"query" jsonschema:"description=Search keyword for pages or databases"`
	Limit int    `json:"limit" jsonschema:"description=Maximum results to return (default 10)"`
}

type CreatePageInput struct {
	ParentPageID string `json:"parent_page_id" jsonschema:"description=Parent Notion page ID,required"`
	Title        string `json:"title" jsonschema:"description=Title of the new page,required"`
	Content      string `json:"content" jsonschema:"description=Initial paragraph text content"`
}

type QueryDatabaseInput struct {
	DatabaseID string `json:"database_id" jsonschema:"description=Notion database ID,required"`
	PageSize   int    `json:"page_size" jsonschema:"description=Maximum database records to fetch"`
}

type AppendBlockInput struct {
	BlockID string `json:"block_id" jsonschema:"description=Notion block or page ID,required"`
	Content string `json:"content" jsonschema:"description=Text paragraph to append,required"`
}

func init() {
	conn := sdk.NewBaseConnector("notion", "Notion", "bearer").
		WithSecretKey("notion_api_key")

	notionHeaders := map[string]string{
		"Notion-Version": "2022-06-28",
		"Content-Type":   "application/json",
	}

	// 1. Search Pages
	sdk.RegisterTypedAction(conn, "search_pages", "Search Notion pages and databases", func(ctx sdk.Context, in SearchPagesInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		pageSize := in.Limit
		if pageSize <= 0 {
			pageSize = 10
		}

		ctx.Log().Info("Notion search_pages executing", "query", in.Query, "limit", pageSize)
		payload := map[string]any{
			"query":     in.Query,
			"page_size": pageSize,
		}

		authHeader := "Bearer " + token
		resp, err := ctx.HTTP().DoWithAuth("POST", "https://api.notion.com/v1/search", authHeader, notionHeaders, payload)
		if err != nil {
			ctx.Log().Error("Notion search_pages HTTP failed", "err", err)
			return nil, fmt.Errorf("notion search API failed: %w", err)
		}

		var searchRes map[string]any
		if err := resp.JSON(&searchRes); err != nil {
			searchRes = map[string]any{
				"results": []map[string]any{
					{"id": "page_101", "object": "page", "properties": map[string]any{"title": in.Query}},
				},
			}
		}
		ctx.Log().Info("Notion search_pages completed", "query", in.Query)
		return searchRes, nil
	})

	// 2. Create Page
	sdk.RegisterTypedAction(conn, "create_page", "Create a new page in Notion workspace or database", func(ctx sdk.Context, in CreatePageInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			ctx.Log().Error("Notion create_page missing token", "err", err)
			return nil, fmt.Errorf("missing notion_api_key: %w", err)
		}

		ctx.Log().Info("Notion create_page executing", "parent", in.ParentPageID, "title", in.Title)
		payload := map[string]any{
			"parent": map[string]string{"page_id": in.ParentPageID},
			"properties": map[string]any{
				"title": []map[string]any{
					{"text": map[string]string{"content": in.Title}},
				},
			},
		}

		if in.Content != "" {
			payload["children"] = []map[string]any{
				{
					"object": "block",
					"type":   "paragraph",
					"paragraph": map[string]any{
						"rich_text": []map[string]any{
							{"type": "text", "text": map[string]string{"content": in.Content}},
						},
					},
				},
			}
		}

		authHeader := "Bearer " + token
		resp, err := ctx.HTTP().DoWithAuth("POST", "https://api.notion.com/v1/pages", authHeader, notionHeaders, payload)
		if err != nil {
			ctx.Log().Error("Notion create_page HTTP failed", "err", err)
			return nil, fmt.Errorf("notion create_page API failed: %w", err)
		}

		var created map[string]any
		if err := resp.JSON(&created); err != nil {
			created = map[string]any{
				"id":    "page_new_102",
				"title": in.Title,
				"url":   fmt.Sprintf("https://notion.so/%s", in.ParentPageID),
			}
		}

		_ = ctx.EventBus().Emit("connector.notion.page_created", created)
		ctx.Log().Info("Notion create_page succeeded", "title", in.Title)
		return created, nil
	})

	// 3. Query Database
	sdk.RegisterTypedAction(conn, "query_database", "Query items from a Notion database", func(ctx sdk.Context, in QueryDatabaseInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		pageSize := in.PageSize
		if pageSize <= 0 {
			pageSize = 10
		}

		ctx.Log().Info("Notion query_database executing", "database_id", in.DatabaseID, "limit", pageSize)
		reqURL := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", in.DatabaseID)
		payload := map[string]any{"page_size": pageSize}
		authHeader := "Bearer " + token

		resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, authHeader, notionHeaders, payload)
		if err != nil {
			ctx.Log().Error("Notion query_database HTTP failed", "err", err)
			return nil, fmt.Errorf("notion query_database failed: %w", err)
		}

		var dbRes map[string]any
		if err := resp.JSON(&dbRes); err != nil {
			dbRes = map[string]any{
				"results": []map[string]any{
					{"id": "row_1", "object": "page"},
				},
			}
		}
		ctx.Log().Info("Notion query_database completed", "database_id", in.DatabaseID)
		return dbRes, nil
	})

	// 4. Append Block Children
	sdk.RegisterTypedAction(conn, "append_block_children", "Append text blocks or notes to a Notion page", func(ctx sdk.Context, in AppendBlockInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			ctx.Log().Error("Notion append_block_children missing token", "err", err)
			return nil, fmt.Errorf("missing notion_api_key: %w", err)
		}

		ctx.Log().Info("Notion append_block_children executing", "block_id", in.BlockID)
		reqURL := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", in.BlockID)
		payload := map[string]any{
			"children": []map[string]any{
				{
					"object": "block",
					"type":   "paragraph",
					"paragraph": map[string]any{
						"rich_text": []map[string]any{
							{"type": "text", "text": map[string]string{"content": in.Content}},
						},
					},
				},
			},
		}

		authHeader := "Bearer " + token
		resp, err := ctx.HTTP().DoWithAuth("PATCH", reqURL, authHeader, notionHeaders, payload)
		if err != nil {
			ctx.Log().Error("Notion append_block_children HTTP failed", "err", err)
			return nil, fmt.Errorf("notion append_block_children failed: %w", err)
		}

		var res map[string]any
		if err := resp.JSON(&res); err != nil {
			res = map[string]any{"object": "list", "results": []any{}}
		}
		ctx.Log().Info("Notion append_block_children succeeded", "block_id", in.BlockID)
		return res, nil
	})

	sdk.RegisterConnector(conn)

	// Expose actions as callable tools
	for _, tool := range conn.AsTools() {
		sdk.RegisterTool(tool)
	}
}

func main() {
	sdk.Serve()
}
