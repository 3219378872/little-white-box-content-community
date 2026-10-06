package safety

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var ErrBlocked = errors.New("assistant content blocked by safety policy")

// Filter 检查文本是否命中安全策略。
type Filter interface {
	Check(ctx context.Context, text string) error
}

// KeywordFilter 按屏蔽词检查文本，同时比较保留空格与去掉分隔符两种形式，防止插入符号绕过。
type KeywordFilter struct {
	terms        []string
	compactTerms []string
	maxScanRunes int
}

// NewKeywordFilter 规范化屏蔽词；扫描长度上限与屏蔽词均为必填。
func NewKeywordFilter(terms []string, maxScanRunes int) (*KeywordFilter, error) {
	if maxScanRunes <= 0 {
		return nil, fmt.Errorf("assistant safety max scan runes must be positive")
	}
	filter := &KeywordFilter{maxScanRunes: maxScanRunes}
	for _, term := range terms {
		normalized, compact := normalize(term)
		if compact == "" {
			continue
		}
		filter.terms = append(filter.terms, normalized)
		filter.compactTerms = append(filter.compactTerms, compact)
	}
	if len(filter.terms) == 0 {
		return nil, fmt.Errorf("assistant safety blocked terms must not be empty")
	}
	return filter, nil
}

// Check 超过扫描长度上限的文本直接拦截，避免绕过扫描。
func (f *KeywordFilter) Check(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len([]rune(text)) > f.maxScanRunes {
		return ErrBlocked
	}
	normalized, compact := normalize(text)
	for index, term := range f.terms {
		if strings.Contains(normalized, term) || strings.Contains(compact, f.compactTerms[index]) {
			return ErrBlocked
		}
	}
	return nil
}

// normalize 返回小写且只保留字母数字的两种形式：分隔符折叠为单个空格，以及完全去掉分隔符。
func normalize(value string) (string, string) {
	var spaced strings.Builder
	var compact strings.Builder
	lastSpace := true
	for _, current := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			spaced.WriteRune(current)
			compact.WriteRune(current)
			lastSpace = false
			continue
		}
		if !lastSpace {
			spaced.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(spaced.String()), compact.String()
}
