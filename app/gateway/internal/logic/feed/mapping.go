package feed

// copyStrings 复制切片并把 nil 规范为空切片，使 JSON 输出 [] 而不是 null。
func copyStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
