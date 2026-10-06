// Package policy 加载按版本发布的审核政策（policy as code）。
//
// 政策文件随代码评审发布、嵌入二进制；worker 按配置激活一个 active 版本与可选的
// shadow 版本。加载失败时拒绝启动，不使用旧缓存猜测（DES-review-platform「失败模式」）。
package policy

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"path"
	"slices"
	"strings"

	"esx/app/review/internal/snapshot"
	"esx/pkg/adpolicy"

	yaml "go.yaml.in/yaml/v3"
)

//go:embed versions/*.yaml
var versions embed.FS

const (
	ActionReject = "reject"
	ActionHuman  = "human"
)

// Threshold 是某（issue，市场）的召回、通过与拒绝阈值（RVW-012、RVW-013）。
type Threshold struct {
	Route      float64 `yaml:"route"`
	Pass       float64 `yaml:"pass"`
	Reject     float64 `yaml:"reject"`
	AutoReject bool    `yaml:"autoReject"`
}

// thresholdOverride 为指定问题类型与市场覆盖默认阈值。
type thresholdOverride struct {
	Issues    []string  `yaml:"issues"`
	Markets   []string  `yaml:"markets"`
	Threshold Threshold `yaml:"threshold"`
}

// KeywordRule 是按语言的关键词硬规则。
type KeywordRule struct {
	Code   string              `yaml:"code"`
	Action string              `yaml:"action"`
	Terms  map[string][]string `yaml:"terms"`
}

// MediaRule 是按 sha256 精确匹配的黑样本。
type MediaRule struct {
	SHA256 string `yaml:"sha256"`
	Code   string `yaml:"code"`
}

// document 是政策 YAML 的原始结构，解析校验后转换为 Policy。
type document struct {
	Version               string   `yaml:"version"`
	IndustryMatrix        string   `yaml:"industryMatrix"`
	QASampleRate          float64  `yaml:"qaSampleRate"`
	ProtectionSubmissions int      `yaml:"protectionSubmissions"`
	HumanDeadlineHours    int      `yaml:"humanDeadlineHours"`
	RouterTopK            int      `yaml:"routerTopK"`
	Issues                []string `yaml:"issues"`
	Thresholds            struct {
		Default   Threshold           `yaml:"default"`
		Overrides []thresholdOverride `yaml:"overrides"`
	} `yaml:"thresholds"`
	Rules struct {
		Keywords       []KeywordRule `yaml:"keywords"`
		BlockedDomains []string      `yaml:"blockedDomains"`
		BlockedMedia   []MediaRule   `yaml:"blockedMedia"`
	} `yaml:"rules"`
}

// Policy 是一个已校验、只读的政策版本。
type Policy struct {
	Version               string
	ConfigHash            string
	QASampleRate          float64
	ProtectionSubmissions int
	HumanDeadlineHours    int
	RouterTopK            int
	Issues                []string
	Keywords              []KeywordRule
	BlockedDomains        []string
	BlockedMedia          map[string]string
	defaultThreshold      Threshold
	overrides             map[string]Threshold
}

