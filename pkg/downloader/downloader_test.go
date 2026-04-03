package downloader

import (
	"testing"

	"github.com/rs/zerolog"
)

func newTestDownloader(t *testing.T) *Downloader {
	t.Helper()
	return &Downloader{dstDir: t.TempDir(), l: zerolog.Nop()}
}

func TestIsURLEligible_Twitter(t *testing.T) {
	d := newTestDownloader(t)
	cases := []struct {
		url  string
		want bool
	}{
		{"https://twitter.com/user/status/123", true},
		{"https://x.com/user/status/456", true},
		{"https://www.twitter.com/user/status/789", true},
		{"https://www.x.com/user/status/999", true},
		{"https://example.com/tweet", false},
	}
	for _, c := range cases {
		got := d.IsURLEligible(c.url)
		if got != c.want {
			t.Errorf("IsURLEligible(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestDownload_TwitterRouting(t *testing.T) {
	d := newTestDownloader(t)
	// A twitter.com URL should not return ErrURLNotSupported.
	_, err := d.Download("https://twitter.com/user/status/123")
	if err == ErrURLNotSupported {
		t.Error("twitter.com URL returned ErrURLNotSupported, expected routing to getTwitter")
	}
	// An x.com URL should not return ErrURLNotSupported.
	_, err = d.Download("https://x.com/user/status/456")
	if err == ErrURLNotSupported {
		t.Error("x.com URL returned ErrURLNotSupported, expected routing to getTwitter")
	}
}
