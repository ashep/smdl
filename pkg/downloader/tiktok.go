package downloader

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// getTikTok downloads a TikTok video or photo post into a subdirectory of
// dstDir and returns the subdirectory path. Videos are handled by yt-dlp;
// photo slideshows (URLs containing "/photo/") fall back to gallery-dl because
// yt-dlp does not support them.
func (d *Downloader) getTikTok(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/@user/video/123" -> "@user_video_123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "tiktok"
	}

	subDir := filepath.Join(d.dstDir, slug)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	var downloadErr error
	defer func() {
		if downloadErr != nil {
			os.RemoveAll(subDir)
		}
	}()

	outputTmpl := filepath.Join(subDir, "%(title)s.%(ext)s")

	args := d.proxyArgs()
	args = append(args,
		"--output", outputTmpl,
		"--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]/best",
		"--write-info-json",
		rawURL,
	)

	errMsg, err := d.runCmd("yt-dlp", args)
	// Photo slideshows (including short URLs that resolve to /photo/) are not
	// supported by yt-dlp; fall back to gallery-dl on "Unsupported URL".
	if err != nil && strings.Contains(errMsg, "Unsupported URL") {
		d.l.Info().Msg("unsupported by yt-dlp, trying gallery-dl")

		if entries, _ := os.ReadDir(subDir); len(entries) > 0 {
			if err := os.RemoveAll(subDir); err != nil {
				downloadErr = fmt.Errorf("clear subdir: %w", err)
				return "", downloadErr
			}
			if err := os.MkdirAll(subDir, 0o755); err != nil {
				downloadErr = fmt.Errorf("recreate subdir: %w", err)
				return "", downloadErr
			}
		}

		gdlArgs := d.proxyArgs()
		gdlArgs = append(gdlArgs,
			"-D", subDir,
			"--filename", "{num:>02}_{post_id}.{extension}",
			"--write-metadata",
			rawURL,
		)
		errMsg, err = d.runCmd("gallery-dl", gdlArgs)
	}
	if err != nil {
		downloadErr = fmt.Errorf("%s: %w", errMsg, err)
		return "", downloadErr
	}

	return subDir, nil
}
