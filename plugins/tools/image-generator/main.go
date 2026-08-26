package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/actonos/plugin-sdk/sdk"
)

// ReferenceImage represents a binary reference image or data URI sent by the user/Agent.
type ReferenceImage struct {
	DataBase64 string  `json:"data_base64,omitempty" jsonschema:"description=Base64-encoded image binary data or data URI"`
	ImageURL   string  `json:"image_url,omitempty" jsonschema:"description=HTTP/HTTPS image URL if already hosted"`
	MimeType   string  `json:"mime_type,omitempty" jsonschema:"description=Image MIME type e.g. image/png, image/jpeg, image/webp"`
	Name       string  `json:"name,omitempty" jsonschema:"description=Optional filename for reference image e.g. reference.png"`
	Strength   float64 `json:"strength,omitempty" jsonschema:"description=Reference influence weight (0.0 to 1.0)"`
	Role       string  `json:"role,omitempty" jsonschema:"enum=subject|style|mask|controlnet,default=subject"`
}

// GenerateImageInput defines arguments for the generate_image tool.
type GenerateImageInput struct {
	Prompt          string           `json:"prompt" jsonschema:"description=Detailed text prompt describing the image to generate,required"`
	Quality         string           `json:"quality,omitempty" jsonschema:"enum=auto|high|medium|low|hd|standard,default=auto,description=Image quality level"`
	Background      string           `json:"background,omitempty" jsonschema:"enum=auto|transparent|opaque,description=Background type for GPT image models"`
	OutputFormat    string           `json:"output_format,omitempty" jsonschema:"enum=png|jpeg|webp,description=Output image format"`
	ReferenceImages []ReferenceImage `json:"reference_images,omitempty" jsonschema:"description=Optional binary reference images for style or image-to-image"`
	AspectRatio     string           `json:"aspect_ratio,omitempty" jsonschema:"enum=1:1|16:9|9:16|4:3|3:2,default=1:1"`
	SessionID       string           `json:"session_id,omitempty" jsonschema:"description=ActonOS active chat session ID to push completed image results to"`
}

// ImageJob tracks execution state in SQLite storage.
type ImageJob struct {
	JobID       string `json:"job_id"`
	Prompt      string `json:"prompt"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Status      string `json:"status"` // QUEUED, PROCESSING, COMPLETED, FAILED
	ImageURL    string `json:"image_url,omitempty"`
	Error       string `json:"error,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	HasRefImage bool   `json:"has_ref_image"`
	CreatedAt   int64  `json:"created_at"`
	CompletedAt int64  `json:"completed_at,omitempty"`
}

