package logic

import (
	"esx/app/content/rpc/internal/model"
	"esx/pkg/cursorx"
	"esx/pkg/errx"
)

// 帖子列表游标负载字段约定：
// b=排序模式 c=created_at(Unix秒) l=like_count v=view_count i=id（二级键）
//
// 编码总是携带全部字段，解码按目标列表的 keyset 列序取前缀，
// 与 model.listKeysetColumns / userPostsKeysetColumns 严格对应。

type postListKind int

const (
	postListGlobal postListKind = iota
	postListUserPosts
)

// encodePostCursor 从上一页边界行构造下一页游标。
func encodePostCursor(sortBy int, p *model.Post) (string, error) {
	return cursorx.Encode(cursorx.Data{
		"b": int64(sortBy),
		"c": p.CreatedAt.Unix(),
		"l": p.LikeCount,
		"v": p.ViewCount,
		"i": p.Id,
	})
}

// decodePostCursor 解析游标；空串返回 nil 表示首页。
// 非法游标统一返回 ParamError。
func decodePostCursor(token string, kind postListKind) (*model.PostListCursor, error) {
	if token == "" {
		return nil, nil
	}
	data, err := cursorx.Decode(token)
	if err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	sortBy := int(data["b"])
	switch sortBy {
	case model.SortByLatest, model.SortByHot, model.SortByViewed:
	default:
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if data["i"] <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	build := func(vals ...int64) *model.PostListCursor {
		return &model.PostListCursor{SortBy: sortBy, Vals: append(vals, data["i"])}
	}
	switch {
	case kind == postListUserPosts && sortBy == model.SortByHot:
		// (like_count, created_at, id)
		return build(data["l"], data["c"]), nil
	case kind == postListUserPosts:
		// (created_at, id)
		return build(data["c"]), nil
	case sortBy == model.SortByHot:
		// (like_count, id)
		return build(data["l"]), nil
	case sortBy == model.SortByViewed:
		// (view_count, id)
		return build(data["v"]), nil
	default:
		// 全局最新：(created_at, id)
		return build(data["c"]), nil
	}
}
