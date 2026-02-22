package bot

import (
	"context"
	"fmt"
	"net/url"
	"os"
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
	case strings.Contains(u.Host, "youtube.com"):
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
	close(stopTyping)

	if err != nil {
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

		if info.Size() > tgMaxFileSize {
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

		path := filepath.Join(dstDir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".mp4", ".webm", ".mkv", ".mov", ".avi":
			media = append(media, tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(path)))
		case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".gif":
			media = append(media, tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(path)))
		default:
			l.Warn().Str("file", entry.Name()).Msg("skipping unsupported file type")
			continue
		}
	}

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
