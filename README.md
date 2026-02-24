# smdl — Social Media Downloader Bot

A Telegram bot that downloads media from Instagram and YouTube Shorts and sends it back to you.

## Features

- **Instagram** — reels, posts (images, videos, carousels)
- **YouTube Shorts** — single videos
- **TikTok** — single videos
- Oversized videos (>50 MB) are automatically compressed with ffmpeg before sending
- Non-MP4 videos (WebM, MKV, MOV, AVI) are converted to MP4 for inline playback
- Typing indicator while downloading/processing

## Requirements

- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [gallery-dl](https://github.com/mikf/gallery-dl) (Instagram image fallback)
- [ffmpeg](https://ffmpeg.org) (video compression)

```bash
brew install yt-dlp gallery-dl ffmpeg
```

## Configuration

```yaml
# config.yml
telegram:
  token: YOUR_TELEGRAM_BOT_TOKEN

instagram:
  cookies: |
    [{"domain": ".instagram.com", "name": "sessionid", "value": "...", ...}]
  # or base64-encoded (useful for env-var injection or secrets managers):
  # cookies64: W3siZG9tYWluIjogIi5pbnN0YWdyYW0uY29tIiwgLi4ufV0=
```

### Instagram cookies

Instagram requires authentication. Export your browser cookies as JSON using an extension like [EditThisCookie](https://chromewebstore.google.com/detail/EditThisCookie%20%28V3%29/ojfebgpkimhlhcblbalbfjblapadhbol).

- **`cookies`** — paste the JSON array directly
- **`cookies64`** — paste the JSON array base64-encoded (fallback if `cookies` is not set)

To encode: `cat cookies.json | base64`

## Running

```bash
go run main.go
```

## Usage

Send an Instagram, YouTube Shorts, or TikTok URL to the bot — it replies with the media files.

## License

MIT
