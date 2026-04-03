package downloader

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestGetTwitter_InvalidURL(t *testing.T) {
	d := &Downloader{dstDir: t.TempDir(), l: zerolog.Nop()}
	_, err := d.getTwitter("://bad-url")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestGetTwitter_CreatesSubDir(t *testing.T) {
	d := &Downloader{dstDir: t.TempDir(), l: zerolog.Nop()}
	// getTwitter will fail because yt-dlp won't find real media in a test,
	// but the subdir should be cleaned up on failure (not left behind).
	rawURL := "https://twitter.com/user/status/123456789"
	_, err := d.getTwitter(rawURL)
	// We expect an error (yt-dlp not actually downloading a real tweet),
	// but we verify no stale subdirectory was left behind.
	if err == nil {
		t.Skip("yt-dlp unexpectedly succeeded; skipping cleanup check")
	}
	entries, readErr := os.ReadDir(d.dstDir)
	if readErr != nil {
		t.Fatalf("read dstDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("expected dstDir to be empty after failed download, got %d entries", len(entries))
	}
}
