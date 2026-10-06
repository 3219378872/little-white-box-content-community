package logic

import (
	"crypto/rand"
	"errors"
	"fmt"

	"esx/app/content/rpc/contentservice"
	"esx/app/feed/rpc/internal/model"
	pb "esx/kitex_gen/feed"
	"esx/pkg/errx"

	"esx/pkg/logging"

	"google.golang.org/protobuf/proto"
)

// startFallback 在推荐服务不可用时开始一次规则兜底：登录用户依次轮询关注、热门、最新三路，
// 匿名用户只有热门与最新。
func (l *GetRecommendFeedLogic) startFallback(in *pb.GetRecommendFeedReq, binding model.FallbackBinding) (*pb.GetRecommendFeedResp, error) {
	sources := make([]model.FallbackSource, 0, 3)
	if in.UserId > 0 && l.svcCtx.InboxModel != nil && l.svcCtx.OutboxModel != nil && l.svcCtx.UserService != nil {
		sources = append(sources, model.FallbackSource{Name: "follow"})
	}
	sources = append(sources, model.FallbackSource{Name: "popular"}, model.FallbackSource{Name: "latest"})
	state := model.FallbackState{Binding: binding, ExpiresAt: l.now().Add(fallbackCursorTTL).Unix(), Sources: sources}
	return l.fallbackPage(in, state)
}

// continueFallback 按游标载入兜底状态续读；状态必须与本次请求绑定一致且结构合法，否则视为参数错误。
func (l *GetRecommendFeedLogic) continueFallback(in *pb.GetRecommendFeedReq, binding model.FallbackBinding, id string, expiresAt int64) (*pb.GetRecommendFeedResp, error) {
	if l.svcCtx.FallbackStates == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	state, err := l.svcCtx.FallbackStates.Load(l.ctx, id)
	if errors.Is(err, model.ErrFallbackStateMissing) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err != nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	if state.Binding != binding || state.ExpiresAt != expiresAt || state.ExpiresAt <= l.now().Unix() || len(state.Sources) < 2 || len(state.Sources) > 3 || state.NextSource < 0 || state.NextSource >= len(state.Sources) || state.Position < 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return l.fallbackPage(in, state)
}

