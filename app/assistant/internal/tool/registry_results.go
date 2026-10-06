package tool

import (
	"encoding/json"
	"unicode/utf8"
)

// unavailableResult 是工具不可用时给模型的结构化结果，引导其改用其它能力。
func unavailableResult(name string) string {
	raw, _ := json.Marshal(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": "TOOL_UNAVAILABLE", "tool": name, "message": "工具当前不可用，请改用其它能力或直接说明限制。"},
	})
	return string(raw)
}

// limitResult 把超长结果截断并包装为带 truncated 标记的 JSON，保证结果不超过上限。
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

// truncateUTF8Bytes 按字节截断且不切开 UTF-8 字符。
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
