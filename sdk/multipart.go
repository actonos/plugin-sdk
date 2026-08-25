package sdk

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
)

// EncodeMultipart builds a multipart/form-data body the host HTTP proxy can POST.
func EncodeMultipart(fields map[string]string, fileField, fileName string, data []byte) (contentType, body string, err error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", "", fmt.Errorf("writing multipart field %s: %w", key, err)
		}
	}
	if fileField != "" {
		part, err := writer.CreateFormFile(fileField, fileName)
		if err != nil {
			return "", "", fmt.Errorf("creating multipart file field: %w", err)
		}
		if _, err := part.Write(data); err != nil {
			return "", "", fmt.Errorf("writing multipart file: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("closing multipart body: %w", err)
	}
	return writer.FormDataContentType(), buf.String(), nil
}

// FileKind classifies a host-forwarded file for platform send APIs.
func FileKind(name, mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case strings.HasPrefix(mimeType, "image/") && ext != ".svg" && ext != ".svgz":
		return "photo"
	case mimeType == "image/svg+xml":
		return "document"
	case strings.HasPrefix(mimeType, "audio/") || ext == ".ogg" || ext == ".mp3" || ext == ".wav" || ext == ".m4a":
		return "voice"
	case strings.HasPrefix(mimeType, "video/") || ext == ".mp4" || ext == ".mov" || ext == ".webm":
		return "video"
	case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp":
		return "photo"
	default:
		return "document"
	}
}
