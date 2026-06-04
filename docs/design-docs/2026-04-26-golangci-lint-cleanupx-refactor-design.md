# golangci-lint cleanupx 小重构设计

**日期**: 2026-04-26
**范围**: `pkg/cleanupx`、`go.work`、当前 `golangci-lint` 报告涉及的文件
**关联输入**: `doc/examination/golangci-lint`

---

## 背景

当前 `golangci-lint` 报告共有 53 条问题：

- `errcheck`: 9 条
- `gofmt`: 3 条
- `staticcheck` / `QF1008`: 38 条
- `unused`: 3 条

其中 `errcheck` 暴露的是重复模式：`Close`、`Remove`、`Shutdown` 这类清理操作在 `defer` 中直接调用，错误被忽略。项目约定要求显式处理错误，并要求日志带上下文，因此本次采用小型公共 helper 收敛这类模式。

## 目标

1. 修复 `doc/examination/golangci-lint` 中列出的 53 条问题。
2. 新增 `pkg/cleanupx`，统一 defer 清理错误的日志记录方式。
3. 保持业务语义不变：清理失败只记录日志，不覆盖主流程返回值。
4. 不修改 `.api`、`.proto`、数据库 schema、生成代码或业务接口。
5. 不引入第三方依赖。

## 非目标

- 不重构 feed、media、message 的启动流程结构。
- 不调整 MQ 消费语义、上传流程、文件检测策略或返回错误码。
- 不为了使用 helper 改变底层工具函数签名。
- 不修复本报告之外的历史 lint 或测试问题。

## 方案

采用公共包 `pkg/cleanupx`。它只封装清理函数的错误处理，不承载业务决策。

### 1. 包结构

新增独立 workspace module：

| 路径 | 说明 |
|------|------|
| `pkg/cleanupx/go.mod` | module 名称为 `cleanupx` |
| `pkg/cleanupx/cleanup.go` | 清理 helper 实现 |
| `pkg/cleanupx/cleanup_test.go` | helper 单元测试 |
| `go.work` | 加入 `./pkg/cleanupx` |

`pkg/cleanupx` 依赖标准库和 `github.com/zeromicro/go-zero/core/logx`。调用方继续传递已有 `ctx` 和 logger，不创建新的业务 context。

### 2. API 设计

```go
package cleanupx

import (
    "io"

    "github.com/zeromicro/go-zero/core/logx"
)

func Close(logger logx.Logger, resource string, closer io.Closer)
func Remove(logger logx.Logger, path string)
func Shutdown(logger logx.Logger, resource string, shutdown func() error)
```

行为约定：

- `closer` 或 `shutdown` 为 `nil` 时直接返回，避免 defer 清理引入 panic。
- `logger` 必须由调用方提供；Logic 层传 `l.Logger`，入口层传 `logx.WithContext(...)`。
- 清理成功时不写日志，避免噪音。
- 清理失败时调用 `logger.Errorw(...)`，字段包含 `resource` 或 `path` 以及 `err`。
- helper 不返回错误，适合 `defer` 场景；主流程错误仍由调用方负责。

不把 `context.Context` 放进 helper 入参，因为项目中 `logx.WithContext(ctx)` 已在 logic 初始化时完成。入口函数中也可以显式传入 `logx.WithContext(context.Background())` 或启动上下文对应 logger，但不能使用裸 `logx.Error`。

### 3. 调用点改造

#### MQ consumer shutdown

替换裸 `defer Shutdown()`：

- `app/feed/feed.go`
- `app/media/media.go`
- `app/message/message.go`

设计：

```go
defer cleanupx.Shutdown(logx.WithContext(context.Background()), "post publish consumer", postConsumer.Shutdown)
```

入口层当前没有请求级 ctx。此处允许使用最外层入口 context，目的是为服务生命周期清理日志提供统一 logger；不向业务 RPC 调用传递新 context。

#### Media 上传临时资源

