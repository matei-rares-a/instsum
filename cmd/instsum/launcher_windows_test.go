//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func launcherDir(t *testing.T, executable bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"start.bat", filepath.Join("scripts", "start.ps1"), filepath.Join("scripts", "nvidia-smi.ps1")} {
		b, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, file), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if executable {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "instsum.exe"), b, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runLauncher(t *testing.T, root, input string, env ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Getenv("ComSpec"), "/d", "/c", filepath.Join(root, "start.bat"))
	cmd.Dir = t.TempDir()
	cmd.Stdin = strings.NewReader(input + "\n\n")
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return string(output), exit.ExitCode()
	}
	t.Fatal(err)
	return "", 1
}

func launcherLog(t *testing.T, root string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "logs", "*.log"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("expected one log: %v, %v", paths, err)
	}
	b, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWindowsLauncherLogsOutputAndPreservesURL(t *testing.T) {
	url := "https://www.instagram.com/reel/ABC/?a=1&b=two%20words"
	for _, tc := range []struct {
		name string
		code int
	}{
		{"complete", 0}, {"partial", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := launcherDir(t, true)
			partial := "0"
			if tc.code == 2 {
				partial = "1"
			}
			output, code := runLauncher(t, root, url, "INSTSUM_TEST_LAUNCHER_HELPER=1", "INSTSUM_TEST_PARTIAL="+partial)
			log := launcherLog(t, root)
			if code != tc.code {
				t.Fatalf("exit %d, want %d; output=%s log=%s", code, tc.code, output, log)
			}
			for _, text := range []string{"argument: " + url, "diagnostic from stderr", "cod: " + strconv.Itoa(tc.code)} {
				if !strings.Contains(log, text) || !strings.Contains(output, text) {
					t.Fatalf("missing %q: output=%s log=%s", text, output, log)
				}
			}
		})
	}
}

