// Package adpolicy 定义付费广告的政策码、演示市场与行业处置矩阵
// （SPEC-sponsored-ads、DES-review-platform「政策配置」）。
//
// 本包内容是演示配置：只用于验证按市场分派的机制，不代表 TikTok 或任何法域的现行规则。
// 广告服务据此做送审前校验，审核平台据此执行行业硬规则；审核政策版本通过
// MatrixVersion 固定所依赖的矩阵版本。
package adpolicy

import (
	"net"
	"net/url"
	"slices"
	"strings"
)

// MatrixVersion 标识市场与行业矩阵；审核政策文件必须引用同一版本。
const MatrixVersion = "demo-matrix-2026-10-01"

// MaxLandingURLLength 与 ADS-016 一致。
const MaxLandingURLLength = 2048

// PolicyCode 是业务规范定义的单个违规类型。
type PolicyCode struct {
	Code     string
	Title    string
	Category string
}

// Codes 按 SPEC-sponsored-ads「政策码」表维护，顺序即展示顺序。
var Codes = []PolicyCode{
	{"INDUSTRY.ADULT", "成人内容与服务", "Adult Content"},
	{"INDUSTRY.ALCOHOL", "酒精", "Alcohol"},
	{"INDUSTRY.GAMBLING", "博彩与游戏", "Gambling and Games"},
	{"INDUSTRY.FINANCIAL", "金融服务", "Financial Services"},
	{"INDUSTRY.HEALTHCARE", "医疗与药品", "Healthcare and Pharmaceuticals"},
	{"INDUSTRY.WEIGHT", "体重管理与身体形象", "Weight Management and Body Image"},
	{"INDUSTRY.DANGEROUS", "危险商品或服务", "Dangerous Products or Services"},
	{"INDUSTRY.POLITICAL", "政治、政府与选举", "Politics, Governments, and Elections"},
	{"INDUSTRY.RESTRICTED_OTHER", "其他受限商品与服务", "Other Products and Services"},
	{"INDUSTRY.QUALIFICATION", "缺少目标市场要求的行业资质", "Industry entry"},
	{"MISLEADING.CLAIM", "承诺或夸大效果", "Misleading claims"},
	{"MISLEADING.ABSOLUTE", "涉及时间、地域或品牌的绝对化用语", "Misleading claims"},
	{"MISLEADING.INCONSISTENT", "广告与落地页的产品、价格或优惠不一致，或缺少必要说明与条款", "Inconsistent information"},
	{"MISLEADING.CLICKBAIT", "虚假交互元素或诱导点击", "Clickbait"},
	{"MISLEADING.COMPARISON", "前后对比、恶意比较或主观贬低", "Comparisons"},
	{"MISLEADING.AIGC", "未标注的显著 AI 生成或编辑内容", "Edited media and AIGC"},
	{"MISLEADING.IDENTITY", "未经许可使用他人形象或虚假背书", "Identity misuse"},
	{"CONTENT.DECEPTIVE", "欺诈或欺骗性做法", "Deceptive practices"},
	{"CONTENT.MISINFORMATION", "虚假信息", "Misinformation"},
	{"CONTENT.DISCRIMINATION", "歧视、骚扰与霸凌", "Discrimination, Harassment, and Bullying"},
	{"CONTENT.VIOLENCE", "暴力与危险活动", "Violence and Dangerous Activities"},
	{"CONTENT.SELF_HARM", "自杀与自残", "Suicide and Self-Harm"},
	{"CONTENT.TEEN_SAFETY", "危害青少年安全与福祉", "Teen Safety and Wellbeing"},
	{"CONTENT.IP", "知识产权侵权", "Intellectual Property Infringement"},
	{"FORMAT.FUNCTIONALITY", "广告格式或功能不符合要求", "Ad Format and Functionality"},
	{"LANDING.URL", "落地页地址不合规", "Ad Review FAQs"},
	{"LANDING.DOMAIN", "落地页域名被禁止", "Ad Review FAQs"},
	{"LANDING.MISMATCH", "落地页与广告内容、语言或目标市场不一致", "Ad Review FAQs"},
	{"LANDING.PRIVACY", "落地页收集个人信息但缺少隐私政策", "Ad Review FAQs"},
	{"ACCOUNT.RISK", "广告主反复违规或规避审核", "Advertiser Account Policy"},
}

