package logic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/model"
	"esx/pkg/errx"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	defaultPageSize          = 20
	defaultCandidateMultiple = 8
	defaultCursorTTLSeconds  = 600
	defaultInferenceTimeout  = 80 * time.Millisecond
	rrfConstant              = 60.0
)

// postRecallResult 是一路帖子召回的结果，index 用于按来源顺序合并。
type postRecallResult struct {
	index      int
	candidates []model.PostCandidate
	err        error
}

// userRecallResult 是一路用户召回的结果，index 用于按来源顺序合并。
type userRecallResult struct {
	index      int
	candidates []model.UserCandidate
	err        error
}

// markPostDegradation 在模型版本上标记降级原因，便于离线评估区分结果来源。
func markPostDegradation(candidates []model.PostCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

// markUserDegradation 在模型版本上标记降级原因。
func markUserDegradation(candidates []model.UserCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

// recommendationRPCError 把依赖错误映射为业务错误：取消与超时为系统错误，其余为服务不可用（调用方可据此降级）。
func recommendationRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errx.Wrap(err, errx.SystemError)
	}
	return errx.Wrap(err, errx.ServiceUnavailable)
}

// normalizedScene 缺省场景为 home。
func normalizedScene(scene string) string {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return "home"
	}
	return scene
}

// validIdentity 要求登录用户或匿名设备至少有一个身份，并限制匿名 ID 长度。
func validIdentity(userID int64, anonymousID string) bool {
	anonymousID = strings.TrimSpace(anonymousID)
	return userID >= 0 && len(anonymousID) <= 256 && (userID > 0 || anonymousID != "")
}

// validRequestMetadata 要求 requestId 存在，并限制各元数据长度。
func validRequestMetadata(requestID, scene, sessionID, experimentID string) bool {
	requestID = strings.TrimSpace(requestID)
	return requestID != "" && len(requestID) <= 128 && len(strings.TrimSpace(scene)) <= 64 &&
		len(strings.TrimSpace(sessionID)) <= 128 && len(strings.TrimSpace(experimentID)) <= 128
}

// configuredPageSize 返回请求的页大小，缺省用配置默认值；超过上限视为参数错误。
func configuredPageSize(requested int32, c config.Config) (int, error) {
	pageSize := int(requested)
	if pageSize == 0 {
		pageSize = c.DefaultPageSize
		if pageSize <= 0 {
			pageSize = defaultPageSize
		}
	}
	maximum := c.MaxPageSize
	if maximum <= 0 {
		maximum = 50
	}
	if pageSize <= 0 || pageSize > maximum {
		return 0, errx.NewWithCode(errx.ParamError)
	}
	return pageSize, nil
}

// candidateLimit 按页大小的倍数确定召回规模，为过滤与重排留出余量。
func candidateLimit(pageSize int, c config.Config) int {
	multiplier := c.CandidateMultiplier
	if multiplier <= 0 {
		multiplier = defaultCandidateMultiple
	}
	return pageSize * multiplier
}

// cursorTTL 返回推荐快照与游标的有效期。
func cursorTTL(c config.Config) int {
	if c.CursorTTLSeconds > 0 {
		return c.CursorTTLSeconds
	}
	return defaultCursorTTLSeconds
}

// ruleModelVersion 返回规则排序的模型版本标识。
func ruleModelVersion(c config.Config) string {
	if c.RuleModelVersion != "" {
		return c.RuleModelVersion
	}
	return "rules-v2"
}

// appendSource 把召回来源追加到逗号分隔的来源列表并去重。
func appendSource(current, next string) string {
	if next == "" || hasSource(current, next) {
		return current
	}
	if current == "" {
		return next
	}
	return current + "," + next
}

// hasSource 判断来源列表是否包含指定来源。
func hasSource(sources, source string) bool {
	for current := range strings.SplitSeq(sources, ",") {
		if current == source {
			return true
		}
	}
	return false
}

// appendDegradation 把降级标记以 + 连接到模型版本后并去重。
func appendDegradation(version, degradation string) string {
	if degradation == "" || strings.Contains(version, degradation) {
		return version
	}
	if version == "" {
		return degradation
	}
	return version + "+" + degradation
}

// emptyViewerFeatures 返回没有任何行为的观看者特征，用于匿名或特征不可用时。
func emptyViewerFeatures() model.ViewerFeatures {
	return model.ViewerFeatures{
		PositivePostIDs: make(map[int64]struct{}),
		NegativePostIDs: make(map[int64]struct{}),
		SeenPostIDs:     make(map[int64]struct{}),
		BlockedAuthors:  make(map[int64]struct{}),
	}
}

// ruleOnlyPostSources 过滤出非个性化的规则召回源（REL-023）。
// 关闭个性化后只使用热门/探索/内容冷启动，不使用行为或关系个性化召回。
func ruleOnlyPostSources(sources []model.PostRecallSource) []model.PostRecallSource {
	ruleOnly := map[string]struct{}{
		"hot":           {},
		"explore":       {},
		"content_hot":   {},
		"content_fresh": {},
	}
	result := make([]model.PostRecallSource, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		if _, ok := ruleOnly[source.Name()]; ok {
			result = append(result, source)
		}
	}
	return result
}

// containsID 判断集合是否包含 ID。
func containsID(ids map[int64]struct{}, id int64) bool {
	_, exists := ids[id]
	return exists
}

// isPublic 判断可见性是否为公开；缺失按公开处理。
func isPublic(visibility string) bool {
	return visibility == "" || strings.EqualFold(visibility, "public")
}

// boundedFeature 把非负特征压缩到 [0,1)，避免单个大值主导打分。
func boundedFeature(value float64) float64 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value <= 1 {
		return value
	}
	return value / (1 + value)
}

// sortPostCandidates 按最终分降序稳定排序，同分按帖子 ID 升序，保证结果可复现。
func sortPostCandidates(candidates []model.PostCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].PostID < candidates[j].PostID
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
}

// setPostModelVersion 统一设置候选的模型版本。
func setPostModelVersion(candidates []model.PostCandidate, version string) {
	for index := range candidates {
		candidates[index].ModelVersion = version
	}
}

// deterministicJitter 生成由种子与 ID 决定的微小扰动，同一请求结果稳定、不同请求略有变化。
func deterministicJitter(seed string, id int64) float64 {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, id)))
	value := binary.BigEndian.Uint64(digest[:8])
	return float64(value%1000) / 1_000_000
}
