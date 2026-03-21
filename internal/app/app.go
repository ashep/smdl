package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/smdl/pkg/downloader"
	"github.com/ashep/smdl/pkg/telegram"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

func Run(rt *runner.Runtime[Config]) error {
	ctx := rt.Ctx
	cfg := rt.Cfg
	l := rt.Log

	dstDir, err := os.MkdirTemp("", "smdl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dstDir); err != nil {
			l.Err(err).Str("path", dstDir).Msg("remove temp dir")
		} else {
			l.Info().Str("path", dstDir).Msg("temp dir removed")
		}
	}()

	igCookies := cfg.Instagram.Cookies
	if igCookies == "" && cfg.Instagram.Cookies64 != "" {
		decoded, derr := base64.StdEncoding.DecodeString(cfg.Instagram.Cookies64)
		if derr != nil {
			return fmt.Errorf("decode instagram cookies64: %w", derr)
		}
		igCookies = string(decoded)
	}

	ytCookies := cfg.YouTube.Cookies
	if ytCookies == "" && cfg.YouTube.Cookies64 != "" {
		decoded, derr := base64.StdEncoding.DecodeString(cfg.YouTube.Cookies64)
		if derr != nil {
			return fmt.Errorf("decode youtube cookies64: %w", derr)
		}
		ytCookies = string(decoded)
	}

	fbCookies := cfg.Facebook.Cookies
	if fbCookies == "" && cfg.Facebook.Cookies64 != "" {
		decoded, derr := base64.StdEncoding.DecodeString(cfg.Facebook.Cookies64)
		if derr != nil {
			return fmt.Errorf("decode facebook cookies64: %w", derr)
		}
		fbCookies = string(decoded)
	}

	dl, err := downloader.New(dstDir, igCookies, ytCookies, fbCookies, cfg.Proxy, l)
	if err != nil {
		return fmt.Errorf("new downloader: %w", err)
	}

	tgAPI, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return fmt.Errorf("tgbotapi.NewBotAPI: %w", err)
	}

	runBot(ctx, tgAPI, telegram.NewMessageHandler(tgAPI, dl, l), l)

	return nil
}

func runBot(ctx context.Context, tgAPI *tgbotapi.BotAPI, msgHandler *telegram.MessageHandler, l zerolog.Logger) {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 60

	updates := tgAPI.GetUpdatesChan(cfg)
	l.Info().Msg("starting")

loop:
	for {
		select {
		case <-ctx.Done():
			tgAPI.StopReceivingUpdates()
			l.Info().Msg("stopped")
			break loop
		case upd := <-updates:
			switch {
			case upd.Message != nil:
				if upd.Message.IsCommand() {
					switch upd.Message.Command() {
					case "start":
						welcome := "Send me an Instagram, YouTube Shorts, TikTok, or Facebook link, and I'll download the media for you."
						if _, err := tgAPI.Send(tgbotapi.NewMessage(upd.Message.Chat.ID, welcome)); err != nil {
							l.Error().Err(err).Msg("failed to send welcome message")
						}
					default:
						if err := msgHandler.Handle(upd.Message); err != nil {
							l.Error().Err(err).Msg("failed to handle new message")
						}
					}
				}
			case upd.EditedMessage != nil:
				l.Warn().Msg("edited messages are not supported")
			}
		}
	}
}
