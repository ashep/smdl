package downloader

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

const maxFileSize int64 = 50 * 1024 * 1024

var ErrURLNotSupported = errors.New("url not supported")
var ErrNotAShort = errors.New("not a youtube short")

type Downloader struct {
	dstDir                  string
	cookiesFilename         string
	youtubeCookiesFilename  string
	facebookCookiesFilename string
	proxyURL                string
	l                       zerolog.Logger
}

func New(igCookies, ytCookies, fbCookies64, proxyURL string, l zerolog.Logger) (*Downloader, error) {
	dstDir, err := os.MkdirTemp("", "smdl-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	d := &Downloader{dstDir: dstDir, proxyURL: proxyURL, l: l}

	if igCookies != "" {
		igCookiesJSON, err := base64.StdEncoding.DecodeString(igCookies)
		if err != nil {
			return nil, fmt.Errorf("decode instagram cookies: %w", err)
		}
		cfn, err := d.jsonCookiesToNetscape(string(igCookiesJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to load instagram cookies: %v", err)
		}
		d.cookiesFilename = cfn
		l.Info().Msgf("instagram cookies loaded to %s", cfn)
	}

	if ytCookies != "" {
		ytCookiesJSON, err := base64.StdEncoding.DecodeString(ytCookies)
		if err != nil {
			return nil, fmt.Errorf("decode youtube cookies: %w", err)
		}
		ycfn, err := d.jsonCookiesToNetscape(string(ytCookiesJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to load youtube cookies: %v", err)
		}
		d.youtubeCookiesFilename = ycfn
		l.Info().Msgf("youtube cookies loaded to %s", ycfn)
	}

	if fbCookies64 != "" {
		fbCookiesJSON, err := base64.StdEncoding.DecodeString(fbCookies64)
		if err != nil {
			return nil, fmt.Errorf("decode facebook cookies: %w", err)
		}
		fcfn, err := d.jsonCookiesToNetscape(string(fbCookiesJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to load facebook cookies: %v", err)
		}
		d.facebookCookiesFilename = fcfn
		l.Info().Msgf("facebook cookies loaded to %s", fcfn)
	}

	return d, nil
}

func (d *Downloader) Close() {
	if err := os.RemoveAll(d.dstDir); err != nil {
		d.l.Err(err).Str("path", d.dstDir).Msg("remove temp dir")
	} else {
		d.l.Info().Str("path", d.dstDir).Msg("temp dir removed")
	}
}

func (d *Downloader) IsURLEligible(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}

	return strings.Contains(u.Host, "instagram.com") ||
		strings.Contains(u.Host, "youtube.com") ||
		strings.Contains(u.Host, "youtu.be") ||
		strings.Contains(u.Host, "tiktok.com") ||
		strings.Contains(u.Host, "facebook.com") ||
		strings.Contains(u.Host, "fb.watch")
}

func (d *Downloader) Download(rawURL string) ([]MediaFile, error) {
	if !d.IsURLEligible(rawURL) {
		return nil, ErrURLNotSupported
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrURLNotSupported
	}

	var subDir string
	switch {
	case strings.Contains(u.Host, "instagram.com"):
		subDir, err = d.getInstagram(rawURL)
	case strings.Contains(u.Host, "youtube.com"), strings.Contains(u.Host, "youtu.be"):
		subDir, err = d.getYouTube(rawURL)
	case strings.Contains(u.Host, "tiktok.com"):
		subDir, err = d.getTikTok(rawURL)
	case strings.Contains(u.Host, "facebook.com"), strings.Contains(u.Host, "fb.watch"):
		subDir, err = d.getFacebook(rawURL)
	default:
		return nil, ErrURLNotSupported
	}
	if err != nil {
		return nil, err
	}

	return d.processDir(subDir)
}

func (d *Downloader) processDir(subDir string) ([]MediaFile, error) {
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return nil, fmt.Errorf("read download dir: %w", err)
	}

	var result []MediaFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", entry.Name(), err)
		}

		path := filepath.Join(subDir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))

		d.l.Info().
			Str("file", entry.Name()).
			Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
			Msg("raw file downloaded")

		switch ext {
		case ".mp4", ".webm", ".mkv", ".mov", ".avi":
			finalPath := path

			if info.Size() > maxFileSize {
				d.l.Info().
					Str("file", entry.Name()).
					Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
					Msg("file too large, compressing")

				compressed, cerr := compressVideo(path)
				if cerr != nil {
					d.l.Error().Err(cerr).Msg("compression failed")
				} else {
					cinfo, serr := os.Stat(compressed)
					if serr == nil && cinfo.Size() <= maxFileSize {
						d.l.Info().
							Str("size", fmt.Sprintf("%.2f MB", float64(cinfo.Size())/1024/1024)).
							Msg("compression succeeded")
						finalPath = compressed
					} else {
						d.l.Warn().Msg("compressed file still too large")
						os.Remove(compressed)
					}
				}
			} else if ext != ".mp4" {
				d.l.Info().Str("file", entry.Name()).Msg("converting to mp4")
				converted, cerr := convertToMP4(path)
				if cerr != nil {
					d.l.Error().Err(cerr).Msg("mp4 conversion failed, skipping file")
					continue
				}
				finalPath = converted
			}

			result = append(result, MediaFile{Path: finalPath, Type: MediaTypeVideo})

		case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".gif":
			result = append(result, MediaFile{Path: path, Type: MediaTypePhoto})

		default:
			d.l.Warn().Str("file", entry.Name()).Msg("skipping unsupported file type")
		}
	}

	return result, nil
}

// proxyArgs returns ["--proxy", d.proxyURL] when a proxy is configured, nil otherwise.
func (d *Downloader) proxyArgs() []string {
	if d.proxyURL == "" {
		return nil
	}
	return []string{"--proxy", d.proxyURL}
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
