package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"instsum/internal/core"
)

type Registry struct{ Dir string }

func (r Registry) Load(name string) (core.Profile, error) {
	var p core.Profile
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return p, fmt.Errorf("invalid profile name %q", name)
	}
	dir := filepath.Join(r.Dir, name)
	b, err := os.ReadFile(filepath.Join(dir, "profile.yaml"))
	if err != nil {
		return p, fmt.Errorf("profile %q: %w", name, err)
	}
	var meta struct {
		Name               string            `yaml:"name"`
		Description        string            `yaml:"description"`
		Hints              []string          `yaml:"hints"`
		VisualInstructions string            `yaml:"visual_instructions"`
		StepField          string            `yaml:"step_field"`
		UnitAliases        map[string]string `yaml:"unit_aliases"`
	}
	if err = yaml.Unmarshal(b, &meta); err != nil {
		return p, err
	}
	p.Name = meta.Name
	p.Description = meta.Description
	p.Hints = meta.Hints
	p.VisualInstructions = meta.VisualInstructions
	p.StepField = meta.StepField
	p.UnitAliases = meta.UnitAliases
	if p.StepField == "" {
		p.StepField = "steps"
	}
	if p.Name != name {
		return p, fmt.Errorf("profile name must match folder %q", name)
	}
	p.Schema, err = os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil {
		return p, err
	}
	b, err = os.ReadFile(filepath.Join(dir, "extract.tmpl"))
	if err != nil {
		return p, err
	}
	p.ExtractTemplate = string(b)
	b, err = os.ReadFile(filepath.Join(dir, "render.md.tmpl"))
	if err != nil {
		return p, err
	}
	p.RenderTemplate = string(b)
	return p, nil
}
func (r Registry) Detect(text string) (core.Profile, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return core.Profile{}, err
	}
	text = strings.ToLower(text)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "notes" {
			continue
		}
		p, err := r.Load(entry.Name())
		if err != nil {
			return p, err
		}
		for _, hint := range p.Hints {
			if hint != "" && strings.Contains(text, strings.ToLower(hint)) {
				return p, nil
			}
		}
	}
	return r.Load("notes")
}
