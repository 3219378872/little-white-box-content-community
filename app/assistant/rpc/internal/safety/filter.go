package safety

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var ErrBlocked = errors.New("assistant content blocked by safety policy")

type Filter interface {
	Check(ctx context.Context, text string) error
}

type KeywordFilter struct {
	terms        []string
	compactTerms []string
	maxScanRunes int
}

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
