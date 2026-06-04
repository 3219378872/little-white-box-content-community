# Media 模块实现设计

- 作者：风过无痕
- 日期：2026-04-17
- 状态：设计通过，待生成实现计划
- 范围：`app/media/` 5 个 RPC 方法全量落地、对象存储接入 SeaweedFS（S3 兼容）、MySQL + Redis 持久化与缓存

## 1. 背景与目标

`proto/media/media.proto` 已定义 `MediaService` 的 5 个方法，`app/media/` 脚手架已由 goctl 生成但所有 `logic` 为 TODO 空壳。gateway 侧 `UploadImageMultipart` 已按 client streaming 协议调用 `media.UploadImage`，但尚无可用实现，致使图片上传链路断裂；GetMedia / DeleteMedia / BatchGetMedia 亦未实现。

### 1.1 本轮目标

- 将 5 个 RPC 方法落地，使得帖子发布链路中的图片上传、列表/详情中的媒体回读、用户侧媒体删除全部贯通。
- 引入 SeaweedFS 作为对象存储后端（通过 S3 兼容 API 接入，Go 侧仍用 `minio-go/v7` SDK）。
- 视频仅做原始存储，不引入 ffmpeg/ffprobe。

### 1.2 Out-of-scope

- `media_task` 表驱动的异步任务（压缩/转码/截图/水印）——保留表结构供未来使用，本轮不实现。
- 视频截图、转码、多分辨率——Phase-5 运维优化范畴。
- MinIO 孤儿对象清理脚本——Phase-5。
- gateway 侧代码改动（现有实现已兼容）。
- 预签名 URL、私密媒体访问控制（公开社区场景暂不需要）。

## 2. 架构

### 2.1 包结构

```
app/media/
├── etc/media.yaml              # 追加 DataSource / CacheRedis / S3Storage / Upload 段
├── internal/
│   ├── config/config.go        # 嵌入 MysqlConf / cache.CacheConf / S3StorageConf / UploadConf
│   ├── model/                  # goctl 生成 + 自写 FindByIds
│   │   ├── mediamodel_gen.go
│   │   ├── mediamodel.go       # goctl 产物，追加 FindByIds
│   │   ├── vars.go
│   │   └── mediataskmodel_gen.go  # 仅生成备用，本轮不实现业务
│   ├── storage/
│   │   └── s3.go               # minio-go 客户端封装（面向 SeaweedFS S3 gateway）
│   ├── mediautil/
│   │   ├── detect.go           # magic bytes 嗅探 + 白名单
│   │   ├── image.go            # imaging 压缩 + 缩略图
│   │   └── bufferfile.go       # 临时文件封装（有大小上限）
│   ├── svc/service_context.go  # 组装 Model + Storage + Config
│   ├── server/                 # 既有，无需改
│   └── logic/                  # 5 个 logic 落地
└── pb/  (已生成，不动)
```

### 2.2 分层职责

| 层 | 职责 | 不做什么 |
|---|---|---|
| `logic` | 流程编排：收流 → 落盘 → 校验 → 处理 → 入库 → 返 URL | 不直接调用 minio / sqlx 原生 API |
| `storage` | `ObjectStorage` 接口与 S3 实现 | 不关心业务规则 |
| `mediautil` | 纯函数：嗅探 / 压缩 / 临时文件 | 不依赖 `svcCtx` |
| `model` | DB CRUD + 缓存（goctl 生成） | 不感知业务语义 |

### 2.3 新增依赖（`go.mod`）

- `github.com/minio/minio-go/v7`：S3 兼容 SDK，连接 SeaweedFS `weed s3` gateway。
- `github.com/disintegration/imaging`：纯 Go 图片处理，无 cgo。
- `golang.org/x/image/webp`：为 `imaging.Decode` 增加 WebP 读支持（`imaging` 内置只支持 JPEG/PNG/GIF/TIFF/BMP）。以空白 import 注册即可。
- `github.com/h2non/filetype`：magic bytes 嗅探，比 `net/http.DetectContentType` 覆盖更全。
- `github.com/google/uuid`：对象键 UUID 生成。

## 3. 关键决策与理由

