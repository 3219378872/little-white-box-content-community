package ads

import (
	"context"
	"encoding/base64"
	"io"

	"esx/app/ad/rpc/adservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/review/rpc/reviewservice"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
)

// MaxAssetBytes 与 ad-rpc 私有存储上限一致（受内部 gRPC 单消息上限约束）。
const MaxAssetBytes int64 = 2 << 20

// UploadAdAssetLogic 承载 UploadAdAsset 接口的业务逻辑；每个请求新建一个实例。
type UploadAdAssetLogic struct{ base }

// NewUploadAdAssetLogic 绑定请求上下文与服务依赖。
func NewUploadAdAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAdAssetLogic {
	return &UploadAdAssetLogic{newBase(ctx, svcCtx)}
}

// UploadAdAsset 把素材或证件写入私有存储；未过审素材不经公开地址访问（ADS-015）。
func (l *UploadAdAssetLogic) UploadAdAsset(req *types.UploadAdAssetReq, file io.Reader, fileName, idempotencyKey string) (*types.AdAssetResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	if req.Kind != "creative" && req.Kind != "document" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxAssetBytes+1))
	if err != nil {
		return nil, errx.NewWithCode(errx.UploadFailed)
	}
	if int64(len(content)) > MaxAssetBytes {
		return nil, errx.NewWithCode(errx.FileTooLarge)
	}
	resp, err := l.svcCtx.AdService.UploadAsset(l.ctx, &adservice.UploadAssetReq{
		UserId: userID, Kind: req.Kind, FileName: fileName, Content: content, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.UploadAsset")
	}
	asset := resp.GetAsset()
	return &types.AdAssetResp{
		AssetId: asset.GetAssetId(), Kind: asset.GetKind(), Sha256: asset.GetSha256(),
		MimeType: asset.GetMimeType(), Size: asset.GetSize(),
	}, nil
}

// GetAdAssetLogic 承载 GetAdAsset 接口的业务逻辑；每个请求新建一个实例。
type GetAdAssetLogic struct{ base }

// NewGetAdAssetLogic 绑定请求上下文与服务依赖。
func NewGetAdAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAdAssetLogic {
	return &GetAdAssetLogic{newBase(ctx, svcCtx)}
}

// GetAdAsset 只向本人返回私有素材或证件（ADS-040）。
func (l *GetAdAssetLogic) GetAdAsset(req *types.GetAdAssetReq) (*types.AdAssetContentResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.ReadAsset(l.ctx, &adservice.ReadAssetReq{UserId: userID, AssetId: req.AssetId})
	if err != nil {
		return nil, l.rpcError(err, "AdService.ReadAsset")
	}
	return AssetContent(resp), nil
}

// AssetContent 把私有资产编码为 JSON 响应，供鉴权读取（客户端以内存图片或文件展示）。
func AssetContent(resp *adservice.ReadAssetResp) *types.AdAssetContentResp {
	return &types.AdAssetContentResp{
		MimeType: resp.GetMimeType(), ContentBase64: base64.StdEncoding.EncodeToString(resp.GetContent()),
	}
}

// ListAdPoliciesLogic 承载 ListAdPolicies 接口的业务逻辑；每个请求新建一个实例。
type ListAdPoliciesLogic struct{ base }

// NewListAdPoliciesLogic 绑定请求上下文与服务依赖。
func NewListAdPoliciesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAdPoliciesLogic {
	return &ListAdPoliciesLogic{newBase(ctx, svcCtx)}
}

// ListAdPolicies 返回政策码、演示市场与行业，供控制台与工作台本地化展示。
func (l *ListAdPoliciesLogic) ListAdPolicies(*types.ListAdPoliciesReq) (*types.ListAdPoliciesResp, error) {
	if _, err := l.userID(); err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.ListPolicies(l.ctx, &reviewservice.ListPoliciesReq{})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.ListPolicies")
	}
	out := &types.ListAdPoliciesResp{
		PolicyVersion: resp.GetPolicyVersion(), Codes: []types.AdPolicyCodeItem{}, Markets: nonNil(resp.GetMarkets()),
		Industries: append([]string(nil), adpolicy.Industries...), Demo: true,
	}
	for _, code := range resp.GetCodes() {
		out.Codes = append(out.Codes, types.AdPolicyCodeItem{Code: code.GetCode(), Title: code.GetTitle(), Category: code.GetCategory()})
	}
	return out, nil
}
