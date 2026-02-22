package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// jsonCookie matches the JSON format exported by browser cookie extensions.
type jsonCookie struct {
	Domain         string  `json:"domain"`
	ExpirationDate float64 `json:"expirationDate"`
	HostOnly       bool    `json:"hostOnly"`
	HTTPOnly       bool    `json:"httpOnly"`
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	Session        bool    `json:"session"`
	Value          string  `json:"value"`
}

// jsonCookiesToNetscape converts a JSON cookie list to a Netscape cookies file
// written to a temporary file, returning its path. The caller must delete it.
func jsonCookiesToNetscape(jsonPath string) (string, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return "", fmt.Errorf("read cookies file: %w", err)
	}

	var cookies []jsonCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", fmt.Errorf("parse cookies JSON: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range cookies {
		includeSubdomains := "FALSE"
		if strings.HasPrefix(c.Domain, ".") {
			includeSubdomains = "TRUE"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		expiry := int64(0)
		if !c.Session {
			expiry = int64(c.ExpirationDate)
		}
		fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			c.Domain, includeSubdomains, c.Path, secure, expiry, c.Name, c.Value)
	}

	tmp, err := os.CreateTemp("", "smdl-cookies-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp cookies file: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp cookies file: %w", err)
	}
	tmp.Close()
	return tmp.Name(), nil
}

// GetInstagram downloads all media from an Instagram URL into a subdirectory
// of destDir and returns the subdirectory path.
//
// It shells out to yt-dlp which must be installed on the system.
// If the environment variable INSTAGRAM_COOKIES_FILE is set, it must point to
// a JSON cookies file (as exported by browser extensions). The cookies are
// converted to Netscape format and passed to yt-dlp for authenticated downloads.
func GetInstagram(rawURL, destDir string) (string, error) {
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

	subDir := filepath.Join(destDir, slug)
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

	args := []string{
		"--output", outputTmpl,
		"--no-playlist",
	}

	if jsonPath := os.Getenv("INSTAGRAM_COOKIES_FILE"); jsonPath != "" {
		netscapePath, err := jsonCookiesToNetscape(jsonPath)
		if err != nil {
			downloadErr = fmt.Errorf("INSTAGRAM_COOKIES_FILE: %w", err)
			return "", downloadErr
		}
		defer os.Remove(netscapePath)
		args = append(args, "--cookies", netscapePath)
	}

	// First attempt: default format selection (handles videos and carousels).
	errMsg, err := runYtDlp(append(args, rawURL))
	if err != nil && strings.Contains(errMsg, "No video formats found") {
		// Image-only posts have no video formats. Retry downloading the image
		// as a thumbnail (for Instagram image posts this is the full-resolution image).
		imageArgs := append(args, "--write-thumbnail", "--skip-download", "--convert-thumbnails", "jpg", rawURL)
		errMsg, err = runYtDlp(imageArgs)
	}
	if err != nil {
		downloadErr = fmt.Errorf("yt-dlp: %s", errMsg)
		return "", downloadErr
	}

	return subDir, nil
}

// runYtDlp executes yt-dlp with the given arguments and returns (stderr, error).
func runYtDlp(args []string) (string, error) {
	cmd := exec.Command("yt-dlp", args...)
	// Stdout is intentionally discarded; yt-dlp writes downloaded files to disk.
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
