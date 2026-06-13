package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestExtractCaption_YtDlpInfoJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{"title":"t","description":"hello world"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "hello world" {
		t.Errorf("extractCaption = %q, want %q", got, "hello world")
	}
}

func TestExtractCaption_GalleryDLJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01_abc.jpg.json"),
		[]byte(`{"description":"an image caption"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "an image caption" {
		t.Errorf("extractCaption = %q, want %q", got, "an image caption")
	}
}

func TestExtractCaption_NoSidecar(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}

func TestExtractCaption_EmptyDescription(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{"description":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}

func TestExtractCaption_MalformedJSON(t *testing.T) {
	d := &Downloader{l: zerolog.Nop()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.info.json"),
		[]byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.extractCaption(dir); got != "" {
		t.Errorf("extractCaption = %q, want empty", got)
	}
}
