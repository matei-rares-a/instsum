package media

import (
	"image"
	"image/color"
	"image/jpeg"
	"instsum/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestSharpnessDedupCoverageAndStepSelection(t *testing.T) {
	dir := t.TempDir()
	flat := image.NewRGBA(image.Rect(0, 0, 32, 32))
	sharp := image.NewRGBA(flat.Bounds())
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if (x+y)%2 == 0 {
				sharp.Set(x, y, color.White)
			}
		}
	}
	if Sharpness(sharp) <= Sharpness(flat) {
		t.Fatal("sharpness order")
	}
	path := filepath.Join(dir, "frame.jpg")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	e = jpeg.Encode(f, sharp, nil)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	in := []core.Frame{{Path: path, Time: 0, MediaID: "m"}, {Path: path, Time: 1, MediaID: "m"}, {Path: path, Time: 2, MediaID: "m"}}
	got, e := Dedup(in, 4, 2)
	if e != nil || len(got) != 2 || got[1].Time != 2 {
		t.Fatalf("coverage: %+v, %v", got, e)
	}
	in[1].Sharpness = 10
	best, e := Best(in, "m", 0, 0, 1)
	if e != nil || best.Time != 1 {
		t.Fatalf("best %+v %v", best, e)
	}
	if _, e = Best(in, "other", 0, 0, 1); e == nil {
		t.Fatal("wrong media used")
	}
}
