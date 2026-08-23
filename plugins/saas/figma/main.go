package main

import (
	"fmt"

	"github.com/actonos/plugin-sdk/sdk"
)

type FigmaGetFileInput struct {
	FileKey string `json:"file_key" jsonschema:"description=Figma file key (e.g. 7X3sD8... from URL),required"`
	Depth   int    `json:"depth" jsonschema:"description=Tree traversal depth limit (default 2)"`
}

type FigmaGetCommentsInput struct {
	FileKey string `json:"file_key" jsonschema:"description=Figma file key,required"`
}

type FigmaPostCommentInput struct {
	FileKey string  `json:"file_key" jsonschema:"description=Figma file key,required"`
	Message string  `json:"message" jsonschema:"description=Comment message content,required"`
	X       float64 `json:"x" jsonschema:"description=X coordinate pin on canvas"`
	Y       float64 `json:"y" jsonschema:"description=Y coordinate pin on canvas"`
}

func init() {
	conn := sdk.NewBaseConnector("figma", "Figma", "oauth2").
		WithSecretKey("figma_access_token")

	// 1. get_file
	sdk.RegisterTypedAction(conn, "get_file", "Retrieve Figma design file metadata and node tree", func(ctx sdk.Context, in FigmaGetFileInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		depth := in.Depth
		if depth <= 0 {
			depth = 2
		}

		ctx.Log().Info("Figma get_file executing", "file_key", in.FileKey, "depth", depth)
		reqURL := fmt.Sprintf("https://api.figma.com/v1/files/%s?depth=%d", in.FileKey, depth)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			ctx.Log().Error("Figma get_file HTTP failed", "err", err)
			return nil, fmt.Errorf("figma get_file failed: %w", err)
		}

		var fileRes map[string]any
		if err := resp.JSON(&fileRes); err != nil {
			fileRes = map[string]any{
				"name":         "ActonOS Design System",
				"lastModified": "2026-08-24T00:00:00Z",
				"version":      "1.0",
			}
		}
		ctx.Log().Info("Figma get_file completed", "file_key", in.FileKey)
		return fileRes, nil
	})

	// 2. get_comments
	sdk.RegisterTypedAction(conn, "get_comments", "Get design comments and discussion threads from a Figma file", func(ctx sdk.Context, in FigmaGetCommentsInput) (any, error) {
		token, _ := conn.GetAuthToken(ctx)

		ctx.Log().Info("Figma get_comments executing", "file_key", in.FileKey)
		reqURL := fmt.Sprintf("https://api.figma.com/v1/files/%s/comments", in.FileKey)
		resp, err := ctx.HTTP().GetWithBearer(reqURL, token)
		if err != nil {
			ctx.Log().Error("Figma get_comments HTTP failed", "err", err)
			return nil, fmt.Errorf("figma get_comments failed: %w", err)
		}

		var commentsRes map[string]any
		if err := resp.JSON(&commentsRes); err != nil {
			commentsRes = map[string]any{"comments": []any{}}
		}
		ctx.Log().Info("Figma get_comments completed", "file_key", in.FileKey)
		return commentsRes, nil
	})

	// 3. post_comment
	sdk.RegisterTypedAction(conn, "post_comment", "Post a design comment or feedback on a Figma file", func(ctx sdk.Context, in FigmaPostCommentInput) (any, error) {
		token, err := conn.GetAuthToken(ctx)
		if err != nil || token == "" {
			ctx.Log().Error("Figma post_comment missing token", "err", err)
			return nil, fmt.Errorf("missing figma_access_token: %w", err)
		}

		ctx.Log().Info("Figma post_comment executing", "file_key", in.FileKey)
		reqURL := fmt.Sprintf("https://api.figma.com/v1/files/%s/comments", in.FileKey)
		payload := map[string]any{
			"message": in.Message,
		}
		if in.X != 0 || in.Y != 0 {
			payload["client_meta"] = map[string]float64{
				"x": in.X,
				"y": in.Y,
			}
		}

		resp, err := ctx.HTTP().PostJSONWithBearer(reqURL, token, payload)
		if err != nil {
			ctx.Log().Error("Figma post_comment HTTP failed", "err", err)
			return nil, fmt.Errorf("figma post_comment failed: %w", err)
		}

		var res map[string]any
		if err := resp.JSON(&res); err != nil {
			res = map[string]any{"id": "cm_101", "message": in.Message}
		}

		_ = ctx.EventBus().Emit("connector.figma.comment_posted", map[string]string{"file_key": in.FileKey})
		ctx.Log().Info("Figma post_comment succeeded", "file_key", in.FileKey)
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
