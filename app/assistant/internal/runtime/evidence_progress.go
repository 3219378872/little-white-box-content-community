package runtime

import (
	"encoding/json"
	"sort"
	"strings"
)

// Fresh handles and retrieval timestamps are not new information.
func normalizeEvidenceResult(raw string) string {
	var envelope map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return raw
	}
	text, ok := envelope["text"].(string)
	if !ok {
		return raw
	}
	var result map[string]any
	decoder = json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		return raw
	}
	changed := false
	order := func(values []any) {
		sort.Slice(values, func(i, j int) bool {
			left, _ := json.Marshal(values[i])
			right, _ := json.Marshal(values[j])
			return string(left) < string(right)
		})
	}
	clean := func(value any) {
		if fragments, ok := value.([]any); ok {
			for _, value := range fragments {
				if fragment, ok := value.(map[string]any); ok {
					delete(fragment, "id")
					delete(fragment, "handle")
					delete(fragment, "retrievedAtMs")
				}
			}
			order(fragments)
		}
	}
	if sources, ok := result["sources"].([]any); ok {
		changed = true
		for _, value := range sources {
			if source, ok := value.(map[string]any); ok {
				delete(source, "handle")
				clean(source["retrieved_evidence"])
			}
		}
		order(sources)
	}
	if _, ok := result["excerpts"].([]any); ok {
		changed = true
		clean(result["excerpts"])
	}
	if !changed {
		return raw
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return raw
	}
	envelope["text"] = string(encoded)
	encoded, err = json.Marshal(envelope)
	if err != nil {
		return raw
	}
	return string(encoded)
}
