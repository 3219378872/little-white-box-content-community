# golangci-lint cleanupx Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the 53 issues reported in `doc/examination/golangci-lint` while adding a small `pkg/cleanupx` helper for defer cleanup error handling.

**Architecture:** Add `cleanupx` as a focused workspace module that logs cleanup failures without changing business flow. Apply it only to defer-style `Close`, `Remove`, and `Shutdown` call sites, and use mechanical edits for formatting, unused test mocks, and embedded-field staticcheck findings.

**Tech Stack:** Go 1.26.1, go-zero `logx`, existing Go workspace modules, `golangci-lint` v2 config in `.golangci.yml`.

**Execution Constraint:** Do not run `git commit` unless the user explicitly asks. Checkpoint steps record status only.

---

## File Structure

### New Files

- `pkg/cleanupx/go.mod` — declares the standalone `cleanupx` module.
- `pkg/cleanupx/cleanup.go` — implements `Close`, `Remove`, and `Shutdown` helpers.
- `pkg/cleanupx/cleanup_test.go` — unit tests for nil-safe, success, and error cleanup paths.

### Modified Files

- `go.work` — adds `./pkg/cleanupx` to workspace modules.
- `app/feed/feed.go` — replaces unchecked `postConsumer.Shutdown`.
- `app/media/media.go` — replaces unchecked `mqConsumer.Shutdown`.
- `app/message/message.go` — replaces unchecked `messageConsumer.Shutdown`.
- `app/media/internal/logic/upload_image_logic.go` — replaces unchecked temp sink/file cleanup and source file close.
- `app/media/internal/logic/upload_video_logic.go` — replaces unchecked temp sink cleanup.
- `app/media/internal/mediautil/detect.go` — handles file close error locally without adding logger dependency.
- `app/content/internal/logic/mock_models_test.go` — gofmt-only fix.
- `app/media/internal/logic/delete_media_logic.go` — gofmt-only fix.
- `app/media/internal/mediautil/detect_test.go` — gofmt-only fix.
- `app/content/internal/model/comment_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/content/internal/model/post_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/content/internal/model/post_tag_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/content/internal/model/tag_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/interaction/internal/logic/batch_check_favorited_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/batch_check_liked_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/check_favorited_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/check_liked_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/favorite_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/get_counts_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/get_favorite_list_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/like_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/unfavorite_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/logic/unlike_logic.go` — `l.Logger.` to `l.` staticcheck fix.
- `app/interaction/internal/model/favorite_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/interaction/internal/model/like_record_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/media/internal/model/media_model.go` — `m.CachedConn.` to `m.` staticcheck fix.
- `app/feed/internal/mqs/post_publish_consumer_test.go` — removes unused `mockContentService`.

---

## Task 1: Add `pkg/cleanupx` Module

**Files:**
- Create: `pkg/cleanupx/go.mod`
- Create: `pkg/cleanupx/cleanup_test.go`
- Create: `pkg/cleanupx/cleanup.go`
- Modify: `go.work`

- [ ] **Step 1: Add module skeleton and workspace entry**

Create `pkg/cleanupx/go.mod`:

```go
module cleanupx

go 1.26.1

require github.com/zeromicro/go-zero v1.10.1
```

Update `go.work` so it contains `./pkg/cleanupx`:

```go
go 1.26.1

use (
	.
	./app/gateway
	./app/user
	./pkg/cachex
	./pkg/cleanupx
	./pkg/errx
	./pkg/interceptor
	./pkg/jwtx
	./pkg/middleware
	./pkg/mqx
	./pkg/util
)
```

- [ ] **Step 2: Write failing tests for cleanup helpers**

Create `pkg/cleanupx/cleanup_test.go`:

