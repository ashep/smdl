package downloader

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// threadsUserAgent makes Threads serve the server-rendered post page with the
// post data embedded as JSON; with a regular browser UA the page is an empty
// client-rendered shell.
const threadsUserAgent = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

var threadsJSONScriptRe = regexp.MustCompile(`(?s)<script type="application/json"[^>]*>(.*?)</script>`)

// threadsMedia is a single downloadable item of a Threads post.
type threadsMedia struct {
	URL   string
	Video bool
}

// threadsPost is the downloadable content of a Threads post.
type threadsPost struct {
	Caption string
	Media   []threadsMedia
}

type threadsImageCandidate struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type threadsMediaItem struct {
	ImageVersions2 *struct {
		Candidates []threadsImageCandidate `json:"candidates"`
	} `json:"image_versions2"`
	VideoVersions []struct {
		URL string `json:"url"`
	} `json:"video_versions"`
}

type threadsMediaObject struct {
	threadsMediaItem
	Caption *struct {
		Text string `json:"text"`
	} `json:"caption"`
	CarouselMedia []threadsMediaItem `json:"carousel_media"`
}

// getThreads downloads images and videos from a public Threads post into a
// subdirectory of dstDir and returns the subdirectory path. Neither yt-dlp nor
// gallery-dl support Threads, so the post page is fetched and parsed directly.
func (d *Downloader) getThreads(rawURL string) (string, error) {
	code, err := threadsPostCode(rawURL)
	if err != nil {
		return "", err
	}

	client, err := d.httpClient()
	if err != nil {
		return "", err
	}

	page, err := httpGet(client, rawURL, threadsUserAgent)
	if err != nil {
		return "", fmt.Errorf("fetch threads page: %w", err)
	}

	post, err := parseThreadsPost(string(page), code)
	if err != nil {
		return "", err
	}
	if len(post.Media) == 0 {
		return "", fmt.Errorf("threads post %s has no media", code)
	}

	subDir := filepath.Join(d.dstDir, "threads_"+code)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	var downloadErr error
	defer func() {
		if downloadErr != nil {
			os.RemoveAll(subDir)
		}
	}()

	for i, m := range post.Media {
		name := filepath.Join(subDir, fmt.Sprintf("%02d_%s", i+1, code))
		if downloadErr = downloadThreadsMedia(client, m, name); downloadErr != nil {
			return "", downloadErr
		}
	}

	// Write the caption in the same sidecar format yt-dlp/gallery-dl use, so
	// extractCaption picks it up.
	meta, err := json.Marshal(map[string]string{"description": post.Caption})
	if err != nil {
		downloadErr = fmt.Errorf("marshal metadata: %w", err)
		return "", downloadErr
	}
	if err := os.WriteFile(filepath.Join(subDir, code+".info.json"), meta, 0o644); err != nil {
		downloadErr = fmt.Errorf("write metadata: %w", err)
		return "", downloadErr
	}

	return subDir, nil
}

// threadsPostCode extracts the post shortcode from a Threads URL such as
// https://www.threads.com/@user/post/CODE or https://www.threads.net/t/CODE.
func threadsPostCode(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid URL: %q", rawURL)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if (parts[i] == "post" || parts[i] == "t") && parts[i+1] != "" {
			return parts[i+1], nil
		}
	}

	return "", fmt.Errorf("not a threads post URL: %q", rawURL)
}

// parseThreadsPost finds the post with the given shortcode in the JSON
// payloads embedded in a Threads post page and returns its caption and media.
func parseThreadsPost(html, code string) (*threadsPost, error) {
	for _, m := range threadsJSONScriptRe.FindAllStringSubmatch(html, -1) {
		var payload any
		if err := json.Unmarshal([]byte(m[1]), &payload); err != nil {
			continue
		}

		raw := findThreadsMediaObject(payload, code)
		if raw == nil {
			continue
		}

		b, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("re-encode threads post: %w", err)
		}
		var obj threadsMediaObject
		if err := json.Unmarshal(b, &obj); err != nil {
			return nil, fmt.Errorf("decode threads post: %w", err)
		}

		post := &threadsPost{}
		if obj.Caption != nil {
			post.Caption = obj.Caption.Text
		}

		items := obj.CarouselMedia
		if len(items) == 0 {
			items = []threadsMediaItem{obj.threadsMediaItem}
		}
		for _, it := range items {
			if m, ok := it.media(); ok {
				post.Media = append(post.Media, m)
			}
		}

		return post, nil
	}

	return nil, fmt.Errorf("threads post %s not found in page", code)
}

// findThreadsMediaObject walks a decoded JSON value and returns the first
// object that describes the post with the given shortcode.
func findThreadsMediaObject(v any, code string) map[string]any {
	switch o := v.(type) {
	case map[string]any:
		if c, _ := o["code"].(string); c == code {
			if _, ok := o["media_type"]; ok {
				return o
			}
		}
		for _, child := range o {
			if found := findThreadsMediaObject(child, code); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range o {
			if found := findThreadsMediaObject(child, code); found != nil {
				return found
			}
		}
	}
	return nil
}

// media returns the item's video if it has one, otherwise its largest image.
func (it threadsMediaItem) media() (threadsMedia, bool) {
	for _, v := range it.VideoVersions {
		if v.URL != "" {
			return threadsMedia{URL: v.URL, Video: true}, true
		}
	}

	if it.ImageVersions2 == nil {
		return threadsMedia{}, false
	}
	var best threadsImageCandidate
	for _, c := range it.ImageVersions2.Candidates {
		if c.URL != "" && (best.URL == "" || c.Width*c.Height > best.Width*best.Height) {
			best = c
		}
	}
	if best.URL == "" {
		return threadsMedia{}, false
	}

	return threadsMedia{URL: best.URL}, true
}

// downloadThreadsMedia saves a media item to name plus an extension derived
// from the response content type.
func downloadThreadsMedia(client *http.Client, m threadsMedia, name string) error {
	resp, err := client.Get(m.URL)
	if err != nil {
		return fmt.Errorf("download threads media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download threads media: unexpected status %s", resp.Status)
	}

	f, err := os.Create(name + threadsMediaExt(resp.Header.Get("Content-Type"), m))
	if err != nil {
		return fmt.Errorf("create media file: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write media file: %w", err)
	}

	return f.Close()
}

func threadsMediaExt(contentType string, m threadsMedia) string {
	switch strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]) {
	case "video/mp4":
		return ".mp4"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	}

	if u, err := url.Parse(m.URL); err == nil {
		if ext := strings.ToLower(path.Ext(u.Path)); ext != "" {
			return ext
		}
	}
	if m.Video {
		return ".mp4"
	}
	return ".jpg"
}

// httpClient returns an HTTP client that honours the configured proxy.
func (d *Downloader) httpClient() (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if d.proxyURL != "" {
		pu, err := url.Parse(d.proxyURL)
		if err != nil {
			return nil, errors.New("invalid proxy URL")
		}
		transport.Proxy = http.ProxyURL(pu)
	}

	return &http.Client{Transport: transport, Timeout: 5 * time.Minute}, nil
}

func httpGet(client *http.Client, rawURL, userAgent string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}
