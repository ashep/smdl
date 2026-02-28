package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Downloader interface {
	GetInstagram(rawURL string) (string, error)
	GetYouTube(rawURL string) (string, error)
	GetTikTok(rawURL string) (string, error)
	GetFacebook(rawURL string) (string, error)
}

type Bot struct {
	bot *tgbotapi.BotAPI
	dl  Downloader
	l   zerolog.Logger
}

func New(tgToken string, dl Downloader, l zerolog.Logger) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}

	return &Bot{
		bot: bot,
		dl:  dl,
		l:   l,
	}, nil
}

func (b *Bot) Run(ctx context.Context) {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 60

	updates := b.bot.GetUpdatesChan(cfg)
	b.l.Info().Msg("starting")

loop:
	for {
		select {
		case <-ctx.Done():
			b.bot.StopReceivingUpdates()
			b.l.Info().Msg("stopped")
			break loop
		case upd := <-updates:
			switch {
			case upd.Message != nil:
				if err := b.handleMessage(upd.Message); err != nil {
					b.l.Error().Err(err).Msg("failed to handle new message")
				}
			case upd.EditedMessage != nil:
				b.l.Warn().Msg("edited messages are not supported")
			}
		}
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) error {
	l := b.l.With().
		Int64("chat_id", msg.Chat.ID).
		Int64("user_id", msg.From.ID).
		Str("user_name", msg.From.UserName).
		Str("first_name", msg.From.FirstName).
		Str("last_name", msg.From.LastName).
		Logger()

	if msg.Text == "" {
		l.Warn().Msg("received empty message, skipping")
		return nil
	}

	l.Info().Str("text", msg.Text).Msg("incoming message")

	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			welcome := "Send me an Instagram, YouTube Shorts, TikTok, or Facebook link, and I'll download the media for you."
			welcome += "\n\nВСЬО БЕСПЛАТНО! Сделано по спецзаказу Марины Владимировны."
			if _, err := b.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, welcome)); err != nil {
				l.Error().Err(err).Msg("failed to send welcome message")
			}
		default:
			l.Info().Str("command", msg.Command()).Msg("unknown command, ignoring")
		}
		return nil
	}

	rawURL := strings.TrimSpace(msg.Text)
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		l.Info().Msg("not a URL, skipping")
		return nil
	}

	var download func() (string, error)
	switch {
	case strings.Contains(u.Host, "instagram.com"):
		download = func() (string, error) { return b.dl.GetInstagram(rawURL) }
	case strings.Contains(u.Host, "youtube.com"), strings.Contains(u.Host, "youtu.be"):
		download = func() (string, error) { return b.dl.GetYouTube(rawURL) }
	case strings.Contains(u.Host, "tiktok.com"):
		download = func() (string, error) { return b.dl.GetTikTok(rawURL) }
	case strings.Contains(u.Host, "facebook.com"), strings.Contains(u.Host, "fb.watch"):
		download = func() (string, error) { return b.dl.GetFacebook(rawURL) }
	default:
		l.Info().Str("host", u.Host).Msg("unsupported URL, skipping")
		return nil
	}

	// Send "Typing..." every 4s while downloading (Telegram clears it after ~5s).
	stopTyping := make(chan struct{})
	go func() {
		for {
			if _, err := b.bot.Request(tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping)); err != nil {
				l.Warn().Err(err).Msg("failed to send chat action")
			}
			select {
			case <-stopTyping:
				return
			case <-time.After(4 * time.Second):
			}
		}
	}()

	dstDir, err := download()

	if err != nil {
		close(stopTyping)
		l.Error().Err(err).Msg("download failed")
		if _, serr := b.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Sorry, downloading this media is currently not possible.")); serr != nil {
			l.Error().Err(serr).Msg("failed to send download error notice")
		}
		return nil
	}
	defer os.RemoveAll(dstDir)

	entries, err := os.ReadDir(dstDir)
	if err != nil {
		return fmt.Errorf("read download dir: %w", err)
	}

	const tgMaxFileSize = 50 * 1024 * 1024 // 50 MB — Telegram bot upload limit

	var totalSize int64
	var media []interface{}
	for i, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", entry.Name(), err)
		}

		path := filepath.Join(dstDir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))

		if info.Size() > tgMaxFileSize {
			switch ext {
			case ".mp4", ".webm", ".mkv", ".mov", ".avi":
				l.Info().
					Str("file", entry.Name()).
					Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
					Msg("file too large, compressing")

				compressed, cerr := compressVideo(path)
				if cerr != nil {
					l.Error().Err(cerr).Msg("compression failed")
				} else {
					defer os.Remove(compressed)
					cinfo, serr := os.Stat(compressed)
					if serr == nil && cinfo.Size() <= tgMaxFileSize {
						l.Info().
							Str("size", fmt.Sprintf("%.2f MB", float64(cinfo.Size())/1024/1024)).
							Msg("compression succeeded")
						totalSize += cinfo.Size()
						media = append(media, newInputMediaVideo(compressed, l))
						continue
					}
					l.Warn().Msg("compressed file still too large")
					os.Remove(compressed)
				}
			}

			l.Warn().
				Str("file", entry.Name()).
				Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
				Msg("file exceeds Telegram limit, skipping")
			notice := fmt.Sprintf("File %d is too big: %.2fMB", i+1, float64(info.Size())/1024/1024)
			if _, err := b.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, notice)); err != nil {
				l.Error().Err(err).Msg("failed to send size limit notice")
			}
			continue
		}

		totalSize += info.Size()

		switch ext {
		case ".webm", ".mkv", ".mov", ".avi":
			l.Info().Str("file", entry.Name()).Msg("converting to mp4")
			converted, cerr := convertToMP4(path)
			if cerr != nil {
				l.Error().Err(cerr).Msg("mp4 conversion failed, sending as document")
				media = append(media, tgbotapi.NewInputMediaDocument(tgbotapi.FilePath(path)))
			} else {
				defer os.Remove(converted)
				media = append(media, newInputMediaVideo(converted, l))
			}
		case ".mp4":
			media = append(media, newInputMediaVideo(path, l))
		case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".gif":
			media = append(media, tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(path)))
		default:
			l.Warn().Str("file", entry.Name()).Msg("skipping unsupported file type")
		}
	}

	close(stopTyping)

	if len(media) == 0 {
		l.Warn().Msg("no files downloaded")
		return nil
	}

	l.Info().
		Int("files", len(media)).
		Str("total_size", fmt.Sprintf("%.2f MB", float64(totalSize)/1024/1024)).
		Msg("downloaded")

	// Telegram allows at most 10 items per media group.
	for i := 0; i < len(media); i += 10 {
		end := i + 10
		if end > len(media) {
			end = len(media)
		}
		batch := media[i:end]
		if i == 0 {
			batch[0] = withCaption(batch[0], rawURL)
		}
		mg := tgbotapi.NewMediaGroup(msg.Chat.ID, batch)
		if _, err := b.bot.SendMediaGroup(mg); err != nil {
			l.Error().Err(err).Msg("failed to send media group")
		}
	}

	return nil
}

