# Twitter/X Media Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add support for downloading images and videos from public Twitter/X tweets via the existing Telegram bot.

**Architecture:** Add a new `twitter.go` file in `pkg/downloader/` with a `getTwitter()` method using `yt-dlp`, then wire it into the URL eligibility check and dispatch switch in `downloader.go`. No cookies or config changes needed.

**Tech Stack:** Go, `yt-dlp` (external binary, already used by TikTok/Facebook/YouTube handlers)

---

### Task 1: Add `getTwitter()` downloader

**Files:**
- Create: `pkg/downloader/twitter.go`
- Create: `pkg/downloader/twitter_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/downloader/twitter_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/ashep/src/my/smdl && go test ./pkg/downloader/ -run TestGetTwitter -v
```

Expected: compile error — `d.getTwitter` undefined.

- [ ] **Step 3: Implement `getTwitter()`**

Create `pkg/downloader/twitter.go`:

```go
package downloader

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// getTwitter downloads images and videos from a public Twitter/X tweet into a
// subdirectory of dstDir and returns the subdirectory path. It shells out to
// yt-dlp which must be installed on the system.
func (d *Downloader) getTwitter(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/user/status/123" -> "user_status_123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "twitter"
	}

	subDir := filepath.Join(d.dstDir, slug)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	var downloadErr error
	defer func() {
		if downloadErr != nil {
			os.RemoveAll(subDir)
		}
	}()

	outputTmpl := filepath.Join(subDir, "%(title)s.%(ext)s")

	args := d.proxyArgs()
	args = append(args,
		"--output", outputTmpl,
		"--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]/best",
		rawURL,
	)

	errMsg, err := d.runCmd("yt-dlp", args)
	if err != nil {
		downloadErr = fmt.Errorf("%s", errMsg)
		return "", downloadErr
	}

	return subDir, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/ashep/src/my/smdl && go test ./pkg/downloader/ -run TestGetTwitter -v
```

Expected:
```
--- PASS: TestGetTwitter_InvalidURL (0.00s)
--- PASS: TestGetTwitter_CreatesSubDir (...)
PASS
```

(The second test passes because yt-dlp fails on a fake tweet URL and the subdir is cleaned up.)

- [ ] **Step 5: Commit**

```bash
git add pkg/downloader/twitter.go pkg/downloader/twitter_test.go
git commit -m "feat: add Twitter/X media downloader"
```

---

### Task 2: Wire Twitter into URL eligibility and dispatch

**Files:**
- Modify: `pkg/downloader/downloader.go` (lines 99–104 and 119–128)
- Create: `pkg/downloader/downloader_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/downloader/downloader_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/ashep/src/my/smdl && go test ./pkg/downloader/ -run "TestIsURLEligible_Twitter|TestDownload_TwitterRouting" -v
```

Expected: both tests fail — `twitter.com` / `x.com` not yet in eligible hosts.

- [ ] **Step 3: Update `IsURLEligible` in `downloader.go`**

In `pkg/downloader/downloader.go`, update the `return` statement in `IsURLEligible` (currently lines 99–104):

```go
	return strings.Contains(u.Host, "instagram.com") ||
		strings.Contains(u.Host, "youtube.com") ||
		strings.Contains(u.Host, "youtu.be") ||
		strings.Contains(u.Host, "tiktok.com") ||
		strings.Contains(u.Host, "facebook.com") ||
		strings.Contains(u.Host, "fb.watch") ||
		strings.Contains(u.Host, "twitter.com") ||
		strings.Contains(u.Host, "x.com")
```

- [ ] **Step 4: Update the `Download` switch in `downloader.go`**

In the `switch` block in `Download` (currently lines 119–128), add a case before `default`:

```go
	case strings.Contains(u.Host, "twitter.com"), strings.Contains(u.Host, "x.com"):
		subDir, err = d.getTwitter(rawURL)
```

The full switch block should look like:

```go
	var subDir string
	switch {
	case strings.Contains(u.Host, "instagram.com"):
		subDir, err = d.getInstagram(rawURL)
	case strings.Contains(u.Host, "youtube.com"), strings.Contains(u.Host, "youtu.be"):
		subDir, err = d.getYouTube(rawURL)
	case strings.Contains(u.Host, "tiktok.com"):
		subDir, err = d.getTikTok(rawURL)
	case strings.Contains(u.Host, "facebook.com"), strings.Contains(u.Host, "fb.watch"):
		subDir, err = d.getFacebook(rawURL)
	case strings.Contains(u.Host, "twitter.com"), strings.Contains(u.Host, "x.com"):
		subDir, err = d.getTwitter(rawURL)
	default:
		return nil, ErrURLNotSupported
	}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /Users/ashep/src/my/smdl && go test ./pkg/downloader/ -v
```

Expected: all tests pass.

- [ ] **Step 6: Verify the whole project builds**

```bash
cd /Users/ashep/src/my/smdl && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 7: Commit**

```bash
git add pkg/downloader/downloader.go pkg/downloader/downloader_test.go
git commit -m "feat: wire Twitter/X URLs into downloader dispatch"
```
