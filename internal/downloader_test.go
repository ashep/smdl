package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashep/smdl/internal"
)

// TestGetInstagram_InvalidURL verifies that a malformed URL returns an error.
func TestGetInstagram_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	_, err := internal.GetInstagram("not-a-url", dir)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// TestGetInstagram_CreatesSubdir verifies the subdir is created under destDir.
// Requires yt-dlp in PATH and INSTAGRAM_TEST_URL env var to be set.
func TestGetInstagram_CreatesSubdir(t *testing.T) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp not in PATH, skipping integration test")
	}
	rawURL := os.Getenv("INSTAGRAM_TEST_URL")
	if rawURL == "" {
		t.Skip("INSTAGRAM_TEST_URL not set, skipping integration test")
	}
	dir := t.TempDir()
	result, err := internal.GetInstagram(rawURL, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rel, err := filepath.Rel(dir, result)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("result %q is not under destDir %q", result, dir)
	}
	entries, err := os.ReadDir(result)
	if err != nil {
		t.Fatalf("cannot read result dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one downloaded file, got none")
	}
}