// Load 读取嵌入的政策版本并校验。
func Load(version string) (*Policy, error) {
	if strings.TrimSpace(version) == "" || strings.ContainsAny(version, "/\\") {
		return nil, fmt.Errorf("policy: invalid version %q", version)
	}
	raw, err := versions.ReadFile(path.Join("versions", version+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("policy: version %s not found: %w", version, err)
	}
	return parse(raw)
}

// parse 严格解析政策文件（拒绝未知字段），校验版本、行业矩阵、抽样比例、阈值与规则后构建 Policy。
func parse(raw []byte) (*Policy, error) {
	var doc document
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("policy: decode: %w", err)
	}
	if doc.Version == "" {
		return nil, fmt.Errorf("policy: version is required")
	}
	if doc.IndustryMatrix != adpolicy.MatrixVersion {
		return nil, fmt.Errorf("policy %s: industry matrix %q does not match %q", doc.Version, doc.IndustryMatrix, adpolicy.MatrixVersion)
	}
	// RVW-014：抽样比例默认不低于 5%。
	if doc.QASampleRate < 0.05 || doc.QASampleRate > 1 {
		return nil, fmt.Errorf("policy %s: qaSampleRate must be within [0.05, 1]", doc.Version)
	}
	// RVW-012：每个广告主的前 3 次送审必须人审。
	if doc.ProtectionSubmissions < 3 {
		return nil, fmt.Errorf("policy %s: protectionSubmissions must be at least 3", doc.Version)
	}
	if doc.HumanDeadlineHours <= 0 || doc.RouterTopK <= 0 {
		return nil, fmt.Errorf("policy %s: humanDeadlineHours and routerTopK must be positive", doc.Version)
	}
	if len(doc.Issues) == 0 {
		return nil, fmt.Errorf("policy %s: at least one issue is required", doc.Version)
	}
	for _, issue := range doc.Issues {
		if !adpolicy.IsCode(issue) {
			return nil, fmt.Errorf("policy %s: unknown issue %s", doc.Version, issue)
		}
	}
	if err := validThreshold(doc.Thresholds.Default); err != nil {
		return nil, fmt.Errorf("policy %s: default threshold: %w", doc.Version, err)
	}
	p := &Policy{
		Version: doc.Version, QASampleRate: doc.QASampleRate,
		ProtectionSubmissions: doc.ProtectionSubmissions, HumanDeadlineHours: doc.HumanDeadlineHours,
		RouterTopK: doc.RouterTopK, Issues: slices.Clone(doc.Issues),
		defaultThreshold: doc.Thresholds.Default, overrides: map[string]Threshold{},
	}
	for _, override := range doc.Thresholds.Overrides {
		if err := validThreshold(override.Threshold); err != nil {
			return nil, fmt.Errorf("policy %s: override: %w", doc.Version, err)
		}
		for _, issue := range override.Issues {
			if !slices.Contains(doc.Issues, issue) {
				return nil, fmt.Errorf("policy %s: override for disabled issue %s", doc.Version, issue)
			}
			for _, market := range override.Markets {
				if !adpolicy.IsMarket(market) {
					return nil, fmt.Errorf("policy %s: unknown market %s", doc.Version, market)
				}
				p.overrides[issue+"|"+market] = override.Threshold
			}
		}
	}
	for _, rule := range doc.Rules.Keywords {
		if !adpolicy.IsCode(rule.Code) || (rule.Action != ActionReject && rule.Action != ActionHuman) {
			return nil, fmt.Errorf("policy %s: invalid keyword rule %s/%s", doc.Version, rule.Code, rule.Action)
		}
		normalized := KeywordRule{Code: rule.Code, Action: rule.Action, Terms: map[string][]string{}}
		for language, terms := range rule.Terms {
			for _, term := range terms {
				normalizedTerm := strings.ToLower(snapshot.NormalizeText(term))
				if normalizedTerm == "" {
					return nil, fmt.Errorf("policy %s: empty keyword for %s", doc.Version, rule.Code)
				}
				normalized.Terms[language] = append(normalized.Terms[language], normalizedTerm)
			}
		}
		p.Keywords = append(p.Keywords, normalized)
	}
	for _, domain := range doc.Rules.BlockedDomains {
		p.BlockedDomains = append(p.BlockedDomains, strings.ToLower(strings.TrimSpace(domain)))
	}
	p.BlockedMedia = map[string]string{}
	for _, rule := range doc.Rules.BlockedMedia {
		sum := strings.ToLower(strings.TrimSpace(rule.SHA256))
		if len(sum) != 64 || !adpolicy.IsCode(rule.Code) {
			return nil, fmt.Errorf("policy %s: invalid blocked media rule", doc.Version)
		}
		p.BlockedMedia[sum] = rule.Code
	}
	hash := sha256.Sum256(raw)
	p.ConfigHash = hex.EncodeToString(hash[:])
	return p, nil
}

// validThreshold 要求阈值满足 0 < pass < reject <= 1 且 0 < route <= 1。
func validThreshold(t Threshold) error {
	if t.Route <= 0 || t.Route > 1 || t.Pass <= 0 || t.Reject > 1 || t.Pass >= t.Reject {
		return fmt.Errorf("thresholds must satisfy 0 < pass < reject <= 1 and 0 < route <= 1")
	}
	return nil
}

// ThresholdFor 返回（issue，市场）的阈值。
func (p *Policy) ThresholdFor(issue, market string) Threshold {
	if t, ok := p.overrides[issue+"|"+market]; ok {
		return t
	}
	return p.defaultThreshold
}

// DeadlineMs 返回人审截止时间。
func (p *Policy) DeadlineMs(submittedAtMs int64) int64 {
	return submittedAtMs + int64(p.HumanDeadlineHours)*3600*1000
}
