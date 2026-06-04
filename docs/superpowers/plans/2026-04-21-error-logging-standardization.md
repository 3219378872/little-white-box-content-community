# Error Response & Logging Standardization Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Standardize all modules to use `errx` error responses and structured `l.Errorw` logging, with a global error handler in Gateway.

**Architecture:** Gateway registers `httpx.SetErrorHandlerCtx` to map `errx.BizError` to HTTP JSON responses. All RPC Logic layers return `errx.NewWithCode`/`errx.Wrap` instead of `fmt.Errorf`. All error logging uses `l.Errorw` with structured fields. `errx.BizError` gains an `HTTPStatus()` method for status code mapping.

**Tech Stack:** go-zero v1.10.1, errx (pkg/errx), logx structured logging

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `pkg/errx/errors.go` | Add `HTTPStatus()` method to BizError |
| Modify | `app/gateway/gateway.go` | Register global error handler |
| Modify | `app/gateway/internal/logic/user/update_profile_logic.go` | Fix `logx.Errorf` + `fmt.Errorf` |
| Modify | `app/gateway/internal/logic/user/get_user_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/user/get_user_favorites_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/user/get_user_posts_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/posts/get_post_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/posts/get_post_list_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/posts/update_post_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/posts/delete_post_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/comment/get_comment_list_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/comment/create_comment_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/comment/delete_comment_logic.go` | Replace `fmt.Errorf` with `errx` |
| Modify | `app/gateway/internal/logic/image/upload_image_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/user/internal/logic/get_user_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/user/internal/logic/update_profile_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/content/internal/logic/create_post_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/content/internal/logic/create_comment_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/delete_post_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/content/internal/logic/delete_comment_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_post_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_post_list_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_comment_list_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/content/internal/logic/get_posts_by_ids_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_posts_by_tag_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_user_posts_logic.go` | Replace `fmt.Errorf` + fix `l.Logger.Errorf` |
| Modify | `app/content/internal/logic/get_tags_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |
| Modify | `app/content/internal/logic/update_post_logic.go` | Replace `fmt.Errorf` with `errx` + add logging |

---

### Task 1: Add HTTPStatus() to BizError + Register Global Error Handler

**Files:**
- Modify: `pkg/errx/errors.go`
- Modify: `app/gateway/gateway.go`

- [ ] **Step 1: Add `HTTPStatus()` method to `pkg/errx/errors.go`**

在文件末尾添加：

```go
// HTTPStatus maps business error codes to HTTP status codes.
func (e *BizError) HTTPStatus() int {
	switch {
	case e.Code == SUCCESS:
		return http.StatusOK
	case e.Code == ParamError:
		return http.StatusBadRequest
	case e.Code == NotFound, e.Code == UserNotFound, e.Code == ContentNotFound, e.Code == MediaNotFound:
		return http.StatusNotFound
	case e.Code == LoginRequired, e.Code == TokenExpired, e.Code == TokenInvalid:
		return http.StatusUnauthorized
	case e.Code == PermissionDenied, e.Code == ContentForbidden, e.Code == FavoritesPrivate:
		return http.StatusForbidden
	case e.Code == TooManyReq:
		return http.StatusTooManyRequests
	case e.Code == UserAlreadyExist:
		return http.StatusConflict
	case e.Code == AlreadyLiked, e.Code == AlreadyFavorited, e.Code == NotLikedYet, e.Code == NotFavoritedYet,
		e.Code == CannotLikeSelf, e.Code == CannotFollowSelf:
		return http.StatusBadRequest
	case e.Code == TitleEmpty, e.Code == ContentEmpty, e.Code == ContentTooLong,
		e.Code == FileTooLarge, e.Code == FileTypeNotAllowed, e.Code == MediaMetaMissing:
		return http.StatusBadRequest
	case e.Code == PostAlreadyDeleted:
		return http.StatusGone
	case e.Code == ServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
```

同时在 import 中添加 `"net/http"`。

- [ ] **Step 2: Run tests to verify BizError changes compile**

Run: `go build ./pkg/errx/...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Register `httpx.SetErrorHandlerCtx` in `app/gateway/gateway.go`**

在 `main()` 中，`server.Start()` 之前添加：

```go
httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
	var bizErr *errx.BizError
	if errors.As(err, &bizErr) {
		return bizErr.HTTPStatus(), map[string]any{
			"code":    bizErr.Code,
			"message": bizErr.Message,
		}
	}
	return http.StatusInternalServerError, map[string]any{
		"code":    errx.SystemError,
		"message": errx.GetMsg(errx.SystemError),
	}
})
```

同时在 import 中添加 `"context"`, `"errors"`, `"net/http"`, `"errx"`, `"github.com/zeromicro/go-zero/rest/httpx"`。

- [ ] **Step 4: Run build to verify gateway compiles**

Run: `go build ./app/gateway/...`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add pkg/errx/errors.go app/gateway/gateway.go
git commit -m "feat(errx): add HTTPStatus() and register global error handler in gateway"
```

---

### Task 2: Fix User RPC Logic (get_user + update_profile)

**Files:**
- Modify: `app/user/internal/logic/get_user_logic.go`
- Modify: `app/user/internal/logic/update_profile_logic.go`

- [ ] **Step 1: Fix `app/user/internal/logic/get_user_logic.go`**

将整个 `GetUser` 方法体替换为：

```go
func (l *GetUserLogic) GetUser(in *pb.GetUserReq) (*pb.GetUserResp, error) {
	one, err := l.svcCtx.UserProfileModel.FindOne(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserProfileModel.FindOne failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.UserNotFound)
		}
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.GetUserResp{
		User: UserProfileToUserInfo(one),
	}, nil
}
```

import 中将 `"fmt"` 替换为 `"errors"`, `"errx"`, 并添加 model 包（如 `"user/internal/model"`）。

- [ ] **Step 2: Fix `app/user/internal/logic/update_profile_logic.go`**

将整个 `UpdateProfile` 方法体替换为：

```go
func (l *UpdateProfileLogic) UpdateProfile(in *pb.UpdateProfileReq) (*pb.UpdateProfileResp, error) {
	err := l.svcCtx.UserProfileModel.UpdateUserDes(l.ctx, in.UserId, in.Nickname, in.AvatarUrl, in.Bio)
	if err != nil {
		l.Errorw("UserProfileModel.UpdateUserDes failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.UpdateProfileResp{}, nil
}
```

import 中将 `"fmt"` 替换为 `"errx"`。

- [ ] **Step 3: Run build**

Run: `go build ./app/user/...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add app/user/internal/logic/get_user_logic.go app/user/internal/logic/update_profile_logic.go
git commit -m "fix(user): replace fmt.Errorf with errx + add structured logging"
```

---

### Task 3: Fix Content RPC Logic - Write Operations (create/update/delete)

**Files:**
- Modify: `app/content/internal/logic/create_post_logic.go`
- Modify: `app/content/internal/logic/update_post_logic.go`
- Modify: `app/content/internal/logic/delete_post_logic.go`
- Modify: `app/content/internal/logic/create_comment_logic.go`
- Modify: `app/content/internal/logic/delete_comment_logic.go`

- [ ] **Step 1: Fix `create_post_logic.go` 行 58, 72, 84, 92**

行 58（裸 `return nil, err`）:
```go
// Before:
// return nil, err
// After:
l.Errorw("json convert images failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 72（`fmt.Errorf("创建帖子失败: %w", err)`）:
```go
// Before:
// return nil, fmt.Errorf("创建帖子失败: %w", err)
// After:
l.Errorw("PostModel.InsertPost failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 84（`fmt.Errorf("生成标签ID失败: %w", idErr)`）:
```go
// Before:
// return nil, fmt.Errorf("生成标签ID失败: %w", idErr)
// After:
l.Errorw("generate tag id failed", logx.Field("err", idErr.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 92（`fmt.Errorf("创建帖子标签失败: %w", err)`）:
```go
// Before:
// return nil, fmt.Errorf("创建帖子标签失败: %w", err)
// After:
l.Errorw("PostTagModel.BatchInsertTagsByPostId failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`（如果其他地方不再用）。

- [ ] **Step 2: Fix `update_post_logic.go` 行 50, 80, 94, 101**

行 50（`fmt.Errorf("查询帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindPostById failed", logx.Field("postId", in.PostId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 80（`fmt.Errorf("更新帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.UpdateFields failed", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 94（`fmt.Errorf("生成标签ID失败: %w", idErr)`）:
```go
l.Errorw("generate tag id failed", logx.Field("err", idErr.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 101（`fmt.Errorf("更新标签失败: %w", err)`）:
```go
l.Errorw("PostTagModel.TransactReplaceTagsByPostId failed", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`。

- [ ] **Step 3: Fix `delete_post_logic.go` 行 40, 51**

行 40（`fmt.Errorf("查询帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindPostById failed", logx.Field("postId", in.PostId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 51（`fmt.Errorf("删除帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.UpdateStatus failed", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`。同时将 `if err == model.ErrNotFound` 改为 `if errors.Is(err, model.ErrNotFound)` 并添加 `"errors"` import。

- [ ] **Step 4: Fix `create_comment_logic.go` 行 47, 73, 78**

行 47（`fmt.Errorf("查询帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindPostById failed", logx.Field("postId", in.PostId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 73（`fmt.Errorf("创建评论失败: %w", err)`）:
```go
l.Errorw("CommentModel.InsertComment failed", logx.Field("postId", in.PostId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 78（`l.Logger.Errorf("更新评论数失败 postId=%d err=%v", ...)`）改为结构化：
```go
l.Errorw("PostModel.IncrCommentCount failed",
	logx.Field("postId", in.PostId),
	logx.Field("err", err.Error()),
)
```

import 中移除 `"fmt"`。

- [ ] **Step 5: Fix `delete_comment_logic.go` 行 40, 51, 56**

行 40（`fmt.Errorf("查询评论失败: %w", err)`）:
```go
l.Errorw("CommentModel.FindCommentById failed", logx.Field("commentId", in.CommentId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 51（`fmt.Errorf("删除评论失败: %w", err)`）:
```go
l.Errorw("CommentModel.UpdateStatus failed", logx.Field("commentId", comment.Id), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 56（`l.Logger.Errorf("更新评论数失败 postId=%d err=%v", ...)`）改为结构化：
```go
l.Errorw("PostModel.DecrCommentCount failed",
	logx.Field("postId", comment.PostId),
	logx.Field("err", err.Error()),
)
```

import 中移除 `"fmt"`。同时将 `if err == model.ErrNotFound` 改为 `if errors.Is(err, model.ErrNotFound)` 并添加 `"errors"` import。

- [ ] **Step 6: Run build**

Run: `go build ./app/content/...`
Expected: BUILD SUCCESS

- [ ] **Step 7: Commit**

```bash
git add app/content/internal/logic/create_post_logic.go app/content/internal/logic/update_post_logic.go app/content/internal/logic/delete_post_logic.go app/content/internal/logic/create_comment_logic.go app/content/internal/logic/delete_comment_logic.go
git commit -m "fix(content): replace fmt.Errorf with errx + structured logging in write operations"
```

---

### Task 4: Fix Content RPC Logic - Read Operations

**Files:**
- Modify: `app/content/internal/logic/get_post_logic.go`
- Modify: `app/content/internal/logic/get_post_list_logic.go`
- Modify: `app/content/internal/logic/get_comment_list_logic.go`
- Modify: `app/content/internal/logic/get_posts_by_ids_logic.go`
- Modify: `app/content/internal/logic/get_posts_by_tag_logic.go`
- Modify: `app/content/internal/logic/get_user_posts_logic.go`
- Modify: `app/content/internal/logic/get_tags_logic.go`

- [ ] **Step 1: Fix `get_post_logic.go` 行 41, 56**

行 41（`fmt.Errorf("查询帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindPostById failed", logx.Field("postId", in.PostId), logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 56（`l.Logger.Errorf("查询标签失败 postId=%d err=%v", post.Id, err)`）改为：
```go
l.Errorw("PostTagModel.FindTagNamesByPostId failed",
	logx.Field("postId", post.Id),
	logx.Field("err", err.Error()),
)
```

import 中移除 `"fmt"`。

- [ ] **Step 2: Fix `get_post_list_logic.go` 行 40, 53**

行 40（`fmt.Errorf("查询帖子列表失败: %w", err)`）:
```go
l.Errorw("PostModel.FindList failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 53（`l.Logger.Errorf("批量查询标签失败 err=%v", err)`）改为：
```go
l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 3: Fix `get_comment_list_logic.go` 行 40**

行 40（`fmt.Errorf("查询评论列表失败: %w", err)`）:
```go
l.Errorw("CommentModel.FindByPostId failed",
	logx.Field("postId", in.PostId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 4: Fix `get_posts_by_ids_logic.go` 行 35, 47**

行 35（`fmt.Errorf("批量查询帖��失败: %w", err)`）:
```go
l.Errorw("PostModel.FindByIds failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 47（`l.Logger.Errorf("批量查询标签失败 err=%v", err)`）改为：
```go
l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 5: Fix `get_posts_by_tag_logic.go` 行 46, 52, 61**

行 46（`fmt.Errorf("查询标签帖子失败: %w", err)`）:
```go
l.Errorw("PostTagModel.FindPostIdsByTagName failed",
	logx.Field("tagName", in.TagName),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

行 52（`fmt.Errorf("批量查询帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindByIds failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 61（`l.Logger.Errorf("批量查询标签失败 err=%v", err)`）改为：
```go
l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
```

import 中移除 `"fmt"`。

- [ ] **Step 6: Fix `get_user_posts_logic.go` 行 48, 61**

行 48（`fmt.Errorf("查询用户帖子失败: %w", err)`）:
```go
l.Errorw("PostModel.FindByAuthorId failed",
	logx.Field("userId", in.UserId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

行 61（`l.Logger.Errorf("批量查询标签失败 err=%v", err)`）改为：
```go
l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 7: Fix `get_tags_logic.go` 行 36**

行 36（`fmt.Errorf("查询标签列表失败: %w", err)`）:
```go
l.Errorw("TagModel.FindList failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 8: Run build**

Run: `go build ./app/content/...`
Expected: BUILD SUCCESS

- [ ] **Step 9: Commit**

```bash
git add app/content/internal/logic/get_post_logic.go app/content/internal/logic/get_post_list_logic.go app/content/internal/logic/get_comment_list_logic.go app/content/internal/logic/get_posts_by_ids_logic.go app/content/internal/logic/get_posts_by_tag_logic.go app/content/internal/logic/get_user_posts_logic.go app/content/internal/logic/get_tags_logic.go
git commit -m "fix(content): replace fmt.Errorf with errx + structured logging in read operations"
```

---

### Task 5: Fix Gateway Logic - User Module

**Files:**
- Modify: `app/gateway/internal/logic/user/update_profile_logic.go`
- Modify: `app/gateway/internal/logic/user/get_user_logic.go`
- Modify: `app/gateway/internal/logic/user/get_user_favorites_logic.go`
- Modify: `app/gateway/internal/logic/user/get_user_posts_logic.go`

- [ ] **Step 1: Fix `update_profile_logic.go` 行 37-38**

当前代码：
```go
logx.Errorf("获取userId上下文错误")
return nil, fmt.Errorf("服务器内部错误:%w", errx.NewWithCode(errx.SystemError))
```

两个问题：(1) `logx.Errorf` 缺少 ctx；(2) `fmt.Errorf` 包装 `errx.BizError` 导致类型断言失败。

替换为：
```go
l.Errorw("jwtx.GetUserIdFromContext failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"` 和 `"errx"` 改为只留 `"errx"`（移除 `"fmt"`）。

- [ ] **Step 2: Fix `get_user_logic.go` 行 35, 38**

行 35（`fmt.Errorf("获取用户远程RPC错误: %w", err)`）:
```go
l.Errorw("UserService.GetUser RPC failed",
	logx.Field("userId", req.UserId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

行 38（`fmt.Errorf("用户不存在: userId=%d", req.UserId)`）:
```go
return nil, errx.NewWithCode(errx.UserNotFound)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 3: Fix `get_user_favorites_logic.go` 行 40, 43**

行 40（`fmt.Errorf("获取用户信息失败: %w", err)`）:
```go
l.Errorw("UserService.GetUser RPC failed",
	logx.Field("userId", req.UserId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

行 43（`fmt.Errorf("用户不存在: userId=%d", req.UserId)`）:
```go
return nil, errx.NewWithCode(errx.UserNotFound)
```

import 中移除 `"fmt"`。

- [ ] **Step 4: Fix `get_user_posts_logic.go` 行 40**

行 40（`fmt.Errorf("获取用户帖子失败: %w", err)`）:
```go
l.Errorw("ContentService.GetUserPosts RPC failed",
	logx.Field("userId", req.UserId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 5: Run build**

Run: `go build ./app/gateway/...`
Expected: BUILD SUCCESS

- [ ] **Step 6: Commit**

```bash
git add app/gateway/internal/logic/user/
git commit -m "fix(gateway/user): replace fmt.Errorf with errx + structured logging"
```

---

### Task 6: Fix Gateway Logic - Posts Module

**Files:**
- Modify: `app/gateway/internal/logic/posts/get_post_logic.go`
- Modify: `app/gateway/internal/logic/posts/get_post_list_logic.go`
- Modify: `app/gateway/internal/logic/posts/update_post_logic.go`
- Modify: `app/gateway/internal/logic/posts/delete_post_logic.go`

- [ ] **Step 1: Fix `get_post_logic.go` 行 42, 47**

行 42（`fmt.Errorf("获取帖子失败: %w", err)`）:
```go
l.Errorw("ContentService.GetPost RPC failed",
	logx.Field("postId", req.PostId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

行 47（`fmt.Errorf("帖子不存在")`）:
```go
return nil, errx.NewWithCode(errx.ContentNotFound)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 2: Fix `get_post_list_logic.go` 行 39**

行 39（`fmt.Errorf("获取帖子列表失败: %w", err)`）:
```go
l.Errorw("ContentService.GetPostList RPC failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 3: Fix `update_post_logic.go` 行 48**

行 48（`fmt.Errorf("更新帖子失败: %w", err)`）:
```go
l.Errorw("ContentService.UpdatePost RPC failed",
	logx.Field("postId", req.PostId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 4: Fix `delete_post_logic.go` 行 44**

行 44（`fmt.Errorf("删除帖子失败: %w", err)`）:
```go
l.Errorw("ContentService.DeletePost RPC failed",
	logx.Field("postId", req.PostId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 5: Run build**

Run: `go build ./app/gateway/...`
Expected: BUILD SUCCESS

- [ ] **Step 6: Commit**

```bash
git add app/gateway/internal/logic/posts/
git commit -m "fix(gateway/posts): replace fmt.Errorf with errx + structured logging"
```

---

### Task 7: Fix Gateway Logic - Comment + Image Modules

**Files:**
- Modify: `app/gateway/internal/logic/comment/get_comment_list_logic.go`
- Modify: `app/gateway/internal/logic/comment/create_comment_logic.go`
- Modify: `app/gateway/internal/logic/comment/delete_comment_logic.go`
- Modify: `app/gateway/internal/logic/image/upload_image_logic.go`

- [ ] **Step 1: Fix `comment/get_comment_list_logic.go` 行 40**

行 40（`fmt.Errorf("获取评论列表失败: %w", err)`）:
```go
l.Errorw("ContentService.GetCommentList RPC failed",
	logx.Field("postId", req.PostId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 2: Fix `comment/create_comment_logic.go` 行 47**

行 47（`fmt.Errorf("创建评论失败: %w", err)`）:
```go
l.Errorw("ContentService.CreateComment RPC failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 3: Fix `comment/delete_comment_logic.go` 行 44**

行 44（`fmt.Errorf("删除评论失败: %w", err)`）:
```go
l.Errorw("ContentService.DeleteComment RPC failed",
	logx.Field("commentId", req.CommentId),
	logx.Field("err", err.Error()),
)
return nil, errx.NewWithCode(errx.SystemError)
```

import 中移除 `"fmt"`，添加 `"errx"`。

- [ ] **Step 4: Fix `image/upload_image_logic.go` 行 51, 63, 75, 82, 88**

行 51（`fmt.Errorf("建立 media 流失败: %w", err)`）:
```go
l.Errorw("MediaService.UploadImage stream failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.SystemError)
```

行 63（`fmt.Errorf("发送 meta 失败: %w", err)`）:
```go
l.Errorw("stream.Send meta failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.UploadFailed)
```

行 75（`fmt.Errorf("发送 chunk 失败: %w", err)`）:
```go
l.Errorw("stream.Send chunk failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.UploadFailed)
```

行 82（`fmt.Errorf("读取文件失败: %w", readErr)`）:
```go
l.Errorw("file.Read failed", logx.Field("err", readErr.Error()))
return nil, errx.NewWithCode(errx.UploadFailed)
```

行 88（`fmt.Errorf("关闭流失败: %w", err)`）:
```go
l.Errorw("stream.CloseAndRecv failed", logx.Field("err", err.Error()))
return nil, errx.NewWithCode(errx.UploadFailed)
```

import 中移除 `"fmt"`。

- [ ] **Step 5: Run build**

Run: `go build ./app/gateway/...`
Expected: BUILD SUCCESS

- [ ] **Step 6: Commit**

```bash
git add app/gateway/internal/logic/comment/ app/gateway/internal/logic/image/upload_image_logic.go
git commit -m "fix(gateway/comment,image): replace fmt.Errorf with errx + structured logging"
```

---

### Task 8: Final Verification

- [ ] **Step 1: Full build check**

Run: `go build ./...`
Expected: BUILD SUCCESS with no errors

- [ ] **Step 2: Verify no remaining `fmt.Errorf` in logic files (excluding tests)**

Run: `grep -rn "fmt\.Errorf" app/*/internal/logic/ --include="*.go" | grep -v "_test.go"`
Expected: No output (zero matches)

- [ ] **Step 3: Verify no remaining `l.Logger.Errorf` pattern**

Run: `grep -rn "l\.Logger\.Errorf" app/ --include="*.go" | grep -v "_test.go"`
Expected: No output (zero matches)

- [ ] **Step 4: Verify no remaining bare `logx.Errorf` (without ctx)**

Run: `grep -rn "logx\.\(Info\|Error\|Slow\|Severe\)f\?\b" app/ --include="*.go" | grep -v "_test.go" | grep -v "logx\.WithContext" | grep -v "logx\.Field"`
Expected: No output (zero matches)

- [ ] **Step 5: Run all tests**

Run: `go test ./... -race -count=1`
Expected: All tests PASS

- [ ] **Step 6: Commit verification results (if any straggler fixes needed)**

```bash
git add -A
git commit -m "fix: final verification cleanup for error/logging standardization"
```
