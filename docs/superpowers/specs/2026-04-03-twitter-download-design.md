---
title: Twitter/X Media Downloading
date: 2026-04-03
status: approved
---

## Overview

Add support for downloading images and videos from public Twitter/X tweets via the existing Telegram bot interface. Both `twitter.com` and `x.com` domains are supported.

## Scope

- Images and videos from public tweets only (no authentication/cookies required)
- Both `twitter.com` and `x.com` URLs accepted
- No restrictions on tweet type (unlike YouTube which is limited to Shorts)

## Architecture

No new dependencies or configuration. The change fits entirely within the `pkg/downloader` package, mirroring the TikTok/Facebook pattern.

### New file: `pkg/downloader/twitter.go`

Implements `(d *Downloader) getTwitter(rawURL string) (string, error)`:

- Parse and validate the URL
- Derive a filesystem-safe slug from the URL path for the download subdirectory
- Run `yt-dlp` with `--format bestvideo[filesize<50M]+bestaudio/best[filesize<50M]/best` and no `--cookies` flag
- Return the subdirectory path on success; remove the subdirectory on failure

### Modified: `pkg/downloader/downloader.go`

Two additions:

1. `IsURLEligible`: add `twitter.com` and `x.com` to the host check
2. `Download`: add a `case` routing both hosts to `getTwitter()`

### Unchanged

- `internal/app/config.go` — no new config fields needed
- `pkg/telegram/handler.go` — already platform-agnostic

## Error handling

Follows the same pattern as TikTok/Facebook: `yt-dlp` stderr is captured and wrapped as an error; the subdirectory is cleaned up on failure. No new error sentinel values needed.
