package downloader

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MediaType identifies whether a downloaded file is a video or a photo.
type MediaType int

const (
	MediaTypeVideo MediaType = iota
	MediaTypePhoto
)

// MediaFile represents a single downloaded and processed media file.
type MediaFile struct {
	Path string
	Type MediaType
}

// compressVideo re-encodes a video to 720p at CRF 28 using ffmpeg,
// writing the result to a temp file in the same directory as inputPath.
func compressVideo(inputPath string) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(inputPath), "compressed-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()

	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-crf", "28",
		"-preset", "fast",
		"-c:a", "aac",
		"-b:a", "128k",
		"-y",
		tmp.Name(),
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}

	return tmp.Name(), nil
}

// convertToMP4 remuxes or re-encodes a video to MP4/H.264 so Telegram plays
// it inline. Returns the path to a temp file in the same directory as inputPath.
func convertToMP4(inputPath string) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(inputPath), "converted-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()

	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-y",
		tmp.Name(),
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}

	return tmp.Name(), nil
}
