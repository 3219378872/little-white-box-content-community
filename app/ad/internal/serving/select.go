package serving

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strconv"
)

// SlotPositions 是默认槽位：第 4 与第 12 条帖子之后，每页最多 2 个（DES-sponsored-ads「选择」）。
var SlotPositions = []int{4, 12}

// Candidate 是通过资格校验的广告。
type Candidate struct {
	Entry      Entry
	TodayCount int64
}

// Choice 是一个选中的槽位。
type Choice struct {
	SlotID     string
	AfterIndex int
	Entry      Entry
}

// Choose 用可解释的规则选择广告（ADS-041）：排除已达频控上限者，同一页同一广告主最多一个，
// 优先当日下发最少者，同分按 requestId 种子的稳定随机排序。不使用任何个性化特征（ADS-025）。
func Choose(requestID string, pageItems int, candidates []Candidate) []Choice {
	positions := make([]int, 0, len(SlotPositions))
	for _, p := range SlotPositions {
		if p <= pageItems {
			positions = append(positions, p)
		}
	}
	if len(positions) == 0 {
		return nil
	}
	eligible := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.TodayCount < DailyCap {
			eligible = append(eligible, c)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].TodayCount != eligible[j].TodayCount {
			return eligible[i].TodayCount < eligible[j].TodayCount
		}
		hi, hj := tieBreak(requestID, eligible[i].Entry.AdID), tieBreak(requestID, eligible[j].Entry.AdID)
		if hi != hj {
			return hi < hj
		}
		return eligible[i].Entry.AdID < eligible[j].Entry.AdID
	})
	used := map[int64]bool{}
	var out []Choice
	for _, c := range eligible {
		if len(out) == len(positions) {
			break
		}
		if used[c.Entry.AdvertiserID] {
			continue
		}
		used[c.Entry.AdvertiserID] = true
		out = append(out, Choice{SlotID: "s" + strconv.Itoa(len(out)+1), AfterIndex: positions[len(out)], Entry: c.Entry})
	}
	return out
}

// tieBreak 为同分候选生成按请求稳定、跨请求打散的排序键，同一请求重试得到相同顺序。
func tieBreak(requestID string, adID int64) uint64 {
	sum := sha256.Sum256([]byte(requestID + ":" + strconv.FormatInt(adID, 10)))
	return binary.BigEndian.Uint64(sum[:8])
}

// InWindow 报告当前时间是否在投放期内（0 表示不限）。
func (e Entry) InWindow(nowMs int64) bool {
	return (e.StartMs == 0 || nowMs >= e.StartMs) && (e.EndMs == 0 || nowMs < e.EndMs)
}
