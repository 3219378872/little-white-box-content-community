package agent

import "strings"

const (
	IntentGeneral          = "general"
	IntentCommunityOpinion = "community_opinion"
	IntentFactualLookup    = "factual_lookup"
	IntentRecommend        = "recommend"
	IntentWatch            = "watch"
	IntentMemoryQuery      = "memory_query"
	IntentContinueTask     = "continue_task"
	IntentWritePost        = "write_post"

	CurrentConsentVersion int32 = 2
)

// Version1Tools 是 consent_version=1 已覆盖的工具（AGNT-007）。
func Version1Tools() []string {
	return []string{ToolSearchPosts, ToolWebSearch, ToolCreatePost, ToolUpdatePost, ToolDeletePost}
}

// ClassifyIntent 用规则把用户话轮映射到内容域 intent。复杂查询仍走 general，
// 由模型自行选工具；本层只识别高置信的续写、追踪、推荐与写帖线索。
func ClassifyIntent(message string) QueryPlan {
	text := strings.TrimSpace(message)
	plan := QueryPlan{Intent: IntentGeneral, TimeRange: "unspecified"}
	if text == "" {
		return plan
	}
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "还有吗", "换一批", "再来几个", "还有别的"):
		plan.Intent = IntentContinueTask
	case containsAny(lower, "帮我盯", "有更新告诉我", "出了叫我", "盯着"):
		plan.Intent = IntentWatch
	case containsAny(lower, "我喜欢", "我不喜欢", "记住", "别再推荐"):
		plan.Intent = IntentMemoryQuery
	case containsAny(lower, "发帖", "写一篇", "帮我发", "改一下帖"):
		plan.Intent = IntentWritePost
	case containsAny(lower, "推荐", "找几个", "有什么好看", "帮我找"):
		plan.Intent = IntentRecommend
	case containsAny(lower, "最近", "讨论", "风向", "评价怎么样", "口碑"):
		plan.Intent = IntentCommunityOpinion
		plan.TimeRange = "recent"
	}
	return plan
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// RestrictToolsForConsent 按 AGNT-007 裁剪工具表：版本 1 只保留五件套。
func RestrictToolsForConsent(registry *ToolRegistry, consentVersion int32) *ToolRegistry {
	if registry == nil || consentVersion >= CurrentConsentVersion {
		return registry
	}
	return registry.Restrict(Version1Tools())
}