```go
package cleanupx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeromicro/go-zero/core/logx"
)

type recordingCloser struct {
	called bool
	err    error
}

func (c *recordingCloser) Close() error {
	c.called = true
	return c.err
}

func testLogger() logx.Logger {
	logx.Disable()
	return logx.WithContext(context.Background())
}

func TestCloseNilDoesNotPanic(t *testing.T) {
	Close(testLogger(), "nil resource", nil)
}

func TestCloseCallsCloser(t *testing.T) {
	closer := &recordingCloser{}

	Close(testLogger(), "temp file", closer)

	if !closer.called {
		t.Fatal("Close did not call closer")
	}
}

func TestCloseErrorDoesNotPanic(t *testing.T) {
	closer := &recordingCloser{err: errors.New("close failed")}

	Close(testLogger(), "temp file", closer)

	if !closer.called {
		t.Fatal("Close did not call closer")
	}
}

func TestShutdownNilDoesNotPanic(t *testing.T) {
	Shutdown(testLogger(), "nil shutdown", nil)
}

func TestShutdownCallsFunction(t *testing.T) {
	called := false

	Shutdown(testLogger(), "consumer", func() error {
		called = true
		return nil
	})

	if !called {
		t.Fatal("Shutdown did not call function")
	}
}

func TestShutdownErrorDoesNotPanic(t *testing.T) {
	called := false

	Shutdown(testLogger(), "consumer", func() error {
		called = true
		return errors.New("shutdown failed")
	})

	if !called {
		t.Fatal("Shutdown did not call function")
	}
}

func TestRemoveDeletesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup.txt")
	if err := os.WriteFile(path, []byte("cleanup"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	Remove(testLogger(), path)

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still exists after Remove, stat err=%v", err)
	}
}

func TestRemoveMissingFileDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")

	Remove(testLogger(), path)
}

func TestRemoveEmptyPathDoesNotPanic(t *testing.T) {
	Remove(testLogger(), "")
}
```

- [ ] **Step 3: Run tests to verify they fail before implementation**

Run:

```bash
go test ./pkg/cleanupx/...
```

Expected: FAIL with undefined identifiers similar to:

```text
undefined: Close
undefined: Shutdown
undefined: Remove
```

- [ ] **Step 4: Implement cleanup helpers**

Create `pkg/cleanupx/cleanup.go`:

```go
package cleanupx

import (
	"io"
	"os"

	"github.com/zeromicro/go-zero/core/logx"
)

func Close(logger logx.Logger, resource string, closer io.Closer) {
	if closer == nil {
		return
	}

	if err := closer.Close(); err != nil {
		logger.Errorw("close resource failed",
			logx.Field("resource", resource),
			logx.Field("err", err.Error()),
		)
	}
}

func Remove(logger logx.Logger, path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil {
		logger.Errorw("remove path failed",
			logx.Field("path", path),
			logx.Field("err", err.Error()),
		)
	}
}

func Shutdown(logger logx.Logger, resource string, shutdown func() error) {
	if shutdown == nil {
		return
	}

	if err := shutdown(); err != nil {
		logger.Errorw("shutdown resource failed",
			logx.Field("resource", resource),
			logx.Field("err", err.Error()),
		)
	}
}
```

- [ ] **Step 5: Format and verify cleanupx**

Run:

```bash
gofmt -w pkg/cleanupx/cleanup.go pkg/cleanupx/cleanup_test.go
go test ./pkg/cleanupx/...
```

Expected:

```text
ok  	cleanupx
```

- [ ] **Step 6: Checkpoint**

Run:

```bash
git status --short
```

Expected: new `pkg/cleanupx` files and modified `go.work`. Do not commit.

---

## Task 2: Apply `cleanupx` to `errcheck` Call Sites

**Files:**
- Modify: `app/feed/feed.go`
- Modify: `app/media/media.go`
- Modify: `app/message/message.go`
- Modify: `app/media/internal/logic/upload_image_logic.go`
- Modify: `app/media/internal/logic/upload_video_logic.go`

- [ ] **Step 1: Update feed service shutdown cleanup**

In `app/feed/feed.go`, update imports:

```go
import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/feed/internal/config"
	"esx/app/feed/internal/mqs"
	"esx/app/feed/internal/server"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"
	"mqx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)
```

Replace the unchecked defer:

```go
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "post publish consumer", postConsumer.Shutdown)
```

- [ ] **Step 2: Update media service shutdown cleanup**

