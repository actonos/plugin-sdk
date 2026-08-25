package sdk

import (
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
	ct, body, err := EncodeMultipart(map[string]string{"chat_id": "888"}, "document", name, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "multipart/form-data") || !strings.Contains(body, "report.pdf") || !strings.Contains(body, "888") {
		t.Fatalf("multipart ct=%q body=%q", ct, body)
	}
}
