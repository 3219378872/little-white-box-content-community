// Package milvusx holds the Milvus connection behavior shared by services:
// direct (proxy-free) gRPC dialing and retrying while Milvus is still booting.
package milvusx

import (
	"context"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"google.golang.org/grpc"
)

// IsStartupError reports the transient errors Milvus returns while its
// coordinators are still starting; every other error is permanent.
func IsStartupError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not ready") || strings.Contains(message, "service unavailable")
}

// RetryStartup calls fn until it succeeds or returns a non-startup error,
// waiting interval between attempts. When ctx ends first, the last startup
// error is returned so callers can report what Milvus was still saying.
func RetryStartup(ctx context.Context, interval time.Duration, fn func() error) error {
	for {
		err := fn()
		if err == nil || !IsStartupError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(interval):
		}
	}
}

// Dial connects with config, bypassing any HTTP(S) proxy from the environment
// (Milvus is always reached directly), and retries startup errors until ctx ends.
func Dial(ctx context.Context, config client.Config, retryInterval time.Duration) (client.Client, error) {
	config.DialOptions = append(config.DialOptions, grpc.WithNoProxy())
	var cli client.Client
	err := RetryStartup(ctx, retryInterval, func() error {
		var dialErr error
		cli, dialErr = client.NewClient(ctx, config)
		return dialErr
	})
	if err != nil {
		return nil, err
	}
	return cli, nil
}
