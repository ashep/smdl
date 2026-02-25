# HTTP Proxy Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a single global `proxy` string field to the app config and pass it as `--proxy <url>` to every `yt-dlp` and `gallery-dl` invocation.

**Architecture:** The proxy URL is stored as a plain string on the `Downloader` struct. A helper method `proxyArgs()` returns the flag slice when non-empty, nil otherwise. Each per-platform downloader prepends `proxyArgs()` to its args. No startup validation — the tools surface errors themselves.

**Tech Stack:** Go 1.25, `yt-dlp`, `gallery-dl`, zerolog, gopkg.in/yaml.v3

---

### Task 1: Add `Proxy` field to `Config`

**Files:**
- Modify: `internal/app/config.go`

**Step 1: Add the field**

In `internal/app/config.go`, add `Proxy` to the `Config` struct:

```go
type Config struct {
	Telegram  Telegram  `yaml:"telegram"`
	Instagram Instagram `yaml:"instagram"`
	YouTube   YouTube   `yaml:"youtube"`
	Proxy     string    `yaml:"proxy"`
}
```

**Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/app/config.go
git commit -m "feat: add proxy field to config"
```

---

### Task 2: Add `proxy` field and `proxyArgs()` helper to `Downloader`

**Files:**
- Modify: `internal/downloader/downloader.go`

**Step 1: Add the field to the struct**

In `internal/downloader/downloader.go`, update the `Downloader` struct:

```go
type Downloader struct {
	dstDir                 string
	cookiesFilename        string
	youtubeCookiesFilename string
	proxy                  string
	l                      zerolog.Logger
}
```

**Step 2: Add the `proxy` parameter to `New`**

Update the `New` function signature and store the value:

```go
func New(dstDir, instagramCookiesJSON, youtubeCookiesJSON, proxy string, l zerolog.Logger) (*Downloader, error) {
	if instagramCookiesJSON == "" {
		return nil, fmt.Errorf("instagram cookies json is required")
	}

	d := &Downloader{dstDir: dstDir, proxy: proxy, l: l}
	// ... rest unchanged
```

**Step 3: Add the `proxyArgs()` helper**

Append this method after `New` (before `runCmd`):

```go
// proxyArgs returns ["--proxy", d.proxy] when a proxy is configured, nil otherwise.
func (d *Downloader) proxyArgs() []string {
	if d.proxy == "" {
		return nil
	}
	return []string{"--proxy", d.proxy}
}
```

**Step 4: Verify it compiles**

```bash
go build ./...
```

Expected: compile error in `internal/app/app.go` — `downloader.New` now requires a fourth argument. That's fine; we'll fix it in the next task.

**Step 5: Commit**

```bash
git add internal/downloader/downloader.go
git commit -m "feat: add proxy field and proxyArgs helper to Downloader"
```

---

### Task 3: Wire proxy through `app.go`

**Files:**
- Modify: `internal/app/app.go`

**Step 1: Pass `cfg.Proxy` to `downloader.New`**

Find the `downloader.New` call and add `cfg.Proxy` as the fourth argument:

```go
dl, err := downloader.New(dstDir, igCookies, ytCookies, cfg.Proxy, l)
```

**Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: pass proxy config to downloader"
```

---

### Task 4: Apply proxy to YouTube downloader

**Files:**
- Modify: `internal/downloader/youtube.go`

**Step 1: Prepend `proxyArgs()` to the args slice**

In `GetYouTube`, update the args construction:

```go
args := d.proxyArgs()
args = append(args,
    "--output", outputTmpl,
    "--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]",
)
if d.youtubeCookiesFilename != "" {
    args = append(args, "--cookies", d.youtubeCookiesFilename)
}
args = append(args, rawURL)
```

**Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/downloader/youtube.go
git commit -m "feat: apply proxy to YouTube downloader"
```

---

### Task 5: Apply proxy to TikTok downloader

**Files:**
- Modify: `internal/downloader/tiktok.go`

**Step 1: Prepend `proxyArgs()` to the args slice**

In `GetTikTok`, replace the inline `[]string{...}` literal with:

```go
args := d.proxyArgs()
args = append(args,
    "--output", outputTmpl,
    "--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]",
    rawURL,
)

errMsg, err := d.runCmd("yt-dlp", args)
```

**Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/downloader/tiktok.go
git commit -m "feat: apply proxy to TikTok downloader"
```

---

### Task 6: Apply proxy to Instagram downloader

**Files:**
- Modify: `internal/downloader/instagram.go`

**Step 1: Prepend `proxyArgs()` to the yt-dlp args**

In `GetInstagram`, update the `args` variable:

```go
args := d.proxyArgs()
args = append(args,
    "--output", outputTmpl,
    "--no-playlist",
    "--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]/best",
    "--cookies", d.cookiesFilename,
)
```

**Step 2: Prepend `proxyArgs()` to the gallery-dl fallback args**

Update the `gdlArgs` variable:

```go
gdlArgs := d.proxyArgs()
gdlArgs = append(gdlArgs,
    "-D", subDir,
    "--filename", "{num:>02}_{post_id}.{extension}",
    "--cookies", d.cookiesFilename,
    rawURL,
)
errMsg, err = d.runCmd("gallery-dl", gdlArgs)
```

Note: remove the separate `gdlArgs = append(gdlArgs, rawURL)` line that follows, since `rawURL` is now included above.

**Step 3: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 4: Commit**

```bash
git add internal/downloader/instagram.go
git commit -m "feat: apply proxy to Instagram downloader"
```

---

### Task 7: Final verification and push

**Step 1: Full build**

```bash
go build ./...
```

Expected: no errors.

**Step 2: Push**

```bash
git push
```
