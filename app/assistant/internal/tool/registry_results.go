package tool

import (
	"encoding/json"
	"unicode/utf8"
)

func unavailableResult(name string) string {
	raw, _ := json.Marshal(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": "TOOL_UNAVAILABLE", "tool": name, "message": "工具当前不可用，请改用其它能力或直接说明限制。"},
	})
	return string(raw)
}

func limitResult(text string, limit int) string {
	if limit <= 0 {
		limit = defaultMaxResultBytes
	}
	if len(text) <= limit {
		return text
	}
	const reserve = 256
	maxText := limit - reserve
	if maxText < 0 {
		maxText = 0
	}
	truncated := truncateUTF8Bytes(text, maxText)
	raw, _ := json.Marshal(map[string]any{
		"ok": true, "truncated": true, "original_bytes": len(text), "text": truncated,
	})
	if len(raw) <= limit {
		return string(raw)
	}
	return `{"ok":true,"truncated":true,"text":""}`
}

func truncateUTF8Bytes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}
