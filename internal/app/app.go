package app

import (
	"context"
	"fmt"

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

	dl, err := downloader.New(cfg.Instagram.Cookies, cfg.YouTube.Cookies, cfg.Facebook.Cookies, cfg.Proxy, l)
	if err != nil {
		return fmt.Errorf("new downloader: %w", err)
	}
	defer dl.Close()

	tgAPI, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return fmt.Errorf("tgbotapi.NewBotAPI: %w", err)
	}

	runBot(ctx, tgAPI, telegram.NewMessageHandler(tgAPI, dl, cfg.Telegram.Users, l), l)

	return nil
}

// messageSenderUsername returns the Telegram username of the message's
// sender, or "" if the sender is unknown (e.g. an anonymous group admin
// or a message with no From at all).
func messageSenderUsername(msg *tgbotapi.Message) string {
	if msg.From == nil {
		return ""
	}
	return msg.From.UserName
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
			if upd.Message == nil {
				continue
			}

			userName := messageSenderUsername(upd.Message)
			if !msgHandler.IsAllowed(userName) {
				l.Debug().
					Int64("chat_id", upd.Message.Chat.ID).
					Str("user_name", userName).
					Msg("ignoring request from disallowed user")
				continue
			}

			if upd.Message.IsCommand() {
				if upd.Message.Command() == "start" {
					welcome := "Send me an Instagram, YouTube Shorts, TikTok, Facebook, or Twitter/X link, and I'll download the media for you."
					if _, err := tgAPI.Send(tgbotapi.NewMessage(upd.Message.Chat.ID, welcome)); err != nil {
						l.Error().Err(err).Msg("failed to send welcome message")
					}
				}
				continue
			}

			if err := msgHandler.Handle(upd.Message); err != nil {
				l.Error().Err(err).Msg("failed to handle new message")
			}
		}
	}
}
