package downloader

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

const MaxFileSize int64 = 50 * 1024 * 1024

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
	for _, f := range []string{d.cookiesFilename, d.youtubeCookiesFilename, d.facebookCookiesFilename} {
		if f != "" {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				d.l.Err(err).Str("path", f).Msg("remove cookie file")
			}
		}
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
		strings.Contains(u.Host, "fb.watch") ||
		strings.Contains(u.Host, "twitter.com") ||
		u.Hostname() == "x.com" || strings.HasSuffix(u.Hostname(), ".x.com")
}

func (d *Downloader) Download(rawURL string) (*Result, error) {
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
	case strings.Contains(u.Host, "twitter.com"), u.Hostname() == "x.com" || strings.HasSuffix(u.Hostname(), ".x.com"):
		subDir, err = d.getTwitter(rawURL)
	default:
		return nil, ErrURLNotSupported
	}
	if err != nil {
		return nil, err
	}

	files, err := d.processDir(subDir)
	if err != nil {
		return nil, err
	}

	return &Result{Files: files, Caption: d.extractCaption(subDir)}, nil
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

			if info.Size() > MaxFileSize {
				d.l.Info().
					Str("file", entry.Name()).
					Str("size", fmt.Sprintf("%.2f MB", float64(info.Size())/1024/1024)).
					Msg("file too large, compressing")

				compressed, cerr := compressVideo(path)
				if cerr != nil {
					d.l.Error().Err(cerr).Msg("compression failed")
				} else {
					cinfo, serr := os.Stat(compressed)
					if serr == nil && cinfo.Size() <= MaxFileSize {
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

		case ".json":
			// Metadata sidecar written by yt-dlp/gallery-dl; not media.
			continue

		default:
			d.l.Warn().Str("file", entry.Name()).Msg("skipping unsupported file type")
		}
	}

	return result, nil
}

// extractCaption reads the post's caption/description from the first JSON
// metadata sidecar written by yt-dlp (--write-info-json) or gallery-dl
// (--write-metadata) in subDir. Returns "" when no sidecar is found or it
// cannot be parsed; all failures are non-fatal.
func (d *Downloader) extractCaption(subDir string) string {
	entries, err := os.ReadDir(subDir)
	if err != nil {
		d.l.Warn().Err(err).Str("dir", subDir).Msg("read dir for caption")
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(subDir, entry.Name()))
		if err != nil {
			d.l.Warn().Err(err).Str("file", entry.Name()).Msg("read caption sidecar")
			continue
		}

		var meta struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			d.l.Warn().Err(err).Str("file", entry.Name()).Msg("parse caption sidecar")
			continue
		}
		if meta.Description != "" {
			return meta.Description
		}
	}

	return ""
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
	safeArgs := make([]string, len(args))
	copy(safeArgs, args)
	for i, a := range safeArgs {
		if a == "--proxy" && i+1 < len(safeArgs) {
			safeArgs[i+1] = "[redacted]"
			break
		}
	}
	d.l.Debug().Msgf("executing %s %s", name, strings.Join(safeArgs, " "))

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
