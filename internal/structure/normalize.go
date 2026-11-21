package structure

import (
	"encoding/json"
	"strings"
)

func NormalizeUnits(notes json.RawMessage, aliases map[string]string) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(notes, &value); err != nil {
		return nil, err
	}
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			if unit, ok := item["unit"].(string); ok {
				if normalized, exists := aliases[strings.ToLower(strings.TrimSpace(unit))]; exists && normalized != unit {
					item["unit_raw"] = unit
					item["unit"] = normalized
				}
			}
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return json.Marshal(value)
}