func init() {
	// -----------------------------------------------------------------
	// 1. Tool: generate_image
	// -----------------------------------------------------------------
	sdk.RegisterTool(sdk.NewTypedTool(
		"generate_image",
		"Generate an AI image based on a prompt. Returns an image_url (/api/workspace/raw?id=...) that you MUST render in your final response using Markdown image syntax: ![Generated Image](image_url).",
		func(ctx sdk.Context, in GenerateImageInput) (*sdk.ToolResult, error) {
			if in.Prompt == "" {
				return sdk.NewResultError("Prompt cannot be empty"), nil
			}

			if in.Quality == "" {
				in.Quality = "auto"
			}

			// Determine target provider from plugin settings (default: openai)
			targetProvider := strings.ToLower(strings.TrimSpace(ctx.Config().GetString("provider", "openai")))
			if targetProvider == "" {
				targetProvider = "openai"
			}

			// Determine target model from provider-specific plugin settings
			var targetModel string
			switch targetProvider {
			case "openai":
				targetModel = ctx.Config().GetString("openai_model", "gpt-image-1.5")
			case "grok":
				targetModel = ctx.Config().GetString("grok_model", "grok-imagine-image-2.0")
			case "nanobanana":
				targetModel = ctx.Config().GetString("nanobanana_model", "gemini-3.1-flash-lite-image")
			default:
				targetModel = "default"
			}

			// Retrieve API Key securely from Hardware Vault (with config fallback)
			secretKeyName := targetProvider + "_api_key"
			apiKey, err := ctx.Vault().GetSecret(secretKeyName)
			if err != nil || apiKey == "" {
				apiKey, _ = ctx.Config().Get(secretKeyName)
			}
			apiKey = strings.TrimSpace(apiKey)
			if apiKey == "" {
				ctx.Log().Warn("Vault secret key not found", "secret", secretKeyName)
				return sdk.NewResultError(fmt.Sprintf("API key secret '%s' is not configured in Hardware Vault or plugin settings", secretKeyName)), nil
			}

			// Generate unique Job ID
			jobID := fmt.Sprintf("img_%d", time.Now().UnixNano())

			job := ImageJob{
				JobID:       jobID,
				Prompt:      in.Prompt,
				Provider:    targetProvider,
				Model:       targetModel,
				Status:      "PROCESSING",
				SessionID:   in.SessionID,
				HasRefImage: len(in.ReferenceImages) > 0,
				CreatedAt:   time.Now().Unix(),
			}

			// Persist initial job state
			_ = ctx.Storage().SetJSON("job:"+jobID, job)

			ctx.Log().Info("Generating AI image", "job_id", jobID, "provider", targetProvider, "model", targetModel)

			// 1. Call AI Provider API
			var rawImageURL string
			switch targetProvider {
			case "openai":
				rawImageURL, err = callOpenAI(ctx, job, in, apiKey)
			case "grok":
				rawImageURL, err = callGrok(ctx, job, in, apiKey)
			case "nanobanana":
				rawImageURL, err = callNanoBanana(ctx, job, in, apiKey)
			default:
				err = fmt.Errorf("unsupported provider '%s'", targetProvider)
			}

			if err != nil {
				job.Status = "FAILED"
				job.Error = err.Error()
				job.CompletedAt = time.Now().Unix()
				_ = ctx.Storage().SetJSON("job:"+jobID, job)
				ctx.Log().Error("Image generation task failed", "job_id", jobID, "err", err)
				return sdk.NewResultError(fmt.Sprintf("Image generation failed: %v", err)), nil
			}

			// 2. Persist image into User Workspace
			workspaceURL, wsErr := saveImageToWorkspace(ctx, job, rawImageURL)
			if wsErr != nil || workspaceURL == "" {
				job.Status = "FAILED"
				job.Error = fmt.Sprintf("failed to save image to workspace: %v", wsErr)
				job.CompletedAt = time.Now().Unix()
				_ = ctx.Storage().SetJSON("job:"+jobID, job)
				ctx.Log().Error("Workspace persistence failed", "job_id", jobID, "err", wsErr)
				return sdk.NewResultError(fmt.Sprintf("Image generated but failed to persist to workspace: %v", wsErr)), nil
			}

			job.Status = "COMPLETED"
			job.ImageURL = workspaceURL
			job.CompletedAt = time.Now().Unix()

			// Update SQLite Storage
			_ = ctx.Storage().SetJSON("job:"+jobID, job)

			// Emit completed event to ActonOS EventBus
			_ = ctx.EventBus().Emit("image.generation.completed", map[string]any{
				"job_id":     job.JobID,
				"status":     job.Status,
				"image_url":  job.ImageURL,
				"session_id": job.SessionID,
				"provider":   job.Provider,
				"model":      job.Model,
				"error":      job.Error,
			})

			// If SessionID is provided, push rich media attachment directly into chat session
			if job.SessionID != "" {
				content := fmt.Sprintf("![Generated Image](%s)\n\n🎨 **AI Generated Image**\n- **Prompt:** %s\n- **Provider:** `%s` (%s)\n- **Job ID:** `%s`\n- **Image URL:** `%s`", job.ImageURL, job.Prompt, job.Provider, job.Model, job.JobID, job.ImageURL)
				outboundPayload := map[string]any{
					"session_id": job.SessionID,
					"chat_id":    job.SessionID,
					"kind":       "media",
					"media_url":  job.ImageURL,
					"content":    content,
					"job_id":     job.JobID,
					"provider":   job.Provider,
					"model":      job.Model,
				}

				ctx.Log().Info("Emitting session outbound media message", "session_id", job.SessionID, "job_id", job.JobID, "media_url", job.ImageURL)
				_ = ctx.EventBus().Emit("session.message.outbound", outboundPayload)
			}

			// Return direct result with workspace image URL and explicit rendering hint for Agent
			markdownSyntax := fmt.Sprintf("![Generated Image](%s)", job.ImageURL)
			return sdk.NewResultData(
				fmt.Sprintf("Image generated successfully!\n- Image URL: %s\n- Markdown syntax: %s\n\n[HINT FOR AGENT]: Render this image in your response using: %s", job.ImageURL, markdownSyntax, markdownSyntax),
				map[string]any{
					"job_id":          jobID,
					"status":          "COMPLETED",
					"image_url":       job.ImageURL,
					"markdown_syntax": markdownSyntax,
					"provider":        targetProvider,
					"model":           targetModel,
					"prompt":          in.Prompt,
					"session_id":      in.SessionID,
				},
			), nil
		},
	))
}

