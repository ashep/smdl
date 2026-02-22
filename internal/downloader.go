package internal

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetInstagram downloads all media from an Instagram URL into a subdirectory
// of destDir and returns the subdirectory path.
//
// It shells out to yt-dlp which must be installed on the system.
// If the environment variable INSTAGRAM_COOKIES_FILE is set, its value is
// passed to yt-dlp as a Netscape cookies file for authenticated downloads.
func GetInstagram(rawURL, destDir string) (string, error) {
	// Validate the URL and derive a safe directory name from the path.
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	// Build a filesystem-safe name from the URL path (e.g. "/p/ABC123/" -> "ABC123").
	slug := strings.Trim(u.Path, "/")
	slug = strings.ReplaceAll(slug, "/", "_")
	if slug == "" {
		slug = "instagram"
	}

	subDir := filepath.Join(destDir, slug)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	outputTmpl := filepath.Join(subDir, "%(title)s.%(ext)s")

	args := []string{
		"--output", outputTmpl,
		"--no-playlist",
	}

	if cookiesFile := os.Getenv("INSTAGRAM_COOKIES_FILE"); cookiesFile != "" {
		args = append(args, "--cookies", cookiesFile)
	}

	args = append(args, rawURL)

	cmd := exec.Command("yt-dlp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("yt-dlp: %s", msg)
	}

	return subDir, nil
}