// compressVideo re-encodes a video to 720p at CRF 28 using ffmpeg,
// writing the result to a temp file and returning its path.
func compressVideo(inputPath string) (string, error) {
	tmp, err := os.CreateTemp("", "smdl-compressed-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()

	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-crf", "28",
		"-preset", "fast",
		"-c:a", "aac",
		"-b:a", "128k",
		"-y",
		tmp.Name(),
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}

	return tmp.Name(), nil
}

// convertToMP4 remuxes or re-encodes a video to MP4/H.264 so Telegram plays
// it inline. Returns the path to a temp file the caller must delete.
func convertToMP4(inputPath string) (string, error) {
	tmp, err := os.CreateTemp("", "smdl-converted-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()

	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-y",
		tmp.Name(),
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}

	return tmp.Name(), nil
}

// withCaption returns a copy of the InputMedia item with the given caption set.
func withCaption(item interface{}, caption string) interface{} {
	switch v := item.(type) {
	case tgbotapi.InputMediaVideo:
		v.Caption = caption
		return v
	case tgbotapi.InputMediaPhoto:
		v.Caption = caption
		return v
	case tgbotapi.InputMediaDocument:
		v.Caption = caption
		return v
	default:
		return item
	}
}

// newInputMediaVideo creates an InputMediaVideo and attempts to set the correct
// display dimensions by probing the file with ffprobe. If probing fails the
// video is still returned without explicit dimensions.
func newInputMediaVideo(path string, l zerolog.Logger) tgbotapi.InputMediaVideo {
	v := tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(path))
	w, h, err := probeVideoDimensions(path)
	if err != nil {
		l.Warn().Err(err).Str("file", path).Msg("could not probe video dimensions")
		return v
	}
	v.Width = w
	v.Height = h
	return v
}

// probeVideoDimensions returns the display width and height of a video file,
// accounting for the sample aspect ratio (SAR). This is needed because some
// Instagram reels have non-1:1 SAR, causing Telegram to render them at the
// wrong aspect ratio when dimensions are not specified explicitly.
func probeVideoDimensions(path string) (width, height int, err error) {
	type stream struct {
		Width             int    `json:"width"`
		Height            int    `json:"height"`
		SampleAspectRatio string `json:"sample_aspect_ratio"`
	}
	var out struct {
		Streams []stream `json:"streams"`
	}

	var stdout bytes.Buffer
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,sample_aspect_ratio",
		"-of", "json",
		path,
	)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("ffprobe: %w", err)
	}

	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return 0, 0, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if len(out.Streams) == 0 {
		return 0, 0, fmt.Errorf("no video streams found")
	}

	s := out.Streams[0]
	w, h := s.Width, s.Height

	// Apply SAR to get display width. SAR "N:D" means display_width = w * N/D.
	if sar := s.SampleAspectRatio; sar != "" && sar != "1:1" && sar != "0:1" {
		if parts := strings.SplitN(sar, ":", 2); len(parts) == 2 {
			sarNum, e1 := strconv.Atoi(parts[0])
			sarDen, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil && sarDen != 0 {
				w = w * sarNum / sarDen
			}
		}
	}

	return w, h, nil
}
