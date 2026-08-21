package logic

import (
	"context"
	"math"
	"sort"

	"esx/app/feed/rpc/internal/model"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/user/rpc/userservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	followingLookupPageSize = 100
	outboxAuthorBatchSize   = 100
	maxFollowingLookupPages = 100
)

type GetFollowFeedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFollowFeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowFeedLogic {
	return &GetFollowFeedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetFollowFeedLogic) GetFollowFeed(in *pb.GetFollowFeedReq) (*pb.GetFollowFeedResp, error) {
	if in == nil || in.UserId <= 0 || in.PageSize <= 0 || in.PageSize > maxFeedPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	cursorCreatedAt := in.CursorCreatedAt
	cursorPostID := in.CursorPostId
	if cursorCreatedAt <= 0 {
		cursorCreatedAt = math.MaxInt64
	}
	if cursorPostID <= 0 {
		cursorPostID = math.MaxInt64
	}
	limit := int64(in.PageSize) + 1

	authorIDs, followingSet, err := l.currentFollowingAuthorIDs(in.UserId)
	if err != nil {
		return nil, err
	}
	if len(authorIDs) == 0 {
		return &pb.GetFollowFeedResp{Items: []*pb.FeedItem{}}, nil
	}

	inboxRows, err := l.svcCtx.InboxModel.FindByUserBefore(l.ctx, in.UserId, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		l.Errorw("InboxModel.FindByUserBefore failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	outboxRows, outboxHasMore, err := l.outboxRowsForAuthors(authorIDs, cursorCreatedAt, cursorPostID, limit)
	if err != nil {
		return nil, err
	}

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
	sort.Slice(rawItems, func(i, j int) bool {
		if rawItems[i].CreatedAt == rawItems[j].CreatedAt {
			return rawItems[i].PostId > rawItems[j].PostId
		}
		return rawItems[i].CreatedAt > rawItems[j].CreatedAt
	})

	rendered, err := enrichFeedItems(l.ctx, l.svcCtx.ContentService, rawItems)
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	renderedByPostID := make(map[int64]*pb.FeedItem, len(rendered))
	for _, item := range rendered {
		renderedByPostID[item.PostId] = item
	}

	items := make([]*pb.FeedItem, 0, in.PageSize)
	var lastScanned *pb.FeedItem
	hasUnscanned := false
	for _, rawItem := range rawItems {
		if len(items) == int(in.PageSize) {
			hasUnscanned = true
			break
		}
		lastScanned = rawItem
		if item := renderedByPostID[rawItem.PostId]; item != nil {
			items = append(items, item)
		}
	}
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
			l.Errorw("UserService.GetFollowing failed", logx.Field("err", err.Error()), logx.Field("page", page))
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
	l.Errorw("following lookup exceeded page cap", logx.Field("userId", userID), logx.Field("loaded", len(authorIDs)))
	return nil, nil, errx.NewWithCode(errx.SystemError)
}

func (l *GetFollowFeedLogic) outboxRowsForAuthors(authorIDs []int64, cursorCreatedAt, cursorPostID, limit int64) ([]*model.FeedOutbox, bool, error) {
	merged := make([]*model.FeedOutbox, 0)
	hasMore := false
	for start := 0; start < len(authorIDs); start += outboxAuthorBatchSize {
		end := start + outboxAuthorBatchSize
		if end > len(authorIDs) {
			end = len(authorIDs)
		}
		rows, err := l.svcCtx.OutboxModel.FindByAuthorsBefore(l.ctx, authorIDs[start:end], cursorCreatedAt, cursorPostID, limit)
		if err != nil {
			l.Errorw("OutboxModel.FindByAuthorsBefore failed", logx.Field("err", err.Error()))
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