| # | 决策点 | 选择 | 理由 |
|---|---|---|---|
| Q1 | 范围 | 全量 5 个 RPC | 一次打通链路，避免 gateway 调用失败 |
| Q2 | 视频处理 | 仅原始存储 | 不引入 ffmpeg 系统依赖 |
| Q3 | 图片处理 | imaging 库 + 256px 缩略图 | 纯 Go、跨平台、与 gateway 传入的 quality=85 契合 |
| Q4 | 校验 | magic bytes 白名单 + 大小上限 | 防御改后缀攻击，`mime_type` 字段可写真值 |
| Q5 | URL 策略 | SeaweedFS 公开桶直链 | 公开社区媒体、CDN 友好、URL 不过期 |
| Q6 | 删除 | 软删 + 异步清理 | 历史引用不 404，可回滚；对象回收交给后续运维脚本 |
| Q7 | Model | goctl + 缓存 | 与 content/user 模块风格一致，`GetMedia` 高频走 Redis |
| Q8 | 缓冲策略 | 磁盘临时文件 + 流式上传 | 100 MB 视频不爆内存；多一次磁盘 IO 可接受 |
| Q8b | SeaweedFS 接入 | S3 兼容 API（`weed s3`） | 复用 `minio-go`，未来切 S3/OSS 零代码改动 |

## 4. 数据流

### 4.1 UploadImage / UploadVideo（client streaming）

```
client (gateway)                 media.logic                          storage(S3)           model
    │── stream: meta(首包) ─────────▶│ 校验 meta.UserId                  │                   │
    │── stream: chunk × N ──────────▶│ 写入 os.CreateTemp                 │                   │
    │                                │ io.Copy 累计，超限 → 早停返错     │                   │
    │── EOF ────────────────────────▶│ seek(0) → magic bytes 嗅探         │                   │
    │                                │ 不在白名单 → FileTypeNotAllowed   │                   │
    │                                │  [图片分支]                        │                   │
    │                                │    imaging.Decode(temp)            │                   │
    │                                │    Resize + JPEG 压缩 → 新 temp    │                   │
    │                                │    生成 256px 缩略图 → 新 temp     │                   │
    │                                │    PUT original/{YYYYMM}/{uuid}.jpg ▶│                  │
    │                                │    PUT thumb/{YYYYMM}/{uuid}.jpg ──▶│                  │
    │                                │  [视频分支]                        │                   │
    │                                │    PUT original/{YYYYMM}/{uuid}.{ext} ▶│               │
    │                                │  Insert row（url, thumbnail_url, width, height, mime_type, ...）──▶│
    │◀── UploadXxxResp { MediaInfo } ┤                                                         │
```

要点：
- Meta 必须是第一包，否则返 `MediaMetaMissing`；后续若再出现 meta，视为协议错误返 `ParamError`。
- 边收边写临时文件，每包检查累计 size 是否超限（早停）。
- `defer tempSink.Close()` 保证无论成功失败都清理临时文件。
- object_key 格式：`{original|thumb}/{YYYYMM}/{uuid}.{ext}`，按月分目录便于运维归档。
- `storage_type` 列统一写 `3`（SeaweedFS）；`bucket` 写配置中的 `Bucket`；`object_key` 写相对路径。

### 4.2 GetMedia / BatchGetMedia

```
logic ──▶ model.FindOne(ctx, id) / model.FindByIds(ctx, ids)
  │        （goctl 缓存自动命中 Redis）
  ├─ 过滤 status != 1 （软删不返）
  └─ 组装 pb.MediaInfo 返回
```

- `BatchGetMedia`：`len(media_ids) ∈ [1, 100]`；为空或超限返 `ParamError`；部分不存在或软删**不报错**，结果静默跳过。
- `FindByIds` 在 `mediamodel.go` 中自写：遍历 id 优先走 `FindOne` 走缓存，未命中的集合聚合一次 `WHERE id IN (?)` 批查，保证缓存语义一致。

### 4.3 DeleteMedia

```
logic ──▶ model.FindOne(ctx, id)
  │        ├─ not found → MediaNotFound
  │        ├─ status=0 → 已删，幂等返 nil
  │        └─ user_id != req.UserId → PermissionDenied
  │
  └─ model.Update(status=0)  （goctl 自动失效缓存）
       S3 对象不动（异步清理由 Phase-5 处理）
```

## 5. 接口与方法签名

### 5.1 logic 入口

```go
func (l *UploadImageLogic)    UploadImage(stream pb.MediaService_UploadImageServer) error
func (l *UploadVideoLogic)    UploadVideo(stream pb.MediaService_UploadVideoServer) error
func (l *GetMediaLogic)       GetMedia(in *pb.GetMediaReq) (*pb.GetMediaResp, error)
func (l *DeleteMediaLogic)    DeleteMedia(in *pb.DeleteMediaReq) (*pb.DeleteMediaResp, error)
func (l *BatchGetMediaLogic)  BatchGetMedia(in *pb.BatchGetMediaReq) (*pb.BatchGetMediaResp, error)
```

### 5.2 storage 层

```go
type ObjectStorage interface {
    Put(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error
    Delete(ctx context.Context, objectKey string) error     // 本轮 DeleteMedia 不调用（软删策略）；供集成测试清理与 Phase-5 孤儿对象清理脚本使用
    BuildPublicURL(objectKey string) string
}

type S3Client struct { /* minio.Client + bucket + publicBaseURL */ }
func NewS3Client(cfg config.S3StorageConf) (*S3Client, error)
```

