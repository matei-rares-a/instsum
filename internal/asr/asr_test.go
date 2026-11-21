package asr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"instsum/internal/config"
	"instsum/internal/core"
)

func TestParseWhisperOutputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		source   string
		expected core.Segment
		count    int
		wantErr  bool
	}{
		{
			name:     "server timestamps and confidence",
			input:    `{"segments":[{"start":1.25,"end":2.5,"text":" hello ","avg_logprob":-0.1}]}`,
			source:   "whisper-server",
			expected: core.Segment{Start: 1.25, End: 2.5, Text: "hello", Source: "whisper-server", Confidence: 0.9048374180359595},
			count:    1,
		},
		{
			name:     "CLI timestamps",
			input:    `{"transcription":[{"timestamps":{"from":"00:00:01,250","to":"00:00:02,500"},"text":"Hello","tokens":[{"p":0.8},{"p":0.6}]}]}`,
			source:   "whisper-cli",
			expected: core.Segment{Start: 1.25, End: 2.5, Text: "Hello", Source: "whisper-cli", Confidence: 0.7},
			count:    1,
		},
		{name: "successful no speech", input: `{"segments":[]}`, count: 0},
		{name: "malformed output", input: `{not-json`, wantErr: true},
		{name: "missing segments", input: `{"text":"unexpected"}`, wantErr: true},
		{name: "missing timestamps", input: `{"segments":[{"text":"speech"}]}`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseOutput([]byte(test.input), test.source)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != test.count {
				t.Fatalf("got %d segments, want %d", len(got), test.count)
			}
			if test.count == 1 {
				if got[0] != test.expected {
					t.Fatalf("got %#v, want %#v", got[0], test.expected)
				}
			}
		})
	}
}

func TestDisabledASRReturnsError(t *testing.T) {
	c := config.Defaults()
	c.ASR.Engine = "disabled"
	if _, err := New(c).Transcribe(context.Background(), "unused"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected explicit disabled error, got %v", err)
	}
}

func TestWhisperCLIUsesSuccessfulEmptyResultAsNoSpeech(t *testing.T) {
	binDir, tempDir := setupFakeTools(t)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("INSTSUM_FAKE_RESULT", `{"transcription":[]}`)

	c := config.Defaults()
	c.ASR.Engine = "whisper-cli"
	c.ASR.Executable = filepath.Join(binDir, helperName("whisper"))
	c.ASR.ModelPath = "fake-model.bin"
	segments, err := New(c).Transcribe(context.Background(), "input.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 0 {
		t.Fatalf("got %#v, want no-speech result", segments)
	}
	assertTempClean(t, tempDir)
}

func TestWhisperCLIFailureIsNotNoSpeech(t *testing.T) {
	binDir, tempDir := setupFakeTools(t)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("INSTSUM_FAKE_FAIL", "1")

	c := config.Defaults()
	c.ASR.Engine = "whisper-cli"
	c.ASR.Executable = filepath.Join(binDir, helperName("whisper"))
	c.ASR.ModelPath = "fake-model.bin"
	if _, err := New(c).Transcribe(context.Background(), "input.mp4"); err == nil {
		t.Fatal("expected model failure instead of no-speech result")
	}
	assertTempClean(t, tempDir)
}

func TestWhisperServerProtocolAndContext(t *testing.T) {
	binDir, tempDir := setupFakeTools(t)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		if r.Method != http.MethodPost || r.URL.Path != "/inference" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		if r.FormValue("response_format") != "verbose_json" || r.FormValue("language") != "ro" {
			t.Errorf("unexpected form fields: response_format=%q language=%q", r.FormValue("response_format"), r.FormValue("language"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("missing WAV upload: %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Filename != "input.wav" {
			t.Errorf("unexpected uploaded name %q", header.Filename)
		}
		w.Write([]byte(`{"segments":[{"start":0.5,"end":1.5,"text":"ok","tokens":[502],"words":[{"probability":0.9}]}]}`))
	}))
	defer server.Close()

	c := config.Defaults()
	c.ASR.Engine = "whisper-server"
	c.ASR.URL = server.URL
	c.ASR.Language = "ro"
	segments, err := New(c).Transcribe(context.Background(), "input.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(called) == 0 || len(segments) != 1 || segments[0].Text != "ok" || segments[0].Confidence != 0.9 {
		t.Fatalf("unexpected server transcription: called=%v segments=%#v", len(called) != 0, segments)
	}
	assertTempClean(t, tempDir)
}

func TestSubprocessHelper(t *testing.T) {
	t.Skip("fake tools are standalone executables built by the package tests")
}

func setupFakeTools(t *testing.T) (string, string) {
	t.Helper()
	base, err := filepath.Abs(filepath.Join("..", "..", ".adapter-test-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(base, "bin")
	tempDir := filepath.Join(base, "temp")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	source := filepath.Join(binDir, "fake_tool.go")
	if err := os.WriteFile(source, []byte(fakeToolSource), 0600); err != nil {
		t.Fatal(err)
	}
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	goExecutable := filepath.Join(runtime.GOROOT(), "bin", goName)
	fakePath := filepath.Join(binDir, helperName("ffmpeg"))
	cmd := exec.Command(goExecutable, "build", "-o", fakePath, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake ASR executable: %v: %s", err, output)
	}
	for _, name := range []string{"whisper"} {
		if err := copyFile(fakePath, filepath.Join(binDir, helperName(name))); err != nil {
			t.Fatal(err)
		}
	}
	return binDir, tempDir
}

func helperName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0700)
}

const fakeToolSource = `package main
import ("os"; "path/filepath"; "strings")
func main() {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), filepath.Ext(os.Args[0]))
	args := os.Args[1:]
	switch name {
	case "ffmpeg":
		hasRate, hasMono := false, false
		for i := 0; i+1 < len(args); i++ {
			hasRate = hasRate || (args[i] == "-ar" && args[i+1] == "16000")
			hasMono = hasMono || (args[i] == "-ac" && args[i+1] == "1")
		}
		if len(args) == 0 || !hasRate || !hasMono { os.Exit(2) }
		if os.WriteFile(args[len(args)-1], wav(), 0600) != nil { os.Exit(2) }
	case "whisper":
		if os.Getenv("INSTSUM_FAKE_FAIL") == "1" { os.Exit(2) }
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-of" {
				if os.WriteFile(args[i+1]+".json", []byte(os.Getenv("INSTSUM_FAKE_RESULT")), 0600) != nil { os.Exit(2) }
				return
			}
		}
		os.Exit(2)
	default:
		os.Exit(2)
	}
}
func wav() []byte {
	data := make([]byte, 44)
	copy(data[0:4], "RIFF"); copy(data[8:12], "WAVE"); copy(data[12:16], "fmt ")
	data[16] = 16; data[20] = 1; data[22] = 1; data[24] = 0x80; data[25] = 0x3e
	data[28] = 0x00; data[29] = 0x7d; data[32] = 2; data[34] = 16; copy(data[36:40], "data")
	return data
}`

func assertTempClean(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("ASR temporary files were not cleaned: %v", entries)
	}
}

func fakeWAV() []byte {
	data := make([]byte, 44)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	data[16] = 16
	data[20] = 1
	data[22] = 1
	data[24] = 0x80
	data[25] = 0x3e
	data[28] = 0x00
	data[29] = 0x7d
	data[32] = 2
	data[34] = 16
	copy(data[36:40], "data")
	return data
}
