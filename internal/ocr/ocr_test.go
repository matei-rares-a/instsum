package ocr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"instsum/internal/config"
	"instsum/internal/core"
)

func TestTesseractLanguageMapping(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{input: []string{"ro", "en"}, want: "ron+eng"},
		{input: []string{"ro", "ron", "en"}, want: "ron+eng"},
		{input: []string{"de"}, want: "de"},
		{want: "ron+eng"},
	}
	for _, test := range tests {
		if got := tesseractLanguages(test.input); got != test.want {
			t.Errorf("tesseractLanguages(%v) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestParseTesseractTSVGroupsWordsAndFiltersConfidence(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t0\t0\t20\t10\t90\thello\n" +
		"5\t1\t1\t1\t1\t2\t21\t0\t20\t10\t70\tworld\n" +
		"5\t1\t1\t1\t2\t1\t0\t20\t20\t10\t30\tnoise\n"
	lines, err := parseTSV([]byte(tsv), 0.75)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Text != "hello world" || lines[0].Confidence < 0.79 || lines[0].Confidence > 0.81 {
		t.Fatalf("unexpected TSV result: %#v", lines)
	}
	if _, err = parseTSV([]byte("bad\theader\n"), 0); err == nil {
		t.Fatal("expected invalid TSV header error")
	}
	if _, err = parseTSV([]byte(tsv+"malformed row\n"), 0); err == nil {
		t.Fatal("expected malformed TSV row error")
	}
}

func TestCTCDecoderCollapsesRepeatsAndBlank(t *testing.T) {
	values := []float32{
		0.1, 0.8, 0.1,
		0.1, 0.7, 0.2,
		0.8, 0.1, 0.1,
		0.1, 0.1, 0.8,
	}
	text, confidence, err := ctcDecode(values, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "ab" || confidence < 0.79 || confidence > 0.81 {
		t.Fatalf("got text=%q confidence=%f", text, confidence)
	}
	if _, _, err = ctcDecode([]float32{1, 0}, []string{"a", "b"}); err == nil {
		t.Fatal("expected invalid model output shape error")
	}
}

func TestDisabledOCREngineReturnsError(t *testing.T) {
	c := config.Defaults()
	c.OCR.Engine = "disabled"
	if _, err := New(c).Recognize(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected explicit disabled error, got %v", err)
	}
}

func TestTesseractRejectsGPUDevice(t *testing.T) {
	c := config.Defaults()
	c.OCR.Engine = "tesseract"
	c.OCR.Device = "gpu"
	if _, err := New(c).Recognize(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "only cpu") {
		t.Fatalf("expected explicit CPU-only error, got %v", err)
	}
}

func TestTesseractInvocationProducesTimestampedSegments(t *testing.T) {
	binDir, err := filepath.Abs(filepath.Join("..", "..", ".ocr-test-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(binDir) })
	tesseractPath := filepath.Join(binDir, helperName("tesseract"))
	source := filepath.Join(binDir, "fake_tesseract.go")
	if err := os.WriteFile(source, []byte(fakeTesseractSource), 0600); err != nil {
		t.Fatal(err)
	}
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	goExecutable := filepath.Join(runtime.GOROOT(), "bin", goName)
	cmd := exec.Command(goExecutable, "build", "-o", tesseractPath, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake tesseract executable: %v: %s", err, output)
	}

	c := config.Defaults()
	c.OCR.Engine = "tesseract"
	c.OCR.Executable = tesseractPath
	c.OCR.Langs = []string{"ro", "en"}
	segments, err := New(c).Recognize(context.Background(), []core.Frame{{
		Path: "frame.png", Time: 2.5, MediaID: "media-1", Slide: 3,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 {
		t.Fatalf("got %#v, want one OCR segment", segments)
	}
	segment := segments[0]
	if segment.Text != "test line" || segment.Source != "tesseract" || segment.Start != 2.5 ||
		segment.End != 2.5 || segment.Confidence != 0.9 || segment.MediaID != "media-1" || segment.Slide != 3 {
		t.Fatalf("unexpected OCR segment: %#v", segment)
	}
	t.Setenv("INSTSUM_TEST_OCR_FAIL", "1")
	_, err = New(c).Recognize(context.Background(), []core.Frame{{Path: "frame.png"}})
	if err == nil || !strings.Contains(err.Error(), "model failed") || len(err.Error()) > 4600 {
		t.Fatalf("expected explicit bounded stderr error, got %v", err)
	}
}

func helperName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

const fakeTesseractSource = `package main
import ("os"; "strings")
func main() {
	args := strings.Join(os.Args[1:], " ")
	if !strings.Contains(args, "-l ron+eng") || !strings.HasSuffix(args, " tsv") { os.Exit(2) }
	if os.Getenv("INSTSUM_TEST_OCR_FAIL") == "1" {
		_, _ = os.Stderr.WriteString("model failed: " + strings.Repeat("x", 10000))
		os.Exit(1)
	}
	_, _ = os.Stderr.WriteString("Estimating resolution as 741\n")
	_, _ = os.Stdout.Write([]byte("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t0\t0\t40\t10\t90\ttest\n" +
		"5\t1\t1\t1\t1\t2\t41\t0\t30\t10\t90\tline\n"))
}`
