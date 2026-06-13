# Post Text in Telegram Captions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Include the source post's caption/description text under the original link in the Telegram message the bot sends back, instead of sending the link alone.

**Architecture:** During the existing download, yt-dlp and gallery-dl write a JSON metadata sidecar next to the media. A new `extractCaption` reads the `description` field from that sidecar. `Download` returns a `Result{Files, Caption}` struct. The handler assembles the final caption as `link + blank line + post text`, truncated to Telegram's 1024-char media-caption limit.

**Tech Stack:** Go 1.26, yt-dlp, gallery-dl, go-telegram-bot-api/v5, zerolog.

---

## File Structure

- `pkg/downloader/media.go` — add `Result` struct alongside `MediaFile`.
- `pkg/downloader/downloader.go` — change `Download` signature to return `*Result`; add `extractCaption`; skip `.json` files in `processDir`.
- `pkg/downloader/instagram.go`, `youtube.go`, `tiktok.go`, `facebook.go`, `twitter.go` — add metadata-writing flags (`--write-info-json` for yt-dlp, `--write-metadata` for gallery-dl).
- `pkg/telegram/handler.go` — update the `Downloader` interface, consume `*Result`, add `truncateCaption` helper, use it when setting the caption.
- `pkg/downloader/caption_test.go` (new) — tests for `extractCaption`.
- `pkg/telegram/caption_test.go` (new) — tests for `truncateCaption`.

---

## Task 1: Add the `Result` struct

**Files:**
- Modify: `pkg/downloader/media.go`

- [ ] **Step 1: Add the `Result` struct**

In `pkg/downloader/media.go`, after the `MediaFile` struct (after line 24), add:

```go
// Result is the outcome of a Download: the processed media files plus the
// post's caption/description text (empty when none was found).
type Result struct {
	Files   []MediaFile
	Caption string
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: success (no other code references `Result` yet).

- [ ] **Step 3: Commit**

```bash
git add pkg/downloader/media.go
git commit -m "Add Result struct for download output"
```

---

## Task 2: Implement `extractCaption`

**Files:**
- Modify: `pkg/downloader/downloader.go`
- Test: `pkg/downloader/caption_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/downloader/caption_test.go`:

```go
package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestExtractCaption_YtDlpInfoJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{"title":"t","description":"hello world"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "hello world" {
		t.Errorf("extractCaption = %q, want %q", got, "hello world")
	}
}

