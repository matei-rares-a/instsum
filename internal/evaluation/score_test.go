package evaluation

import "testing"

func TestScoresPenalizeMissingDuplicateAndReorderedFacts(t *testing.T) {
	want := Notes{Ingredients: []Ingredient{{"flour", "500", "g"}, {"milk", "200", "ml"}}, Steps: []Step{{"mix"}, {"bake"}}}
	got := Notes{Ingredients: []Ingredient{{"flour", "50", "g"}, {"flour", "500", "g"}}, Steps: []Step{{"bake"}, {"mix"}}}
	m := Score(want, got)
	if m.Precision != .5 || m.Recall != .5 || m.QuantityAccuracy != 0 || m.StepOrder != .5 {
		t.Fatalf("metrics %+v", m)
	}
}
