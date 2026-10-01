// Package snapshot 冻结送审内容并计算内容哈希（RVW-002）。
//
// 规范化先于哈希：Unicode NFC、去除零宽字符、全角半角折叠并压缩空白，
// 避免用不可见字符或字形变体绕过硬规则与指纹复用（DES-review-platform）。
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"esx/pkg/event"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// Frozen 是规范化后的快照及其哈希。
type Frozen struct {
	BizType  string
	Hash     string
	Content  []byte
	Snapshot event.ReviewSnapshot
}

// Freeze 规范化快照并返回稳定哈希。业务类型参与哈希，避免跨业务误复用结论。
func Freeze(bizType string, in event.ReviewSnapshot) (Frozen, error) {
	normalized := Normalize(in)
	payload := struct {
		BizType  string               `json:"bizType"`
		Snapshot event.ReviewSnapshot `json:"snapshot"`
	}{BizType: bizType, Snapshot: normalized}
	content, err := json.Marshal(payload)
	if err != nil {
		return Frozen{}, fmt.Errorf("snapshot: marshal canonical form: %w", err)
	}
	sum := sha256.Sum256(content)
	return Frozen{BizType: bizType, Hash: hex.EncodeToString(sum[:]), Content: content, Snapshot: normalized}, nil
}

// Normalize 返回规范化副本，不修改入参。
func Normalize(in event.ReviewSnapshot) event.ReviewSnapshot {
	out := event.ReviewSnapshot{
		Texts:       make(map[string]string, len(in.Texts)),
		LandingURL:  strings.TrimSpace(stripInvisible(norm.NFC.String(in.LandingURL))),
		Market:      strings.ToUpper(strings.TrimSpace(in.Market)),
		Language:    strings.ToLower(strings.TrimSpace(in.Language)),
		Industry:    strings.ToUpper(strings.TrimSpace(in.Industry)),
		SubmitterID: in.SubmitterID,
	}
	for key, value := range in.Texts {
		out.Texts[key] = NormalizeText(value)
	}
	out.Media = make([]event.ReviewMedia, 0, len(in.Media))
	for _, media := range in.Media {
		out.Media = append(out.Media, event.ReviewMedia{
			MediaID: media.MediaID,
			SHA256:  strings.ToLower(strings.TrimSpace(media.SHA256)),
			Kind:    media.Kind,
		})
	}
	sort.SliceStable(out.Media, func(i, j int) bool {
		if out.Media[i].SHA256 != out.Media[j].SHA256 {
			return out.Media[i].SHA256 < out.Media[j].SHA256
		}
		return out.Media[i].MediaID < out.Media[j].MediaID
	})
	if len(out.Media) == 0 {
		out.Media = nil
	}
	if len(in.Qualifications) > 0 {
		out.Qualifications = append([]event.ReviewQualification(nil), in.Qualifications...)
		sort.SliceStable(out.Qualifications, func(i, j int) bool {
			return out.Qualifications[i].QualificationID < out.Qualifications[j].QualificationID
		})
	}
	return out
}

// NormalizeText 是规则匹配与快照共用的文本规范化。
func NormalizeText(value string) string {
	folded := width.Fold.String(norm.NFC.String(value))
	folded = stripInvisible(folded)
	return strings.Join(strings.FieldsFunc(folded, unicode.IsSpace), " ")
}

func stripInvisible(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\u2060', '\u00ad', '\u180e':
			return -1
		}
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, value)
}
