package logic

import (
	"database/sql"
	"errors"
	"errx"
	"fmt"
	"io"
	"time"

	"esx/app/media/internal/mediautil"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/google/uuid"
)

const storageTypeSeaweedFS = 3

// receiveUploadStream 从 streaming 接收首包 meta + 后续 chunk，写入 sink。
func receiveUploadStream[Req any](
	recv func() (*Req, error),
	getMeta func(*Req) *pb.UploadMeta,
	getChunk func(*Req) []byte,
	sink *mediautil.TempSink,
) (*pb.UploadMeta, error) {
	first, err := recv()
	if err != nil {
		return nil, fmt.Errorf("media: recv first packet: %w", err)
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
			return nil, fmt.Errorf("media: recv chunk: %w", err)
		}
		chunk := getChunk(req)
		if chunk == nil {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if _, werr := sink.Write(chunk); werr != nil {
			if errors.Is(werr, mediautil.ErrSizeExceeded) {
				return nil, errx.NewWithCode(errx.FileTooLarge)
			}
			return nil, fmt.Errorf("media: write chunk: %w", werr)
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