`NewS3Client` 初始化时：
1. 构造 `minio.Client`。
2. `EnsureBucket`：不存在则 `MakeBucket(Region: cfg.Region)`。
3. `SetBucketPolicy`：公开读策略（`s3:GetObject` on `arn:aws:s3:::{bucket}/*`）。

### 5.3 mediautil 层（纯函数）

```go
type MediaKind int
const (
    KindUnknown MediaKind = iota
    KindImage
    KindVideo
)
type DetectedType struct { Kind MediaKind; MIME, Ext string }

func Detect(path string, allowImage, allowVideo bool) (DetectedType, error)

func CompressImage(srcPath string, maxW, maxH, quality int) (outPath string, w, h int, err error)
func MakeThumbnail(srcPath string) (outPath string, err error)

type TempSink struct { /* file, path, written, limit */ }
func NewTempSink(dir string, limit int64) (*TempSink, error)
func (t *TempSink) Write(p []byte) (int, error)
func (t *TempSink) Path() string
func (t *TempSink) Size() int64
func (t *TempSink) Close() error
```

- `CompressImage` 对 PNG/WebP 输入一律转 JPEG 输出（JPEG 对照片型内容压缩率最高），内部用 `imaging.Fit`（不放大）；输出 `mime_type` 统一写 `image/jpeg`，object_key 后缀 `.jpg`。
- WebP 解码依赖 `golang.org/x/image/webp` 的空白 import（在 `mediautil/image.go` 顶部 `import _ "golang.org/x/image/webp"`），否则 `imaging.Decode` 对 WebP 会报 unknown format。
- `TempSink.Close()` 关闭文件句柄后再 `os.Remove`，即使 Write 过程中途失败也能清理。
- `maxW == 0 || maxH == 0` 表示不限制该维度。

### 5.4 model 层补丁

```go
type (
    MediaModel interface {
        mediaModel
        FindByIds(ctx context.Context, ids []int64) ([]*Media, error)
    }
    customMediaModel struct { *defaultMediaModel }
)

func (m *customMediaModel) FindByIds(ctx context.Context, ids []int64) ([]*Media, error)
```

### 5.5 Config 结构

```go
type Config struct {
    zrpc.RpcServerConf
    DataSource string
    CacheRedis cache.CacheConf
    S3Storage  S3StorageConf
    Upload     UploadConf
}

type S3StorageConf struct {
    Endpoint      string
    AccessKey     string
    SecretKey     string
    UseSSL        bool
    Region        string
    Bucket        string
    PublicBaseURL string
}

type UploadConf struct {
    MaxImageSize      int64
    MaxVideoSize      int64
    DefaultQuality    int
    ThumbnailLongSide int
    TempDir           string
}
```

## 6. 错误处理

### 6.1 错误码增量（追加到 `pkg/errx/codes.go`）

```go
MediaNotFound      = 4004
MediaMetaMissing   = 4005
MediaProcessFailed = 4006
```

### 6.2 错误码映射

| 场景 | 错误码 |
|---|---|
| 参数校验失败（media_id ≤ 0、ids 为空/过多、quality 越界） | `ParamError` (2) |
| 文件超过大小上限 | `FileTooLarge` (4001) |
| magic bytes 不在白名单 | `FileTypeNotAllowed` (4002) |
| S3 / DB / 磁盘等基础设施异常 | `UploadFailed` (4003) 或 `SystemError` (3) |
| DeleteMedia 时 user_id 不匹配 | `PermissionDenied` (1007) |
| GetMedia 找不到 / 软删 | `MediaNotFound` (4004) |
| Upload 首包非 meta | `MediaMetaMissing` (4005) |
| 图片压缩/缩略图失败 | `MediaProcessFailed` (4006) |
| BatchGetMedia 中部分缺失 | 不报错，静默跳过 |

### 6.3 错误透传策略

所有 logic 一律通过 `errx.NewWithCode(code)` 返回；原始错误仅记录到 `logx` 日志，不透传给客户端。

### 6.4 Streaming 早停

发现超限或类型错误时，logic 立即返回错误；client 的后续 `Send` 会拿到 EOF 并通过 `CloseAndRecv` 取回状态。临时文件由 `defer` 清理，不尝试吸完剩余 chunk。

## 7. 测试策略

### 7.1 单元测试（mediautil）

- `detect_test.go`：table-driven，覆盖 6 类合法魔数、改扩展名伪装、空文件、Kind 过滤（仅允许图片时收到 MP4）。
- `image_test.go`：压缩后尺寸受限、缩略图长边 = 256、不放大。
- `bufferfile_test.go`：正常写入、超限返 `ErrSizeExceeded`、`Close()` 后文件被删除。

