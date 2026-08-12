package logic

import (
	"database/sql"
	"errors"
	"errx"
	"esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const storageTypeSeaweedFS = 3

// mediaIdempotencyRecord 从上传元数据构造幂等记录（CORE-050）。
func mediaIdempotencyRecord(meta *pb.UploadMeta) model.IdempotencyRecord {
	if meta == nil {
		return model.IdempotencyRecord{}
	}
	return model.IdempotencyRecord{
		Scope:  "media:upload",
		UserID: meta.GetUserId(),
		Key:    strings.TrimSpace(meta.GetIdempotencyKey()),
		CommandHash: model.CommandHash(
			meta.GetFileName(),
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
