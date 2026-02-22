# Instagram Downloader Design

**Date:** 2026-02-22

## Function Signature

```go
func GetInstagram(url, destDir string) (string, error)
```

## Behavior

1. Create a unique subdirectory inside `destDir` derived from the URL shortcode
2. Read `INSTAGRAM_COOKIES_FILE` env var; if set, pass `--cookies <path>` to `yt-dlp`
3. Run `yt-dlp` with output template `<subdir>/%(title)s.%(ext)s`
4. Return the subdirectory path on success, or a wrapped error containing stderr on failure

## Credentials

- Source: `INSTAGRAM_COOKIES_FILE` environment variable
- Format: Netscape cookies file (exported from browser)
- If not set: attempt unauthenticated download (works for some public posts)

## Error Handling

- `yt-dlp` not in PATH → descriptive error
- Non-zero exit → error wraps stderr output
- `destDir` doesn't exist → created via `os.MkdirAll`

## Dependencies

- None beyond Go stdlib (`os/exec`, `os`, `path/filepath`, `strings`)
- External: `yt-dlp` must be installed on the system