func TestWindowsLauncherUsesSavedURLWithoutPrompt(t *testing.T) {
	root := launcherDir(t, true)
	url := "https://www.instagram.com/reel/ABC/?a=1&b=two%20words"
	if err := os.WriteFile(filepath.Join(root, "start-url.txt"), []byte(" \r\n"+url+"\t\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, code := runLauncher(t, root, "", "INSTSUM_TEST_LAUNCHER_HELPER=1")
	log := launcherLog(t, root)
	for _, text := range []string{"argument: " + url, "URL incarcat din start-url.txt."} {
		if code != 0 || !strings.Contains(output, text) || !strings.Contains(log, text) {
			t.Fatalf("saved URL: exit=%d, missing %q output=%s log=%s", code, text, output, log)
		}
	}
	if strings.Contains(output, "Lipeste URL-ul") {
		t.Fatalf("launcher prompted despite saved URL: %s", output)
	}
}

func TestWindowsLauncherPromptsWhenSavedURLIsMissingOrBlank(t *testing.T) {
	for _, tc := range []struct {
		name string
		file bool
	}{
		{name: "missing"},
		{name: "blank", file: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := launcherDir(t, true)
			if tc.file {
				if err := os.WriteFile(filepath.Join(root, "start-url.txt"), []byte(" \r\n\t "), 0600); err != nil {
					t.Fatal(err)
				}
			}
			url := "https://instagram.com/reel/prompted/?a=1&b=2"
			output, code := runLauncher(t, root, url, "INSTSUM_TEST_LAUNCHER_HELPER=1")
			log := launcherLog(t, root)
			if code != 0 || !strings.Contains(output, "Lipeste URL-ul") ||
				!strings.Contains(output, "argument: "+url) || !strings.Contains(log, "argument: "+url) {
				t.Fatalf("prompt fallback: exit=%d output=%s log=%s", code, output, log)
			}
		})
	}
}

func TestWindowsLauncherBuildsWhenExecutableIsMissing(t *testing.T) {
	root := launcherDir(t, false)
	if err := os.MkdirAll(filepath.Join(root, "cmd", "instsum"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": `module local.test/launcher

go 1.23
`,
		filepath.Join("cmd", "instsum", "main.go"): `package main
import ("fmt"; "os")
func main() { fmt.Println("built argument:", os.Args[1]); fmt.Fprintln(os.Stderr, "built stderr"); os.Exit(2) }
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	url := "https://instagram.com/reel/abc/?a=1&b=2"
	output, code := runLauncher(t, root, url, "GOWORK=off")
	log := launcherLog(t, root)
	for _, text := range []string{"Se construieste", "built argument: " + url, "built stderr", "cod: 2"} {
		if code != 2 || !strings.Contains(output, text) || !strings.Contains(log, text) {
			t.Fatalf("build path: exit=%d, missing %q output=%s log=%s", code, text, output, log)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "instsum.exe")); !os.IsNotExist(err) {
		t.Fatalf("temporary executable should not remain in project: %v", err)
	}
}

func TestWindowsLauncherReportsStartupFailures(t *testing.T) {
	t.Run("build failure", func(t *testing.T) {
		root := launcherDir(t, false)
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module local.test/launcher\n\ngo 1.23\n"), 0600); err != nil {
			t.Fatal(err)
		}

		output, code := runLauncher(t, root, "https://instagram.com/reel/abc/", "GOWORK=off")
		log := launcherLog(t, root)
		if code != 1 || !strings.Contains(log, "Eroare la compilare") || !strings.Contains(output, "Eroare la compilare") {
			t.Fatalf("build failure: exit=%d, output=%s, log=%s", code, output, log)
		}
	})
	t.Run("missing Go", func(t *testing.T) {
		root := launcherDir(t, false)
		system := filepath.Join(os.Getenv("SystemRoot"), "System32")
		path := system + ";" + filepath.Join(system, "WindowsPowerShell", "v1.0")
		output, code := runLauncher(t, root, "https://instagram.com/reel/abc/", "PATH="+path)
		log := launcherLog(t, root)
		if code != 1 || !strings.Contains(log, "Go nu este disponibil") || !strings.Contains(output, "Go nu este disponibil") {
			t.Fatalf("missing Go: exit=%d, output=%s, log=%s", code, output, log)
		}
	})
	t.Run("empty input", func(t *testing.T) {
		root := launcherDir(t, true)
		output, code := runLauncher(t, root, "")
		if log := launcherLog(t, root); code != 64 || !strings.Contains(log, "nu ai introdus") {
			t.Fatalf("empty: exit=%d output=%s log=%s", code, output, log)
		}
	})
	t.Run("log path unavailable", func(t *testing.T) {
		root := launcherDir(t, true)
		if err := os.WriteFile(filepath.Join(root, "logs"), []byte("not a directory"), 0600); err != nil {
			t.Fatal(err)
		}
		output, code := runLauncher(t, root, "")
		if code != 1 || !strings.Contains(output, "Nu se poate crea jurnalul") {
			t.Fatalf("log creation: exit=%d output=%s", code, output)
		}
	})
	t.Run("invalid executable", func(t *testing.T) {
		root := launcherDir(t, false)
		if err := os.WriteFile(filepath.Join(root, "instsum.exe"), []byte("not an executable"), 0700); err != nil {
			t.Fatal(err)
		}
		output, code := runLauncher(t, root, "https://instagram.com/reel/abc/")
		log := launcherLog(t, root)
		if code != 1 || !strings.Contains(log, "Eroare") || !bytes.Contains([]byte(output), []byte("Eroare")) {
			t.Fatalf("start failure: exit=%d output=%s log=%s", code, output, log)
		}
	})
}

func TestWindowsLauncherUsesReadySetup(t *testing.T) {
	root := launcherDir(t, true)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	runtime := filepath.Join(root, "runtime", "setup-v1")
	for _, directory := range []string{filepath.Join(runtime, "env"), filepath.Join(root, "configs")} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary, err := os.ReadFile(filepath.Join(root, "instsum.exe"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(runtime, "instsum.exe"), filepath.Join(runtime, "env", "python.exe")} {
		if err := os.WriteFile(file, binary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(root, "configs", "config.yaml"), filepath.Join(root, "scripts", "setup-config.py")} {
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runtime, "ready"), []byte("setup-v1"), 0600); err != nil {
		t.Fatal(err)
	}
	url := "https://instagram.com/reel/abc/?a=1&b=2"
	if err := os.WriteFile(filepath.Join(root, "start-url.txt"), []byte(url+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, code := runLauncher(t, root, "", "INSTSUM_TEST_SETUP_HELPER=1")
	log := launcherLog(t, root)
	for _, text := range []string{"arguments: --config|", "|" + url, "tessdata: " + filepath.Join(runtime, "tessdata"), filepath.Join(runtime, "env", "Library", "bin"), "cod: 2"} {
		if code != 2 || !strings.Contains(output, text) || !strings.Contains(log, text) {
			t.Fatalf("setup path: exit=%d, missing %q output=%s log=%s", code, text, output, log)
		}
	}
	if strings.Contains(output, "Lipeste URL-ul") {
		t.Fatalf("setup-ready launcher prompted despite saved URL: %s", output)
	}
}

func TestWindowsLauncherRejectsIncompleteSetup(t *testing.T) {
	root := launcherDir(t, true)
	runtime := filepath.Join(root, "runtime", "setup-v1")
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "ready"), []byte("setup-v1"), 0600); err != nil {
		t.Fatal(err)
	}
	output, code := runLauncher(t, root, "https://instagram.com/reel/abc/")
	if log := launcherLog(t, root); code != 1 || !strings.Contains(log, "Instalare locala incompleta") {
		t.Fatalf("incomplete setup: exit=%d output=%s log=%s", code, output, log)
	}
}

func TestWindowsSetupHelperFixtures(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "setup_test.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup helper tests: %v\n%s", err, output)
	}
}
