package app

import (
	"fmt"
	"os"

	"github.com/ashep/go-app/runner"
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

	dl, err := downloader.New(dstDir, cfg.Instagram.Cookies, l)
	if err != nil {
		return fmt.Errorf("new downloader: %w", err)
	}

	b, err := bot.New(cfg.Telegram.Token, dl, l)
	if err != nil {
		return fmt.Errorf("new bot: %w", err)
	}

	b.Run(ctx)

	return nil
}