In `app/media/media.go`, update imports:

```go
import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/media/internal/config"
	"esx/app/media/internal/mqs"
	"esx/app/media/internal/server"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)
```

Replace the unchecked defer:

```go
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "media cleanup consumer", mqConsumer.Shutdown)
```

- [ ] **Step 3: Update message service shutdown cleanup**

In `app/message/message.go`, update imports:

```go
import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/message/internal/config"
	"esx/app/message/internal/mqs"
	"esx/app/message/internal/server"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"
	"mqx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)
```

Replace the unchecked defer:

```go
		defer cleanupx.Shutdown(logx.WithContext(context.Background()), "message consumer", messageConsumer.Shutdown)
```

- [ ] **Step 4: Update image upload cleanup**

In `app/media/internal/logic/upload_image_logic.go`, add the `cleanupx` import:

```go
import (
	"context"
	"errx"
	"os"

	"cleanupx"
	"esx/app/media/internal/mediautil"
	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)
```

Replace the temp sink defer:

```go
	defer cleanupx.Close(l.Logger, "upload image temp sink", sink)
```

Replace compressed and thumbnail cleanup:

```go
	defer cleanupx.Remove(l.Logger, compressedPath)
```

```go
	defer cleanupx.Remove(l.Logger, thumbPath)
```

Replace `putFile` close handling:

```go
func putFile(ctx context.Context, svcCtx *svc.ServiceContext, localPath, objectKey, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer cleanupx.Close(logx.WithContext(ctx), "upload source file", f)

	info, err := f.Stat()
	if err != nil {
		return err
	}
	return svcCtx.Storage.Put(ctx, objectKey, f, info.Size(), contentType)
}
```

- [ ] **Step 5: Update video upload cleanup**

In `app/media/internal/logic/upload_video_logic.go`, add the `cleanupx` import:

```go
import (
	"context"
	"errx"

	"cleanupx"
	"esx/app/media/internal/mediautil"
	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)
```

Replace the temp sink defer:

```go
	defer cleanupx.Close(l.Logger, "upload video temp sink", sink)
```

- [ ] **Step 6: Format and run focused compile tests**

Run:

```bash
gofmt -w app/feed/feed.go app/media/media.go app/message/message.go app/media/internal/logic/upload_image_logic.go app/media/internal/logic/upload_video_logic.go
go test ./app/media/internal/logic ./app/feed/internal/mqs ./app/message/internal/mqs
```

Expected: packages compile. If tests need external services, record the exact failure and continue to lint-focused verification.

- [ ] **Step 7: Checkpoint**

Run:

```bash
git status --short
```

Expected: modified service entrypoints and media upload logic. Do not commit.

---

## Task 3: Fix `Detect` File Close Handling Locally

**Files:**
- Modify: `app/media/internal/mediautil/detect.go`

- [ ] **Step 1: Replace `Detect` with named-return close handling**

In `app/media/internal/mediautil/detect.go`, replace the full `Detect` function with:

```go
// Detect 读取文件前 262 字节嗅探类型并按白名单过滤。
func Detect(path string, allowImage, allowVideo bool) (detected DetectedType, err error) {
	f, err := os.Open(path)
	if err != nil {
		return DetectedType{}, fmt.Errorf("media: open for detect: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("media: close detect file: %w", closeErr)
		}
	}()

	head := make([]byte, 262)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return DetectedType{}, fmt.Errorf("media: read head: %w", err)
	}
	if n == 0 {
		return DetectedType{}, ErrUnsupportedType
	}

	kind, err := filetype.Match(head[:n])
	if err != nil || kind == filetype.Unknown {
		return DetectedType{}, ErrUnsupportedType
	}

	mime := kind.MIME.Value
	if mime == "video/x-matroska" {
		mime = "video/webm"
	}

	k := mimeToKind(mime, allowImage, allowVideo)
	if k == KindUnknown {
		return DetectedType{}, ErrUnsupportedType
	}
	return DetectedType{Kind: k, MIME: mime, Ext: kind.Extension}, nil
}
```

