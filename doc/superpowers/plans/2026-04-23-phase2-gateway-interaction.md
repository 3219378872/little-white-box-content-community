# Phase 2 Gateway Interaction 对接实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Gateway 中配置 Interaction RPC 客户端，并完善现有的 5 个互动接口 Logic 使其实际调用 Interaction RPC。

**Architecture:** 在 Gateway 的 Config 和 ServiceContext 中注入 Interaction RPC 客户端；Like/Unlike/Favorite/Unfavorite 直接透传调用；GetUserFavorites 需要串联 Interaction.GetFavoriteList + Content.GetPostsByIds 两次 RPC。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, gRPC, standard `go test` with fake service mocks

---

## File Map

| 文件 | 职责 |
|------|------|
| `app/gateway/internal/config/config.go` | 新增 `InteractionRpc` 配置字段 |
| `app/gateway/internal/svc/service_context.go` | 新增 `InteractionService` 并初始化 zrpc client |
| `app/gateway/internal/logic/like_favorite/like_logic.go` | 调用 `InteractionService.Like` |
| `app/gateway/internal/logic/like_favorite/like_logic_test.go` | LikeLogic 单元测试 |
| `app/gateway/internal/logic/like_favorite/unlike_logic.go` | 调用 `InteractionService.Unlike` |
| `app/gateway/internal/logic/like_favorite/unlike_logic_test.go` | UnlikeLogic 单元测试 |
| `app/gateway/internal/logic/like_favorite/favorite_logic.go` | 调用 `InteractionService.Favorite` |
| `app/gateway/internal/logic/like_favorite/favorite_logic_test.go` | FavoriteLogic 单元测试 |
| `app/gateway/internal/logic/like_favorite/unfavorite_logic.go` | 调用 `InteractionService.Unfavorite` |
| `app/gateway/internal/logic/like_favorite/unfavorite_logic_test.go` | UnfavoriteLogic 单元测试 |
| `app/gateway/internal/logic/user/get_user_favorites_logic.go` | 串联 `Interaction.GetFavoriteList` + `Content.GetPostsByIds` |
| `app/gateway/internal/logic/user/get_user_favorites_logic_test.go` | GetUserFavoritesLogic 单元测试 |

---

### Task 1: 配置层与依赖注入

**Files:**
- Modify: `app/gateway/internal/config/config.go`
- Modify: `app/gateway/internal/svc/service_context.go`

- [ ] **Step 1: 在 Config 中新增 InteractionRpc**

修改 `app/gateway/internal/config/config.go`，在 `MediaRpc` 下方新增：

```go
InteractionRpc zrpc.RpcClientConf
```

- [ ] **Step 2: 在 ServiceContext 中新增 InteractionService**

修改 `app/gateway/internal/svc/service_context.go`：

1. 新增 import：
```go
"esx/app/interaction/interactionservice"
```

2. 在 `ServiceContext` 结构体中新增：
```go
InteractionService interactionservice.InteractionService
```

3. 在 `NewServiceContext` 中，在 `mediaService` 初始化之后添加：
```go
interactionClient := zrpc.MustNewClient(c.InteractionRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
interactionService := interactionservice.NewInteractionService(interactionClient)
```

4. 在 return 的 `ServiceContext` 中新增：
```go
InteractionService: interactionService,
```

- [ ] **Step 3: 编译检查**

Run: `go build ./app/gateway/...`
Expected: 编译通过（此时 Logic 还是空的，但配置层应该能编译）

- [ ] **Step 4: Commit**

```bash
git add app/gateway/internal/config/config.go app/gateway/internal/svc/service_context.go
git commit -m "feat(gateway): add Interaction RPC client config and service context"
```

---

### Task 2: LikeLogic TDD

**Files:**
- Modify: `app/gateway/internal/logic/like_favorite/like_logic.go`
- Create: `app/gateway/internal/logic/like_favorite/like_logic_test.go`

- [ ] **Step 1: 写失败测试**

创建 `app/gateway/internal/logic/like_favorite/like_logic_test.go`：

