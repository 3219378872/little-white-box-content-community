package logic

import (
	"context"
	"errors"

	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// GetReviewerLogic 承载 GetReviewer 接口的业务逻辑；每个请求新建一个实例。
type GetReviewerLogic struct{ base }

// NewGetReviewerLogic 绑定请求上下文与服务依赖。
func NewGetReviewerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewerLogic {
	return &GetReviewerLogic{newBase(ctx, svcCtx)}
}

// GetReviewer 返回当前用户的角色；非审核员返回 active=false，便于客户端隐藏入口（FX-112）。
func (l *GetReviewerLogic) GetReviewer(in *pb.GetReviewerReq) (*pb.GetReviewerResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	r, err := l.svcCtx.Store.Reviewer(l.ctx, in.GetUserId())
	if errors.Is(err, store.ErrReviewerUnknown) {
		return &pb.GetReviewerResp{}, nil
	}
	if err != nil {
		l.Errorw("load reviewer failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if !r.Active {
		return &pb.GetReviewerResp{}, nil
	}
	return &pb.GetReviewerResp{Active: true, Roles: r.Roles(), Markets: r.Markets(), Languages: r.Languages()}, nil
}

// GetQueueSummaryLogic 承载 GetQueueSummary 接口的业务逻辑；每个请求新建一个实例。
type GetQueueSummaryLogic struct{ base }

// NewGetQueueSummaryLogic 绑定请求上下文与服务依赖。
func NewGetQueueSummaryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetQueueSummaryLogic {
	return &GetQueueSummaryLogic{newBase(ctx, svcCtx)}
}

// GetQueueSummary 按送审目的汇总审核员可领取的待审数量与最老等待时长。
func (l *GetQueueSummaryLogic) GetQueueSummary(in *pb.GetQueueSummaryReq) (*pb.GetQueueSummaryResp, error) {
	r, err := l.reviewer(in.GetUserId(), store.RoleReviewer, store.RoleQA, store.RoleQualificationReviewer)
	if err != nil {
		return nil, err
	}
	buckets, err := l.svcCtx.Store.QueueSummary(l.ctx, r, l.svcCtx.Now())
	if err != nil {
		return nil, l.mapStoreError(err, "queue summary")
	}
	resp := &pb.GetQueueSummaryResp{PolicyVersion: l.svcCtx.Policy.Version}
	for _, bucket := range buckets {
		resp.Buckets = append(resp.Buckets, &pb.QueueBucket{Purpose: bucket.Purpose, Pending: bucket.Pending, OldestAgeMs: bucket.OldestAgeMs})
	}
	return resp, nil
}

// ListPoliciesLogic 承载 ListPolicies 接口的业务逻辑；每个请求新建一个实例。
type ListPoliciesLogic struct{ base }

// NewListPoliciesLogic 绑定请求上下文与服务依赖。
func NewListPoliciesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPoliciesLogic {
	return &ListPoliciesLogic{newBase(ctx, svcCtx)}
}

// ListPolicies 返回政策码定义；本项目政策码与市场是演示配置。
func (l *ListPoliciesLogic) ListPolicies(*pb.ListPoliciesReq) (*pb.ListPoliciesResp, error) {
	resp := &pb.ListPoliciesResp{PolicyVersion: l.svcCtx.Policy.Version}
	for _, code := range adpolicy.Codes {
		resp.Codes = append(resp.Codes, &pb.PolicyCode{Code: code.Code, Title: code.Title, Category: code.Category})
	}
	for _, market := range adpolicy.Markets {
		resp.Markets = append(resp.Markets, market.Code)
	}
	return resp, nil
}