const (
	CodeQualification = "INDUSTRY.QUALIFICATION"
	CodeLandingURL    = "LANDING.URL"
	CodeLandingDomain = "LANDING.DOMAIN"
)

// IsCode 报告 code 是否为本规范定义的政策码。
func IsCode(code string) bool {
	return slices.ContainsFunc(Codes, func(c PolicyCode) bool { return c.Code == code })
}

// Market 是演示市场。
type Market struct {
	Code     string
	Language string
}

// Markets 是演示市场（DES-review-platform）：US/en、DE/de、ID/id。
var Markets = []Market{{"US", "en"}, {"DE", "de"}, {"ID", "id"}}

// LanguageOf 返回市场语言；未知市场返回空串。
func LanguageOf(market string) string {
	for _, m := range Markets {
		if m.Code == market {
			return m.Language
		}
	}
	return ""
}

// IsMarket 报告是否为演示市场。
func IsMarket(market string) bool { return LanguageOf(market) != "" }

// 行业代码。
const (
	IndustryGeneral    = "GENERAL"
	IndustryAdult      = "ADULT"
	IndustryDangerous  = "DANGEROUS"
	IndustryPolitical  = "POLITICAL"
	IndustryAlcohol    = "ALCOHOL"
	IndustryGambling   = "GAMBLING"
	IndustryFinancial  = "FINANCIAL"
	IndustryHealthcare = "HEALTHCARE"
	IndustryWeight     = "WEIGHT"
)

// Industries 是可选行业，顺序即展示顺序。
var Industries = []string{
	IndustryGeneral, IndustryFinancial, IndustryHealthcare, IndustryWeight,
	IndustryAlcohol, IndustryGambling, IndustryAdult, IndustryDangerous, IndustryPolitical,
}

// IsIndustry 报告行业代码是否有效。
func IsIndustry(industry string) bool { return slices.Contains(Industries, industry) }

// Disposition 是某行业在某市场的处置。
type Disposition struct {
	Forbidden          bool
	NeedsQualification bool
	AutoPassAllowed    bool
	// Forbidden 时用于拒绝的政策码
	PolicyCode string
}

// DispositionOf 返回演示矩阵中的处置。酒精与博彩在所有市场禁止（ADS-017：平台无年龄能力）。
func DispositionOf(market, industry string) Disposition {
	switch industry {
	case IndustryAdult, IndustryDangerous, IndustryPolitical, IndustryAlcohol, IndustryGambling:
		return Disposition{Forbidden: true, PolicyCode: "INDUSTRY." + industry}
	case IndustryFinancial, IndustryHealthcare:
		return Disposition{NeedsQualification: true}
	case IndustryWeight:
		switch market {
		case "US":
			return Disposition{}
		case "DE":
			return Disposition{NeedsQualification: true}
		default:
			return Disposition{Forbidden: true, PolicyCode: "INDUSTRY.WEIGHT"}
		}
	default:
		return Disposition{AutoPassAllowed: true}
	}
}

// LandingURLValid 实现 ADS-016 的结构校验：只接受 https；拒绝用户信息、IP 主机、
// 本地或保留域名，以及超过 2,048 个字符的地址。返回规范化域名。系统不抓取落地页。
func LandingURLValid(raw string) (domain string, ok bool) {
	if raw == "" || len(raw) > MaxLandingURLLength || strings.TrimSpace(raw) != raw {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || parsed.Port() != "" && parsed.Port() != "443" {
		return "", false
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return "", false
	}
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return "", false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", false
			}
		}
	}
	for _, reserved := range reservedSuffixes {
		if host == strings.TrimPrefix(reserved, ".") || strings.HasSuffix(host, reserved) {
			return "", false
		}
	}
	return host, true
}

// RFC 6761/6762/2606 与常见内网后缀。
var reservedSuffixes = []string{
	".localhost", ".local", ".internal", ".intranet", ".lan", ".home", ".corp",
	".test", ".example", ".invalid", ".onion", ".arpa", ".home.arpa",
}
