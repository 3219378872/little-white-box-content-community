package logic

import (
	"context"
	"errors"
	"unicode/utf8"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/logging"
	"esx/pkg/util"
	"esx/pkg/visibilityx"
)

// 帖子字段上限；创建与更新共用，更新时对合并后的完整字段校验。
const (
	maxPostTitleRunes   = 120
	maxPostContentRunes = 20000
	maxPostImages       = 9
	maxPostMediaIDs     = 9
	maxPostTags         = 10
	maxPostTagRunes     = 32
)

// validatePostText 校验标题与正文长度：空值与超长分别返回不同错误码，便于客户端提示。
func validatePostText(title, content string) error {
	titleRunes := utf8.RuneCountInString(title)
	if titleRunes < 1 {
		return errx.NewWithCode(errx.TitleEmpty)
	}
	if titleRunes > maxPostTitleRunes {
		return errx.NewWithCode(errx.ContentTooLong)
	}
	contentRunes := utf8.RuneCountInString(content)
	if contentRunes < 1 {
		return errx.NewWithCode(errx.ContentEmpty)
	}
	if contentRunes > maxPostContentRunes {
		return errx.NewWithCode(errx.ContentTooLong)
	}
	return nil
}

// validatePostCollections 校验图片、标签、媒体的数量与标签长度；空标签会被忽略，不计入长度校验。
func validatePostCollections(images, tags []string, mediaIDs []int64) error {
	if len(images) > maxPostImages || len(tags) > maxPostTags || len(mediaIDs) > maxPostMediaIDs {
		return errx.NewWithCode(errx.ParamError)
	}
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxPostTagRunes {
			return errx.NewWithCode(errx.ParamError)
		}
	}
	return nil
}

// allocateTagIDs 过滤空标签并为每个标签预生成分布式 ID；ID 在事务外生成，事务内只做写入。
func allocateTagIDs(logger logging.Logger, tags []string) ([]string, []int64, error) {
	values := make([]string, 0, len(tags))
	ids := make([]int64, 0, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		id, err := util.NextID()
		if err != nil {
			logger.Errorw("generate tag id failed", logging.Field("err", err.Error()))
			return nil, nil, errx.NewWithCode(errx.SystemError)
		}
		values = append(values, tag)
		ids = append(ids, id)
	}
	return values, ids, nil
}

// replayIdempotentPostCommand 在带幂等键的更新/删除前查找已提交的同一命令。
// 返回可执行幂等写入的模型；found=true 时 result 是首次执行编码后的结果。
// 无幂等键时直接返回，命令按普通写入执行。
func replayIdempotentPostCommand(ctx context.Context, svcCtx *svc.ServiceContext, idem idempotencyx.IdempotencyRecord) (model.IdempotentPostCommandModel, int64, bool, error) {
	idemModel, idemEnabled := svcCtx.PostCommandModel.(model.IdempotentPostCommandModel)
	if idem.Key == "" {
		return idemModel, 0, false, nil
	}
	// 带幂等键的请求必须由支持幂等的模型处理，否则无法保证重试只执行一次。
	if !idemEnabled {
		return nil, 0, false, errx.NewWithCode(errx.SystemError)
	}
	result, found, err := idemModel.ReplayPostCommand(ctx, idem)
	if err != nil {
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, 0, false, errx.NewWithCode(errx.IdempotencyConflict)
		}
		return nil, 0, false, errx.NewWithCode(errx.SystemError)
	}
	return idemModel, result, found, nil
}

// loadOwnedPost 读取待修改的帖子并确认：未删除、属于作者、且修订号等于客户端期望值。
// 读到的帖子只用于鉴权与合并现值，写入时仍由模型按期望修订号 CAS，防止丢失更新。
func loadOwnedPost(ctx context.Context, logger logging.Logger, svcCtx *svc.ServiceContext, postID, authorID, expectedRevision int64) (*model.Post, error) {
	post, err := svcCtx.PostModel.FindPostById(ctx, postID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		logger.Errorw("PostModel.FindPostById failed",
			logging.Field("postId", postID),
			logging.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if visibilityx.IsDeleted(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}
	if post.AuthorId != authorID {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}
	if expectedRevision <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if post.Revision != expectedRevision {
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}
	return post, nil
}

// postCommandError 把更新/删除事务的错误映射为业务错误码；未知错误记录日志后统一为系统错误。
func postCommandError(logger logging.Logger, action string, postID int64, err error) error {
	if errors.Is(err, model.ErrVersionConflict) {
		return errx.NewWithCode(errx.ContentVersionConflict)
	}
	if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
		return errx.NewWithCode(errx.IdempotencyConflict)
	}
	logger.Errorw(action+" post transaction failed",
		logging.Field("postId", postID), logging.Field("err", err.Error()))
	return errx.NewWithCode(errx.SystemError)
}

// invalidatePostCacheAfter 在事务提交后清理帖子缓存。失败只记日志：数据已提交，
// 缓存会按 TTL 过期，不应让已成功的写操作对客户端报错。
func invalidatePostCacheAfter(ctx context.Context, logger logging.Logger, svcCtx *svc.ServiceContext, action string, postID int64) {
	if err := svcCtx.PostModel.InvalidatePostCache(ctx, postID); err != nil {
		logger.Errorw("invalidate post cache after "+action+" failed",
			logging.Field("postId", postID), logging.Field("err", err.Error()))
	}
}
