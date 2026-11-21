package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"instsum/internal/config"
	"instsum/internal/core"
)

type runnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

func TestMixedPostUsesCarouselDownloaderAndPreservesAllMedia(t *testing.T) {
	c := config.Defaults()
	c.Fetch.Retries = 0
	c.Fetch.CookiesFile = "test-cookies.txt"
	f := New(c)
	dir := t.TempDir()
	f.Runner = runnerFunc(func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "gallery-dl" {
			t.Fatalf("mixed posts must not use a video-only downloader: %s", name)
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "?") || !strings.Contains(joined, "--cookies test-cookies.txt") {
			t.Fatalf("unexpected downloader arguments: %q", args)
		}
		for _, name := range []string{"slide10.jpg", "slide2.mp4", "slide1.png"} {
			if err := os.WriteFile(filepath.Join(dir, "image-download", name), []byte("media"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return nil, nil
	})
	meta, paths, err := f.Fetch(context.Background(), "https://www.instagram.com/p/ABC/?tracking=removed", dir)
	if err != nil || len(paths) != 3 || len(meta.Media) != 3 {
		t.Fatalf("incomplete carousel: %+v, %v", meta, err)
	}
	if filepath.Base(paths[0]) != "slide1.png" || meta.Media[1].Kind != "video" || meta.Media[2].Slide != 3 {
		t.Fatalf("wrong mixed carousel order: %+v", meta.Media)
	}
}

func TestReelVideoDownloadAndImageFallback(t *testing.T) {
	for _, failVideo := range []bool{false, true} {
		t.Run(fmt.Sprint(failVideo), func(t *testing.T) {
			c := config.Defaults()
			c.Fetch.Retries = 0
			f := New(c)
			dir := t.TempDir()
			var calls []string
			f.Runner = runnerFunc(func(_ context.Context, name string, _ ...string) ([]byte, error) {
				calls = append(calls, name)
				if name == "yt-dlp" {
					if failVideo {
						return nil, fmt.Errorf("video unavailable")
					}
					if err := os.WriteFile(filepath.Join(dir, "video-download", "download.mp4"), []byte("video"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, "video-download", "download.info.json"), []byte(`{"description":"Recipe"}`), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(filepath.Join(dir, "image-download", "slide1.jpg"), []byte("image"), 0600); err != nil {
					t.Fatal(err)
				}
				return nil, nil
			})
			meta, _, err := f.Fetch(context.Background(), "https://instagram.com/reel/ABC/", dir)
			if err != nil || len(meta.Media) != 1 {
				t.Fatalf("download: %+v, %v", meta, err)
			}
			if calls[0] != "yt-dlp" || (failVideo && (len(calls) != 2 || calls[1] != "gallery-dl")) || (!failVideo && len(calls) != 1) {
				t.Fatalf("wrong fallback order: %v", calls)
			}
		})
	}
}

func TestLocalCarouselOrderAndMetadata(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"slide10.jpg", "slide2.jpg"} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte("image"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := core.Metadata{Caption: "reteta #food @cook", Media: []core.Media{{Path: "slide10.jpg"}, {Path: "slide2.jpg"}}}
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "input.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	meta, paths, e := New(config.Defaults()).Fetch(context.Background(), path, dir)
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Base(paths[0]) != "slide10.jpg" || meta.Media[1].Slide != 2 {
		t.Fatalf("manifest order changed: %+v", meta)
	}
}
func TestMetadataAndNaturalOrder(t *testing.T) {
	var m core.Metadata
	e := parseMetadata([]byte(`{"uploader":"cook","description":"Use 50 g #reteta @chef","view_count":12,"track":"song"}`), &m)
	if e != nil || m.Author != "cook" || m.Hashtags[0] != "reteta" || m.Mentions[0] != "chef" || m.Stats["view_count"] != 12 {
		t.Fatalf("metadata %+v, %v", m, e)
	}
	if !naturalLess("slide2.jpg", "slide10.jpg") {
		t.Fatal("numeric slide order lost")
	}
}
