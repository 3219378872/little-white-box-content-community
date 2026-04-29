package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	Bucket        string
	PublicBaseURL string
}

type ObjectStorage interface {
	Delete(ctx context.Context, objectKey string) error
	BuildPublicURL(objectKey string) string
}

type S3Client struct {
	cli           *minio.Client
	bucket        string
	publicBaseURL string
}

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
	if err = client.ensureBucket(context.Background(), cfg.Region); err != nil {
		return nil, err
	}
	return client, nil
}

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

func (s *S3Client) Delete(ctx context.Context, objectKey string) error {
	if err := s.cli.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("media-mq: remove object %s: %w", objectKey, err)
	}
	return nil
}

func (s *S3Client) BuildPublicURL(objectKey string) string {
	return s.publicBaseURL + "/" + objectKey
}
