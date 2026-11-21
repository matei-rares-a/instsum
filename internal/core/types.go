package core

import (
	"context"
	"encoding/json"
)

type Frame struct {
	Path      string  `json:"path"`
	Time      float64 `json:"time"`
	MediaID   string  `json:"media_id,omitempty"`
	Slide     int     `json:"slide,omitempty"`
	Sharpness float64 `json:"sharpness,omitempty"`
}
type Metadata struct {
	URL      string           `json:"url"`
	Author   string           `json:"author,omitempty"`
	Date     string           `json:"date,omitempty"`
	Caption  string           `json:"caption,omitempty"`
	Title    string           `json:"title,omitempty"`
	ID       string           `json:"id,omitempty"`
	Hashtags []string         `json:"hashtags,omitempty"`
	Mentions []string         `json:"mentions,omitempty"`
	Stats    map[string]int64 `json:"stats,omitempty"`
	Music    string           `json:"music,omitempty"`
	Media    []Media          `json:"media,omitempty"`
}
type Media struct {
	ID        string  `json:"id"`
	Path      string  `json:"path"`
	Kind      string  `json:"kind"`
	Slide     int     `json:"slide,omitempty"`
	Duration  float64 `json:"duration_s,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	FPS       float64 `json:"fps,omitempty"`
	HasAudio  bool    `json:"has_audio"`
	AudioPath string  `json:"audio_path,omitempty"`
}
type Segment struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Text       string  `json:"text"`
	Source     string  `json:"source"`
	ID         string  `json:"id,omitempty"`
	MediaID    string  `json:"media_id,omitempty"`
	Slide      int     `json:"slide,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Estimated  bool    `json:"estimated"`
	TimeKnown  bool    `json:"time_known"`
}
type Stage struct {
	Name     string  `json:"name"`
	MediaID  string  `json:"media_id,omitempty"`
	Status   string  `json:"status"`
	Engine   string  `json:"engine,omitempty"`
	Mode     string  `json:"mode,omitempty"`
	Duration float64 `json:"duration_s"`
	Reason   string  `json:"reason,omitempty"`
}
type Event struct {
	Time         float64  `json:"t"`
	Action       string   `json:"action"`
	Objects      []string `json:"objects"`
	Quantities   []string `json:"quantities"`
	OnScreenText string   `json:"on_screen_text"`
	Estimated    bool     `json:"estimated"`
	Confidence   float64  `json:"confidence"`
	EvidenceIDs  []string `json:"evidence_ids,omitempty"`
	MediaID      string   `json:"media_id,omitempty"`
	Slide        int      `json:"slide,omitempty"`
}
type Window struct {
	Window    []float64 `json:"window"`
	Events    []Event   `json:"events"`
	MediaID   string    `json:"media_id,omitempty"`
	Slide     int       `json:"slide,omitempty"`
	TimeKnown bool      `json:"time_known"`
}
type Request struct {
	Prompt   string
	Images   []Frame
	Schema   json.RawMessage
	Kind     string
	Profile  string
	Model    string
	Window   [2]float64
	MediaID  string
	Slide    int
	Evidence []Segment
}
type Fetcher interface {
	Fetch(context.Context, string, string) (Metadata, []string, error)
}
type Transcriber interface {
	Transcribe(context.Context, string) ([]Segment, error)
}
type FrameExtractor interface {
	Extract(context.Context, string, string) ([]Frame, float64, error)
}
type OCREngine interface {
	Recognize(context.Context, []Frame) ([]Segment, error)
}
type VideoUnderstander interface {
	Complete(context.Context, Request) (json.RawMessage, error)
}
type TextModel interface {
	Complete(context.Context, Request) (json.RawMessage, error)
}
type Structurer interface {
	Structure(context.Context, Request) (json.RawMessage, error)
}
type ProfileRegistry interface{ Load(string) (Profile, error) }
type Profile struct {
	Name               string
	Description        string
	Hints              []string
	VisualInstructions string
	Schema             json.RawMessage
	ExtractTemplate    string
	RenderTemplate     string
	StepField          string
	UnitAliases        map[string]string
}
