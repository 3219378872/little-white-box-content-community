package mediautil

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/h2non/filetype"
)

// MediaKind 媒体大类。
type MediaKind int

const (
	KindUnknown MediaKind = iota
	KindImage
	KindVideo
)

// DetectedType 是 Detect 的结果。
type DetectedType struct {
	Kind MediaKind
	MIME string
	Ext  string
}

// ErrUnsupportedType 表示检测到的类型不在白名单内。
var ErrUnsupportedType = errors.New("media: unsupported file type")

var (
	allowedImageMIMEs = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
		"image/webp": {},
	}
	allowedVideoMIMEs = map[string]struct{}{
		"video/mp4":        {},
		"video/quicktime":  {},
		"video/webm":       {},
		"video/x-matroska": {},
	}
)

func mimeToKind(mime string, allowImage, allowVideo bool) MediaKind {
	if allowImage {
		if _, ok := allowedImageMIMEs[mime]; ok {
			return KindImage
		}
	}
	if allowVideo {
		if _, ok := allowedVideoMIMEs[mime]; ok {
			return KindVideo
		}
	}
	return KindUnknown
}

// Detect 读取文件前 262 字节嗅探类型并按白名单过滤。
func Detect(path string, allowImage, allowVideo bool) (DetectedType, error) {
	f, err := os.Open(path)
	if err != nil {
		return DetectedType{}, fmt.Errorf("media: open for detect: %w", err)
	}
	defer f.Close()

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