- [ ] **Step 2: Verify mediautil tests and lint package**

Run:

```bash
gofmt -w app/media/internal/mediautil/detect.go
go test ./app/media/internal/mediautil
golangci-lint run ./app/media/internal/mediautil
```

Expected: tests pass and no `errcheck` issue remains for `detect.go`.

- [ ] **Step 3: Checkpoint**

Run:

```bash
git status --short
```

Expected: `app/media/internal/mediautil/detect.go` modified. Do not commit.

---

## Task 4: Apply Mechanical Lint Fixes

**Files:**
- Modify: `app/content/internal/logic/mock_models_test.go`
- Modify: `app/media/internal/logic/delete_media_logic.go`
- Modify: `app/media/internal/mediautil/detect_test.go`
- Modify: `app/content/internal/model/comment_model.go`
- Modify: `app/content/internal/model/post_model.go`
- Modify: `app/content/internal/model/post_tag_model.go`
- Modify: `app/content/internal/model/tag_model.go`
- Modify: `app/interaction/internal/logic/batch_check_favorited_logic.go`
- Modify: `app/interaction/internal/logic/batch_check_liked_logic.go`
- Modify: `app/interaction/internal/logic/check_favorited_logic.go`
- Modify: `app/interaction/internal/logic/check_liked_logic.go`
- Modify: `app/interaction/internal/logic/favorite_logic.go`
- Modify: `app/interaction/internal/logic/get_counts_logic.go`
- Modify: `app/interaction/internal/logic/get_favorite_list_logic.go`
- Modify: `app/interaction/internal/logic/like_logic.go`
- Modify: `app/interaction/internal/logic/unfavorite_logic.go`
- Modify: `app/interaction/internal/logic/unlike_logic.go`
- Modify: `app/interaction/internal/model/favorite_model.go`
- Modify: `app/interaction/internal/model/like_record_model.go`
- Modify: `app/media/internal/model/media_model.go`
- Modify: `app/feed/internal/mqs/post_publish_consumer_test.go`

- [ ] **Step 1: Run gofmt on reported formatting files**

Run:

```bash
gofmt -w app/content/internal/logic/mock_models_test.go app/media/internal/logic/delete_media_logic.go app/media/internal/mediautil/detect_test.go
```

Expected: no output.

- [ ] **Step 2: Remove embedded `CachedConn` selectors**

Run:

```bash
perl -0pi -e 's/\bm\.CachedConn\./m./g' \
  app/content/internal/model/comment_model.go \
  app/content/internal/model/post_model.go \
  app/content/internal/model/post_tag_model.go \
  app/content/internal/model/tag_model.go \
  app/interaction/internal/model/favorite_model.go \
  app/interaction/internal/model/like_record_model.go \
  app/media/internal/model/media_model.go
```

Expected: no output.

- [ ] **Step 3: Remove embedded `Logger` selectors**

Run:

```bash
perl -0pi -e 's/\bl\.Logger\./l./g' \
  app/interaction/internal/logic/batch_check_favorited_logic.go \
  app/interaction/internal/logic/batch_check_liked_logic.go \
  app/interaction/internal/logic/check_favorited_logic.go \
  app/interaction/internal/logic/check_liked_logic.go \
  app/interaction/internal/logic/favorite_logic.go \
  app/interaction/internal/logic/get_counts_logic.go \
  app/interaction/internal/logic/get_favorite_list_logic.go \
  app/interaction/internal/logic/like_logic.go \
  app/interaction/internal/logic/unfavorite_logic.go \
  app/interaction/internal/logic/unlike_logic.go
```

Expected: no output.

- [ ] **Step 4: Remove unused content service mock**

In `app/feed/internal/mqs/post_publish_consumer_test.go`, remove this import:

```go
	"esx/app/content/contentservice"
```

Delete this unused type and its methods:

```go
type mockContentService struct{ mock.Mock }

func (m *mockContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*contentservice.GetPostListResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockContentService) GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*contentservice.GetPostsByIdsResp), args.Error(1)
	}
	return nil, args.Error(1)
}
```