// fallbackPage 从兜底状态产出一页：补充候选、过滤不可见/已隐藏/已下发的帖子，
// 有剩余时保存新状态并签发下一页游标。
func (l *GetRecommendFeedLogic) fallbackPage(in *pb.GetRecommendFeedReq, state model.FallbackState) (*pb.GetRecommendFeedResp, error) {
	if l.svcCtx.ContentService == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	// 登录用户必须能读到负反馈，否则宁可失败也不下发已被隐藏的帖子。
	hidden := make(map[int64]struct{})
	if in.UserId > 0 {
		if l.svcCtx.NegativeFeedback == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		var err error
		hidden, err = l.svcCtx.NegativeFeedback.HiddenPosts(l.ctx, in.UserId)
		if err != nil {
			l.Errorw("fallback negative feedback unavailable", logging.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
	}
	// Refill each source at most once per request. Unconsumed candidates and an
	// exhausted marker are persisted, so an empty cursor never restarts a source.
	failures := 0
	for i := range state.Sources {
		source := &state.Sources[i]
		if len(source.Pending) == 0 && !source.Exhausted {
			if err := l.refillFallbackSource(in, source); err != nil {
				l.Errorw("fallback source unavailable", logging.Field("source", source.Name), logging.Field("err", err.Error()))
				failures++
			}
		}
	}
	// 一次性批量补全所有候选，避免逐路请求内容服务。
	visible, err := l.visibleFallbackCandidates(in.UserId, state.Sources)
	if err != nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	seen := make(map[int64]struct{}, len(state.Seen))
	for _, id := range state.Seen {
		seen[id] = struct{}{}
	}
	// 按来源轮询取候选，直到凑满一页或候选耗尽。
	items := make([]*pb.FeedItem, 0, in.PageSize)
	for len(items) < int(in.PageSize) {
		candidate, source, found := nextFallbackCandidate(&state)
		if !found {
			break
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		if _, excluded := hidden[candidate]; excluded {
			continue
		}
		item := visible[source+":"+fmt.Sprint(candidate)]
		if item == nil {
			continue
		}
		state.Position++
		item.Position = state.Position
		item.ExperimentId = state.Binding.ExperimentID
		items = append(items, item)
		state.Seen = append(state.Seen, candidate)
		seen[candidate] = struct{}{}
	}
	// 所有来源都失败且没有可下发内容时报告不可用，而不是返回空页让客户端误以为已到底。
	if len(items) == 0 && failures > 0 {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	hasMore := false
	for _, source := range state.Sources {
		hasMore = hasMore || len(source.Pending) > 0 || !source.Exhausted
	}
	response := &pb.GetRecommendFeedResp{Items: items, HasMore: hasMore, RequestId: state.Binding.RequestID}
	// 每页保存为新状态 ID，旧游标对应的状态不被改写。
	if hasMore {
		if l.svcCtx.FallbackStates == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		ttl := int(state.ExpiresAt - l.now().Unix())
		if ttl <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		id := rand.Text()
		if err := l.svcCtx.FallbackStates.Save(l.ctx, id, state, ttl); err != nil {
			l.Errorw("save fallback state failed", logging.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response.NextCursor, err = encodeFallbackCursor(l.svcCtx.Config.CursorSecret, id, state.Binding, state.ExpiresAt, l.now())
		if err != nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
	}
	return response, nil
}

// nextFallbackCandidate 从当前来源起轮询，取出下一个待下发候选并推进轮询位置。
func nextFallbackCandidate(state *model.FallbackState) (int64, string, bool) {
	for range state.Sources {
		index := state.NextSource
		state.NextSource = (index + 1) % len(state.Sources)
		source := &state.Sources[index]
		if len(source.Pending) == 0 {
			continue
		}
		id := source.Pending[0]
		source.Pending = source.Pending[1:]
		return id, source.Name, true
	}
	return 0, "", false
}

// refillFallbackSource 为一路来源再拉取一页候选；游标不前进时报错，防止反复读取同一页。
func (l *GetRecommendFeedLogic) refillFallbackSource(in *pb.GetRecommendFeedReq, source *model.FallbackSource) error {
	if source.Name == "follow" {
		response, err := NewGetFollowFeedLogic(l.ctx, l.svcCtx).GetFollowFeed(&pb.GetFollowFeedReq{UserId: in.UserId, PageSize: in.PageSize, CursorCreatedAt: source.CursorCreatedAt, CursorPostId: source.CursorPostID})
		if err != nil {
			return err
		}
		if response == nil {
			return fmt.Errorf("nil follow response")
		}
		nextCreatedAt, nextPostID := response.NextCursorCreatedAt, response.NextCursorPostId
		if response.HasMore && (nextCreatedAt <= 0 || nextPostID <= 0 ||
			(source.CursorCreatedAt > 0 && (nextCreatedAt > source.CursorCreatedAt ||
				(nextCreatedAt == source.CursorCreatedAt && nextPostID >= source.CursorPostID)))) {
			return fmt.Errorf("fallback follow cursor did not advance")
		}
		for _, item := range response.Items {
			if item != nil && item.PostId > 0 {
				source.Pending = append(source.Pending, item.PostId)
			}
		}
		source.Exhausted = !response.HasMore
		source.CursorCreatedAt, source.CursorPostID = response.NextCursorCreatedAt, response.NextCursorPostId
		return nil
	}
	// popular 按浏览量排序（内容服务 SortBy=3），latest 按发布时间排序。
	sortBy := int32(1)
	if source.Name == "popular" {
		sortBy = 3
	} else if source.Name != "latest" {
		return fmt.Errorf("unknown fallback source")
	}
	response, err := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{PageSize: in.PageSize, SortBy: sortBy, Cursor: source.Cursor})
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("nil content response")
	}
	if response.NextCursor != "" && response.NextCursor == source.Cursor {
		return fmt.Errorf("fallback source cursor did not advance")
	}
	for _, post := range response.Posts {
		if post != nil && post.Id > 0 {
			source.Pending = append(source.Pending, post.Id)
		}
	}
	source.Cursor = response.NextCursor
	source.Exhausted = source.Cursor == ""
	return nil
}

// visibleFallbackCandidates 补全候选详情并标注兜底来源，按 "来源:帖子ID" 索引；
// 关注来源还要过滤已取关作者的帖子。
func (l *GetRecommendFeedLogic) visibleFallbackCandidates(userID int64, sources []model.FallbackSource) (map[string]*pb.FeedItem, error) {
	base := make([]*pb.FeedItem, 0)
	for _, source := range sources {
		for _, id := range source.Pending {
			base = append(base, &pb.FeedItem{PostId: id})
		}
	}
	items, err := enrichFeedItems(l.ctx, l.svcCtx.ContentService, base)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*pb.FeedItem, len(items))
	for _, item := range items {
		byID[item.PostId] = item
	}
	result := make(map[string]*pb.FeedItem)
	for _, source := range sources {
		if len(source.Pending) == 0 {
			continue
		}
		var following map[int64]struct{}
		if source.Name == "follow" {
			_, following, err = NewGetFollowFeedLogic(l.ctx, l.svcCtx).currentFollowingAuthorIDs(userID)
			if err != nil {
				return nil, err
			}
		}
		for _, id := range source.Pending {
			item := byID[id]
			if item == nil {
				continue
			}
			if source.Name == "follow" {
				if _, ok := following[item.AuthorId]; !ok {
					continue
				}
			}
			copy := proto.Clone(item).(*pb.FeedItem)
			copy.FeedType = feedTypeRecommend
			copy.Reason = source.Name + " fallback"
			copy.RecallSource = source.Name
			copy.ModelVersion = "rule-fallback-v2"
			result[source.Name+":"+fmt.Sprint(id)] = copy
		}
	}
	return result, nil
}