```go
package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"google.golang.org/grpc"
	"jwtx"
)

type fakeInteractionService struct {
	interactionservice.InteractionService
	likeFn func(ctx context.Context, in *interactionpb.LikeReq, opts ...grpc.CallOption) (*interactionpb.LikeResp, error)
}

func (f *fakeInteractionService) Like(ctx context.Context, in *interactionpb.LikeReq, opts ...grpc.CallOption) (*interactionpb.LikeResp, error) {
	return f.likeFn(ctx, in, opts...)
}

func TestLike_Success(t *testing.T) {
	var capturedReq *interactionpb.LikeReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionService{
			likeFn: func(_ context.Context, in *interactionpb.LikeReq, _ ...grpc.CallOption) (*interactionpb.LikeResp, error) {
				capturedReq = in
				return &interactionpb.LikeResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewLikeLogic(ctx, svcCtx)

	_, err := l.Like(&types.LikeReq{TargetId: 100, TargetType: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Like RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.TargetId != 100 || capturedReq.TargetType != 1 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestLike_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionService{
			likeFn: func(_ context.Context, _ *interactionpb.LikeReq, _ ...grpc.CallOption) (*interactionpb.LikeResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewLikeLogic(ctx, svcCtx)

	_, err := l.Like(&types.LikeReq{TargetId: 100, TargetType: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestLike -v`
Expected: FAIL（因为 like_logic.go 还是空的 todo）

- [ ] **Step 3: 实现 LikeLogic**

修改 `app/gateway/internal/logic/like_favorite/like_logic.go`：

```go
package like_favorite

import (
	"context"

	"esx/app/interaction/interactionservice"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type LikeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikeLogic) Like(req *types.LikeReq) (resp *types.LikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.Unauthorized)
	}

	_, err = l.svcCtx.InteractionService.Like(l.ctx, &interactionservice.LikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		l.Errorw("InteractionService.Like RPC failed",
			logx.Field("userId", userId),
			logx.Field("targetId", req.TargetId),
			logx.Field("targetType", req.TargetType),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.LikeResp{}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestLike -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/like_favorite/like_logic.go app/gateway/internal/logic/like_favorite/like_logic_test.go
git commit -m "feat(gateway): implement Like logic with Interaction RPC"
```

---

### Task 3: UnlikeLogic TDD

**Files:**
- Modify: `app/gateway/internal/logic/like_favorite/unlike_logic.go`
- Create: `app/gateway/internal/logic/like_favorite/unlike_logic_test.go`

- [ ] **Step 1: 写失败测试**

创建 `app/gateway/internal/logic/like_favorite/unlike_logic_test.go`：

```go
package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"google.golang.org/grpc"
	"jwtx"
)

type fakeInteractionServiceUnlike struct {
	interactionservice.InteractionService
	unlikeFn func(ctx context.Context, in *interactionpb.UnlikeReq, opts ...grpc.CallOption) (*interactionpb.UnlikeResp, error)
}

func (f *fakeInteractionServiceUnlike) Unlike(ctx context.Context, in *interactionpb.UnlikeReq, opts ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
	return f.unlikeFn(ctx, in, opts...)
}

func TestUnlike_Success(t *testing.T) {
	var capturedReq *interactionpb.UnlikeReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnlike{
			unlikeFn: func(_ context.Context, in *interactionpb.UnlikeReq, _ ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
				capturedReq = in
				return &interactionpb.UnlikeResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnlikeLogic(ctx, svcCtx)

	_, err := l.Unlike(&types.UnlikeReq{TargetId: 100, TargetType: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Unlike RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.TargetId != 100 || capturedReq.TargetType != 1 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestUnlike_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnlike{
			unlikeFn: func(_ context.Context, _ *interactionpb.UnlikeReq, _ ...grpc.CallOption) (*interactionpb.UnlikeResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnlikeLogic(ctx, svcCtx)

	_, err := l.Unlike(&types.UnlikeReq{TargetId: 100, TargetType: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestUnlike -v`
Expected: FAIL

- [ ] **Step 3: 实现 UnlikeLogic**

修改 `app/gateway/internal/logic/like_favorite/unlike_logic.go`：

```go
package like_favorite

import (
	"context"

	"esx/app/interaction/interactionservice"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnlikeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnlikeLogic) Unlike(req *types.UnlikeReq) (resp *types.UnlikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.Unauthorized)
	}

	_, err = l.svcCtx.InteractionService.Unlike(l.ctx, &interactionservice.UnlikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		l.Errorw("InteractionService.Unlike RPC failed",
			logx.Field("userId", userId),
			logx.Field("targetId", req.TargetId),
			logx.Field("targetType", req.TargetType),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.UnlikeResp{}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestUnlike -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/like_favorite/unlike_logic.go app/gateway/internal/logic/like_favorite/unlike_logic_test.go
git commit -m "feat(gateway): implement Unlike logic with Interaction RPC"
```

---

### Task 4: FavoriteLogic TDD

