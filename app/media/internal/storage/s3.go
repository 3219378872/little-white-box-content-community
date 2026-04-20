package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zeromicro/go-zero/core/logx"
)

// Config 聚合对象存储所需参数。
type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	Bucket        string
	PublicBaseURL string
}

// ObjectStorage 对 logic 层暴露的最小接口。
type ObjectStorage interface {
	Put(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, objectKey string) error
	BuildPublicURL(objectKey string) string
}

// S3Client 基于 minio-go 的 ObjectStorage 实现。
type S3Client struct {
	cli           *minio.Client
	bucket        string
	publicBaseURL string
}

// NewS3Client 构建并初始化：连接 + EnsureBucket + SetPublicReadPolicy。
func NewS3Client(cfg Config) (*S3Client, error) {
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("media: init s3 client: %w", err)
	}

	client := &S3Client{
		cli:           cli,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}
	if err = client.ensureBucket(context.Background(), cfg.Region); err != nil {
		return nil, err
	}
	if err = client.setPublicReadPolicy(context.Background()); err != nil {
		logx.Errorw("set public read policy failed (non-blocking, may need manual anonymous-read config)",
			logx.Field("bucket", cfg.Bucket),
			logx.Field("err", err.Error()),
		)
	}
	return client, nil
}

func (s *S3Client) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.cli.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("media: bucket exists check: %w", err)
	}
	if exists {
		return nil
	}
	if err = s.cli.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		return fmt.Errorf("media: make bucket: %w", err)
	}
	return nil
}

func (s *S3Client) setPublicReadPolicy(ctx context.Context) error {
	policy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": ["*"]},
    "Action": ["s3:GetObject"],
    "Resource": ["arn:aws:s3:::%s/*"]
  }]
}`, s.bucket)
	if err := s.cli.SetBucketPolicy(ctx, s.bucket, policy); err != nil {
		return fmt.Errorf("media: set bucket policy: %w", err)
	}
	return nil
}

// Put 流式上传对象。
func (s *S3Client) Put(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := s.cli.PutObject(ctx, s.bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("media: put object %s: %w", objectKey, err)
	}
	return nil
}

// Delete 删除对象，对象不存在时返 nil（幂等）。
func (s *S3Client) Delete(ctx context.Context, objectKey string) error {
	if err := s.cli.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("media: remove object %s: %w", objectKey, err)
	}
	return nil
}

// BuildPublicURL 根据 publicBaseURL 拼接完整公开直链。
func (s *S3Client) BuildPublicURL(objectKey string) string {
	return s.publicBaseURL + "/" + objectKey
}