Keep the `google.golang.org/grpc` import because `mockUserService` still uses `grpc.CallOption`.

- [ ] **Step 5: Format all mechanically edited files**

Run:

```bash
gofmt -w \
  app/content/internal/model/comment_model.go \
  app/content/internal/model/post_model.go \
  app/content/internal/model/post_tag_model.go \
  app/content/internal/model/tag_model.go \
  app/interaction/internal/logic/batch_check_favorited_logic.go \
  app/interaction/internal/logic/batch_check_liked_logic.go \
  app/interaction/internal/logic/check_favorited_logic.go \
  app/interaction/internal/logic/check_liked_logic.go \
  app/interaction/internal/logic/favorite_logic.go \
  app/interaction/internal/logic/get_counts_logic.go \
  app/interaction/internal/logic/get_favorite_list_logic.go \
  app/interaction/internal/logic/like_logic.go \
  app/interaction/internal/logic/unfavorite_logic.go \
  app/interaction/internal/logic/unlike_logic.go \
  app/interaction/internal/model/favorite_model.go \
  app/interaction/internal/model/like_record_model.go \
  app/media/internal/model/media_model.go \
  app/feed/internal/mqs/post_publish_consumer_test.go
```

Expected: no output.

- [ ] **Step 6: Run focused tests for changed packages**

Run:

```bash
go test ./app/content/internal/model ./app/interaction/internal/logic ./app/interaction/internal/model ./app/media/internal/model ./app/feed/internal/mqs
```

Expected: packages compile and unit tests pass. If a package needs external services, record the exact dependency failure.

- [ ] **Step 7: Checkpoint**

Run:

```bash
git status --short
```

Expected: only planned files are modified. Do not commit.

---

## Task 5: Final Verification

**Files:**
- Read: `doc/examination/golangci-lint`
- Read: `.golangci.yml`
- Read: changed files from Tasks 1-4

- [ ] **Step 1: Verify no reported lint patterns remain by text search**

Run:

```bash
rg -n 'defer .*\.(Close|Shutdown)\(\)|defer os\.Remove|m\.CachedConn\.|l\.Logger\.' \
  app/feed/feed.go \
  app/media/media.go \
  app/message/message.go \
  app/media/internal/logic/upload_image_logic.go \
  app/media/internal/logic/upload_video_logic.go \
  app/media/internal/mediautil/detect.go \
  app/content/internal/model/comment_model.go \
  app/content/internal/model/post_model.go \
  app/content/internal/model/post_tag_model.go \
  app/content/internal/model/tag_model.go \
  app/interaction/internal/logic \
  app/interaction/internal/model \
  app/media/internal/model
```

Expected: no matches for the old lint-triggering patterns.

- [ ] **Step 2: Run cleanupx and focused package tests**

Run:

```bash
go test ./pkg/cleanupx/...
go test ./app/media/internal/logic ./app/media/internal/mediautil ./app/feed/internal/mqs ./app/message/internal/mqs
go test ./app/content/internal/model ./app/interaction/internal/logic ./app/interaction/internal/model ./app/media/internal/model
```

Expected: tests pass or failures are clearly unrelated external dependency failures.

- [ ] **Step 3: Run full project quality checks**

Run:

```bash
go test ./... -race -cover
go vet ./...
golangci-lint run
```

Expected:

```text
golangci-lint run
```

exits with code 0 and no issues. If `go test ./... -race -cover` or `go vet ./...` fails because MySQL, Redis, RocketMQ, S3, or testcontainers are unavailable, record the exact failing package and command output without changing business logic.

- [ ] **Step 4: Compare against original lint report**

Run:

```bash
golangci-lint run > /tmp/golangci-lint-after.txt
wc -l /tmp/golangci-lint-after.txt
```

Expected:

```text
0 /tmp/golangci-lint-after.txt
```

If the file is non-empty, inspect only issues in the planned scope first.

- [ ] **Step 5: Final checkpoint**

Run:

```bash
git status --short
```

Expected: planned source changes plus ignored docs if visible through direct path. Do not commit.
