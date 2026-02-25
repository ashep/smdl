# HTTP Proxy Support Design

**Date:** 2026-02-25
**Status:** Approved

## Summary

Add a single global HTTP proxy setting to the config, passed as `--proxy <url>` to all `yt-dlp` and `gallery-dl` invocations.

## Approach

Option A: top-level `proxy` string field on `Config`. Optional — empty string means no proxy.

Supported URL format (handled natively by the tools):
- `http://host:port`
- `http://user:pass@host:port`
- `socks5://host:port`

## Changes

### `internal/app/config.go`
Add `Proxy string \`yaml:"proxy"\`` as a top-level field on `Config`.

### `internal/downloader/downloader.go`
- Add `proxy string` field to `Downloader`.
- Add `proxy` parameter to `New(...)`.
- Add `proxyArgs() []string` helper returning `["--proxy", d.proxy]` when non-empty, `nil` otherwise.

### `internal/downloader/youtube.go`
Prepend `d.proxyArgs()` to the `yt-dlp` args slice.

### `internal/downloader/tiktok.go`
Prepend `d.proxyArgs()` to the `yt-dlp` args slice.

### `internal/downloader/instagram.go`
Prepend `d.proxyArgs()` to both the `yt-dlp` args slice and the `gallery-dl` fallback args slice.

### `internal/app/app.go`
Pass `cfg.Proxy` as the new argument to `downloader.New(...)`.

## Error Handling

No startup validation of the proxy URL. Malformed or unreachable proxy errors surface through `yt-dlp`/`gallery-dl` stderr and are propagated to the bot as usual.