func TestExtractCaption_GalleryDLJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01_abc.jpg.json"),
		[]byte(`{"description":"an image caption"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "an image caption" {
		t.Errorf("extractCaption = %q, want %q", got, "an image caption")
	}
}

func TestExtractCaption_NoSidecar(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}

func TestExtractCaption_EmptyDescription(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{"description":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}

func TestExtractCaption_MalformedJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/downloader/ -run TestExtractCaption -v`
Expected: FAIL — `d.extractCaption undefined`.

- [ ] **Step 3: Implement `extractCaption`**

In `pkg/downloader/downloader.go`, add `"encoding/json"` to the import block (keep imports sorted; it goes after `"encoding/base64"`). Then add this method after `processDir` (after line 219):

```go
// extractCaption reads the post's caption/description from the first JSON
// metadata sidecar written by yt-dlp (--write-info-json) or gallery-dl
// (--write-metadata) in subDir. Returns "" when no sidecar is found or it
// cannot be parsed; all failures are non-fatal.
func (d *Downloader) extractCaption(subDir string) string {
	entries, err := os.ReadDir(subDir)
	if err != nil {
		d.l.Warn().Err(err).Str("dir", subDir).Msg("read dir for caption")
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(subDir, entry.Name()))
		if err != nil {
			d.l.Warn().Err(err).Str("file", entry.Name()).Msg("read caption sidecar")
			continue
		}

		var meta struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			d.l.Warn().Err(err).Str("file", entry.Name()).Msg("parse caption sidecar")
			continue
		}
		if meta.Description != "" {
			return meta.Description
		}
	}

	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/downloader/ -run TestExtractCaption -v`
Expected: PASS (all five subtests).

- [ ] **Step 5: Commit**

```bash
git add pkg/downloader/downloader.go pkg/downloader/caption_test.go
git commit -m "Add extractCaption to read post text from metadata sidecar"
```

---

## Task 3: Skip `.json` sidecars in `processDir`

**Files:**
- Modify: `pkg/downloader/downloader.go:173-215`

- [ ] **Step 1: Add a `.json` case to the switch**

In `processDir`, the `switch ext` block currently has `case ".mp4", ...`, `case ".jpg", ...`, and `default`. Add a `.json` case before the `default` so sidecars are skipped silently instead of logging an "unsupported file type" warning:

```go
		case ".json":
			// Metadata sidecar written by yt-dlp/gallery-dl; not media.
			continue
```

- [ ] **Step 2: Verify it builds and existing tests pass**

Run: `go test ./pkg/downloader/ -v`
Expected: PASS (existing tests unaffected; `Download` still returns `[]MediaFile` at this point).

- [ ] **Step 3: Commit**

```bash
git add pkg/downloader/downloader.go
git commit -m "Skip JSON metadata sidecars in processDir"
```

---

## Task 4: Change `Download` to return `*Result`

**Files:**
- Modify: `pkg/downloader/downloader.go:116-146`
- Modify: `pkg/telegram/handler.go:20-23,84-102,150-153`

This task changes the signature and updates the only consumer in the same commit so the build stays green.

- [ ] **Step 1: Update `Download` to return `*Result`**

Replace the `Download` method body (lines 116-146) so it returns `*Result`. The error returns become `nil, err`, and the final return builds a `Result`:

```go
func (d *Downloader) Download(rawURL string) (*Result, error) {
	if !d.IsURLEligible(rawURL) {
		return nil, ErrURLNotSupported
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrURLNotSupported
	}

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
	case strings.Contains(u.Host, "twitter.com"), u.Hostname() == "x.com" || strings.HasSuffix(u.Hostname(), ".x.com"):
		subDir, err = d.getTwitter(rawURL)
	default:
		return nil, ErrURLNotSupported
	}
	if err != nil {
		return nil, err
	}

	files, err := d.processDir(subDir)
	if err != nil {
		return nil, err
	}

	return &Result{Files: files, Caption: d.extractCaption(subDir)}, nil
}
```

- [ ] **Step 2: Update the `Downloader` interface in the handler**

In `pkg/telegram/handler.go`, change the interface (lines 20-23):

```go
type Downloader interface {
	IsURLEligible(rawURL string) bool
	Download(rawURL string) (*downloader.Result, error)
}
```

- [ ] **Step 3: Update `Handle` to consume `*Result`**

In `pkg/telegram/handler.go`, change line 84 from `files, err := h.dl.Download(rawURL)` to:

```go
	res, err := h.dl.Download(rawURL)
```

Then, after the error handling block (currently `if len(files) > 0 {` at line 100), introduce a local `files` from the result. Replace line 100-102:

```go
	files := res.Files
	if len(files) > 0 {
		defer os.RemoveAll(filepath.Dir(files[0].Path))
	}
```

(The loop at line 106 `for i, f := range files` continues to work unchanged.)

- [ ] **Step 4: Run tests to verify the build and existing tests pass**

Run: `go test ./... 2>&1 | tail -20`
Expected: PASS. Note `TestDownload_TwitterRouting` uses `_, err := d.Download(...)` which still compiles with the new signature.

- [ ] **Step 5: Commit**

```bash
git add pkg/downloader/downloader.go pkg/telegram/handler.go
git commit -m "Return Result with caption from Download"
```

---

## Task 5: Implement and use `truncateCaption`

**Files:**
- Modify: `pkg/telegram/handler.go`
- Test: `pkg/telegram/caption_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/telegram/caption_test.go`:

```go
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
	// Multi-byte runes; ensure the result is valid UTF-8 (no split rune).
	long := strings.Repeat("é", 2000) // é
	got := truncateCaption(url, long)
	for _, r := range got {
		if r == '�' {
			t.Fatal("truncation split a multi-byte rune")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/telegram/ -run TestTruncateCaption -v`
Expected: FAIL — `undefined: truncateCaption` and `undefined: captionLimit`.

- [ ] **Step 3: Implement `truncateCaption`**

In `pkg/telegram/handler.go`, add near the top after the imports (before the `Downloader` interface):

```go
// captionLimit is Telegram's media-caption length cap (1024 UTF-16 code
// units). We count runes and target a value slightly under the hard limit to
// stay clear of the rune-vs-UTF-16 counting difference for multi-byte text.
const captionLimit = 1000
```

Then add the helper near `withCaption` (after line 178):

```go
// truncateCaption builds the message caption: the link, a blank line, and the
// post text, cut on a rune boundary with a trailing ellipsis so the whole
// caption fits within captionLimit. When postText is empty it returns rawURL
// unchanged.
func truncateCaption(rawURL, postText string) string {
	if postText == "" {
		return rawURL
	}

	prefix := rawURL + "\n\n"
	budget := captionLimit - len([]rune(prefix))
	if budget <= 0 {
		// URL alone already at/over the limit; nothing to add.
		return rawURL
	}

	runes := []rune(postText)
	if len(runes) <= budget {
		return prefix + postText
	}

	// Reserve one rune for the ellipsis.
	return prefix + string(runes[:budget-1]) + "…"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/telegram/ -run TestTruncateCaption -v`
Expected: PASS (all four subtests).

- [ ] **Step 5: Use `truncateCaption` in `Handle`**

In `pkg/telegram/handler.go`, change the caption line inside the media-group loop (currently line 152 `batch[0] = withCaption(batch[0], rawURL)`) to:

```go
			batch[0] = withCaption(batch[0], truncateCaption(rawURL, res.Caption))
```

- [ ] **Step 6: Run the full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/telegram/handler.go pkg/telegram/caption_test.go
git commit -m "Assemble caption from link and truncated post text"
```

---

## Task 6: Write metadata sidecars during download

**Files:**
- Modify: `pkg/downloader/instagram.go:47-80`
- Modify: `pkg/downloader/youtube.go:46-54`
- Modify: `pkg/downloader/tiktok.go`
- Modify: `pkg/downloader/facebook.go`
- Modify: `pkg/downloader/twitter.go:41-47`

Add the metadata flags so the sidecars `extractCaption` reads actually get written. yt-dlp uses `--write-info-json`; the gallery-dl fallback uses `--write-metadata`.

- [ ] **Step 1: Instagram — add both flags**

In `pkg/downloader/instagram.go`, in the yt-dlp `args` block (after `--no-playlist`, around line 50), add `"--write-info-json",`. In the gallery-dl `gdlArgs` block (after `--filename ...`, around line 77), add `"--write-metadata",`.

- [ ] **Step 2: YouTube — add `--write-info-json`**

In `pkg/downloader/youtube.go`, in the `args` block after `--format ...` (around line 50), add `"--write-info-json",`.

- [ ] **Step 3: TikTok — add `--write-info-json`**

In `pkg/downloader/tiktok.go`, locate the yt-dlp `args` block (the `--output`/`--format` flags) and add `"--write-info-json",` to it.

- [ ] **Step 4: Facebook — add `--write-info-json`**

In `pkg/downloader/facebook.go`, locate the yt-dlp `args` block and add `"--write-info-json",` to it.

- [ ] **Step 5: Twitter — add `--write-info-json`**

In `pkg/downloader/twitter.go`, in the `args` block (after `--format ...`, before `rawURL` at line 45), add `"--write-info-json",`.

- [ ] **Step 6: Verify the build and tests pass**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: PASS. (Sidecars are only written during real downloads, which tests don't perform; this step just confirms nothing broke.)

- [ ] **Step 7: Commit**

```bash
git add pkg/downloader/instagram.go pkg/downloader/youtube.go pkg/downloader/tiktok.go pkg/downloader/facebook.go pkg/downloader/twitter.go
git commit -m "Write metadata sidecars during download for caption extraction"
```

---

## Task 7: Manual end-to-end verification

**Files:** none (manual check)

- [ ] **Step 1: Build the binary**

Run: `go build -o /tmp/smdl-test .`
Expected: success.

- [ ] **Step 2: Verify against a real post (requires configured `config.yml` and tools installed)**

Send a Twitter/X and a YouTube Shorts link to the running bot and confirm the returned media's caption shows the link on the first line, a blank line, then the post text. Send a post with a very long caption and confirm it ends with `…` and isn't rejected by Telegram.

- [ ] **Step 3: Confirm fallback**

Send a link to a post with no caption and confirm the message shows just the link (current behavior preserved).

---

## Self-Review Notes

- **Spec coverage:** `Result` struct (Task 1), `extractCaption` reading `description` from yt-dlp + gallery-dl sidecars (Task 2), `.json` skip in `processDir` (Task 3), `Download` returns `*Result` (Task 4), link-on-top + truncate-with-ellipsis + URL-only fallback in `truncateCaption` (Task 5), metadata flags on all five platforms incl. gallery-dl (Task 6). All spec sections map to tasks.
- **Type consistency:** `Result{Files, Caption}` used identically in Tasks 1, 4, 5; `extractCaption(subDir string) string` and `truncateCaption(rawURL, postText string) string` signatures consistent across tasks; `captionLimit` defined once (Task 5) and referenced by the test in the same task.
- **No placeholders:** every code step shows the actual code and exact run commands.
