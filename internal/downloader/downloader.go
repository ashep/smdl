package downloader

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"
)

type Downloader struct {
	dstDir                  string
	cookiesFilename         string
	youtubeCookiesFilename  string
	l                       zerolog.Logger
}

func New(dstDir, instagramCookiesJSON, youtubeCookiesJSON string, l zerolog.Logger) (*Downloader, error) {
	if instagramCookiesJSON == "" {
		return nil, fmt.Errorf("instagram cookies json is required")
	}

	d := &Downloader{dstDir: dstDir, l: l}

	cfn, err := d.jsonCookiesToNetscape(instagramCookiesJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load instagram cookies: %v", err)
	}
	d.cookiesFilename = cfn

	l.Info().Msgf("instagram cookies loaded to %s", cfn)

	if youtubeCookiesJSON != "" {
		ycfn, err := d.jsonCookiesToNetscape(youtubeCookiesJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to load youtube cookies: %v", err)
		}
		d.youtubeCookiesFilename = ycfn
		l.Info().Msgf("youtube cookies loaded to %s", ycfn)
	}

	return d, nil
}

// runCmd executes a command with the given arguments and returns (stderr, error).
func (d *Downloader) runCmd(name string, args []string) (string, error) {
	d.l.Debug().Msgf("executing %s %s", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)

	// Stdout is intentionally discarded; the tool writes downloaded files to disk.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return msg, err
	}

	return "", nil
}
