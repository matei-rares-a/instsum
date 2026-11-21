package config

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Model struct {
	OllamaTag  string `yaml:"ollama_tag"`
	GGUFPath   string `yaml:"gguf_path"`
	MMProjPath string `yaml:"mmproj_path"`
}
type Config struct {
	Output struct {
		Dir                string `yaml:"dir"`
		MaxReferenceFrames int    `yaml:"max_reference_frames"`
		Language           string `yaml:"language"`
		ImageFormat        string `yaml:"image_format"`
	} `yaml:"output"`
	Fetch struct {
		CookiesFile        string `yaml:"cookies_file"`
		CookiesFromBrowser string `yaml:"cookies_from_browser"`
		Retries            int    `yaml:"retries"`
		RateLimit          int    `yaml:"rate_limit_per_min"`
		Timeout            int    `yaml:"timeout_s"`
	} `yaml:"fetch"`
	GPU struct {
		Device   int    `yaml:"device"`
		Budget   int    `yaml:"vram_budget_mb"`
		Schedule string `yaml:"schedule"`
		Monitor  bool   `yaml:"monitor"`
	} `yaml:"gpu"`
	ASR struct {
		Engine            string  `yaml:"engine"`
		Executable        string  `yaml:"executable"`
		URL               string  `yaml:"server_url"`
		ModelPath         string  `yaml:"model_path"`
		VADModelPath      string  `yaml:"vad_model_path"`
		Language          string  `yaml:"language"`
		VAD               bool    `yaml:"vad"`
		NoSpeechThreshold float64 `yaml:"no_speech_threshold"`
		Timeout           int     `yaml:"timeout_s"`
	} `yaml:"asr"`
	OCR struct {
		Engine           string   `yaml:"engine"`
		Executable       string   `yaml:"executable"`
		Device           string   `yaml:"device"`
		Langs            []string `yaml:"langs"`
		MinConfidence    float64  `yaml:"min_confidence"`
		Timeout          int      `yaml:"timeout_s"`
		RuntimePath      string   `yaml:"runtime_path"`
		DetectionModel   string   `yaml:"detection_model"`
		RecognitionModel string   `yaml:"recognition_model"`
		Dictionary       string   `yaml:"dictionary"`
	} `yaml:"ocr"`
	Frames struct {
		Interval       float64 `yaml:"analysis_interval_s"`
		Size           int     `yaml:"vlm_frame_size"`
		SceneThreshold float64 `yaml:"scene_threshold"`
		DedupHamming   int     `yaml:"dedup_hamming"`
		Timeout        int     `yaml:"timeout_s"`
	} `yaml:"frames"`
	Models map[string]Model `yaml:"models"`
	VLM    struct {
		Engine      string  `yaml:"engine"`
		Fallback    string  `yaml:"fallback_engine"`
		Model       string  `yaml:"model"`
		Context     int     `yaml:"context_size"`
		Window      float64 `yaml:"window_s"`
		Overlap     float64 `yaml:"window_overlap_s"`
		Frames      int     `yaml:"frames_per_window"`
		Temperature float64 `yaml:"temperature"`
		Retries     int     `yaml:"json_retries"`
		NumPredict  int     `yaml:"num_predict"`
		Concurrency int     `yaml:"concurrency"`
	} `yaml:"vlm"`
	Llama struct {
		URL            string `yaml:"server_url"`
		Manage         bool   `yaml:"manage_server"`
		Executable     string `yaml:"executable"`
		StartupTimeout int    `yaml:"startup_timeout_s"`
	} `yaml:"llamacpp"`
	Ollama struct {
		Mode           string `yaml:"mode"`
		URL            string `yaml:"base_url"`
		KeepAlive      string `yaml:"keep_alive"`
		AutoPull       bool   `yaml:"auto_pull"`
		Timeout        int    `yaml:"request_timeout_s"`
		Fixtures       string `yaml:"stub_fixtures_dir"`
		UnloadAfterUse bool   `yaml:"unload_after_use"`
	} `yaml:"ollama"`
	Structure struct {
		Profile   string `yaml:"profile"`
		TextModel string `yaml:"text_model"`
	} `yaml:"structure"`
	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`
	ProfilesDir string   `yaml:"profiles_dir"`
	PromptsDir  string   `yaml:"prompts_dir"`
	AllowCloud  bool     `yaml:"allow_cloud"`
	LocalHosts  []string `yaml:"local_hosts"`
}

func Defaults() Config {
	var c Config
	c.Output.Dir = "./out"
	c.Output.MaxReferenceFrames = 8
	c.Output.ImageFormat = "jpg"
	c.Output.Language = "auto"
	c.Fetch.Timeout = 180
	c.Fetch.RateLimit = 10
	c.Fetch.Retries = 3
	c.GPU.Budget = 9000
	c.GPU.Schedule = "sequential"
	c.ASR.Engine = "whisper-cli"
	c.ASR.Executable = "whisper-cli"
	c.ASR.URL = "http://127.0.0.1:8081"
	c.ASR.ModelPath = "./models/ggml-large-v3-turbo.bin"
	c.ASR.Language = "auto"
	c.ASR.NoSpeechThreshold = .6
	c.ASR.Timeout = 300
	c.OCR.Engine = "tesseract"
	c.OCR.Executable = "tesseract"
	c.OCR.Device = "cpu"
	c.OCR.Langs = []string{"ro", "en"}
	c.OCR.MinConfidence = .6
	c.OCR.Timeout = 120
	c.Frames.Interval = 1
	c.Frames.Size = 768
	c.Frames.SceneThreshold = .25
	c.Frames.DedupHamming = 4
	c.Frames.Timeout = 180
	c.VLM.Engine = "llamacpp"
	c.VLM.Context = 8192
	c.VLM.Window = 10
	c.VLM.Overlap = 1
	c.VLM.Frames = 8
	c.VLM.Temperature = .1
	c.VLM.Retries = 1
	c.VLM.NumPredict = 2048
	c.VLM.Concurrency = 1
	c.Llama.URL = "http://127.0.0.1:8080/v1"
	c.Llama.Executable = "llama-server"
	c.Llama.StartupTimeout = 120
	c.Ollama.Mode = "disabled"
	c.Ollama.URL = "http://127.0.0.1:11434"
	c.Ollama.KeepAlive = "5m"
	c.Ollama.Timeout = 120
	c.Ollama.Fixtures = "./testdata/ollama_fixtures"
	c.Structure.Profile = "auto"
	c.Structure.TextModel = "same_as_vlm"
	c.ProfilesDir = "./profiles"
	c.PromptsDir = "./prompts"
	c.Logging.Level = "info"
	c.Logging.Format = "text"
	return c
}

// Load applies file and environment overrides; validate after applying CLI flags.
func Load(path string) (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err = decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	err = applyEnv(reflect.ValueOf(&c).Elem(), "IGX")
	return c, err
}

var envToken = regexp.MustCompile(`[^A-Z0-9]+`)

func applyEnv(v reflect.Value, prefix string) error {
	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			key := v.Type().Field(i).Tag.Get("yaml")
			if err := applyEnv(v.Field(i), prefix+"_"+envToken.ReplaceAllString(strings.ToUpper(key), "_")); err != nil {
				return err
			}
		}
		return nil
	}
	if v.Kind() == reflect.Map {
		for _, k := range v.MapKeys() {
			item := reflect.New(v.Type().Elem()).Elem()
			item.Set(v.MapIndex(k))
			if err := applyEnv(item, prefix+"_"+envToken.ReplaceAllString(strings.ToUpper(k.String()), "_")); err != nil {
				return err
			}
			v.SetMapIndex(k, item)
		}
		return nil
	}
	raw, ok := os.LookupEnv(prefix)
	if !ok {
		return nil
	}
	var err error
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		var x bool
		x, err = strconv.ParseBool(raw)
		if err == nil {
			v.SetBool(x)
		}
	case reflect.Int:
		var x int64
		x, err = strconv.ParseInt(raw, 10, 64)
		if err == nil {
			v.SetInt(x)
		}
	case reflect.Float64:
		var x float64
		x, err = strconv.ParseFloat(raw, 64)
		if err == nil {
			v.SetFloat(x)
		}
	case reflect.Slice:
		var values []string
		if raw != "" {
			for _, s := range strings.Split(raw, ",") {
				values = append(values, strings.TrimSpace(s))
			}
		}
		v.Set(reflect.ValueOf(values))
	default:
		return fmt.Errorf("unsupported environment override %s", prefix)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return nil
}

func (c Config) CheckEndpoint(key, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must be an HTTP(S) URL without credentials, query or fragment", key)
	}
	ip := net.ParseIP(u.Hostname())
	local := strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	for _, host := range c.LocalHosts {
		if strings.EqualFold(u.Hostname(), host) {
			local = true
		}
	}
	if !c.AllowCloud && !local {
		return fmt.Errorf("%s must point to localhost or an explicitly configured local_hosts entry when allow_cloud is false", key)
	}
	return nil
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (c Config) Validate() error {
	if c.Output.Dir == "" || c.ProfilesDir == "" || c.PromptsDir == "" {
		return fmt.Errorf("output.dir, profiles_dir and prompts_dir are required")
	}
	if c.Output.MaxReferenceFrames < 1 || c.Output.MaxReferenceFrames > 100 {
		return fmt.Errorf("output.max_reference_frames must be between 1 and 100")
	}
	if c.Output.ImageFormat != "jpg" {
		return fmt.Errorf("output.image_format currently supports jpg only")
	}
	if c.GPU.Schedule != "sequential" {
		return fmt.Errorf("gpu.schedule %q requires measured concurrent scheduling; use sequential", c.GPU.Schedule)
	}
	if c.GPU.Device < 0 || c.GPU.Budget < 1 || c.GPU.Budget > 9000 {
		return fmt.Errorf("gpu device or vram_budget_mb invalid for the target GPU")
	}
	if !finite(c.Frames.Interval) || c.Frames.Interval <= 0 || c.Frames.Size < 1 || c.Frames.Timeout < 1 || c.Frames.SceneThreshold < 0 || c.Frames.SceneThreshold > 1 || c.Frames.DedupHamming < 0 || c.Frames.DedupHamming > 64 {
		return fmt.Errorf("frames settings are invalid")
	}
	if !finite(c.VLM.Window) || !finite(c.VLM.Overlap) || c.VLM.Window <= 0 || c.VLM.Overlap < 0 || c.VLM.Overlap >= c.VLM.Window || c.VLM.Frames < 1 || c.VLM.Context < 1 || c.VLM.NumPredict < 1 || c.VLM.Retries < 0 || c.VLM.Concurrency < 1 || !finite(c.VLM.Temperature) || c.VLM.Temperature < 0 {
		return fmt.Errorf("vlm window, overlap, context, concurrency or generation settings invalid")
	}
	if c.VLM.Concurrency != 1 {
		return fmt.Errorf("vlm.concurrency must be 1 with the prototype's sequential GPU schedule")
	}
	if c.VLM.Engine != "llamacpp" && c.VLM.Engine != "ollama" {
		return fmt.Errorf("vlm.engine must be llamacpp or ollama")
	}
	if c.VLM.Fallback != "" && c.VLM.Fallback != "llamacpp" {
		return fmt.Errorf("vlm.fallback_engine must be empty or llamacpp")
	}
	if _, ok := c.Models[c.VLM.Model]; !ok {
		return fmt.Errorf("vlm.model %q missing from models", c.VLM.Model)
	}
	if c.Structure.TextModel != "" && c.Structure.TextModel != "same_as_vlm" {
		if _, ok := c.Models[c.Structure.TextModel]; !ok {
			return fmt.Errorf("structure.text_model %q missing from models", c.Structure.TextModel)
		}
	}
	if c.Ollama.Mode != "disabled" && c.Ollama.Mode != "stub" && c.Ollama.Mode != "live" {
		return fmt.Errorf("ollama.mode must be disabled, stub or live")
	}
	if c.VLM.Engine == "ollama" && c.Ollama.Mode == "disabled" {
		return fmt.Errorf("ollama is disabled; set ollama.mode to stub or live")
	}
	if c.Ollama.Timeout < 1 || c.ASR.Timeout < 1 || c.OCR.Timeout < 1 || c.Fetch.Timeout < 1 || c.Fetch.Retries < 0 || c.Fetch.RateLimit < 1 || c.Llama.StartupTimeout < 1 {
		return fmt.Errorf("timeouts and fetch limits must be positive")
	}
	if c.ASR.Engine != "whisper-cli" && c.ASR.Engine != "whisper-server" && c.ASR.Engine != "disabled" {
		return fmt.Errorf("asr.engine must be whisper-cli, whisper-server or disabled")
	}
	if c.OCR.Engine != "tesseract" && c.OCR.Engine != "paddle-onnx" && c.OCR.Engine != "disabled" {
		return fmt.Errorf("ocr.engine must be tesseract, paddle-onnx or disabled")
	}
	if c.OCR.Device != "cpu" && c.OCR.Device != "gpu" {
		return fmt.Errorf("ocr.device must be cpu or gpu")
	}
	if c.OCR.Engine == "tesseract" && c.OCR.Device != "cpu" {
		return fmt.Errorf("Tesseract uses CPU; set ocr.device:cpu or choose paddle-onnx")
	}
	if !finite(c.OCR.MinConfidence) || c.OCR.MinConfidence < 0 || c.OCR.MinConfidence > 1 || !finite(c.ASR.NoSpeechThreshold) || c.ASR.NoSpeechThreshold < 0 || c.ASR.NoSpeechThreshold > 1 {
		return fmt.Errorf("recognition confidence thresholds must be within 0..1")
	}
	if c.Structure.Profile == "" {
		return fmt.Errorf("structure.profile is required")
	}
	for key, raw := range map[string]string{"llamacpp.server_url": c.Llama.URL, "ollama.base_url": c.Ollama.URL, "asr.server_url": c.ASR.URL} {
		if err := c.CheckEndpoint(key, raw); err != nil {
			return err
		}
	}
	return nil
}
