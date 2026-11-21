package media

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/fetch"
	"instsum/internal/tool"
)

type Tools struct {
	Config config.Config
	Runner tool.Runner
}

func (t Tools) runner() tool.Runner {
	if t.Runner != nil {
		return t.Runner
	}
	return tool.Exec{}
}
func (t Tools) Fetch(ctx context.Context, input, dir string) (core.Metadata, []string, error) {
	f := fetch.New(t.Config)
	f.Runner = t.runner()
	return f.Fetch(ctx, input, dir)
}
func (t Tools) Probe(ctx context.Context, m core.Media, dir string) (core.Media, error) {
	if m.Kind == "image" {
		f, err := os.Open(m.Path)
		if err != nil {
			return m, err
		}
		defer f.Close()
		im, _, err := image.DecodeConfig(f)
		m.Width = im.Width
		m.Height = im.Height
		return m, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(t.Config.Frames.Timeout)*time.Second)
	defer cancel()
	b, err := t.runner().Run(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", m.Path)
	if err != nil {
		return m, err
	}
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			FPS    string `json:"avg_frame_rate"`
		} `json:"streams"`
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return m, fmt.Errorf("ffprobe JSON: %w", err)
	}
	m.Duration, err = strconv.ParseFloat(p.Format.Duration, 64)
	if err != nil || m.Duration <= 0 {
		return m, fmt.Errorf("ffprobe returned no positive video duration")
	}
	for _, s := range p.Streams {
		if s.Type == "audio" {
			m.HasAudio = true
		}
		if s.Type == "video" {
			m.Width = s.Width
			m.Height = s.Height
			parts := strings.Split(s.FPS, "/")
			if len(parts) == 2 {
				x, e := strconv.ParseFloat(parts[0], 64)
				y, f := strconv.ParseFloat(parts[1], 64)
				if e == nil && f == nil && y != 0 {
					m.FPS = x / y
				}
			}
		}
	}
	if m.Width <= 0 || m.Height <= 0 {
		return m, fmt.Errorf("input has no video stream")
	}
	if m.HasAudio {
		m.AudioPath = filepath.Join(dir, "audio.wav")
		_, err = t.runner().Run(ctx, "ffmpeg", "-v", "error", "-nostdin", "-i", m.Path, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-n", m.AudioPath)
		if err != nil {
			return m, fmt.Errorf("normalize audio: %w", err)
		}
	}
	return m, nil
}

var pts = regexp.MustCompile(`pts_time:([0-9.eE+-]+)`)

func (t Tools) Extract(ctx context.Context, path, dir string) ([]core.Frame, float64, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, 0, err
	}
	m := core.Media{Path: path, Kind: fetch.Kind(path)}
	if m.Kind == "image" {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		img, _, err := image.Decode(f)
		closeErr := f.Close()
		if err != nil {
			return nil, 0, err
		}
		if closeErr != nil {
			return nil, 0, closeErr
		}
		img = resize(img, t.Config.Frames.Size)
		outPath := filepath.Join(dir, "frame_000001.jpg")
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return nil, 0, err
		}
		err = jpeg.Encode(out, img, &jpeg.Options{Quality: 90})
		closeErr = out.Close()
		if err != nil {
			return nil, 0, err
		}
		if closeErr != nil {
			return nil, 0, closeErr
		}
		return []core.Frame{{Path: outPath, Time: 0, Sharpness: Sharpness(img)}}, 0, nil
	}
	// Probe here without extracting audio; normalization is a separate pipeline stage.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(t.Config.Frames.Timeout)*time.Second)
	defer cancel()
	b, err := t.runner().Run(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", path)
	if err != nil {
		return nil, 0, err
	}
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return nil, 0, err
	}
	duration, err := strconv.ParseFloat(p.Format.Duration, 64)
	if err != nil || duration <= 0 {
		return nil, 0, fmt.Errorf("ffprobe returned no positive video duration")
	}
	filter := fmt.Sprintf("select='isnan(prev_selected_t)+gte(t-prev_selected_t,%g)+gt(scene,%g)',scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease,showinfo", t.Config.Frames.Interval, t.Config.Frames.SceneThreshold, t.Config.Frames.Size, t.Config.Frames.Size)
	b, err = t.runner().Run(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-i", path, "-vf", filter, "-fps_mode", "vfr", "-q:v", "3", "-n", filepath.Join(dir, "frame_%06d.jpg"))
	if err != nil {
		return nil, 0, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	if err != nil {
		return nil, 0, err
	}
	matches := pts.FindAllSubmatch(b, -1)
	if len(files) == 0 || len(files) != len(matches) {
		return nil, 0, fmt.Errorf("ffmpeg frame/timestamp mismatch: %d images, %d timestamps", len(files), len(matches))
	}
	frames := make([]core.Frame, 0, len(files))
	for i, path := range files {
		x, e := strconv.ParseFloat(string(matches[i][1]), 64)
		if e != nil {
			return nil, 0, e
		}
		im, e := readImage(path)
		if e != nil {
			return nil, 0, e
		}
		frames = append(frames, core.Frame{Path: path, Time: x, Sharpness: Sharpness(im)})
	}
	return frames, duration, nil
}
func readImage(path string) (image.Image, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	im, _, e := image.Decode(f)
	return im, e
}
func resize(im image.Image, max int) image.Image {
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return im
	}
	nw, nh := max, h*max/w
	if h > w {
		nh = max
		nw = w * max / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			out.Set(x, y, im.At(b.Min.X+x*w/nw, b.Min.Y+y*h/nh))
		}
	}
	return out
}
