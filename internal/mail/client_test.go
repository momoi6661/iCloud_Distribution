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

func TestPreviewFromRawRemovesMultipartHeadersAndBoundary(t *testing.T) {
	raw := "--_NmP-7192ffa6f6d1a458-Part_1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n\r\n" +
		"你的验证码是 7192。\r\n" +
		"--_NmP-7192ffa6f6d1a458-Part_1\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<p>你的验证码是 7192。</p>\r\n"
	got := previewFromRaw(strings.NewReader(raw))
	if got != "你的验证码是 7192。" {
		t.Fatalf("unexpected preview: %q", got)
	}
	for _, unwanted := range []string{"Part_1", "Content-Type", "Content-Transfer-Encoding"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("MIME artifact %q leaked into preview %q", unwanted, got)
		}
	}
}

func TestStripForwardedHeaderPreamble(t *testing.T) {
	got := stripForwardedHeaderPreamble("Return-path: sender@example.com\nOriginal-Recipient: target@icloud.com\n\n你的验证码是 482913")
	if got != "你的验证码是 482913" {
		t.Fatalf("unexpected cleaned preview: %q", got)
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

func TestParseForwardedMessageMatchesExactAliasAndBuildsPreview(t *testing.T) {
	raw := []byte("From: Sender <sender@example.com>\r\n" +
		"To: target.alias@icloud.com\r\n" +
		"Subject: Verification\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Your verification code is 482913.\r\n")
	full, matches, err := parseForwardedMessage(&imap.Message{Uid: 42}, raw, "INBOX", "target.alias@icloud.com")
	if err != nil || !matches {
		t.Fatalf("expected exact match, matches=%v err=%v", matches, err)
	}
	if full.ID != "42" || full.Folder != "INBOX" || full.Code != "482913" || !strings.Contains(full.Preview, "verification code") {
		t.Fatalf("unexpected parsed message: %+v", full)
	}
	if _, similar, err := parseForwardedMessage(&imap.Message{Uid: 42}, raw, "INBOX", "alias@icloud.com"); err != nil || similar {
		t.Fatalf("similar alias must not match, matches=%v err=%v", similar, err)
	}
}

func TestParseForwardedSummaryUsesHeadersWithoutReadingBody(t *testing.T) {
	raw := []byte("From: Sender <sender@example.com>\r\n" +
		"To: target.alias@icloud.com\r\n" +
		"Subject: Verification 482913\r\n" +
		"Date: Fri, 12 Sep 2026 22:41:00 +0800\r\n\r\n")
	summary, matches, err := parseForwardedSummary(&imap.Message{Uid: 42}, raw, "INBOX", "target.alias@icloud.com")
	if err != nil || !matches {
		t.Fatalf("expected exact header match, matches=%v err=%v", matches, err)
	}
	if summary.ID != "42" || summary.Folder != "INBOX" || summary.Preview != "" || summary.Code != "482913" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if _, similar, err := parseForwardedSummary(&imap.Message{Uid: 42}, raw, "INBOX", "alias@icloud.com"); err != nil || similar {
		t.Fatalf("similar alias must not match, matches=%v err=%v", similar, err)
	}
}
