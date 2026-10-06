package store

import (
	"strings"
)

// boolToInt encodes booleans for TINYINT columns.
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// nullInt stores zero as NULL for optional columns.
func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// nullString stores an empty string as NULL for optional columns.
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nullBytes stores an empty payload as NULL for optional columns.
func nullBytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

// placeholders builds an IN list; zero items yield NULL so the predicate matches nothing.
func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.Repeat("?,", n-1) + "?"
}

// intsToAny adapts IDs to query arguments.
func intsToAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// stringsToAny adapts strings to query arguments.
func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// reverseMessages restores ascending order after a DESC page query.
func reverseMessages(rows []Message) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}
