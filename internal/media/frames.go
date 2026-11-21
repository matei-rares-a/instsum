package media

import (
	"fmt"
	"image"
	"sort"

	"github.com/corona10/goimagehash"
	"instsum/internal/core"
)

func gray(im image.Image, x, y int) float64 {
	r, g, b, _ := im.At(x, y).RGBA()
	return (.299*float64(r) + .587*float64(g) + .114*float64(b)) / 257
}
func Sharpness(im image.Image) float64 {
	b := im.Bounds()
	if b.Dx() < 3 || b.Dy() < 3 {
		return 0
	}
	sum, squared, n := 0.0, 0.0, 0.0
	for y := b.Min.Y + 1; y < b.Max.Y-1; y++ {
		for x := b.Min.X + 1; x < b.Max.X-1; x++ {
			v := 4*gray(im, x, y) - gray(im, x-1, y) - gray(im, x+1, y) - gray(im, x, y-1) - gray(im, x, y+1)
			sum += v
			squared += v * v
			n++
		}
	}
	return squared/n - (sum/n)*(sum/n)
}
func Dedup(frames []core.Frame, threshold int, maxGap float64) ([]core.Frame, error) {
	var out []core.Frame
	var last *goimagehash.ImageHash
	for _, frame := range frames {
		im, e := readImage(frame.Path)
		if e != nil {
			return nil, e
		}
		hash, e := goimagehash.PerceptionHash(im)
		if e != nil {
			return nil, e
		}
		if last != nil && len(out) > 0 {
			d, e := last.Distance(hash)
			if e != nil {
				return nil, e
			}
			prev := out[len(out)-1]
			if d <= threshold && frame.Time-prev.Time < maxGap && frame.MediaID == prev.MediaID && frame.Slide == prev.Slide {
				continue
			}
		}
		out = append(out, frame)
		last = hash
	}
	return out, nil
}
func Select(frames []core.Frame, start, end float64, limit int) []core.Frame {
	if limit < 1 {
		return nil
	}
	var in []core.Frame
	for _, f := range frames {
		if f.Time >= start && (f.Time < end || end == start) {
			in = append(in, f)
		}
	}
	if len(in) <= limit {
		return in
	}
	out := make([]core.Frame, 0, limit)
	for i := 0; i < limit; i++ {
		a, b := i*len(in)/limit, (i+1)*len(in)/limit
		best := in[a]
		for _, f := range in[a:b] {
			if f.Sharpness > best.Sharpness {
				best = f
			}
		}
		out = append(out, best)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}
func Best(frames []core.Frame, mediaID string, slide int, start, end float64) (core.Frame, error) {
	var best core.Frame
	found := false
	for _, f := range frames {
		if f.MediaID != mediaID || f.Slide != slide || f.Time < start || (end > start && f.Time > end) {
			continue
		}
		if !found || f.Sharpness > best.Sharpness {
			best = f
			found = true
		}
	}
	if !found {
		return best, fmt.Errorf("no reference frame for %s slide %d interval %.3f-%.3f", mediaID, slide, start, end)
	}
	return best, nil
}
func Diverse(frames []core.Frame, limit, threshold int) ([]core.Frame, error) {
	if limit < 1 {
		return nil, nil
	}
	var selected []core.Frame
	var hashes []*goimagehash.ImageHash
	// Try temporal buckets first, then fill any remaining distinct references.
	candidates := Select(frames, 0, 1e15, limit)
	candidates = append(candidates, frames...)
	for _, f := range candidates {
		im, e := readImage(f.Path)
		if e != nil {
			return nil, e
		}
		hash, e := goimagehash.PerceptionHash(im)
		if e != nil {
			return nil, e
		}
		duplicate := false
		for _, h := range hashes {
			distance, e := h.Distance(hash)
			if e != nil {
				return nil, e
			}
			if distance <= threshold {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		hashes = append(hashes, hash)
		selected = append(selected, f)
		if len(selected) == limit {
			break
		}
	}
	return selected, nil
}
