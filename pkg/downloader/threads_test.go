package downloader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// threadsPage wraps a post media object into HTML shaped like a real Threads
// post page: the post is buried in a ScheduledServerJS payload inside a
// <script type="application/json"> tag, next to unrelated reply posts.
func threadsPage(t *testing.T, media map[string]any) string {
	t.Helper()

	reply := map[string]any{"code": "REPLY1", "media_type": 19, "caption": map[string]any{"text": "a reply"}}
	payload := map[string]any{
		"require": []any{[]any{"ScheduledServerJS", "handle", nil, []any{map[string]any{
			"__bbox": map[string]any{"result": map[string]any{"data": map[string]any{
				"media":   media,
				"replies": []any{reply},
			}}},
		}}}},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	return `<!DOCTYPE html><html><head><title>Threads</title></head><body>` +
		`<script type="application/json" data-sjs>{"broken":</script>` +
		`<script type="application/json" data-content-len="1" data-sjs>` + string(b) + `</script>` +
		`</body></html>`
}

func imageVersions(urls ...string) map[string]any {
	var cands []any
	for i, u := range urls {
		cands = append(cands, map[string]any{"url": u, "width": 100 * (i + 1), "height": 100 * (i + 1)})
	}
	return map[string]any{"candidates": cands}
}

func TestThreadsPostCode(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.threads.com/@zuck/post/DctzI_Jj0Hv", "DctzI_Jj0Hv"},
		{"https://www.threads.net/@zuck/post/DctzI_Jj0Hv?xmt=abc", "DctzI_Jj0Hv"},
		{"https://threads.com/@zuck/post/DctzI_Jj0Hv/media", "DctzI_Jj0Hv"},
		{"https://www.threads.net/t/Ddt7cL5EfUG/", "Ddt7cL5EfUG"},
		{"https://www.threads.com/@zuck", ""},
		{"https://www.threads.com/", ""},
	}
	for _, tt := range tests {
		got, err := threadsPostCode(tt.url)
		if tt.want == "" {
			if err == nil {
				t.Errorf("threadsPostCode(%q) = %q, want error", tt.url, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("threadsPostCode(%q) error: %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("threadsPostCode(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestParseThreadsPost_Photo(t *testing.T) {
	html := threadsPage(t, map[string]any{
		"code":           "PHOTO1",
		"media_type":     1,
		"caption":        map[string]any{"text": "photo caption"},
		"image_versions2": map[string]any{"candidates": []any{
			map[string]any{"url": "https://cdn/small.jpg", "width": 320, "height": 320},
			map[string]any{"url": "https://cdn/big.jpg", "width": 1440, "height": 1800},
			map[string]any{"url": "https://cdn/mid.jpg", "width": 640, "height": 800},
		}},
		"video_versions": nil,
		"carousel_media": nil,
	})

	post, err := parseThreadsPost(html, "PHOTO1")
	if err != nil {
		t.Fatal(err)
	}
	if post.Caption != "photo caption" {
		t.Errorf("caption = %q", post.Caption)
	}
	want := []threadsMedia{{URL: "https://cdn/big.jpg", Video: false}}
	if fmt.Sprint(post.Media) != fmt.Sprint(want) {
		t.Errorf("media = %v, want %v", post.Media, want)
	}
}

func TestParseThreadsPost_Video(t *testing.T) {
	html := threadsPage(t, map[string]any{
		"code":            "VIDEO1",
		"media_type":      2,
		"caption":         map[string]any{"text": "video caption"},
		"image_versions2": imageVersions("https://cdn/thumb.jpg"),
		"video_versions": []any{
			map[string]any{"type": 101, "url": "https://cdn/v1.mp4"},
			map[string]any{"type": 102, "url": "https://cdn/v2.mp4"},
		},
	})

	post, err := parseThreadsPost(html, "VIDEO1")
	if err != nil {
		t.Fatal(err)
	}
	want := []threadsMedia{{URL: "https://cdn/v1.mp4", Video: true}}
	if fmt.Sprint(post.Media) != fmt.Sprint(want) {
		t.Errorf("media = %v, want %v", post.Media, want)
	}
}

func TestParseThreadsPost_Carousel(t *testing.T) {
	html := threadsPage(t, map[string]any{
		"code":            "CAROUSEL1",
		"media_type":      8,
		"caption":         nil,
		"image_versions2": imageVersions("https://cdn/cover.jpg"),
		"carousel_media": []any{
			map[string]any{"media_type": 1, "image_versions2": imageVersions("https://cdn/a-small.jpg", "https://cdn/a.jpg"), "video_versions": nil},
			map[string]any{"media_type": 2, "image_versions2": imageVersions("https://cdn/b.jpg"), "video_versions": []any{map[string]any{"url": "https://cdn/b.mp4"}}},
		},
	})

	post, err := parseThreadsPost(html, "CAROUSEL1")
	if err != nil {
		t.Fatal(err)
	}
	if post.Caption != "" {
		t.Errorf("caption = %q, want empty", post.Caption)
	}
	want := []threadsMedia{{URL: "https://cdn/a.jpg", Video: false}, {URL: "https://cdn/b.mp4", Video: true}}
	if fmt.Sprint(post.Media) != fmt.Sprint(want) {
		t.Errorf("media = %v, want %v", post.Media, want)
	}
}

func TestParseThreadsPost_TextOnly(t *testing.T) {
	html := threadsPage(t, map[string]any{
		"code":            "TEXT1",
		"media_type":      19,
		"caption":         map[string]any{"text": "just words"},
		"image_versions2": map[string]any{"candidates": []any{}},
	})

	post, err := parseThreadsPost(html, "TEXT1")
	if err != nil {
		t.Fatal(err)
	}
	if len(post.Media) != 0 {
		t.Errorf("media = %v, want none", post.Media)
	}
	if post.Caption != "just words" {
		t.Errorf("caption = %q", post.Caption)
	}
}

func TestParseThreadsPost_NotFound(t *testing.T) {
	html := threadsPage(t, map[string]any{"code": "OTHER", "media_type": 1})
	if _, err := parseThreadsPost(html, "MISSING"); err == nil {
		t.Fatal("expected error for missing post")
	}
}

func TestGetThreads_DownloadsMediaAndCaption(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@user/post/POST1":
			if !strings.Contains(r.UserAgent(), "Googlebot") {
				http.Error(w, "client-rendered page", http.StatusOK)
				return
			}
			fmt.Fprint(w, threadsPage(t, map[string]any{
				"code":       "POST1",
				"media_type": 8,
				"caption":    map[string]any{"text": "hello threads"},
				"carousel_media": []any{
					map[string]any{"media_type": 1, "image_versions2": imageVersions(srv.URL + "/img.webp")},
					map[string]any{"media_type": 2, "video_versions": []any{map[string]any{"url": srv.URL + "/vid"}}},
				},
			}))
		case "/img.webp":
			w.Header().Set("Content-Type", "image/webp")
			fmt.Fprint(w, "IMG")
		case "/vid":
			w.Header().Set("Content-Type", "video/mp4")
			fmt.Fprint(w, "VID")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := &Downloader{dstDir: t.TempDir(), l: zerolog.Nop()}
	subDir, err := d.getThreads(srv.URL + "/@user/post/POST1")
	if err != nil {
		t.Fatal(err)
	}

	files, err := d.processDir(subDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(files), files)
	}
	if files[0].Type != MediaTypePhoto || filepath.Ext(files[0].Path) != ".webp" {
		t.Errorf("file 0 = %+v, want .webp photo", files[0])
	}
	if files[1].Type != MediaTypeVideo || filepath.Ext(files[1].Path) != ".mp4" {
		t.Errorf("file 1 = %+v, want .mp4 video", files[1])
	}
	if b, _ := os.ReadFile(files[1].Path); string(b) != "VID" {
		t.Errorf("video content = %q", b)
	}

	if got := d.extractCaption(subDir); got != "hello threads" {
		t.Errorf("caption = %q, want %q", got, "hello threads")
	}
}

func TestGetThreads_NoMediaCleansUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, threadsPage(t, map[string]any{"code": "TEXT1", "media_type": 19, "caption": map[string]any{"text": "words"}}))
	}))
	defer srv.Close()

	d := &Downloader{dstDir: t.TempDir(), l: zerolog.Nop()}
	if _, err := d.getThreads(srv.URL + "/@user/post/TEXT1"); err == nil {
		t.Fatal("expected error for post without media")
	}
	entries, _ := os.ReadDir(d.dstDir)
	if len(entries) != 0 {
		t.Errorf("expected dstDir to be empty after failed download, got %d entries", len(entries))
	}
}

func TestIsURLEligible_Threads(t *testing.T) {
	d := &Downloader{}
	for _, u := range []string{
		"https://www.threads.com/@zuck/post/DctzI_Jj0Hv",
		"https://www.threads.net/@zuck/post/DctzI_Jj0Hv",
		"https://threads.net/t/DctzI_Jj0Hv",
	} {
		if !d.IsURLEligible(u) {
			t.Errorf("IsURLEligible(%q) = false, want true", u)
		}
	}
	if d.IsURLEligible("https://notthreads.com/@a/post/X") {
		t.Error("IsURLEligible(notthreads.com) = true, want false")
	}
}
