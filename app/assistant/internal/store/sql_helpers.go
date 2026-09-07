package store

import (
	"encoding/json"
	"strings"
)

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullBytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.Repeat("?,", n-1) + "?"
}

func intsToAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func decodeInt64s(raw []byte) []int64 {
	if len(raw) == 0 {
		return nil
	}
	var out []int64
	_ = json.Unmarshal(raw, &out)
	return out
}

func reverseMessages(rows []Message) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}
