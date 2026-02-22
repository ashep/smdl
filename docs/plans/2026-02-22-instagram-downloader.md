# Instagram Downloader Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `GetInstagram(url, destDir string) (string, error)` in `internal/downloader.go` that shells out to `yt-dlp` to download Instagram media into a caller-specified directory.

**Architecture:** The function creates a unique subdirectory under `destDir`, builds a `yt-dlp` command with an output template pointing to that subdir, optionally passes a cookies file from `INSTAGRAM_COOKIES_FILE` env var, executes the command, and returns the subdir path on success or a wrapped error on failure.

**Tech Stack:** Go stdlib only (`os/exec`, `os`, `path/filepath`, `net/url`). External dependency: `yt-dlp` must be installed on the system.

---

### Task 1: Initialize `internal/downloader.go` with package declaration and imports

**Files:**
- Modify: `internal/downloader.go`

**Step 1: Write the file**

```go
package internal

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
```

**Step 2: Commit**

```bash
git add internal/downloader.go
git commit -m "chore: initialize downloader package"
```

---

### Task 2: Write the failing test

**Files:**
- Create: `internal/downloader_test.go`

**Step 1: Write the failing test**

```go
package internal_test

import (
	"os"
	"path/filepath"
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
// This test requires yt-dlp to be installed and INSTAGRAM_COOKIES_FILE to be set.
// Skip if yt-dlp is not available.
func TestGetInstagram_CreatesSubdir(t *testing.T) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp not in PATH, skipping integration test")
	}
	url := os.Getenv("INSTAGRAM_TEST_URL")
	if url == "" {
		t.Skip("INSTAGRAM_TEST_URL not set, skipping integration test")
	}
	dir := t.TempDir()
	result, err := internal.GetInstagram(url, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// result must be a subdirectory of dir
	rel, err := filepath.Rel(dir, result)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("result %q is not under destDir %q", result, dir)
	}
	// at least one file must exist in the subdir
	entries, err := os.ReadDir(result)
	if err != nil {
		t.Fatalf("cannot read result dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one downloaded file, got none")
	}
}
```

Note: add `"os/exec"` to the import list in the test file.

**Step 2: Run test to verify it fails**

```bash
go test ./internal/... -run TestGetInstagram_InvalidURL -v
```

Expected: compile error — `internal.GetInstagram` undefined.

**Step 3: Commit the test**

```bash
git add internal/downloader_test.go
git commit -m "test: add GetInstagram tests (failing)"
```

---

### Task 3: Implement `GetInstagram`

**Files:**
- Modify: `internal/downloader.go`

**Step 1: Add the function**

```go
// GetInstagram downloads all media from an Instagram URL into a subdirectory
// of destDir and returns the subdirectory path.
//
// It shells out to yt-dlp which must be installed on the system.
// If the environment variable INSTAGRAM_COOKIES_FILE is set, its value is
// passed to yt-dlp as a Netscape cookies file for authenticated downloads.
func GetInstagram(rawURL, destDir string) (string, error) {
	// Validate the URL and derive a safe directory name from the path.
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/p/ABC123/" -> "ABC123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "instagram"
	}

	subDir := filepath.Join(destDir, slug)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	outputTmpl := filepath.Join(subDir, "%(title)s.%(ext)s")

	args := []string{
		"--output", outputTmpl,
		"--no-playlist",
	}

	if cookiesFile := os.Getenv("INSTAGRAM_COOKIES_FILE"); cookiesFile != "" {
		args = append(args, "--cookies", cookiesFile)
	}

	args = append(args, rawURL)

	cmd := exec.Command("yt-dlp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("yt-dlp: %s", msg)
	}

	return subDir, nil
}
```

**Step 2: Run the unit test**

```bash
go test ./internal/... -run TestGetInstagram_InvalidURL -v
```

Expected: PASS

**Step 3: Verify it builds cleanly**

```bash
go build ./...
```

Expected: no output, exit 0.

**Step 4: Commit**

```bash
git add internal/downloader.go
git commit -m "feat: implement GetInstagram using yt-dlp"
```

---

### Task 4: Run integration test (optional, requires environment)

**Step 1: Set up environment**

```bash
export INSTAGRAM_COOKIES_FILE=/path/to/cookies.txt
export INSTAGRAM_TEST_URL=https://www.instagram.com/p/<shortcode>/
```

**Step 2: Run integration test**

```bash
go test ./internal/... -run TestGetInstagram_CreatesSubdir -v
```

Expected: PASS — files downloaded into a temp subdirectory.

---