替换裸 `defer sink.Close()` 和 `defer os.Remove(...)`：

- `app/media/internal/logic/upload_image_logic.go`
- `app/media/internal/logic/upload_video_logic.go`

设计：

```go
defer cleanupx.Close(l.Logger, "upload image temp sink", sink)
defer cleanupx.Remove(l.Logger, compressedPath)
defer cleanupx.Remove(l.Logger, thumbPath)
```

上传主流程仍按现有逻辑返回 `errx`，临时资源清理失败只记录日志。

#### 本地文件关闭

`putFile` 已接收 `ctx context.Context`，因此不改变函数签名，直接在函数内部使用 `logx.WithContext(ctx)` 构造 logger：

```go
defer cleanupx.Close(logx.WithContext(ctx), "upload source file", f)
```

这样可以修复 `errcheck`，同时避免把 logger 继续向下透传到本地文件上传 helper。

`app/media/internal/mediautil/detect.go` 不强行依赖 `cleanupx`。该包是底层工具包，当前函数没有 logger 入参；为了修复 `errcheck` 而改变 `Detect` 签名会扩大影响。设计上采用局部 defer 函数处理 `Close` 返回值：

```go
defer func() {
    if closeErr := f.Close(); closeErr != nil && err == nil {
        err = fmt.Errorf("media: close detect file: %w", closeErr)
    }
}()
```

如实现中采用命名返回值，需要确保原有 open/read/type 检测错误优先级不变。

### 4. 机械 lint 收敛

#### gofmt

直接格式化：

- `app/content/internal/logic/mock_models_test.go`
- `app/media/internal/logic/delete_media_logic.go`
- `app/media/internal/mediautil/detect_test.go`

#### QF1008

删除不必要的嵌入字段选择器，行为不变：

- `m.CachedConn.Query...` 改为 `m.Query...`
- `l.Logger.Error...` 改为 `l.Error...`

只修改报告列出的文件和行附近，不做额外风格迁移。

#### unused

删除 `app/feed/internal/mqs/post_publish_consumer_test.go` 中未使用的 `mockContentService` 和对应方法。若后续测试需要 content service mock，再在对应测试中重新引入。

## 测试设计

### cleanupx 单元测试

覆盖：

1. `nil closer` 和 `nil shutdown` 不 panic。
2. 成功 close/remove/shutdown 不返回错误。
3. 失败 close/shutdown 会执行日志路径。
4. `Remove` 对不存在路径记录错误但不 panic。

日志内容不做强耦合断言，避免测试依赖 go-zero 日志内部实现；测试关注 helper 不 panic、清理函数被调用、错误路径可执行。

### 相关包验证

优先执行小范围命令：

```bash
go test ./pkg/cleanupx/...
go test ./app/media/internal/logic ./app/media/internal/mediautil ./app/feed/internal/mqs
```

最终执行：

```bash
golangci-lint run
```

若全量测试受 MySQL、Redis、RocketMQ、S3 等外部依赖影响失败，记录失败原因，不为通过测试改变业务语义。

## 风险与约束

### 新公共包接入

`pkg/cleanupx` 是独立 module，必须加入 `go.work`。调用方使用 import path `cleanupx`，对齐现有 `errx`、`mqx`、`util` 风格。

### 日志上下文

Logic 层使用已有 `l.Logger`，天然带请求 context。服务入口清理日志没有请求 context，只能使用入口级 logger；不能使用裸 `logx.Error`。

### detect.go 错误优先级

`Detect` 的主要错误仍是 open/read/type 检测错误。文件关闭错误只在没有更早错误时返回，避免掩盖更有价值的根因。

## 验收标准

1. `doc/examination/golangci-lint` 中 53 条问题均被消除。
2. `pkg/cleanupx` 有单元测试，且测试通过。
3. 修改后不改变上传、检测、MQ consumer 启停的业务语义。
4. `golangci-lint run` 不再报告本次范围内的问题。
5. 不新增第三方依赖，不修改生成代码。
