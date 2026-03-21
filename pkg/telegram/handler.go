package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ashep/smdl/pkg/downloader"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Downloader interface {
	IsURLEligible(rawURL string) bool
	Download(rawURL string) ([]downloader.MediaFile, error)
}

type MessageHandler struct {
	bot *tgbotapi.BotAPI
	dl  Downloader
	l   zerolog.Logger
}

func NewMessageHandler(bot *tgbotapi.BotAPI, dl Downloader, l zerolog.Logger) *MessageHandler {
	return &MessageHandler{
		bot: bot,
		dl:  dl,
		l:   l,
	}
}

// Handle processes an incoming Telegram message. Plain-text messages are
// treated as media URLs: the URL is downloaded via the Downloader, and the
// resulting files are sent back to the same chat as a media group (up to 10
// items per batch). Files that exceed the 50 MB Telegram bot upload limit are
// skipped with a notice to the user. Bot commands are handled separately —
// /start sends a welcome message; unknown commands are silently ignored.
// Returns an error only for unexpected I/O failures; Telegram API errors are
// logged and swallowed so the update loop can continue.
func (h *MessageHandler) Handle(msg *tgbotapi.Message) error {
	rawURL := strings.TrimSpace(msg.Text)

	if !h.dl.IsURLEligible(rawURL) {
		return nil
	}

	l := h.l.With().
		Int64("chat_id", msg.Chat.ID).
		Int64("user_id", msg.From.ID).
		Str("user_name", msg.From.UserName).
		Str("first_name", msg.From.FirstName).
		Str("last_name", msg.From.LastName).
		Logger()

	l.Info().Str("url", rawURL).Msg("incoming request")

	// Send "Typing..." every 4s while downloading (Telegram clears it after ~5s).
	stopTyping := make(chan struct{})
	go func() {
		for {
			if _, err := h.bot.Request(tgbotapi.NewChatAction(msg.Chat.ID, tgbotapi.ChatTyping)); err != nil {
				l.Warn().Err(err).Msg("failed to send chat action")
			}
			select {
			case <-stopTyping:
				return
			case <-time.After(4 * time.Second):
			}
		}
	}()

	files, err := h.dl.Download(rawURL)
	if err != nil {
		close(stopTyping)
		l.Error().Err(err).Msg("download failed")
		if _, serr := h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Sorry, downloading this media is currently not possible.")); serr != nil {
			l.Error().Err(serr).Msg("failed to send download error notice")
		}
		return nil
	}

	if len(files) > 0 {
		defer os.RemoveAll(filepath.Dir(files[0].Path))
	}

	const tgMaxFileSize = 50 * 1024 * 1024 // 50 MB — Telegram bot upload limit

	var totalSize int64
	var media []interface{}
	for i, f := range files {
		info, err := os.Stat(f.Path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", f.Path, err)
		}

		if info.Size() > tgMaxFileSize {
			l.Warn().
				Str("file", f.Path).
				Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
				Msg("file exceeds Telegram limit, skipping")
			notice := fmt.Sprintf("File %d is too big: %.2fMB", i+1, float64(info.Size())/1024/1024)
			if _, err := h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, notice)); err != nil {
				l.Error().Err(err).Msg("failed to send size limit notice")
			}
			continue
		}

		totalSize += info.Size()

		switch f.Type {
		case downloader.MediaTypeVideo:
			media = append(media, newInputMediaVideo(f.Path, l))
		case downloader.MediaTypePhoto:
			media = append(media, tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(f.Path)))
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
		if _, err := h.bot.SendMediaGroup(mg); err != nil {
			l.Error().Err(err).Msg("failed to send media group")
		}
	}

	return nil
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
