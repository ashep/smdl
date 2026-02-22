package bot

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Downloader interface {
	GetInstagram(rawURL string) (string, error)
	GetYouTube(rawURL string) (string, error)
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
			welcome := "👋 Hello! Send me an Instagram or YouTube Shorts link and I'll download the media for you."
			welcome += "\n\nВСЬО БЕСПЛАТНО! Сделано специально для Марины Владимировны, которую душит жаба 🐸"
			_, err := b.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, welcome))
			if err != nil {
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
						media = append(media, tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(compressed)))
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
				media = append(media, tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(converted)))
			}
		case ".mp4":
			media = append(media, tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(path)))
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
		mg := tgbotapi.NewMediaGroup(msg.Chat.ID, media[i:end])
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
