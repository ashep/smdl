package telegram

import (
	"strings"
	"testing"
)

func TestTruncateCaption_NoText(t *testing.T) {
	got := truncateCaption("https://x.com/a/status/1", "")
	if got != "https://x.com/a/status/1" {
		t.Errorf("got %q, want URL only", got)
	}
}

func TestTruncateCaption_ShortText(t *testing.T) {
	url := "https://x.com/a/status/1"
	got := truncateCaption(url, "hello")
	want := url + "\n\nhello"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTruncateCaption_LongTextTruncated(t *testing.T) {
	url := "https://x.com/a/status/1"
	long := strings.Repeat("a", 2000)
	got := truncateCaption(url, long)
	if len([]rune(got)) > captionLimit {
		t.Errorf("caption length %d exceeds limit %d", len([]rune(got)), captionLimit)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected trailing ellipsis, got suffix %q", got[len(got)-4:])
	}
	if !strings.HasPrefix(got, url+"\n\n") {
		t.Errorf("expected URL prefix, got %q", got[:40])
	}
}

func TestTruncateCaption_DoesNotSplitRunes(t *testing.T) {
	url := "https://x.com/a/status/1"
	long := strings.Repeat("é", 2000)
	got := truncateCaption(url, long)
	for _, r := range got {
		if r == '�' {
			t.Fatal("truncation split a multi-byte rune")
		}
	}
}
