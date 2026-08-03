package telegram

import (
	"bytes"
	"encoding/json"
	"errors"
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

// captionLimit is Telegram's media-caption cap, measured in UTF-16 code units
// (not runes or bytes). Characters above the BMP — most emoji — count as 2.
const captionLimit = 1024

type Downloader interface {
	IsURLEligible(rawURL string) bool
	Download(rawURL string) (*downloader.Result, error)
}

type MessageHandler struct {
	bot          *tgbotapi.BotAPI
	dl           Downloader
	allowedUsers map[string]struct{}
	l            zerolog.Logger
}

func NewMessageHandler(bot *tgbotapi.BotAPI, dl Downloader, users []string, l zerolog.Logger) *MessageHandler {
	allowed := make(map[string]struct{}, len(users))
	for _, u := range users {
		allowed[strings.ToLower(u)] = struct{}{}
	}

	return &MessageHandler{
		bot:          bot,
		dl:           dl,
		allowedUsers: allowed,
		l:            l,
	}
}

// IsAllowed reports whether username may use the bot. An empty allow-list
// means no restriction is configured, so every username is allowed.
func (h *MessageHandler) IsAllowed(username string) bool {
	if len(h.allowedUsers) == 0 {
		return true
	}

	_, ok := h.allowedUsers[strings.ToLower(username)]
	return ok
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

	if msg.From == nil {
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
	defer close(stopTyping)
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

	res, err := h.dl.Download(rawURL)
	if err != nil {
		if errors.Is(err, downloader.ErrNotAShort) {
			l.Info().Str("url", rawURL).Msg("rejected non-short youtube url")
			if _, serr := h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Only YouTube Shorts are supported. Full-length videos cannot be downloaded.")); serr != nil {
				l.Error().Err(serr).Msg("failed to send not-a-short notice")
			}
			return nil
		}
		l.Error().Err(err).Msg("download failed")
		if _, serr := h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Sorry, downloading this media is currently not possible.")); serr != nil {
			l.Error().Err(serr).Msg("failed to send download error notice")
		}
		return nil
	}

	files := res.Files
	if len(files) > 0 {
		defer os.RemoveAll(filepath.Dir(files[0].Path))
	}

	var totalSize int64
	var media []interface{}
	for i, f := range files {
		info, err := os.Stat(f.Path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", f.Path, err)
		}

		if info.Size() > downloader.MaxFileSize {
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
			batch[0] = withCaption(batch[0], truncateCaption(rawURL, res.Caption))
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

// utf16Len returns the number of UTF-16 code units needed to encode s, which
// is how Telegram measures caption/message length. Code points above U+FFFF
// (e.g. most emoji) require a surrogate pair and count as 2.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// truncateCaption builds the message caption: the link, a blank line, and the
// post text, cut on a rune boundary with a trailing ellipsis so the whole
// caption fits within captionLimit. When postText is empty it returns rawURL
// unchanged.
func truncateCaption(rawURL, postText string) string {
	if postText == "" {
		return rawURL
	}

	prefix := rawURL + "\n\n"
	budget := captionLimit - utf16Len(prefix)
	if budget <= 0 {
		// URL alone already at/over the limit; nothing to add.
		return rawURL
	}

	if utf16Len(postText) <= budget {
		return prefix + postText
	}

	// Truncate, reserving 1 UTF-16 unit for the trailing ellipsis. Append whole
	// runes until the next one would exceed the budget, so we never split a rune.
	var b strings.Builder
	used := 0
	for _, r := range postText {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if used+w > budget-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}

	return prefix + b.String() + "…"
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
