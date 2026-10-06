package logic

import (
	"context"
	"math"
	"sort"

	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/feed"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// 关注列表分页大小、outbox 每批作者数与关注列表最大页数。
const (
	followingLookupPageSize = 100
	outboxAuthorBatchSize   = 100
	maxFollowingLookupPages = 100
)

// GetFollowFeedLogic 承载 GetFollowFeed 接口的业务逻辑；每个请求新建一个实例。
type GetFollowFeedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetFollowFeedLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetFollowFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowFeedLogic {
	return &GetFollowFeedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetFollowFeed 返回关注流一页：推模式 inbox 与拉模式 outbox 合并，只保留当前仍关注的作者，
// 按 (created_at, post_id) 降序分页；不可见的帖子被跳过但仍推进游标。
func (l *GetFollowFeedLogic) GetFollowFeed(in *pb.GetFollowFeedReq) (*pb.GetFollowFeedResp, error) {
	if in == nil || in.UserId <= 0 || in.PageSize <= 0 || in.PageSize > maxFeedPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	cursorCreatedAt, cursorPostID := followFeedCursor(in)
	// 多取一行用于判断是否还有下一页。
	limit := int64(in.PageSize) + 1

	// 以实时关注列表为准，inbox 中已取关作者的旧推送会被过滤。
	authorIDs, followingSet, err := l.currentFollowingAuthorIDs(in.UserId)
	if err != nil {
		return nil, err
	}
	if len(authorIDs) == 0 {
		return &pb.GetFollowFeedResp{Items: []*pb.FeedItem{}}, nil
	}

	// 读取两路候选：inbox 是写扩散的推送；outbox 保存每位作者的全部帖子，
	// 补齐不做写扩散的大 V 以及推送时尚未关注的作者。
	inboxRows, err := l.svcCtx.InboxModel.FindByUserBefore(l.ctx, in.UserId, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		l.Errorw("InboxModel.FindByUserBefore failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxRows, outboxHasMore, err := l.outboxRowsForAuthors(authorIDs, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		return nil, err
	}
	rawItems := mergeFollowCandidates(inboxRows, outboxRows, followingSet)

	// 用内容服务补全帖子详情；未发布或已删除的帖子不会出现在结果中。
	rendered, err := enrichFeedItems(l.ctx, l.svcCtx.ContentService, rawItems)
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	items, lastScanned, hasUnscanned := takeVisiblePage(rawItems, rendered, int(in.PageSize))

	inboxHasMore := len(inboxRows) == int(limit)
	hasMore := hasUnscanned || inboxHasMore || outboxHasMore
	resp := &pb.GetFollowFeedResp{Items: items, HasMore: hasMore}
	if lastScanned != nil {
		resp.NextCursorCreatedAt = lastScanned.CreatedAt
		resp.NextCursorPostId = lastScanned.PostId
	} else if hasMore {
		// 本页没有任何可见项（例如 inbox 前 limit 行全部属于已取关作者，
		// 或候选全部未发布）：仍须推进游标，否则客户端无法翻页，
		// 更早的可见行会被永久跳过。
		fallback := oldestScannedRow(inboxRows, outboxRows)
		if fallback != nil {
			resp.NextCursorCreatedAt = fallback.CreatedAt
			resp.NextCursorPostId = fallback.PostId
		}
	}
	return resp, nil
}

// followFeedCursor 把缺省游标视为“从最新开始”。
func followFeedCursor(in *pb.GetFollowFeedReq) (int64, int64) {
	cursorCreatedAt := in.CursorCreatedAt
	cursorPostID := in.CursorPostId
	if cursorCreatedAt <= 0 {
		cursorCreatedAt = math.MaxInt64
	}
	if cursorPostID <= 0 {
		cursorPostID = math.MaxInt64
	}
	return cursorCreatedAt, cursorPostID
}

// mergeFollowCandidates 按帖子去重合并两路候选并降序排序。inbox 行必须属于仍关注的作者；
// outbox 已按关注列表查询。同一帖子两路都有时取较新的 created_at。
func mergeFollowCandidates(inboxRows []*model.FeedInbox, outboxRows []*model.FeedOutbox, followingSet map[int64]struct{}) []*pb.FeedItem {
	itemsByPostID := make(map[int64]*pb.FeedItem, len(inboxRows)+len(outboxRows))
	for _, row := range inboxRows {
		if row == nil || row.PostId <= 0 {
			continue
		}
		if _, ok := followingSet[row.AuthorId]; !ok {
			continue
		}
		itemsByPostID[row.PostId] = &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: feedTypeFollow}
	}
	for _, row := range outboxRows {
		if row == nil || row.PostId <= 0 {
			continue
		}
		candidate := &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: feedTypeFollow}
		if existing := itemsByPostID[row.PostId]; existing == nil || candidate.CreatedAt > existing.CreatedAt {
			itemsByPostID[row.PostId] = candidate
		}
	}
	rawItems := make([]*pb.FeedItem, 0, len(itemsByPostID))
	for _, item := range itemsByPostID {
		rawItems = append(rawItems, item)
	}
	// post_id 作为同一时间戳的决胜键，保证游标翻页顺序稳定。
	sort.Slice(rawItems, func(i, j int) bool {
		if rawItems[i].CreatedAt == rawItems[j].CreatedAt {
			return rawItems[i].PostId > rawItems[j].PostId
		}
		return rawItems[i].CreatedAt > rawItems[j].CreatedAt
	})
	return rawItems
}

// takeVisiblePage 按候选顺序取最多 pageSize 个可见项。lastScanned 是最后一个被扫描的候选
// （无论是否可见），作为下一页游标；hasUnscanned 表示还有候选没有扫描。
func takeVisiblePage(rawItems, rendered []*pb.FeedItem, pageSize int) ([]*pb.FeedItem, *pb.FeedItem, bool) {
	renderedByPostID := make(map[int64]*pb.FeedItem, len(rendered))
	for _, item := range rendered {
		renderedByPostID[item.PostId] = item
	}
	items := make([]*pb.FeedItem, 0, pageSize)
	var lastScanned *pb.FeedItem
	for _, rawItem := range rawItems {
		if len(items) == pageSize {
			return items, lastScanned, true
		}
		lastScanned = rawItem
		if item := renderedByPostID[rawItem.PostId]; item != nil {
			items = append(items, item)
		}
	}
	return items, lastScanned, false
}

// oldestScannedRow 返回本页已扫描的最旧行（created_at, post_id 字典序最小），
// 用于空可见页的游标推进；inbox/outbox 均为降序返回。
func oldestScannedRow(inboxRows []*model.FeedInbox, outboxRows []*model.FeedOutbox) *model.FeedInbox {
	if len(inboxRows) == 0 {
		return nil
	}
	oldest := inboxRows[len(inboxRows)-1]
	if len(outboxRows) == 0 {
		return oldest
	}
	lastOutbox := outboxRows[len(outboxRows)-1]
	if lastOutbox != nil && (lastOutbox.CreatedAt < oldest.CreatedAt ||
		(lastOutbox.CreatedAt == oldest.CreatedAt && lastOutbox.PostId < oldest.PostId)) {
		return &model.FeedInbox{UserId: 0, AuthorId: lastOutbox.AuthorId, PostId: lastOutbox.PostId, CreatedAt: lastOutbox.CreatedAt}
	}
	return oldest
}

// currentFollowingAuthorIDs 分页拉取完整关注列表并去重；超过页数上限视为异常而不是截断，
// 截断会让部分作者的帖子静默消失。
func (l *GetFollowFeedLogic) currentFollowingAuthorIDs(userID int64) ([]int64, map[int64]struct{}, error) {
	authorIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for page := int32(1); page <= maxFollowingLookupPages; page++ {
		followingResp, err := l.svcCtx.UserService.GetFollowing(l.ctx, &userservice.GetFollowingReq{
			UserId:   userID,
			Page:     page,
			PageSize: followingLookupPageSize,
		})
		if err != nil {
			l.Errorw("UserService.GetFollowing failed", logging.Field("err", err.Error()), logging.Field("page", page))
			return nil, nil, errx.NewWithCode(errx.SystemError)
		}
		if followingResp == nil {
			l.Error("UserService.GetFollowing returned a nil response")
			return nil, nil, errx.NewWithCode(errx.SystemError)
		}
		for _, user := range followingResp.Users {
			if user == nil || user.Id <= 0 {
				continue
			}
			if _, exists := seen[user.Id]; exists {
				continue
			}
			seen[user.Id] = struct{}{}
			authorIDs = append(authorIDs, user.Id)
		}
		if len(followingResp.Users) < int(followingLookupPageSize) {
			return authorIDs, seen, nil
		}
		if followingResp.Total > 0 && int64(len(authorIDs)) >= followingResp.Total {
			return authorIDs, seen, nil
		}
	}
	l.Errorw("following lookup exceeded page cap", logging.Field("userId", userID), logging.Field("loaded", len(authorIDs)))
	return nil, nil, errx.NewWithCode(errx.SystemError)
}

// outboxRowsForAuthors 按批查询作者 outbox 并合并降序；任一批次取满 limit 即说明可能还有更多。
func (l *GetFollowFeedLogic) outboxRowsForAuthors(authorIDs []int64, cursorCreatedAt, cursorPostID, limit int64) ([]*model.FeedOutbox, bool, error) {
	merged := make([]*model.FeedOutbox, 0)
	hasMore := false
	for start := 0; start < len(authorIDs); start += outboxAuthorBatchSize {
		end := min(start+outboxAuthorBatchSize, len(authorIDs))
		rows, err := l.svcCtx.OutboxModel.FindByAuthorsBefore(l.ctx, authorIDs[start:end], cursorCreatedAt, cursorPostID, limit)
		if err != nil {
			l.Errorw("OutboxModel.FindByAuthorsBefore failed", logging.Field("err", err.Error()))
			return nil, false, errx.NewWithCode(errx.SystemError)
		}
		if len(rows) == int(limit) {
			hasMore = true
		}
		merged = append(merged, rows...)
	}
	if len(merged) == 0 {
		return merged, hasMore, nil
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i] == nil || merged[j] == nil {
			return merged[j] == nil
		}
		if merged[i].CreatedAt == merged[j].CreatedAt {
			return merged[i].PostId > merged[j].PostId
		}
		return merged[i].CreatedAt > merged[j].CreatedAt
	})
	return merged, hasMore, nil
}