**Files:**
- Modify: `app/gateway/internal/logic/like_favorite/favorite_logic.go`
- Create: `app/gateway/internal/logic/like_favorite/favorite_logic_test.go`

- [ ] **Step 1: 写失败测试**

创建 `app/gateway/internal/logic/like_favorite/favorite_logic_test.go`：

```go
package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"google.golang.org/grpc"
	"jwtx"
)

type fakeInteractionServiceFavorite struct {
	interactionservice.InteractionService
	favoriteFn func(ctx context.Context, in *interactionpb.FavoriteReq, opts ...grpc.CallOption) (*interactionpb.FavoriteResp, error)
}

func (f *fakeInteractionServiceFavorite) Favorite(ctx context.Context, in *interactionpb.FavoriteReq, opts ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
	return f.favoriteFn(ctx, in, opts...)
}

func TestFavorite_Success(t *testing.T) {
	var capturedReq *interactionpb.FavoriteReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceFavorite{
			favoriteFn: func(_ context.Context, in *interactionpb.FavoriteReq, _ ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
				capturedReq = in
				return &interactionpb.FavoriteResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewFavoriteLogic(ctx, svcCtx)

	_, err := l.Favorite(&types.FavoriteReq{PostId: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Favorite RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.PostId != 100 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestFavorite_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceFavorite{
			favoriteFn: func(_ context.Context, _ *interactionpb.FavoriteReq, _ ...grpc.CallOption) (*interactionpb.FavoriteResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewFavoriteLogic(ctx, svcCtx)

	_, err := l.Favorite(&types.FavoriteReq{PostId: 100})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestFavorite -v`
Expected: FAIL

- [ ] **Step 3: 实现 FavoriteLogic**

修改 `app/gateway/internal/logic/like_favorite/favorite_logic.go`：

```go
package like_favorite

import (
	"context"

	"esx/app/interaction/interactionservice"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FavoriteLogic) Favorite(req *types.FavoriteReq) (resp *types.FavoriteResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.Unauthorized)
	}

	_, err = l.svcCtx.InteractionService.Favorite(l.ctx, &interactionservice.FavoriteReq{
		UserId: userId,
		PostId: req.PostId,
	})
	if err != nil {
		l.Errorw("InteractionService.Favorite RPC failed",
			logx.Field("userId", userId),
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.FavoriteResp{}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestFavorite -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/like_favorite/favorite_logic.go app/gateway/internal/logic/like_favorite/favorite_logic_test.go
git commit -m "feat(gateway): implement Favorite logic with Interaction RPC"
```

---

### Task 5: UnfavoriteLogic TDD

**Files:**
- Modify: `app/gateway/internal/logic/like_favorite/unfavorite_logic.go`
- Create: `app/gateway/internal/logic/like_favorite/unfavorite_logic_test.go`

- [ ] **Step 1: 写失败测试**

创建 `app/gateway/internal/logic/like_favorite/unfavorite_logic_test.go`：

```go
package like_favorite

import (
	"context"
	"testing"

	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"google.golang.org/grpc"
	"jwtx"
)

type fakeInteractionServiceUnfavorite struct {
	interactionservice.InteractionService
	unfavoriteFn func(ctx context.Context, in *interactionpb.UnfavoriteReq, opts ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error)
}

func (f *fakeInteractionServiceUnfavorite) Unfavorite(ctx context.Context, in *interactionpb.UnfavoriteReq, opts ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
	return f.unfavoriteFn(ctx, in, opts...)
}

func TestUnfavorite_Success(t *testing.T) {
	var capturedReq *interactionpb.UnfavoriteReq
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnfavorite{
			unfavoriteFn: func(_ context.Context, in *interactionpb.UnfavoriteReq, _ ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
				capturedReq = in
				return &interactionpb.UnfavoriteResp{}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnfavoriteLogic(ctx, svcCtx)

	_, err := l.Unfavorite(&types.UnfavoriteReq{PostId: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected Unfavorite RPC to be called")
	}
	if capturedReq.UserId != 42 || capturedReq.PostId != 100 {
		t.Fatalf("unexpected request: %+v", capturedReq)
	}
}

func TestUnfavorite_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		InteractionService: &fakeInteractionServiceUnfavorite{
			unfavoriteFn: func(_ context.Context, _ *interactionpb.UnfavoriteReq, _ ...grpc.CallOption) (*interactionpb.UnfavoriteResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewUnfavoriteLogic(ctx, svcCtx)

	_, err := l.Unfavorite(&types.UnfavoriteReq{PostId: 100})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestUnfavorite -v`
