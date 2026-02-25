package downloader

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// GetInstagram downloads all media from an Instagram URL into a subdirectory
// of dstDir and returns the subdirectory path.
func (d *Downloader) GetInstagram(rawURL string) (string, error) {
	// Validate the URL and derive a safe directory name from the path.
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/p/ABC123/" -> "p_ABC123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "instagram"
	}

	subDir := filepath.Join(d.dstDir, slug)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	// Clean up the subdirectory if we return an error, to avoid leaving empty
	// directories behind on failed downloads.
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
		"--no-playlist",
		"--format", "bestvideo[filesize<50M]+bestaudio/best[filesize<50M]/best",
		"--cookies", d.cookiesFilename,
	)

	// First attempt with yt-dlp (handles videos and carousels).
	errMsg, err := d.runCmd("yt-dlp", append(args, rawURL))
	if err != nil && strings.Contains(errMsg, "No video formats found") {
		d.l.Info().Msg("no video found, trying to download gal")

		// yt-dlp cannot handle image-only posts; fall back to gallery-dl.
		// Remove any files yt-dlp may have written before failing so we don't
		// end up with duplicates under different naming schemes.
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
			"--cookies", d.cookiesFilename,
			rawURL,
		)
		errMsg, err = d.runCmd("gallery-dl", gdlArgs)
	}
	if err != nil {
		downloadErr = fmt.Errorf("%s", errMsg)
		return "", downloadErr
	}

	return subDir, nil
}