### 7.2 集成测试（logic）

- 真实 MySQL + Redis + SeaweedFS（依赖 `deploy/docker-compose.middleware.yml` 拉起）。
- 每个用例前清理测试 prefix；通过环境变量 `TEST_SKIP_INTEGRATION=1` 跳过。
- 覆盖用例：
  - `UploadImage`：正常 3 MB → 成功；首包非 meta → `MediaMetaMissing`；超 10 MB → `FileTooLarge`；PDF → `FileTypeNotAllowed`。
  - `GetMedia`：存在 → 成功；不存在 / status=0 → `MediaNotFound`。
  - `DeleteMedia`：归属正确 → DB status=0；user_id 错 → `PermissionDenied`；重复删 → 幂等。
  - `BatchGetMedia`：5 条正常；夹 1 条软删只返 4 条；空入参/>100 → `ParamError`。

### 7.3 覆盖率目标

- `mediautil/` ≥ 90%
- `internal/logic/` ≥ 75%
- `internal/storage/` 不强求（薄封装）

## 8. 部署 & 配置改动

### 8.1 docker-compose

在 `deploy/docker-compose.middleware.yml` 新增 `seaweedfs` 服务（master + volume + filer + s3 gateway），暴露端口 9333/8080/8888/8333；保留现有 MinIO 容器但 media 不再依赖。新增 `deploy/seaweedfs/s3_config.json` 声明 identity。

### 8.2 `app/media/etc/media.yaml`

```yaml
Name: media.rpc
ListenOn: 0.0.0.0:8080
Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: media.rpc

DataSource: "root:${MYSQL_ROOT_PASSWORD}@tcp(127.0.0.1:3306)/xbh_media?parseTime=true"

CacheRedis:
  - Host: 127.0.0.1:6379
    Type: node

S3Storage:
  Endpoint: "127.0.0.1:8333"
  AccessKey: ${S3_ACCESS_KEY}
  SecretKey: ${S3_SECRET_KEY}
  UseSSL: false
  Region: "us-east-1"
  Bucket: "xbh-media"
  PublicBaseURL: "http://127.0.0.1:8333/xbh-media"

Upload:
  MaxImageSize: 10485760
  MaxVideoSize: 104857600
  DefaultQuality: 85
  ThumbnailLongSide: 256
  TempDir: ""
```

### 8.3 `deploy/sql/xbh_media.sql`

`storage_type` 列注释扩展为 `1:MinIO 2:OSS 3:SeaweedFS`，无结构变更；本轮插入记录均 `storage_type = 3`。

## 9. 落地顺序

1. 依赖引入（`go.mod`、`go.sum`）+ 错误码新增（`pkg/errx/codes.go`）
2. goctl 生成 model（`--style go_zero`），追加 `FindByIds`
3. `internal/storage/s3.go`（含 EnsureBucket + SetBucketPolicy）+ 单测骨架
4. `internal/mediautil/*`（三文件 + 各自单测）
5. `internal/config` + `internal/svc` 接线
6. 5 个 logic 依次落地：UploadImage → UploadVideo → GetMedia → DeleteMedia → BatchGetMedia
7. 集成测试用例补齐
8. docker-compose 新增 SeaweedFS + `s3_config.json`
9. `etc/media.yaml` 与 `xbh_media.sql` 注释调整

## 10. 风险与缓解

| 风险 | 等级 | 缓解 |
|---|---|---|
| SeaweedFS S3 API 与 MinIO 行为差异（如 ListObjects 分页） | LOW | 本轮仅用 Put/Delete/GetObject，差异面极小 |
| imaging 对大图内存占用（解码时整图驻留） | MEDIUM | 10 MB 上限下解码后峰值 ~100 MB 可控；必要时加 `imaging.MaxInputWidth` 守护 |
| goctl 生成的 `FindByIds` 不带缓存 | LOW | 自写时手动先走 `FindOne` 命中缓存，未命中再批查 |
| 开发同学本地没启动 SeaweedFS 导致集成测试失败 | LOW | 环境变量 `TEST_SKIP_INTEGRATION=1` 跳过 |
| 公开桶策略被误改为私密 | MEDIUM | `NewS3Client` 每次启动 `SetBucketPolicy`，保证幂等恢复 |

## 11. 变更边界

本设计改动限于：
- `app/media/**`
- `pkg/errx/codes.go`（仅新增 3 个常量 + 文案）
- `deploy/docker-compose.middleware.yml`（新增 seaweedfs 服务）
- `deploy/seaweedfs/s3_config.json`（新增文件）
- `deploy/sql/xbh_media.sql`（仅修改注释）
- `go.mod` / `go.sum`

不涉及 gateway / user / content 现有代码。
