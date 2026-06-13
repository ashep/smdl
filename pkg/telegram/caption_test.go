package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
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
	if utf16Len(got) > captionLimit {
		t.Errorf("caption length %d UTF-16 units exceeds limit %d", utf16Len(got), captionLimit)
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
	if !utf8.ValidString(got) {
		t.Fatal("truncation produced invalid UTF-8 (split a multi-byte rune)")
	}
}

func TestTruncateCaption_EmojiStaysWithinUTF16Limit(t *testing.T) {
	url := "https://x.com/a/status/1"
	// Each 😀 (U+1F600) is one rune but two UTF-16 units.
	long := strings.Repeat("😀", 2000)
	got := truncateCaption(url, long)
	if utf16Len(got) > captionLimit {
		t.Errorf("emoji caption is %d UTF-16 units, exceeds limit %d", utf16Len(got), captionLimit)
	}
	if !utf8.ValidString(got) {
		t.Fatal("produced invalid UTF-8")
	}
}

func TestTruncateCaption_URLExceedsLimit(t *testing.T) {
	url := strings.Repeat("a", captionLimit) // prefix alone exceeds the limit
	got := truncateCaption(url, "some text")
	if got != url {
		t.Errorf("expected URL-only return when budget <= 0, got %q", got)
	}
}
