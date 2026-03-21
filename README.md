# smdl — Social Media Downloader Bot

A Telegram bot that downloads media from Instagram, YouTube, TikTok, and Facebook and sends it back to you.

## Features

- **Instagram** — reels, posts (images, videos, carousels)
- **YouTube** — videos and Shorts
- **TikTok** — videos
- **Facebook** — videos
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
  cookies: BASE64_ENCODED_COOKIES_JSON  # required

youtube:
  cookies: BASE64_ENCODED_COOKIES_JSON  # optional

facebook:
  cookies: BASE64_ENCODED_COOKIES_JSON  # optional

proxy: "http://user:pass@host:port"  # optional
```

### Cookies

Instagram requires authentication. YouTube and Facebook cookies are optional but may be needed for age-restricted or
private content.

Export your browser cookies as JSON using an extension
like [EditThisCookie](https://chromewebstore.google.com/detail/EditThisCookie%20%28V3%29/ojfebgpkimhlhcblbalbfjblapadhbol),
then base64-encode the JSON array:

```bash
cat cookies.json | base64
```

## Running

```bash
go run main.go
```

## Usage

Send an Instagram, YouTube, TikTok, or Facebook URL to the bot — it replies with the media files.

## License

MIT
