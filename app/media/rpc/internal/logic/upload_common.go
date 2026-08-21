package logic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/cleanupx"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

const storageTypeSeaweedFS = 3

// sha256File 返回文件内容的 sha256 十六进制指纹。
// 上传内容属于幂等命令的一部分（CORE-050/051）：同键不同字节必须是不同命令。
// 所有请求上下文透传（AGENTS.md）：提前检查取消，关闭日志使用请求 ctx。
func sha256File(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	handle, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer cleanupx.Close(logx.WithContext(ctx), "hash upload file", handle)
	hasher := sha256.New()
	if _, err := io.Copy(hasher, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// mediaIdempotencyRecord 从上传元数据与文件内容指纹构造幂等记录（CORE-050）。
// contentHash 为接收到的文件字节 sha256：同键不同内容视为不同命令，
// 按 CORE-051 返回幂等冲突，而不是静默返回旧媒体。
func mediaIdempotencyRecord(meta *pb.UploadMeta, contentHash string) idempotencyx.IdempotencyRecord {
	if meta == nil {
		return idempotencyx.IdempotencyRecord{}
	}
	return idempotencyx.IdempotencyRecord{
		Scope:  "media:upload",
		UserID: meta.GetUserId(),
		Key:    strings.TrimSpace(meta.GetIdempotencyKey()),
		CommandHash: idempotencyx.CommandHash(
			meta.GetFileName(),
			contentHash,
			strconv.Itoa(int(meta.GetQuality())),
			strconv.Itoa(int(meta.GetMaxWidth())),
			strconv.Itoa(int(meta.GetMaxHeight())),
		),
	}
}

// receiveUploadStream 从 streaming 接收首包 meta + 后续 chunk，写入 sink。
func receiveUploadStream[Req any](
	recv func() (*Req, error),
	getMeta func(*Req) *pb.UploadMeta,
	getChunk func(*Req) []byte,
	sink *mediautil.TempSink,
) (*pb.UploadMeta, error) {
	first, err := recv()
	if err != nil {
		return nil, errx.Wrap(err, errx.UploadFailed)
	}
	meta := getMeta(first)
	if meta == nil {
		return nil, errx.NewWithCode(errx.MediaMetaMissing)
	}

	for {
		req, err := recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errx.Wrap(err, errx.UploadFailed)
		}
		chunk := getChunk(req)
		if chunk == nil {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if _, werr := sink.Write(chunk); werr != nil {
			if errors.Is(werr, mediautil.ErrSizeExceeded) {
				return nil, errx.NewWithCode(errx.FileTooLarge)
			}
			return nil, errx.Wrap(werr, errx.UploadFailed)
		}
	}
	return meta, nil
}

// buildObjectKey 组织对象键：{prefix}/YYYYMM/{uuid}.{ext}
func buildObjectKey(prefix, ext string) string {
	ym := time.Now().Format("200601")
	return fmt.Sprintf("%s/%s/%s.%s", prefix, ym, uuid.NewString(), ext)
}

// nullStringOr 包装非空字符串为 sql.NullString。
func nullStringOr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullInt 包装非 0 整数为 sql.NullInt64。
func nullInt(v int) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}

// removeOrphanObjects best-effort 删除幂等命中时本次上传的孤儿对象；
// 删除失败只告警（CORE-053：不影响已提交的成功响应）。
func removeOrphanObjects(ctx context.Context, logger logx.Logger, storage interface {
	Delete(ctx context.Context, objectKey string) error
}, objectKeys ...string) {
	if logger == nil || storage == nil {
		return
	}
	for _, key := range objectKeys {
		if key == "" {
			continue
		}
		if err := storage.Delete(ctx, key); err != nil {
			logger.Errorw("delete orphan object on idempotent retry failed",
				logx.Field("object_key", key), logx.Field("err", err.Error()))
		}
	}
}
