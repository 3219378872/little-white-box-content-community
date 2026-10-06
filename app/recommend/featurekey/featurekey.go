// Package featurekey 集中推荐在线特征的 Redis 键格式与个性化身份。
// recommend-mq 写入、recommend-rpc 与 feedback 读取同一批键，任何一侧单独改格式都会让读写错位，
// 所以键只在这里拼接。
package featurekey

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// OptOutKeyPrefix 是 user 服务关闭个性化时写入的标记前缀（REL-023），完整键为 `<前缀><userID>`。
const OptOutKeyPrefix = "personalization:optout:"

// 登录用户与匿名设备的身份前缀；follower 集合等处按前缀区分两类身份。
const (
	UserIdentityPrefix      = "u:"
	AnonymousIdentityPrefix = "a:"
)

// Identity 返回特征键中的观看者身份：登录用户用 `u:<id>`，匿名设备用 `a:<摘要>`，两者都缺失时为空。
func Identity(userID int64, anonymousID string) string {
	if userID > 0 {
		return UserIdentityPrefix + strconv.FormatInt(userID, 10)
	}
	if anonymousID == "" {
		return ""
	}
	// 匿名 ID 只保留 SHA-256 前 8 字节的十六进制，避免把设备标识原样写进键名。
	digest := sha256.Sum256([]byte(anonymousID))
	return AnonymousIdentityPrefix + hex.EncodeToString(digest[:8])
}

// OptOutKey 返回某个用户的个性化关闭标记键。
func OptOutKey(userID int64) string {
	return OptOutKeyPrefix + strconv.FormatInt(userID, 10)
}

// Space 是某个特征版本下的键空间；版本切换时新旧特征互不覆盖。
type Space struct {
	version string
}

// New 绑定特征版本（如 `v2`）。
func New(version string) Space {
	return Space{version: version}
}

// prefix 是该版本全部特征键的公共前缀。
func (s Space) prefix() string {
	return "feature:" + s.version + ":"
}

// Viewer 返回观看者特征键前缀；调用方追加 `:recent`、`:positive` 等后缀得到具体键。
func (s Space) Viewer(identity string) string {
	return s.prefix() + identity
}

// ViewerRecent 是观看者最近行为列表的键，召回种子与已曝光过滤都从这里读取。
func (s Space) ViewerRecent(identity string) string {
	return s.Viewer(identity) + ":recent"
}

// ViewerNegative 是观看者负反馈哈希的键，隐藏帖子过滤读取它。
func (s Space) ViewerNegative(identity string) string {
	return s.Viewer(identity) + ":negative"
}

// LoggedInViewerPattern 匹配全部登录用户的特征键，供关闭个性化后的清理任务枚举。
func (s Space) LoggedInViewerPattern() string {
	return s.prefix() + UserIdentityPrefix + "*"
}

// Dedup 是行为事件的去重键，保证同一事件重复投递只计一次。
func (s Space) Dedup(eventID string) string {
	return s.prefix() + "dedup:" + eventID
}

// Post 是帖子特征哈希的键。
func (s Space) Post(postID int64) string {
	return s.prefix() + "post:" + strconv.FormatInt(postID, 10)
}

// User 是被推荐用户（作者）特征哈希的键。
func (s Space) User(userID int64) string {
	return s.prefix() + "user:" + strconv.FormatInt(userID, 10)
}
