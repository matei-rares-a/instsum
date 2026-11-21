package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"instsum/internal/evaluation"
	"os"
)

func main() {
	expected := flag.String("expected", "", "hand-annotated expected recipe JSON")
	actual := flag.String("actual", "", "actual recipe JSON or result.json")
	flag.Parse()
	if err := run(*expected, *actual); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(expected, actual string) error {
	if expected == "" || actual == "" {
		return fmt.Errorf("usage: igx-eval --expected expected.json --actual result.json")
	}
	a, e := os.ReadFile(expected)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(actual)
	if e != nil {
		return e
	}
	want, e := evaluation.Decode(a)
	if e != nil {
		return e
	}
	got, e := evaluation.Decode(b)
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(evaluation.Score(want, got))
}
