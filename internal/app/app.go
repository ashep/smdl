package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ashep/go-app/runner"
	"github.com/rs/zerolog"

	"github.com/ashep/smdl/internal/bot"
	"github.com/ashep/smdl/internal/downloader"
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

	b, err := bot.New(cfg.Telegram.Token, dl, l)
	if err != nil {
		return fmt.Errorf("new bot: %w", err)
	}

	runBot(ctx, b, l)

	return nil
}

func runBot(ctx context.Context, b *bot.Bot, l zerolog.Logger) {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 60

	updates := b.API().GetUpdatesChan(cfg)
	l.Info().Msg("starting")

loop:
	for {
		select {
		case <-ctx.Done():
			b.API().StopReceivingUpdates()
			l.Info().Msg("stopped")
			break loop
		case upd := <-updates:
			switch {
			case upd.Message != nil:
				if err := b.HandleMessage(upd.Message); err != nil {
					l.Error().Err(err).Msg("failed to handle new message")
				}
			case upd.EditedMessage != nil:
				l.Warn().Msg("edited messages are not supported")
			}
		}
	}
}
