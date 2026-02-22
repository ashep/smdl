package downloader

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// GetYouTube downloads a YouTube Shorts video into a subdirectory of
// dstDir and returns the subdirectory path.
//
// It shells out to yt-dlp which must be installed on the system.
func (d *Downloader) GetYouTube(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/shorts/ABC123" -> "shorts_ABC123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "youtube"
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

	errMsg, err := d.runCmd("yt-dlp", []string{"--output", outputTmpl, rawURL})
	if err != nil {
		downloadErr = fmt.Errorf("%s", errMsg)
		return "", downloadErr
	}

	return subDir, nil
}
