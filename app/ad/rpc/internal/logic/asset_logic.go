package logic

import (
	"context"
	"strings"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

type UploadAssetLogic struct{ base }

func NewUploadAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAssetLogic {
	return &UploadAssetLogic{newBase(ctx, svcCtx)}
}

// UploadAsset 把素材或证件写入私有存储；未过审素材不经公开地址访问（ADS-015）。
func (l *UploadAssetLogic) UploadAsset(in *pb.UploadAssetReq) (*pb.UploadAssetResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(in.GetKind())
	if (kind != store.AssetCreative && kind != store.AssetDocument) || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	inspected, err := assets.Inspect(kind, in.GetContent(), kind == store.AssetDocument)
	if err != nil {
		return nil, l.mapError(err, "inspect asset")
	}
	id, err := l.svcCtx.Store.NewAssetID()
	if err != nil {
		return nil, l.mapError(err, "allocate asset id")
	}
	key := assets.PrivateKey(id)
	if err := l.svcCtx.Assets.PutPrivate(l.ctx, key, in.GetContent(), inspected.MimeType); err != nil {
		return nil, l.mapError(err, "store private asset")
	}
	asset, err := l.svcCtx.Store.CreateAsset(l.ctx, store.Asset{
		ID: id, UserID: in.GetUserId(), Kind: kind, SHA256: inspected.SHA256, MimeType: inspected.MimeType,
		SizeBytes: inspected.Size, ObjectKey: key,
	}, in.GetIdempotencyKey(), l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "record asset")
	}
	return &pb.UploadAssetResp{Asset: &pb.AssetView{
		AssetId: asset.ID, Kind: asset.Kind, Sha256: asset.SHA256, MimeType: asset.MimeType, Size: asset.SizeBytes,
	}}, nil
}

type ReadAssetLogic struct{ base }

func NewReadAssetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReadAssetLogic {
	return &ReadAssetLogic{newBase(ctx, svcCtx)}
}

// ReadAsset 只返回给本人，或 Gateway 已经审核平台授权的审核员（ADS-040、RVW-023）；越权统一不存在。
func (l *ReadAssetLogic) ReadAsset(in *pb.ReadAssetReq) (*pb.ReadAssetResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	asset, err := l.svcCtx.Store.GetAsset(l.ctx, in.GetAssetId())
	if err != nil {
		return nil, l.mapError(err, "get asset")
	}
	if asset.UserID != in.GetUserId() && !in.GetReviewerAuthorized() {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	content, err := l.svcCtx.Assets.GetPrivate(l.ctx, asset.ObjectKey)
	if err != nil {
		return nil, l.mapError(err, "read private asset")
	}
	return &pb.ReadAssetResp{Content: content, MimeType: asset.MimeType}, nil
}
