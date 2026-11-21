package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Ingredient struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Unit     string `json:"unit"`
}
type Step struct {
	Text string `json:"text"`
}
type Notes struct {
	Ingredients []Ingredient `json:"ingredients"`
	Steps       []Step       `json:"steps"`
}
type Metrics struct {
	Precision        float64 `json:"ingredient_precision"`
	Recall           float64 `json:"ingredient_recall"`
	QuantityAccuracy float64 `json:"quantity_accuracy"`
	QuantityCases    int     `json:"quantity_cases"`
	StepOrder        float64 `json:"step_order_lcs_recall"`
}

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}
func Decode(b []byte) (Notes, error) {
	var wrapper struct {
		Structured json.RawMessage `json:"structured_notes"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return Notes{}, err
	}
	if len(wrapper.Structured) > 0 {
		b = wrapper.Structured
	}
	if string(b) == "null" {
		return Notes{}, fmt.Errorf("no valid structured notes")
	}
	var n Notes
	if err := json.Unmarshal(b, &n); err != nil {
		return n, err
	}
	if n.Ingredients == nil || n.Steps == nil {
		return n, fmt.Errorf("evaluation requires ingredients and steps arrays")
	}
	return n, nil
}
func Score(expected, actual Notes) Metrics {
	matched, quantities, correct := 0, 0, 0
	used := make([]bool, len(actual.Ingredients))
	for _, want := range expected.Ingredients {
		if want.Quantity != "" {
			quantities++
		}
		for i, got := range actual.Ingredients {
			if used[i] || norm(got.Name) != norm(want.Name) {
				continue
			}
			used[i] = true
			matched++
			if want.Quantity != "" && norm(want.Quantity) == norm(got.Quantity) && norm(want.Unit) == norm(got.Unit) {
				correct++
			}
			break
		}
	}
	lcs := make([]int, len(actual.Steps)+1)
	for _, want := range expected.Steps {
		prev := 0
		for j, got := range actual.Steps {
			old := lcs[j+1]
			if norm(want.Text) == norm(got.Text) {
				lcs[j+1] = prev + 1
			} else if lcs[j] > lcs[j+1] {
				lcs[j+1] = lcs[j]
			}
			prev = old
		}
	}
	return Metrics{ratio(matched, len(actual.Ingredients)), ratio(matched, len(expected.Ingredients)), ratio(correct, quantities), quantities, ratio(lcs[len(actual.Steps)], len(expected.Steps))}
}
