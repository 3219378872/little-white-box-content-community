package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config 是对象存储连接参数。
type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	Bucket        string
	PublicBaseURL string
}

// ObjectStorage 是清理消费者需要的对象存储操作。
type ObjectStorage interface {
	Delete(ctx context.Context, objectKey string) error
	BuildPublicURL(objectKey string) string
}

// S3Client 基于 minio-go 实现 ObjectStorage。
type S3Client struct {
	cli           *minio.Client
	bucket        string
	publicBaseURL string
}

// NewS3Client 连接对象存储并确保桶存在，启动时即暴露配置错误。
func NewS3Client(cfg Config) (*S3Client, error) {
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("media-mq: init s3 client: %w", err)
	}
	client := &S3Client{
		cli:           cli,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}
	ensureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = client.ensureBucket(ensureCtx, cfg.Region); err != nil {
		return nil, err
	}
	return client, nil
}

// ensureBucket 在桶不存在时创建。
func (s *S3Client) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.cli.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("media-mq: bucket exists check: %w", err)
	}
	if exists {
		return nil
	}
	return s.cli.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region})
}

// Delete 删除对象；对象不存在时也返回成功。
func (s *S3Client) Delete(ctx context.Context, objectKey string) error {
	if err := s.cli.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("media-mq: remove object %s: %w", objectKey, err)
	}
	return nil
}

// BuildPublicURL 拼接对象的公开访问地址。
func (s *S3Client) BuildPublicURL(objectKey string) string {
	return s.publicBaseURL + "/" + objectKey
}