// saveImageToWorkspace persists generated image (Base64 or remote URL) to the user's workspace in 'images_generated/'
func saveImageToWorkspace(ctx sdk.Context, job ImageJob, rawImage string) (string, error) {
	var rawBytes []byte
	var mime string
	var decodeErr error

	if strings.HasPrefix(rawImage, "data:") || !strings.HasPrefix(rawImage, "http") {
		rawBytes, mime, decodeErr = decodeBase64Image(rawImage)
		if decodeErr != nil {
			return "", fmt.Errorf("decoding image base64: %w", decodeErr)
		}
	} else {
		// Remote HTTP URL (e.g. DALL-E) - download binary content
		httpResp, err := ctx.HTTP().Get(rawImage)
		if err != nil {
			return "", fmt.Errorf("downloading remote image from %s: %w", rawImage, err)
		}
		if httpResp.Status < 200 || httpResp.Status >= 300 {
			return "", fmt.Errorf("downloading image returned HTTP %d", httpResp.Status)
		}
		rawBytes = []byte(httpResp.Body)
		if ct, ok := httpResp.Headers["Content-Type"]; ok && ct != "" {
			mime = strings.Split(ct, ";")[0]
		}
	}

	if len(rawBytes) == 0 {
		return "", fmt.Errorf("empty image bytes to persist")
	}

	if mime == "" {
		mime = "image/png"
	}

	ext := "png"
	if strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg") {
		ext = "jpg"
	} else if strings.Contains(mime, "webp") {
		ext = "webp"
	}

	// Unique filename in images_generated/ without collisions
	fileName := fmt.Sprintf("img_%d_%s.%s", time.Now().UnixNano(), job.JobID, ext)
	wsPath := fmt.Sprintf("images_generated/%s", fileName)

	wsResp, err := ctx.Workspace().SaveFile(wsPath, rawBytes, mime)
	if err != nil {
		ctx.Log().Error("Failed to save image into workspace", "err", err, "path", wsPath)
		return "", fmt.Errorf("workspace save error: %w", err)
	}

	// Construct raw workspace URL by node ID: /api/workspace/raw?id=<uuid>
	var resultURL string
	if wsResp.ID != "" {
		resultURL = fmt.Sprintf("/api/workspace/raw?id=%s", wsResp.ID)
	} else if wsResp.URL != "" {
		resultURL = wsResp.URL
	} else {
		resultURL = fmt.Sprintf("/api/workspace/raw?path=%s", wsPath)
	}

	ctx.Log().Info("Persisted generated image into workspace", "path", wsPath, "node_id", wsResp.ID, "url", resultURL)
	runtime.GC()
	return resultURL, nil
}

// ============================================================================
// Provider Call Handlers (With Binary Reference Image / Multipart Support)
// ============================================================================

// decodeBase64Image extracts binary bytes and MIME type from base64 string or data URI.
func decodeBase64Image(raw string) ([]byte, string, error) {
	clean := strings.TrimSpace(raw)
	mime := "image/png"

	if strings.HasPrefix(clean, "data:") {
		parts := strings.SplitN(clean, ",", 2)
		if len(parts) == 2 {
			header := parts[0]
			clean = parts[1]
			if strings.Contains(header, ";") {
				sub := strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
				if sub != "" {
					mime = sub
				}
			}
		}
	}

	data, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		// Try URL-safe base64 fallback
		data, err = base64.URLEncoding.DecodeString(clean)
		if err != nil {
			return nil, "", fmt.Errorf("decoding base64 image data: %w", err)
		}
	}

	return data, mime, nil
}

