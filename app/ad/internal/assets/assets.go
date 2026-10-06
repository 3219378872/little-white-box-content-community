// Package assets 管理广告素材与资质证件的私有存储与过审发布（ADS-015、DES-sponsored-ads「素材」）。
//
// 私有桶不设置公开读策略；过审素材由 ad-mq 按 ads/<sha256> 复制到公开媒体桶，同一内容只写一次、
// 不可覆盖。公开路径经根仓反代的同源 /xbh-media/ 提供。
package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/h2non/filetype"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MaxAssetBytes 受内部 gRPC 单消息上限约束，素材与证件统一 2 MiB。
const MaxAssetBytes = 2 << 20

var (
	ErrTooLarge        = errors.New("assets: file too large")
	ErrTypeNotAllowed  = errors.New("assets: file type not allowed")
	ErrEmpty           = errors.New("assets: empty file")
	creativeMimeTypes  = []string{"image/jpeg", "image/png", "image/webp"}
	documentMimeTypes  = []string{"image/jpeg", "image/png", "image/webp", "application/pdf"}
	publicPrefix       = "ads/"
	privateKeyTemplate = "assets/%d"
)

// Config 是对象存储参数。
type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	PrivateBucket string
	PublicBucket  string
	PublicBaseURL string
}

// Inspected 是上传内容的校验结果。
type Inspected struct {
	SHA256   string
	MimeType string
	Size     int64
}

// Inspect 校验大小与类型（按内容识别，不信任文件名）。
func Inspect(kind string, content []byte, allowDocuments bool) (Inspected, error) {
	if len(content) == 0 {
		return Inspected{}, ErrEmpty
	}
	if len(content) > MaxAssetBytes {
		return Inspected{}, ErrTooLarge
	}
	detected, err := filetype.Match(content)
	if err != nil {
		return Inspected{}, ErrTypeNotAllowed
	}
	allowed := creativeMimeTypes
	if allowDocuments {
		allowed = documentMimeTypes
	}
	mime := detected.MIME.Value
	ok := false
	for _, candidate := range allowed {
		if mime == candidate {
			ok = true
		}
	}
	if !ok {
		return Inspected{}, ErrTypeNotAllowed
	}
	sum := sha256.Sum256(content)
	return Inspected{SHA256: hex.EncodeToString(sum[:]), MimeType: mime, Size: int64(len(content))}, nil
}

// PrivateKey 返回资产在私有桶中的对象键。
func PrivateKey(assetID int64) string { return fmt.Sprintf(privateKeyTemplate, assetID) }

// PublicKey 返回过审素材的内容寻址键。
func PublicKey(sha string) string { return publicPrefix + strings.ToLower(sha) }

// Storage 封装私有桶与公开桶。
type Storage struct {
	cli           *minio.Client
	private       string
	public        string
	publicBaseURL string
}

// New 连接对象存储并确保私有桶与公开桶都存在；两个桶必须不同，未过审素材才不会被公开访问（ADS-015）。
func New(ctx context.Context, cfg Config) (*Storage, error) {
	if cfg.PrivateBucket == "" || cfg.PublicBucket == "" || cfg.PrivateBucket == cfg.PublicBucket {
		return nil, fmt.Errorf("assets: distinct private and public buckets are required")
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("assets: init client: %w", err)
	}
	s := &Storage{cli: cli, private: cfg.PrivateBucket, public: cfg.PublicBucket, publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/")}
	for _, bucket := range []string{cfg.PrivateBucket, cfg.PublicBucket} {
		exists, err := cli.BucketExists(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("assets: bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
				return nil, fmt.Errorf("assets: make bucket %s: %w", bucket, err)
			}
		}
	}
	return s, nil
}

// PutPrivate 写入私有桶。
func (s *Storage) PutPrivate(ctx context.Context, key string, content []byte, mime string) error {
	_, err := s.cli.PutObject(ctx, s.private, key, bytes.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{ContentType: mime})
	if err != nil {
		return fmt.Errorf("assets: put private: %w", err)
	}
	return nil
}

// DeletePrivate 删除私有对象；对象不存在时返回 nil（幂等）。
func (s *Storage) DeletePrivate(ctx context.Context, key string) error {
	if err := s.cli.RemoveObject(ctx, s.private, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("assets: delete private: %w", err)
	}
	return nil
}

// GetPrivate 读取私有对象（经 Gateway 鉴权后流式返回）。
func (s *Storage) GetPrivate(ctx context.Context, key string) ([]byte, error) {
	object, err := s.cli.GetObject(ctx, s.private, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("assets: get private: %w", err)
	}
	defer func() { _ = object.Close() }()
	content, err := io.ReadAll(io.LimitReader(object, MaxAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("assets: read private: %w", err)
	}
	return content, nil
}

// Publish 把过审素材复制到公开内容寻址路径；已存在时不覆盖（ADS-015）。
func (s *Storage) Publish(ctx context.Context, privateKey, sha string) error {
	target := PublicKey(sha)
	if _, err := s.cli.StatObject(ctx, s.public, target, minio.StatObjectOptions{}); err == nil {
		return nil
	} else if minio.ToErrorResponse(err).Code != "NoSuchKey" && minio.ToErrorResponse(err).StatusCode != 404 {
		return fmt.Errorf("assets: stat public: %w", err)
	}
	_, err := s.cli.CopyObject(ctx, minio.CopyDestOptions{Bucket: s.public, Object: target},
		minio.CopySrcOptions{Bucket: s.private, Object: privateKey})
	if err != nil {
		return fmt.Errorf("assets: publish: %w", err)
	}
	return nil
}

// PublicURL 返回过审素材的公开地址。
func (s *Storage) PublicURL(sha string) string { return PublicURLFor(s.publicBaseURL, sha) }

// PublicURLFor 便于不持有存储客户端的组件生成公开地址。
func PublicURLFor(base, sha string) string {
	return strings.TrimRight(base, "/") + "/" + PublicKey(sha)
}
