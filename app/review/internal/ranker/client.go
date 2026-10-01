// Package ranker 是精排 sidecar（moderation-infer）的 gRPC 客户端。
package ranker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"esx/app/review/internal/cascade"
	pb "esx/app/review/xiaobaihe/moderation/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Client 实现 cascade.Ranker。
type Client struct {
	conn   *grpc.ClientConn
	client pb.ModerationInferServiceClient
}

// Dial 建立到 sidecar 的连接；连接是惰性的，不可用在调用时按降级处理。
func Dial(address string) (*Client, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("ranker: address is required")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		return nil, fmt.Errorf("ranker: dial: %w", err)
	}
	return &Client{conn: conn, client: pb.NewModerationInferServiceClient(conn)}, nil
}

// NewWithClient 便于测试注入。
func NewWithClient(client pb.ModerationInferServiceClient) *Client {
	return &Client{client: client}
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Score 调用精排；超时映射为 context.DeadlineExceeded，其余故障映射为 cascade.ErrUnavailable。
func (c *Client) Score(ctx context.Context, in cascade.RankInput) (cascade.RankResult, error) {
	resp, err := c.client.Score(ctx, &pb.ScoreReq{
		TaskId: in.TaskID, Market: in.Market, Language: in.Language, Text: in.Text,
		MediaSha256: in.Media, Issues: in.Issues,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
			return cascade.RankResult{}, fmt.Errorf("ranker: %w", context.DeadlineExceeded)
		}
		return cascade.RankResult{}, fmt.Errorf("ranker: %s: %w", status.Code(err), cascade.ErrUnavailable)
	}
	out := cascade.RankResult{ModelVersion: resp.GetModelVersion()}
	for _, score := range resp.GetScores() {
		out.Scores = append(out.Scores, cascade.IssueScore{Issue: score.GetIssue(), PYes: score.GetPYes(), PNo: score.GetPNo()})
	}
	return out, nil
}