Expected: FAIL

- [ ] **Step 3: 实现 UnfavoriteLogic**

修改 `app/gateway/internal/logic/like_favorite/unfavorite_logic.go`：

```go
package like_favorite

import (
	"context"

	"esx/app/interaction/interactionservice"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfavoriteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnfavoriteLogic) Unfavorite(req *types.UnfavoriteReq) (resp *types.UnfavoriteResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.Unauthorized)
	}

	_, err = l.svcCtx.InteractionService.Unfavorite(l.ctx, &interactionservice.UnfavoriteReq{
		UserId: userId,
		PostId: req.PostId,
	})
	if err != nil {
		l.Errorw("InteractionService.Unfavorite RPC failed",
			logx.Field("userId", userId),
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.UnfavoriteResp{}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/gateway/internal/logic/like_favorite/ -run TestUnfavorite -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/like_favorite/unfavorite_logic.go app/gateway/internal/logic/like_favorite/unfavorite_logic_test.go
git commit -m "feat(gateway): implement Unfavorite logic with Interaction RPC"
```

---

### Task 6: GetUserFavoritesLogic TDD

**Files:**
- Modify: `app/gateway/internal/logic/user/get_user_favorites_logic.go`
- Modify: `app/gateway/internal/logic/user/get_user_favorites_logic_test.go`

- [ ] **Step 1: 写失败测试**

修改 `app/gateway/internal/logic/user/get_user_favorites_logic_test.go`，在现有测试下方追加：

```go
import (
	"esx/app/content/contentservice"
	contentpb "esx/app/content/pb/xiaobaihe/content/pb"
	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
)

type fakeInteractionServiceFavorites struct {
	interactionservice.InteractionService
	getFavoriteListFn func(ctx context.Context, in *interactionpb.GetFavoriteListReq, opts ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error)
}

func (f *fakeInteractionServiceFavorites) GetFavoriteList(ctx context.Context, in *interactionpb.GetFavoriteListReq, opts ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
	return f.getFavoriteListFn(ctx, in, opts...)
}

type fakeContentServiceFavorites struct {
	contentservice.ContentService
	getPostsByIdsFn func(ctx context.Context, in *contentpb.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error)
}

func (f *fakeContentServiceFavorites) GetPostsByIds(ctx context.Context, in *contentpb.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
	return f.getPostsByIdsFn(ctx, in, opts...)
}

func TestGetUserFavorites_WithData_ReturnsPosts(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, in *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				if in.UserId != 42 {
					t.Fatalf("expected userId 42, got %d", in.UserId)
				}
				return &interactionpb.GetFavoriteListResp{
					PostIds: []int64{100, 200},
					Total:   2,
				}, nil
			},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, in *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				if len(in.PostIds) != 2 {
					t.Fatalf("expected 2 post ids, got %d", len(in.PostIds))
				}
				return &contentpb.GetPostsByIdsResp{
					Posts: []*contentpb.PostInfo{
						{Id: 100, AuthorId: 1, Title: "Post A", Content: "Content A", ViewCount: 10, LikeCount: 5, CommentCount: 2, FavoriteCount: 3, CreatedAt: 1000},
						{Id: 200, AuthorId: 2, Title: "Post B", Content: "Content B", ViewCount: 20, LikeCount: 10, CommentCount: 4, FavoriteCount: 6, CreatedAt: 2000},
					},
				}, nil
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	resp, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.List) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.List))
	}
	if resp.List[0].Id != 100 || resp.List[1].Id != 200 {
		t.Fatalf("unexpected items: %+v", resp.List)
	}
	if resp.Total != 2 {
		t.Fatalf("expected Total=2, got %d", resp.Total)
	}
}

func TestGetUserFavorites_InteractionRPCError_ReturnsSystemError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetUserFavorites_ContentRPCError_ReturnsSystemError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: &fakeUserService{
			getUserFn: func(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
				return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, FavoritesVisibility: 1}}, nil
			},
		},
		InteractionService: &fakeInteractionServiceFavorites{
			getFavoriteListFn: func(_ context.Context, _ *interactionpb.GetFavoriteListReq, _ ...grpc.CallOption) (*interactionpb.GetFavoriteListResp, error) {
				return &interactionpb.GetFavoriteListResp{PostIds: []int64{100}, Total: 1}, nil
			},
		},
		ContentService: &fakeContentServiceFavorites{
			getPostsByIdsFn: func(_ context.Context, _ *contentpb.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentpb.GetPostsByIdsResp, error) {
				return nil, context.DeadlineExceeded
			},
		},
	}

	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 42})
	l := NewGetUserFavoritesLogic(ctx, svcCtx)

	_, err := l.GetUserFavorites(&types.GetUserFavoritesReq{UserId: 42, Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./app/gateway/internal/logic/user/ -run TestGetUserFavorites_WithData -v`
