# smdl — Social Media Downloader Bot

A Telegram bot that downloads media from Instagram and YouTube Shorts and sends it back to you.

## Features

- **Instagram** — reels, posts (images, videos, carousels)
- **YouTube Shorts** — single videos
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
  cookies_file: cookies.json
```

### Instagram cookies

Instagram requires authentication. Export your browser cookies using an extension like [Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc) and save the result as `cookies.json` next to `config.yml`.

## Running

```bash
go run main.go
```

## Usage

Send an Instagram or YouTube Shorts URL to the bot — it replies with the media files.

## License

MIT
