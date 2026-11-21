package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/tool"
)

type Fetcher struct {
	Config config.Config
	Runner tool.Runner
}

func New(c config.Config) *Fetcher { return &Fetcher{Config: c, Runner: tool.Exec{}} }
func Kind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png":
		return "image"
	case ".mp4", ".mkv", ".webm", ".mov", ".avi":
		return "video"
	}
	return ""
}
func (f *Fetcher) Fetch(ctx context.Context, input, dir string) (core.Metadata, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(f.Config.Fetch.Timeout)*time.Second)
	defer cancel()
	meta := core.Metadata{URL: input}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		if strings.EqualFold(filepath.Ext(input), ".json") {
			return localManifest(input)
		}
		if _, err := os.Stat(input); err != nil {
			return meta, nil, fmt.Errorf("read local input: %w", err)
		}
		if Kind(input) == "" {
			return meta, nil, fmt.Errorf("unsupported local media; use JPEG/PNG/video or a JSON media manifest")
		}
		meta.Media = []core.Media{{ID: "media-1", Path: input, Kind: Kind(input)}}
		setID(&meta, input)
		return meta, []string{input}, nil
	}
	host := strings.ToLower(u.Hostname())
	if host != "instagram.com" && host != "www.instagram.com" && host != "instagr.am" && host != "www.instagr.am" {
		return meta, nil, fmt.Errorf("only Instagram media URLs are accepted")
	}
	if u.User != nil {
		return meta, nil, fmt.Errorf("Instagram URLs must not contain credentials")
	}
	u.RawQuery = ""
	u.Fragment = ""
	meta.URL = u.String()
	setID(&meta, meta.URL)
	videoDir := filepath.Join(dir, "video-download")
	imageDir := filepath.Join(dir, "image-download")
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return meta, nil, err
	}
	var args []string
	var videoErr error
	// A video-only download cannot establish completeness for a mixed carousel.
	if !strings.HasPrefix(u.Path, "/p/") {
		args = []string{"--no-playlist", "--write-info-json", "--write-thumbnail", "--no-progress", "--merge-output-format", "mp4", "--remux-video", "mp4", "-o", filepath.Join(videoDir, "download.%(ext)s")}
		args = f.cookies(args, false)
		_, videoErr = f.retry(ctx, "yt-dlp", append(args, "--", meta.URL)...)
		if videoErr == nil {
			files, err := mediaFiles(videoDir)
			if err != nil {
				return meta, nil, err
			}
			var videos []string
			for _, p := range files {
				if Kind(p) == "video" {
					videos = append(videos, p)
				}
			}
			if len(videos) > 0 {
				b, err := os.ReadFile(filepath.Join(videoDir, "download.info.json"))
				if err != nil {
					return meta, nil, err
				}
				if err = parseMetadata(b, &meta); err != nil {
					return meta, nil, err
				}
				return finish(meta, videos)
			}
			videoErr = fmt.Errorf("yt-dlp returned no video")
		}
	}
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return meta, nil, err
	}
	args = f.cookies([]string{"--write-metadata", "-D", imageDir}, true)
	_, galleryErr := f.retry(ctx, "gallery-dl", append(args, "--", meta.URL)...)
	if galleryErr != nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 || (parts[0] != "p" && parts[0] != "reel") {
			return meta, nil, fmt.Errorf("video fetch: %v; gallery fetch: %w", videoErr, galleryErr)
		}
		// instaloader's shortcode target is an argument, not a private API call.
		_, fallbackErr := f.retry(ctx, "instaloader", "--no-profile-pic", "--no-compress-json", "--dirname-pattern", imageDir, "--", "-"+parts[1])
		if fallbackErr != nil {
			if videoErr == nil {
				return meta, nil, fmt.Errorf("gallery fetch: %v; instaloader fallback: %w; configure cookies for authenticated posts", galleryErr, fallbackErr)
			}
			return meta, nil, fmt.Errorf("video fetch: %v; gallery fetch: %v; instaloader fallback: %w; configure cookies for authenticated posts", videoErr, galleryErr, fallbackErr)
		}
	}
	files, err := mediaFiles(imageDir)
	if err != nil {
		return meta, nil, err
	}
	if len(files) == 0 {
		return meta, nil, fmt.Errorf("fetch produced no supported media")
	}
	metadataFiles, err := filepath.Glob(filepath.Join(imageDir, "*.json"))
	if err != nil {
		return meta, nil, err
	}
	for _, p := range metadataFiles {
		b, e := os.ReadFile(p)
		if e != nil {
			return meta, nil, e
		}
		if e = parseMetadata(b, &meta); e != nil {
			return meta, nil, fmt.Errorf("%s metadata: %w", filepath.Base(p), e)
		}
	}
	return finish(meta, files)
}
func (f *Fetcher) cookies(args []string, gallery bool) []string {
	if f.Config.Fetch.CookiesFile != "" {
		args = append(args, "--cookies", f.Config.Fetch.CookiesFile)
	}
	if f.Config.Fetch.CookiesFromBrowser != "" {
		flag := "--cookies-from-browser"
		if gallery {
			flag = "--cookies-from-browser"
		}
		args = append(args, flag, f.Config.Fetch.CookiesFromBrowser)
	}
	return args
}
func (f *Fetcher) retry(ctx context.Context, name string, args ...string) ([]byte, error) {
	var b []byte
	var err error
	for i := 0; i <= f.Config.Fetch.Retries; i++ {
		if i > 0 {
			timer := time.NewTimer(time.Minute / time.Duration(f.Config.Fetch.RateLimit))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		b, err = f.Runner.Run(ctx, name, args...)
		if err == nil {
			return b, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return b, err
}
func setID(meta *core.Metadata, value string) {
	s := sha256.Sum256([]byte(value))
	meta.ID = hex.EncodeToString(s[:8])
}
func finish(meta core.Metadata, paths []string) (core.Metadata, []string, error) {
	meta.Media = nil
	for i, p := range paths {
		slide := 0
		if len(paths) > 1 {
			slide = i + 1
		}
		meta.Media = append(meta.Media, core.Media{ID: fmt.Sprintf("media-%d", i+1), Path: p, Kind: Kind(p), Slide: slide})
	}
	return meta, paths, nil
}
func localManifest(path string) (core.Metadata, []string, error) {
	var meta core.Metadata
	b, err := os.ReadFile(path)
	if err != nil {
		return meta, nil, err
	}
	if err = json.Unmarshal(b, &meta); err != nil {
		return meta, nil, fmt.Errorf("media manifest: %w", err)
	}
	if len(meta.Media) == 0 {
		return meta, nil, fmt.Errorf("media manifest requires a nonempty media list")
	}
	var paths []string
	for i := range meta.Media {
		m := &meta.Media[i]
		if !filepath.IsAbs(m.Path) {
			m.Path = filepath.Join(filepath.Dir(path), m.Path)
		}
		if _, err := os.Stat(m.Path); err != nil {
			return meta, nil, err
		}
		m.Kind = Kind(m.Path)
		if m.Kind == "" {
			return meta, nil, fmt.Errorf("unsupported manifest media %q", m.Path)
		}
		m.ID = fmt.Sprintf("media-%d", i+1)
		if len(meta.Media) > 1 {
			m.Slide = i + 1
		}
		paths = append(paths, m.Path)
	}
	if meta.URL == "" {
		meta.URL = path
	}
	setID(&meta, meta.URL)
	return meta, paths, nil
}

var number = regexp.MustCompile(`\d+`)

func mediaFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && Kind(p) != "" {
			paths = append(paths, p)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool { return naturalLess(paths[i], paths[j]) })
	return paths, err
}
func naturalLess(a, b string) bool {
	aa, bb := number.FindAllString(a, -1), number.FindAllString(b, -1)
	for i := 0; i < len(aa) && i < len(bb); i++ {
		x, _ := strconv.ParseUint(aa[i], 10, 64)
		y, _ := strconv.ParseUint(bb[i], 10, 64)
		if x != y {
			return x < y
		}
	}
	return a < b
}

var tag = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_]+)`)
var mention = regexp.MustCompile(`(?:^|\s)@([\p{L}\p{N}_.]+)`)

func parseMetadata(b []byte, meta *core.Metadata) error {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if node, ok := v["node"].(map[string]any); ok {
		v = node
	}
	for _, field := range []struct {
		key string
		dst *string
	}{{"uploader", &meta.Author}, {"username", &meta.Author}, {"upload_date", &meta.Date}, {"date", &meta.Date}, {"description", &meta.Caption}, {"caption", &meta.Caption}, {"title", &meta.Title}, {"track", &meta.Music}} {
		if s, ok := v[field.key].(string); ok && s != "" {
			*field.dst = s
		}
	}
	if user, ok := v["owner"].(map[string]any); ok {
		if s, ok := user["username"].(string); ok {
			meta.Author = s
		}
	}
	if user, ok := v["user"].(map[string]any); ok {
		if s, ok := user["username"].(string); ok {
			meta.Author = s
		}
	}
	if caption, ok := v["edge_media_to_caption"].(map[string]any); ok {
		if edges, ok := caption["edges"].([]any); ok && len(edges) > 0 {
			if first, ok := edges[0].(map[string]any); ok {
				if node, ok := first["node"].(map[string]any); ok {
					if text, ok := node["text"].(string); ok {
						meta.Caption = text
					}
				}
			}
		}
	}
	if stamp, ok := v["taken_at_timestamp"].(float64); ok {
		meta.Date = time.Unix(int64(stamp), 0).UTC().Format(time.RFC3339)
	}
	if meta.Stats == nil {
		meta.Stats = map[string]int64{}
	}
	for _, k := range []string{"view_count", "like_count", "comment_count"} {
		if x, ok := v[k].(float64); ok {
			meta.Stats[k] = int64(x)
		}
	}
	for field, dest := range map[string]string{"edge_media_preview_like": "like_count", "edge_liked_by": "like_count", "edge_media_to_comment": "comment_count"} {
		if count, ok := v[field].(map[string]any); ok {
			if n, ok := count["count"].(float64); ok {
				meta.Stats[dest] = int64(n)
			}
		}
	}
	meta.Hashtags = nil
	meta.Mentions = nil
	for _, m := range tag.FindAllStringSubmatch(meta.Caption, -1) {
		meta.Hashtags = append(meta.Hashtags, m[1])
	}
	for _, m := range mention.FindAllStringSubmatch(meta.Caption, -1) {
		meta.Mentions = append(meta.Mentions, m[1])
	}
	return nil
}