Expected: FAIL（因为 get_user_favorites_logic.go 还是 TODO 占位）

- [ ] **Step 3: 实现 GetUserFavoritesLogic**

修改 `app/gateway/internal/logic/user/get_user_favorites_logic.go`：

```go
package user

import (
	"context"

	"esx/app/content/contentservice"
	contentpb "esx/app/content/pb/xiaobaihe/content/pb"
	"esx/app/interaction/interactionservice"
	interactionpb "esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserFavoritesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserFavoritesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserFavoritesLogic {
	return &GetUserFavoritesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserFavoritesLogic) GetUserFavorites(req *types.GetUserFavoritesReq) (*types.GetPostListResp, error) {
	requesterID, _ := jwtx.GetUserIdFromContext(l.ctx)

	userResp, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId})
	if err != nil {
		l.Errorw("UserService.GetUser RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if userResp.User == nil {
		return nil, errx.NewWithCode(errx.UserNotFound)
	}

	isOwner := requesterID != 0 && requesterID == req.UserId
	isPublic := userResp.User.FavoritesVisibility == 1
	if !isOwner && !isPublic {
		return nil, errx.NewWithCode(errx.FavoritesPrivate)
	}

	favoriteResp, err := l.svcCtx.InteractionService.GetFavoriteList(l.ctx, &interactionservice.GetFavoriteListReq{
		UserId:   req.UserId,
		Page:     int32(req.Page),
		PageSize: int32(req.PageSize),
	})
	if err != nil {
		l.Errorw("InteractionService.GetFavoriteList RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(favoriteResp.PostIds) == 0 {
		return &types.GetPostListResp{
			List:     []types.PostItem{},
			Total:    favoriteResp.Total,
			Page:     req.Page,
			PageSize: req.PageSize,
		}, nil
	}

	postsResp, err := l.svcCtx.ContentService.GetPostsByIds(l.ctx, &contentservice.GetPostsByIdsReq{
		PostIds: favoriteResp.PostIds,
	})
	if err != nil {
		l.Errorw("ContentService.GetPostsByIds RPC failed",
			logx.Field("postIds", favoriteResp.PostIds),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	list := make([]types.PostItem, 0, len(postsResp.Posts))
	for _, post := range postsResp.Posts {
		list = append(list, types.PostItem{
			Id:            post.Id,
			AuthorId:      post.AuthorId,
			Title:         post.Title,
			Content:       post.Content,
			Images:        post.Images,
			Tags:          post.Tags,
			ViewCount:     post.ViewCount,
			LikeCount:     post.LikeCount,
			CommentCount:  post.CommentCount,
			FavoriteCount: post.FavoriteCount,
			CreatedAt:     post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    favoriteResp.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./app/gateway/internal/logic/user/ -run TestGetUserFavorites -v`
Expected: 所有 TestGetUserFavorites_* 测试 PASS（包括已有的权限测试）

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/user/get_user_favorites_logic.go app/gateway/internal/logic/user/get_user_favorites_logic_test.go
git commit -m "feat(gateway): implement GetUserFavorites with Interaction + Content RPC"
```

---

### Task 7: 全量验证

- [ ] **Step 1: 运行 Gateway 全量测试**

Run: `go test ./app/gateway/... -race -cover`
Expected: 全部 PASS，覆盖率满足项目最低 80% 要求

- [ ] **Step 2: 编译验证**

Run: `go build ./app/gateway/...`
Expected: 编译通过

- [ ] **Step 3: 静态检查**

Run: `go vet ./app/gateway/...`
Expected: 无警告

- [ ] **Step 4: Commit（如需要修复）**

如有修复，单独 commit：
```bash
git commit -m "fix(gateway): address review feedback and lint issues"
```

---

## Self-Review Checklist

- [x] **Spec coverage**: Config/ServiceContext (Task 1) + 5 个 Logic 实现（Tasks 2-6）+ 全量验证（Task 7）
- [x] **Placeholder scan**: 无 TBD/TODO，每步含完整代码
- [x] **Type consistency**: `interactionservice.LikeReq` / `UnlikeReq` / `FavoriteReq` / `UnfavoriteReq` / `GetFavoriteListReq` 与 interaction pb 一致；`contentservice.GetPostsByIdsReq` 与 content pb 一致
