package mail

import (
	"encoding/base64"
	"net/mail"
	"strings"
	"testing"

	"github.com/emersion/go-imap"
)

func TestDecodeTextBodyEncodingsAndCharset(t *testing.T) {
	t.Run("base64 utf8", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("你好，邮件"))
		got, err := decodeTextBody(strings.NewReader(encoded), "text/plain", "base64", "utf-8")
		if err != nil || got != "你好，邮件" {
			t.Fatalf("got %q, err=%v", got, err)
		}
	})

	t.Run("quoted printable", func(t *testing.T) {
		got, err := decodeTextBody(strings.NewReader("hello=20world=21"), "text/plain", "quoted-printable", "utf-8")
		if err != nil || got != "hello world!" {
			t.Fatalf("got %q, err=%v", got, err)
		}
	})

	t.Run("iso 8859 1", func(t *testing.T) {
		got, err := decodeTextBody(strings.NewReader("caf\xe9"), "text/plain", "", "iso-8859-1")
		if err != nil || got != "café" {
			t.Fatalf("got %q, err=%v", got, err)
		}
	})
}

func TestReadBodyPrefersPlainTextAcrossAlternative(t *testing.T) {
	raw := "Content-Type: multipart/alternative; boundary=choice\r\n\r\n" +
		"--choice\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>HTML version</p>\r\n" +
		"--choice\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nPlain version\r\n" +
		"--choice--\r\n"
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := readBody(message)
	if err != nil || strings.TrimSpace(got) != "Plain version" {
		t.Fatalf("got %q, err=%v", got, err)
	}
}

func TestStripHTMLKeepsReadableText(t *testing.T) {
	got := stripHTML(`<html><head><title>ignore</title></head><body><h1>Hello &amp; hi</h1><script>bad()</script><style>.bad{}</style><p>Line<br>two</p></body></html>`)
	if strings.Contains(got, "ignore") || strings.Contains(got, "bad") {
		t.Fatalf("hidden content leaked: %q", got)
	}
	for _, want := range []string{"Hello & hi", "Line", "two"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestDecodePreviewBodyFromSelectedHTMLPart(t *testing.T) {
	source := `<html><body><p>Your account application status</p><script>ignore()</script></body></html>`
	encoded := base64.StdEncoding.EncodeToString([]byte(source))
	got, err := decodePreviewBody(strings.NewReader(encoded), "text/html", "base64", "utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Your account application status" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestSelectBodyPartPrefersPlainAndSkipsAttachment(t *testing.T) {
	structure := &imap.BodyStructure{
		MIMEType: "multipart",
		Parts: []*imap.BodyStructure{
			{MIMEType: "text", MIMESubType: "html", Params: map[string]string{"charset": "utf-8"}},
			{MIMEType: "text", MIMESubType: "plain", Disposition: "attachment", Params: map[string]string{"charset": "utf-8"}},
			{MIMEType: "text", MIMESubType: "plain", Encoding: "base64", Params: map[string]string{"charset": "gb18030"}},
		},
	}
	part, ok := selectBodyPart(structure)
	if !ok || len(part.path) != 1 || part.path[0] != 3 || part.contentType != "text/plain" || part.encoding != "base64" || part.charset != "gb18030" {
		t.Fatalf("unexpected selection: %#v, ok=%v", part, ok)
	}
}
