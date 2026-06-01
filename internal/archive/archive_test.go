package archive

import (
	"strings"
	"testing"
	"time"

	"github.com/japer-technology/flat-email/internal/model"
)

func derive(t *testing.T, raw string) *derivedMessage {
	t.Helper()
	dm, err := deriveMessage("me@example.com", model.Message{Raw: []byte(raw)}, defaultKeyLen)
	if err != nil {
		t.Fatalf("deriveMessage: %v", err)
	}
	return dm
}

// Malformed multipart (a boundary that never appears) must degrade to a single
// opaque attachment rather than being guessed at (SPEC §12).
func TestMalformedMultipartBecomesOpaqueAttachment(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"Subject: broken\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"NOPE\"\r\n\r\n" +
		"this body has no boundary markers at all\r\n"
	dm := derive(t, raw)
	if len(dm.attachments) != 1 {
		t.Fatalf("want 1 opaque attachment, got %d", len(dm.attachments))
	}
	if dm.attachments[0].ContentType != "application/octet-stream" {
		t.Errorf("want octet-stream, got %s", dm.attachments[0].ContentType)
	}
}

// Two attachments whose names collide only by case must not share an on-disk
// path (SPEC §4.4/§4.5).
func TestCaseInsensitiveAttachmentCollision(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"M\"\r\n\r\n" +
		"--M\r\nContent-Type: text/plain\r\n\r\nbody\r\n" +
		"--M\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"Report.PDF\"\r\n\r\nONE\r\n" +
		"--M\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"report.pdf\"\r\n\r\nTWO\r\n" +
		"--M--\r\n"
	dm := derive(t, raw)
	if len(dm.attachments) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(dm.attachments))
	}
	a, b := dm.attachments[0].OnDiskName, dm.attachments[1].OnDiskName
	if strings.EqualFold(a, b) {
		t.Errorf("attachment names collide case-insensitively: %q vs %q", a, b)
	}
	if b != "report (1).pdf" {
		t.Errorf("want disambiguated 'report (1).pdf', got %q", b)
	}
}

// An attachment with no declared filename gets a deterministic placeholder.
func TestUnnamedAttachmentPlaceholder(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"M\"\r\n\r\n" +
		"--M\r\nContent-Type: text/plain\r\n\r\nbody\r\n" +
		"--M\r\nContent-Type: application/octet-stream\r\n\r\nDATA\r\n" +
		"--M--\r\n"
	dm := derive(t, raw)
	if len(dm.attachments) != 1 || dm.attachments[0].OnDiskName != "part-01" {
		t.Fatalf("want one 'part-01' attachment, got %#v", dm.attachments)
	}
}

// Garbage/missing Date falls back to the unknown-date bucket (SPEC §4.1).
func TestMissingDateGoesToUnknownBucket(t *testing.T) {
	dm := derive(t, "From: a@example.com\r\nSubject: x\r\n\r\nhi\r\n")
	if dm.dateSource != "unknown" || dm.bucket != "unknown-date" {
		t.Errorf("want unknown-date bucket, got source=%s bucket=%s", dm.dateSource, dm.bucket)
	}
	if dm.date != unknownDate {
		t.Errorf("want sentinel date, got %s", dm.date)
	}
}

func TestSanitizeHTMLNeutralisesActiveContent(t *testing.T) {
	in := `<div><script>evil()</script><a href="javascript:steal()">x</a>` +
		`<img src="http://t/p.gif"><iframe src="http://e"></iframe></div>`
	got, err := sanitizeHTML(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "<iframe", "javascript:", "<img src=\"http"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitized output still contains %q: %s", bad, got)
		}
	}
	if !strings.Contains(got, "data-blocked-src=\"http://t/p.gif\"") {
		t.Errorf("remote image not neutralised: %s", got)
	}
}

// A non-UTF-8 (ISO-8859-1) text body is decoded to UTF-8 (SPEC §12).
func TestCharsetDecodingLatin1(t *testing.T) {
	// 0xE9 is 'é' in ISO-8859-1.
	raw := "From: a@example.com\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"iso-8859-1\"\r\n\r\n" +
		"caf\xe9\r\n"
	dm := derive(t, raw)
	if dm.bodyText == nil || !strings.Contains(*dm.bodyText, "café") {
		t.Errorf("latin1 body not decoded to UTF-8: %q", deref(dm.bodyText))
	}
}

// Producing the same input twice yields identical bytes (SPEC §6).
func TestProducerDeterministic(t *testing.T) {
	in := model.Input{
		SyncTime:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: "flat-email/test",
		Accounts: []model.Account{{
			Address:  "me@example.com",
			Messages: []model.Message{{Raw: []byte("From: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n"), Labels: []string{"INBOX"}}},
		}},
	}
	a := newMemBackend()
	b := newMemBackend()
	if err := Produce(a, in); err != nil {
		t.Fatal(err)
	}
	if err := Produce(b, in); err != nil {
		t.Fatal(err)
	}
	if !a.equal(b) {
		t.Error("two runs produced different output")
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
