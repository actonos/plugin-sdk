package sdk

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestAttachedFileAndEncodeMultipart(t *testing.T) {
	msg := OutboundMessage{
		FileName: "report.pdf",
		MIMEType: "application/pdf",
		FileData: []byte("%PDF-1.7"),
	}
	if !msg.HasMedia() {
		t.Fatal("expected HasMedia")
	}
	name, mime, data, ok := msg.AttachedFile()
	if !ok || name != "report.pdf" || mime != "application/pdf" || string(data) != "%PDF-1.7" {
		t.Fatalf("attached=%q %q %q ok=%v", name, mime, data, ok)
	}
	if FileKind(name, mime) != "document" {
		t.Fatalf("kind=%s", FileKind(name, mime))
	}
	pdf := []byte{0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x37, 0x00, 0xff, 0x80}
	ct, body, err := EncodeMultipart(map[string]string{"chat_id": "888"}, "document", name, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "multipart/form-data") || !bytes.Contains(body, []byte("report.pdf")) || !bytes.Contains(body, []byte("888")) {
		t.Fatalf("multipart ct=%q body=%q", ct, body)
	}
	if !bytes.Contains(body, pdf) {
		t.Fatal("multipart body lost original PDF bytes")
	}

	req := HTTPRequest{Method: "POST", URL: "https://api.telegram.org/botx/sendDocument", Body: string(body)}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, pdf) {
		t.Fatal("JSON body string would be used; binary must not be embedded as UTF-8")
	}
	// Callers must send body_base64 for binary; Do() also rewrites invalid UTF-8.
	safe := HTTPRequest{Method: "POST", URL: "https://example.com", BodyBase64: base64.StdEncoding.EncodeToString(body)}
	safeJSON, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip HTTPRequest
	if err := json.Unmarshal(safeJSON, &roundtrip); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(roundtrip.BodyBase64)
	if err != nil || !bytes.Equal(decoded, body) {
		t.Fatalf("body_base64 roundtrip failed: %v", err)
	}
}
