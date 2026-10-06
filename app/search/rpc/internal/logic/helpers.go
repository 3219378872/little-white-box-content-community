package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/search/rpc/internal/store"
	"esx/app/search/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/search"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"
)

// maxPageSize caps one search page.
const maxPageSize = 100

// keyword trims the query and rejects an empty one with SearchEmpty.
func keyword(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errx.NewWithCode(errx.SearchEmpty)
	}
	return value, nil
}

// validPage checks the page size and that the whole page fits inside ES's
// result window, so deep paging is rejected instead of failing in ES.
func validPage(page, pageSize int32) bool {
	if page <= 0 || pageSize <= 0 || pageSize > maxPageSize {
		return false
	}
	offset := int64(page-1) * int64(pageSize)
	return offset < store.MaxResultWindow && offset+int64(pageSize) <= store.MaxResultWindow
}

// storeError maps store failures: timeouts and cancellation become SearchTimeout,
// anything else ServiceUnavailable.
func storeError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errx.Wrap(err, errx.SearchTimeout)
	}
	return errx.Wrap(err, errx.ServiceUnavailable)
}

// loadAuthorCards reads author display cards. Callers leave author fields
// empty on failure; author ID stays in the result.
func loadAuthorCards(
	ctx context.Context,
	userService svc.UserService,
	posts []store.Post,
) (map[int64]*userservice.UserCard, error) {
	ids := make([]int64, 0, len(posts))
	seen := make(map[int64]struct{}, cap(ids))
	appendID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, post := range posts {
		appendID(post.AuthorID)
	}
	if len(ids) == 0 {
		return map[int64]*userservice.UserCard{}, nil
	}
	if userService == nil {
		return nil, fmt.Errorf("search user service is unavailable")
	}
	response, err := userService.BatchGetUserCards(ctx, &userservice.BatchGetUserCardsReq{UserIds: ids})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("user service returned a nil response")
	}
	profiles := make(map[int64]*userservice.UserCard, len(response.Users))
	for _, user := range response.Users {
		if user != nil && user.Id > 0 {
			profiles[user.Id] = user
		}
	}
	return profiles, nil
}

// postResults converts hits to responses; the author's nickname falls back to
// the username, and a missing profile leaves author fields empty.
func postResults(posts []store.Post, profiles map[int64]*userservice.UserCard) []*pb.PostSearchResult {
	result := make([]*pb.PostSearchResult, 0, len(posts))
	for _, post := range posts {
		authorName := ""
		authorAvatar := ""
		if profile := profiles[post.AuthorID]; profile != nil {
			authorName = strings.TrimSpace(profile.Nickname)
			if authorName == "" {
				authorName = strings.TrimSpace(profile.Username)
			}
			authorAvatar = strings.TrimSpace(profile.AvatarUrl)
		}
		result = append(result, &pb.PostSearchResult{
			Id: post.ID, Title: post.Title, ContentHighlight: post.ContentHighlight,
			AuthorId: post.AuthorID, AuthorName: authorName, AuthorAvatar: authorAvatar,
			LikeCount: post.LikeCount, CommentCount: post.CommentCount, CreatedAt: post.CreatedAt,
		})
	}
	return result
}

// userResults converts user search hits, dropping nil or invalid entries.
func userResults(users []*userservice.UserInfo) []*pb.UserSearchResult {
	result := make([]*pb.UserSearchResult, 0, len(users))
	for _, user := range users {
		if user == nil || user.Id <= 0 {
			continue
		}
		result = append(result, &pb.UserSearchResult{
			Id: user.Id, Username: user.Username, Nickname: user.Nickname,
			AvatarUrl: user.AvatarUrl, Bio: user.Bio, FollowerCount: user.FollowerCount,
		})
	}
	return result
}

// tagResults converts tag hits to responses.
func tagResults(tags []store.Tag) []*pb.TagSearchResult {
	result := make([]*pb.TagSearchResult, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &pb.TagSearchResult{Name: tag.Name, PostCount: tag.PostCount})
	}
	return result
}

// publishedSearchPosts 用 Content 权威状态过滤不可见帖子，并回填标题/摘要。
// 可见性无法验证时整体失败（DISC-001 / DISC-021 / DISC-041）。
func publishedSearchPosts(ctx context.Context, content svc.ContentService, posts []store.Post) ([]store.Post, error) {
	if len(posts) == 0 {
		return posts, nil
	}
	ids := make([]int64, 0, len(posts))
	for _, post := range posts {
		ids = append(ids, post.ID)
	}
	published, err := visibility.PublishedByIDsWithoutTags(ctx, content, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]store.Post, 0, len(posts))
	emitted := make(map[int64]struct{}, len(published))
	for _, post := range posts {
		live, ok := published[post.ID]
		if !ok {
			continue
		}
		if _, exists := emitted[post.ID]; exists {
			continue
		}
		filtered = append(filtered, hydrateSearchPost(post, live))
		emitted[post.ID] = struct{}{}
	}
	return filtered, nil
}

// hydrateSearchPost overwrites indexed fields with the live post so a stale
// index never shows an outdated title, counts or body excerpt.
func hydrateSearchPost(candidate store.Post, live *contentservice.PostInfo) store.Post {
	hydrated := candidate
	hydrated.Title = strings.TrimSpace(live.Title)
	if live.AuthorId > 0 {
		hydrated.AuthorID = live.AuthorId
	}
	if live.CreatedAt > 0 {
		hydrated.CreatedAt = live.CreatedAt
	}
	hydrated.LikeCount = live.LikeCount
	hydrated.CommentCount = live.CommentCount
	hydrated.ContentHighlight = publishedSearchSummary(live.Content, candidate.ContentHighlight)
	return hydrated
}

const searchSummaryRunes = 120

// publishedSearchSummary always derives the summary from the published body
// (DISC-021): the ES highlight fragment is kept only while it still appears in
// that body, otherwise the body's first runes are used. It never returns more
// than one fragment or the whole body.
func publishedSearchSummary(body, highlight string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	plain := stripSearchMarkup(highlight)
	if plain != "" && strings.Contains(body, plain) && len([]rune(plain)) <= searchSummaryRunes {
		return highlight
	}
	return truncateRunes(body, searchSummaryRunes)
}

// stripSearchMarkup removes highlight tags to compare a fragment with the body.
func stripSearchMarkup(value string) string {
	value = strings.ReplaceAll(value, "<em>", "")
	value = strings.ReplaceAll(value, "</em>", "")
	return strings.TrimSpace(value)
}

// truncateRunes cuts to limit runes so multi-byte text is never split.
func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// searchTotalAfterVisibility lowers the ES total by the hits this page dropped
// for visibility.
func searchTotalAfterVisibility(total int64, fetched, visible int) int64 {
	return visibilityx.AdjustPageTotal(total, fetched, visible)
}