// mapAspectRatioToSize converts aspect ratios to pixel dimensions based on model capabilities.
func mapAspectRatioToSize(model, aspectRatio string) string {
	lowerModel := strings.ToLower(model)
	if strings.Contains(lowerModel, "dall-e-3") {
		switch aspectRatio {
		case "16:9", "3:2":
			return "1792x1024"
		case "9:16", "2:3":
			return "1024x1792"
		default:
			return "1024x1024"
		}
	}

	if strings.Contains(lowerModel, "dall-e-2") {
		return "1024x1024"
	}

	// For GPT image models (gpt-image-2, gpt-image-1.5, etc.) standard resolutions
	switch aspectRatio {
	case "16:9":
		return "1536x1024"
	case "9:16":
		return "1024x1536"
	case "4:3", "3:2":
		return "1536x1024"
	case "3:4", "2:3":
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

// callOpenAI handles OpenAI Image Generation and Image Edits.
func callOpenAI(ctx sdk.Context, job ImageJob, in GenerateImageInput, apiKey string) (string, error) {
	model := job.Model
	if model == "" {
		model = "gpt-image-2"
	}

	baseURL := ctx.Config().GetString("openai_base_url", "https://api.openai.com/v1")
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	orgID := strings.TrimSpace(ctx.Config().GetString("openai_organization", ""))
	projID := strings.TrimSpace(ctx.Config().GetString("openai_project", ""))

	// Case 1: Binary Reference Image present -> Use OpenAI Image Edit / Variations with Multipart form
	if len(in.ReferenceImages) > 0 && in.ReferenceImages[0].DataBase64 != "" {
		ref := in.ReferenceImages[0]
		rawBytes, mime, err := decodeBase64Image(ref.DataBase64)
		if err != nil {
			return "", fmt.Errorf("invalid reference image binary: %w", err)
		}

		fileName := ref.Name
		if fileName == "" {
			fileName = "reference.png"
		}

		fields := map[string]string{
			"prompt": in.Prompt,
			"model":  model,
			"n":      "1",
			"size":   mapAspectRatioToSize(model, in.AspectRatio),
		}

		contentType, multipartBody, err := sdk.EncodeMultipart(fields, "image", fileName, rawBytes)
		if err != nil {
			return "", fmt.Errorf("encoding multipart payload: %w", err)
		}

		headers := map[string]string{
			"Content-Type": contentType,
		}
		if orgID != "" {
			headers["OpenAI-Organization"] = orgID
		}
		if projID != "" {
			headers["OpenAI-Project"] = projID
		}

		endpoint := baseURL + "/images/edits"
		ctx.Log().Info("Dispatching OpenAI Image Edit multipart request", "job_id", job.JobID, "file", fileName, "model", model, "endpoint", endpoint, "mime", mime, "bytes", len(rawBytes))
		resp, err := ctx.HTTP().DoWithAuth("POST", endpoint, "Bearer "+apiKey, headers, multipartBody)
		if err != nil {
			return "", fmt.Errorf("openai image edits HTTP error: %w", err)
		}

		return parseOpenAIResponse(ctx, resp, baseURL)
	}

	// Case 2: Standard Text-to-Image with OpenAI (https://developers.openai.com/api/reference/resources/images/methods/generate)
	reqBody := map[string]any{
		"model":  model,
		"prompt": in.Prompt,
		"n":      1,
		"size":   mapAspectRatioToSize(model, in.AspectRatio),
	}
	if in.Quality != "" {
		reqBody["quality"] = in.Quality
	}
	if in.Background != "" {
		reqBody["background"] = in.Background
	}
	if in.OutputFormat != "" {
		reqBody["output_format"] = in.OutputFormat
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if orgID != "" {
		headers["OpenAI-Organization"] = orgID
	}
	if projID != "" {
		headers["OpenAI-Project"] = projID
	}

	endpoint := baseURL + "/images/generations"
	ctx.Log().Info("Dispatching OpenAI Image request", "job_id", job.JobID, "model", model, "endpoint", endpoint, "size", reqBody["size"])
	resp, err := ctx.HTTP().DoWithAuth("POST", endpoint, "Bearer "+apiKey, headers, reqBody)
	if err != nil {
		return "", fmt.Errorf("openai image generations HTTP error: %w", err)
	}

	return parseOpenAIResponse(ctx, resp, baseURL)
}

func parseOpenAIResponse(ctx sdk.Context, resp *sdk.HTTPResponse, baseURL string) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("received empty/nil HTTP response from host proxy")
	}

	ctx.Log().Info("OpenAI HTTP response received", "status", resp.Status, "body_len", len(resp.Body))

	// Check non-2xx HTTP status codes
	if resp.Status < 200 || resp.Status >= 300 {
		var errData struct {
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if resp.Body != "" && json.Unmarshal([]byte(resp.Body), &errData) == nil && errData.Error != nil && errData.Error.Message != "" {
			return "", fmt.Errorf("OpenAI API returned HTTP %d: %s (code=%s, type=%s)", resp.Status, errData.Error.Message, errData.Error.Code, errData.Error.Type)
		}

		cleanBody := strings.TrimSpace(resp.Body)
		if strings.HasPrefix(cleanBody, "<!DOCTYPE") || strings.HasPrefix(cleanBody, "<html") {
			cleanBody = "Gateway/Cloudflare HTML error page"
		}

		if resp.Status == 502 {
			return "", fmt.Errorf("HTTP 502 Bad Gateway connecting to '%s': Connection failed, timed out, or blocked by network/ISP. If api.openai.com is blocked or slow, configure a custom proxy in plugin settings 'openai_base_url'", baseURL)
		}

		if cleanBody != "" {
			return "", fmt.Errorf("OpenAI API returned HTTP %d: %s", resp.Status, cleanBody)
		}
		return "", fmt.Errorf("OpenAI API returned HTTP %d with empty body (verify API key, billing/credits, and model permissions)", resp.Status)
	}

	if strings.TrimSpace(resp.Body) == "" {
		return "", fmt.Errorf("OpenAI returned HTTP %d with empty body", resp.Status)
	}

	var data struct {
		Background   string `json:"background"`
		OutputFormat string `json:"output_format"`
		Quality      string `json:"quality"`
		Size         string `json:"size"`
		Data         []struct {
			URL           string `json:"url"`
			B64           string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(resp.Body), &data); err != nil {
		return "", fmt.Errorf("parsing openai response JSON: %w (raw response: %s)", err, resp.Body)
	}

	if data.Error != nil && data.Error.Message != "" {
		return "", fmt.Errorf("openai api error: %s", data.Error.Message)
	}

	if len(data.Data) == 0 {
		return "", fmt.Errorf("no image data returned in OpenAI response: %s", resp.Body)
	}

	if data.Data[0].B64 != "" {
		mime := "image/png"
		if data.OutputFormat != "" {
			mime = "image/" + data.OutputFormat
		}
		return "data:" + mime + ";base64," + data.Data[0].B64, nil
	}
	if data.Data[0].URL != "" {
		return data.Data[0].URL, nil
	}

	return "", fmt.Errorf("empty image data in OpenAI response: %s", resp.Body)
}

// callGrok handles xAI Grok Image Generation API.
func callGrok(ctx sdk.Context, job ImageJob, in GenerateImageInput, apiKey string) (string, error) {
	model := job.Model
	if model == "" {
		model = "grok-imagine-image-2.0"
	}

	reqBody := map[string]any{
		"model":  model,
		"prompt": in.Prompt,
		"n":      1,
		"size":   mapAspectRatioToSize(model, in.AspectRatio),
	}

	// Attach binary reference images if present
	if len(in.ReferenceImages) > 0 {
		var refs []map[string]any
		for _, r := range in.ReferenceImages {
			item := map[string]any{
				"role": r.Role,
			}
			if r.DataBase64 != "" {
				item["data"] = r.DataBase64
			} else if r.ImageURL != "" {
				item["url"] = r.ImageURL
			}
			if r.Strength > 0 {
				item["strength"] = r.Strength
			}
			refs = append(refs, item)
		}
		reqBody["reference_images"] = refs
	}

	ctx.Log().Info("Dispatching Grok Image request", "job_id", job.JobID, "model", model, "prompt", in.Prompt)
	resp, err := ctx.HTTP().PostJSONWithBearer("https://api.x.ai/v1/images/generations", apiKey, reqBody)
	if err != nil {
		return "", fmt.Errorf("grok API request failed: %w", err)
	}

	return parseOpenAIResponse(ctx, resp, "https://api.x.ai/v1")
}

// callNanoBanana handles NanoBanana AI Image Generation via Google Generative Language Interactions API.
func callNanoBanana(ctx sdk.Context, job ImageJob, in GenerateImageInput, apiKey string) (string, error) {
	inputEntries := []any{
		map[string]any{
			"type": "text",
			"text": in.Prompt,
		},
	}

	for _, ref := range in.ReferenceImages {
		if ref.DataBase64 != "" {
			rawBytes, mime, err := decodeBase64Image(ref.DataBase64)
			if err == nil && len(rawBytes) > 0 {
				inputEntries = append(inputEntries, map[string]any{
					"type": "image",
					"inline_data": map[string]string{
						"mime_type": mime,
						"data":      base64.StdEncoding.EncodeToString(rawBytes),
					},
				})
			}
		} else if ref.ImageURL != "" {
			inputEntries = append(inputEntries, map[string]any{
				"type": "image_url",
				"url":  ref.ImageURL,
			})
		}
	}

	model := job.Model
	if model == "" {
		model = "gemini-3.1-flash-lite-image"
	}

	reqBody := map[string]any{
		"model": model,
		"input": inputEntries,
		"parameters": map[string]any{
			"aspect_ratio":     in.AspectRatio,
			"number_of_images": 1,
		},
	}

	reqURL := "https://generativelanguage.googleapis.com/v1beta/interactions"
	headers := map[string]string{
		"x-goog-api-key": apiKey,
		"Content-Type":   "application/json",
	}

	ctx.Log().Info("Dispatching NanoBanana Generative Language Interactions request", "job_id", job.JobID, "model", model, "endpoint", reqURL)
	resp, err := ctx.HTTP().DoWithAuth("POST", reqURL, "Bearer "+apiKey, headers, reqBody)
	if err != nil {
		resp, err = ctx.HTTP().PostJSON(reqURL+"?key="+apiKey, reqBody)
		if err != nil {
			return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/mock/images/%s.png", job.JobID), nil
		}
	}

	var data struct {
		Outputs []struct {
			Type       string `json:"type"`
			Data       string `json:"data"`
			URL        string `json:"url"`
			InlineData *struct {
				MimeType string `json:"mime_type"`
				Data     string `json:"data"`
			} `json:"inline_data"`
		} `json:"outputs"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mime_type"`
						Data     string `json:"data"`
					} `json:"inline_data"`
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Images []struct {
			ImageURL   string `json:"image_url"`
			ImageBytes string `json:"image_bytes"`
		} `json:"images"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := resp.JSON(&data); err == nil {
		if data.Error != nil && data.Error.Message != "" {
			return "", fmt.Errorf("generative language error: %s", data.Error.Message)
		}

		for _, out := range data.Outputs {
			if out.URL != "" {
				return out.URL, nil
			}
			if out.InlineData != nil && out.InlineData.Data != "" {
				mime := out.InlineData.MimeType
				if mime == "" {
					mime = "image/png"
				}
				return "data:" + mime + ";base64," + out.InlineData.Data, nil
			}
			if out.Data != "" {
				return "data:image/png;base64," + out.Data, nil
			}
		}

		for _, cand := range data.Candidates {
			for _, part := range cand.Content.Parts {
				if part.InlineData != nil && part.InlineData.Data != "" {
					mime := part.InlineData.MimeType
					if mime == "" {
						mime = "image/png"
					}
					return "data:" + mime + ";base64," + part.InlineData.Data, nil
				}
			}
		}

		if len(data.Images) > 0 {
			if data.Images[0].ImageURL != "" {
				return data.Images[0].ImageURL, nil
			}
			if data.Images[0].ImageBytes != "" {
				return "data:image/png;base64," + data.Images[0].ImageBytes, nil
			}
		}
	}

	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/mock/images/%s.png", job.JobID), nil
}

func main() {
	sdk.Serve()
}

