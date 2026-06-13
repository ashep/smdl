# Post text in Telegram captions — design

## Goal

When the bot sends downloaded media back to a chat, include the **source post's
caption/description text** under the original link, instead of sending the link
alone.

Currently the first media item's caption is just `rawURL` (`handler.go:152`).
The new behavior puts the link on top and the post text below it.

## Scope

- All five platforms: Instagram (video, carousel, and image-only posts),
  YouTube Shorts, TikTok, Facebook, Twitter/X.
- Text source: the post's caption/description.
- Layout: link on the first line, blank line, then post text.
- Long text: truncate with a trailing `…` to fit Telegram's 1024-character
  media-caption limit.
- Image-only Instagram posts (gallery-dl fallback) are in scope.

Out of scope: text-only posts that have no media (the bot still only responds
when media is downloaded), separate follow-up messages, rich formatting.

## Approach

Reuse the single download already performed and parse a metadata sidecar:

- yt-dlp getters gain `--write-info-json` → writes `<name>.info.json`.
- The gallery-dl Instagram fallback gains `--write-metadata` → writes
  `<name>.json` per file.

No extra network calls. The `description` field is read from the sidecar
(yt-dlp normalizes `description` across all its extractors; gallery-dl's
Instagram metadata also exposes `description`).

Rejected alternatives:
- A separate metadata-only call (`yt-dlp --print --skip-download`) doubles
  network round-trips and raises rate-limit/blocking risk on Instagram.
- Capturing `--print` stdout during the download requires reworking `runCmd`
  (which discards stdout today) and mixes metadata with progress output.

## Components

### `Result` struct (pkg/downloader)

```go
type Result struct {
    Files   []MediaFile
    Caption string // post text; empty when none found
}

func (d *Downloader) Download(rawURL string) (*Result, error)
```

`Download` calls the platform getter (returns subDir), then `processDir` for
files and `extractCaption` for text, and returns both in `Result`.

### `extractCaption(subDir string) string`

- Reads directory entries, finds the first `.json` sidecar
  (`*.info.json` from yt-dlp or `*.json` from gallery-dl).
- Unmarshals into `struct { Description string \`json:"description"\` }`.
- Returns the `description`, or `""` if no sidecar / parse error / empty field.
- All failures are non-fatal: log a warning, return `""`.

### `processDir` change

Add a `.json` case that skips the sidecar silently (today it falls through to
the default branch and logs an "unsupported file type" warning).

### Caption assembly (pkg/telegram)

A pure helper:

```go
func truncateCaption(rawURL, postText string) string
```

- If `postText` is empty, return `rawURL` (unchanged current behavior).
- Otherwise build `rawURL + "\n\n" + postText`.
- Telegram media captions are limited to 1024 UTF-16 code units. Budget the
  post text to `limit - len(url) - 2` and cut on a **rune** boundary, appending
  `…` when truncated. Use a small safety margin (target ~1000) to stay clear of
  the UTF-16-vs-rune counting difference.

`Handle` replaces `withCaption(batch[0], rawURL)` with
`withCaption(batch[0], truncateCaption(rawURL, res.Caption))`, where `res` is
the new `*Result` from `Download`.

The `telegram.Downloader` interface is updated so `Download` returns `*Result`.

## Error handling

- Missing/empty caption → link-only caption (current behavior preserved).
- Sidecar parse failure → logged warning, empty caption, download still
  succeeds.
- Metadata flags must not change download success/failure semantics.

## Testing

- `extractCaption`: parses a sample yt-dlp `info.json` and a gallery-dl `.json`
  from a temp dir; missing-file and empty-description cases return `""`.
- `truncateCaption`: empty post text → URL only; short text → URL + text
  untouched; long text → cut on rune boundary with trailing `…` and within the
  limit; multi-byte runes not split.
