package llamaserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
)

type Server struct {
	cmd  *exec.Cmd
	done chan error
	log  *os.File
}

func Start(ctx context.Context, c config.Config, model string, dir string) (*Server, error) {
	if !c.Llama.Manage {
		return nil, nil
	}
	u, e := url.Parse(c.Llama.URL)
	if e != nil {
		return nil, e
	}
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return nil, fmt.Errorf("managed llama-server requires a loopback server_url")
	}
	port := u.Port()
	if port == "" {
		return nil, fmt.Errorf("managed llama-server requires an explicit port")
	}
	address := net.JoinHostPort(u.Hostname(), port)
	probe, e := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if e == nil {
		probe.Close()
		return nil, fmt.Errorf("llamacpp.manage_server refuses occupied address %s; use an unused port or manage_server:false", address)
	}
	m, ok := c.Models[model]
	if !ok {
		return nil, fmt.Errorf("unknown logical model %s", model)
	}
	files := []string{m.GGUFPath}
	if m.MMProjPath != "" {
		files = append(files, m.MMProjPath)
	}
	if model == c.VLM.Model && m.MMProjPath == "" {
		return nil, fmt.Errorf("managed vision server requires mmproj_path")
	}
	for _, path := range files {
		if path == "" {
			return nil, fmt.Errorf("managed server requires gguf_path")
		}
		if _, e = os.Stat(path); e != nil {
			return nil, fmt.Errorf("model file %s: %w; configure models.%s", path, e, model)
		}
	}
	log, e := os.OpenFile(filepath.Join(dir, "llama-server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return nil, e
	}
	args := []string{"-m", m.GGUFPath, "--host", u.Hostname(), "--port", port, "-c", strconv.Itoa(c.VLM.Context), "-ngl", "99", "--main-gpu", strconv.Itoa(c.GPU.Device)}
	if m.MMProjPath != "" {
		args = append(args, "--mmproj", m.MMProjPath)
	}
	// Startup context bounds readiness; the caller explicitly owns server lifetime.
	cmd := exec.Command(c.Llama.Executable, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		log.Close()
		return nil, fmt.Errorf("start %s: %w; install CUDA-enabled llama-server or set manage_server:false", c.Llama.Executable, e)
	}
	s := &Server{cmd: cmd, done: make(chan error, 1), log: log}
	go func() { s.done <- cmd.Wait() }()
	startup, cancel := context.WithTimeout(ctx, time.Duration(c.Llama.StartupTimeout)*time.Second)
	defer cancel()
	client := http.Client{Timeout: time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return fmt.Errorf("health redirect rejected") }}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	health := strings.TrimSuffix(strings.TrimRight(c.Llama.URL, "/"), "/v1") + "/health"
	for {
		select {
		case e := <-s.done:
			log.Close()
			return nil, fmt.Errorf("llama-server exited before readiness: %v; inspect %s", e, log.Name())
		case <-startup.Done():
			s.Close()
			return nil, fmt.Errorf("llama-server readiness: %w; inspect %s", startup.Err(), log.Name())
		case <-ticker.C:
			req, e := http.NewRequestWithContext(startup, "GET", health, nil)
			if e != nil {
				s.Close()
				return nil, e
			}
			resp, e := client.Do(req)
			if e == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return s, nil
				}
			}
		}
	}
}
func (s *Server) Close() error {
	if s == nil || s.cmd == nil {
		return nil
	}
	select {
	case e := <-s.done:
		s.log.Close()
		return e
	default:
	}
	// This PID was created by this instance; never signal an external server.
	e := s.cmd.Process.Kill()
	waitErr := <-s.done
	closeErr := s.log.Close()
	if e != nil && e != os.ErrProcessDone {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	_ = waitErr // Forced termination is the portable cleanup path on Windows.
	return nil
}
