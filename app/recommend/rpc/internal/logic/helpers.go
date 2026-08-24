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

type postRecallResult struct {
	index      int
	candidates []model.PostCandidate
	err        error
}

type userRecallResult struct {
	index      int
	candidates []model.UserCandidate
	err        error
}

func markPostDegradation(candidates []model.PostCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

func markUserDegradation(candidates []model.UserCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

func recommendationRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errx.Wrap(err, errx.SystemError)
	}
	return errx.Wrap(err, errx.ServiceUnavailable)
}

func normalizedScene(scene string) string {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return "home"
	}
	return scene
}

func validIdentity(userID int64, anonymousID string) bool {
	anonymousID = strings.TrimSpace(anonymousID)
	return userID >= 0 && len(anonymousID) <= 256 && (userID > 0 || anonymousID != "")
}

func validRequestMetadata(requestID, scene, sessionID, experimentID string) bool {
	requestID = strings.TrimSpace(requestID)
	return requestID != "" && len(requestID) <= 128 && len(strings.TrimSpace(scene)) <= 64 &&
		len(strings.TrimSpace(sessionID)) <= 128 && len(strings.TrimSpace(experimentID)) <= 128
}

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

func candidateLimit(pageSize int, c config.Config) int {
	multiplier := c.CandidateMultiplier
	if multiplier <= 0 {
		multiplier = defaultCandidateMultiple
	}
	return pageSize * multiplier
}

func cursorTTL(c config.Config) int {
	if c.CursorTTLSeconds > 0 {
		return c.CursorTTLSeconds
	}
	return defaultCursorTTLSeconds
}

func ruleModelVersion(c config.Config) string {
	if c.RuleModelVersion != "" {
		return c.RuleModelVersion
	}
	return "rules-v2"
}

func appendSource(current, next string) string {
	if next == "" || hasSource(current, next) {
		return current
	}
	if current == "" {
		return next
	}
	return current + "," + next
}

func hasSource(sources, source string) bool {
	for current := range strings.SplitSeq(sources, ",") {
		if current == source {
			return true
		}
	}
	return false
}

func appendDegradation(version, degradation string) string {
	if degradation == "" || strings.Contains(version, degradation) {
		return version
	}
	if version == "" {
		return degradation
	}
	return version + "+" + degradation
}

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

func containsID(ids map[int64]struct{}, id int64) bool {
	_, exists := ids[id]
	return exists
}

func isPublic(visibility string) bool {
	return visibility == "" || strings.EqualFold(visibility, "public")
}

func boundedFeature(value float64) float64 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value <= 1 {
		return value
	}
	return value / (1 + value)
}

func sortPostCandidates(candidates []model.PostCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].PostID < candidates[j].PostID
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
}

func setPostModelVersion(candidates []model.PostCandidate, version string) {
	for index := range candidates {
		candidates[index].ModelVersion = version
	}
}

func deterministicJitter(seed string, id int64) float64 {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, id)))
	value := binary.BigEndian.Uint64(digest[:8])
	return float64(value%1000) / 1_000_000
}
